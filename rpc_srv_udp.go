package pprpc

import (
	"errors"
	"fmt"
	"io"
	"sync/atomic"

	"github.com/golang/protobuf/proto"
	"xcthings.com/hjyz/logs"
	"xcthings.com/pprpc/packets"
	"xcthings.com/pprpc/ppudp"
)

// RPCUDPServer TCP服务对象
type RPCUDPServer struct {
	//*RPCSess // 会话管理 这部分功能应该放到应用层更合适
	*ppudp.UDPServer
	//cv *sync.Cond
	*Service
	// 进行GO程处理报文
	RunGO bool
	// 定一个各个回调函数
	Attr attrDefine // 定义设置连接属性的回调
	//
	listenIP   string
	listenPort int32
	//
	ConnectCB    ppudp.CloseCallback // 连接建立时的回调
	DisconnectCB ppudp.CloseCallback // 连接断开时的回调
	//
	PkgCB pkgCallBack // 所有数据报文的回调,将不会执行后续的其他回调
	// 在调用 HBCB, CmdCB, AVCB, CustomerCB 之前调用该接口
	// 如果该接口返回值 false 则不执行后续回调(HBCB, CmdCB, AVCB, CustomerCB)
	PreHookCB prehookCallBack
	//
	HBCB       hbCallBack       // 心跳回调
	CmdCB      cmdCallBack      // 控制报文回调
	AVCB       avCallBack       // 音视频流回调
	CustomerCB customerCallBack // 自定义数据回调

	readTimeout int64
	//WriteTimeout int
	count int32
}

// NewRPCUDPServer 创建RPC服务
func NewRPCUDPServer(ip string, port, maxSess int) (*RPCUDPServer, error) {
	us := new(RPCUDPServer)
	srv, err := ppudp.NewUDPServer(ip, port, maxSess)
	if err != nil {
		return nil, err
	}
	us.listenIP = ip
	us.listenPort = int32(port)
	us.UDPServer = srv
	us.RunGO = true
	us.readTimeout = 45
	us.count = 0

	srv.SetReadTimeout(us.readTimeout)

	return us, nil
}

// Serve 启动服务
func (ts *RPCUDPServer) Serve() {
	for {
		conn, err := ts.UDPServer.Accept()
		if err != nil {
			logs.Logger.Warnf("srv.Accept(), error: %s.", err)
			continue
		}
		go ts.handleConnect(conn)
	}
}

// SetReadTimeout 设置读取数据超时时间(def: 45s),最好在 Serve 之前调用
func (ts *RPCUDPServer) SetReadTimeout(to int64) {
	ts.UDPServer.SetReadTimeout(to)
}

// Stop 停止服务
func (ts *RPCUDPServer) Stop() {
	ts.UDPServer.Close()
}

// GetListenInfo .
func (ts *RPCUDPServer) GetListenInfo() (ip string, port int32) {
	ip = ts.listenIP
	port = ts.listenPort
	return
}

func (ts *RPCUDPServer) handleConnect(conn *ppudp.Connection) {
	connInfo := conn.RemoteAddr()
	logs.Logger.Debugf("%s, handleConnect, new connect.", connInfo)

	atomic.AddInt32(&ts.count, 1)
	defer func() {
		atomic.AddInt32(&ts.count, -1)
	}()

	if ts.ConnectCB != nil {
		ts.ConnectCB(conn)
	}
	if ts.DisconnectCB != nil {
		conn.SetCloseCB(ts.DisconnectCB)
	}
	var err error
	for {
		select {
		case <-conn.Ctx.Done():
			err = errors.New("conn.Ctx.Done()")
			goto connEnd
		default:
			pkg, err := packets.ReadUDPPacket(conn)
			if err == io.EOF {
				err = nil
				goto connEnd
			} else if err != nil {
				logs.Logger.Errorf("packets.ReadUDPPacket(), error: %s.", err)
				goto connEnd
			}
			if ts.PkgCB == nil {
				go ts.handlePacket(pkg, conn)
			} else {
				go ts.PkgCB(pkg, conn)
			}
		}
	}
connEnd:
	if err != nil {
		logs.Logger.Warnf("%s, handleConnect exit, error: %s.", connInfo, err)
	}
	// if ts.DisconnectCB != nil {
	// 	ts.DisconnectCB(conn)
	// }
	logs.Logger.Infof("%s, UDP Close.", connInfo)
	conn.Close()
}

func (ts *RPCUDPServer) handlePacket(pkg packets.PPPacket, conn *ppudp.Connection) {
	var err error
	if ts.PreHookCB != nil {
		if ts.PreHookCB(pkg, conn) == false {
			return
		}
	}

	switch pkg.(type) {
	case *packets.HBPacket:
		if ts.HBCB != nil {
			err = ts.HBCB(pkg.(*packets.HBPacket), conn)
		} else {
			err = ts.defhbcb(pkg.(*packets.HBPacket), conn)
		}
	case *packets.CmdPacket:
		cmd := pkg.(*packets.CmdPacket)
		if ts.CmdCB != nil {
			err = ts.CmdCB(cmd, conn)
		} else {
			err = ts.defcmdcb(cmd, conn)
		}
	case *packets.CustomerPacket:
		cus := pkg.(*packets.CustomerPacket)
		if ts.CustomerCB != nil {
			err = ts.CustomerCB(cus, conn)
		} else {
			err = ts.defcustomercb(cus, conn)
		}
	case *packets.AVPacket:
		av := pkg.(*packets.AVPacket)
		if ts.AVCB != nil {
			err = ts.AVCB(av, conn)
		} else {
			err = ts.defavcb(av, conn)
		}
	default:
		err = errors.New("not support pkg.(type)")
	}
	if err != nil {
		logs.Logger.Errorf("%s, handlePacket, error: %s.", conn.RemoteAddr(), err)
	}
}

func (ts *RPCUDPServer) defhbcb(pkg *packets.HBPacket, conn RPCConn) error {
	logs.Logger.Debugf("%s, HBPacket, MessageType: %d.", conn.RemoteAddr(), pkg.FixHeader.MessageType)
	_, err := pkg.Write(conn)
	return err
}

func (ts *RPCUDPServer) defcmdcb(pkg *packets.CmdPacket, conn RPCConn) error {
	var err error
	v := ts.GetService(pkg.CmdID)
	if v != nil {
		pkg.CmdName = v.CmdName

		dec := func(dobj interface{}) error {
			if pkg.Code != 0 {
				return nil
			}
			if pkg.MessageType == packets.TYPEPBBIN {
				if err := proto.Unmarshal(pkg.Payload, dobj.(proto.Message)); err != nil {
					err = fmt.Errorf("proto.Unmarshal error: %s", err)
					return err
				}
			} else if pkg.MessageType == packets.TYPEPBJSON {
				if err := proto.UnmarshalMessageSetJSON(pkg.Payload, dobj.(proto.Message)); err != nil {
					err = fmt.Errorf("proto.UnmarshalMessageSetJSON error: %s", err)
					return err
				}
			} else {
				err := fmt.Errorf("CmdID: %d, Name: %s, MessageType: %d Not Support",
					pkg.CmdID, pkg.CmdName, pkg.MessageType)
				return err
			}
			return nil
		}

		if pkg.RPCType == packets.RPCREQ {
			_, err = v.ReqHandler(v.Hanlder, conn, pkg, true, dec)
		} else if pkg.RPCType == packets.RPCRESP {
			_, err = v.RespHandler(v.Hanlder, conn, pkg, true, dec)
		} else {
			err = fmt.Errorf("CmdId: %d, Name: %s, pkg.RPCType: %d, not support",
				v.CmdID, v.CmdName, pkg.RPCType)
		}
		if err != nil {
			err = fmt.Errorf("%s, CmdId: %d, Name: %s, RPCType: %d, call Handler error: %s",
				conn.RemoteAddr(), v.CmdID, v.CmdName, pkg.RPCType, err)
		}

	} else {
		// 没有注册的命令
		pkg.Code = CmdIDNotReg
		if pkg.RPCType == packets.RPCREQ {
			pkg.RPCType = packets.RPCRESP
		}
		pkg.Payload = []byte{}
		_, err = pkg.Write(conn)
		if err != nil {
			logs.Logger.Errorf("%s, WritePkg error: %s.", conn.RemoteAddr(), err)
		}
		err = fmt.Errorf("%s, not find cmdid: %d register", conn.RemoteAddr(), pkg.CmdID)
	}
	return err
}

func (ts *RPCUDPServer) defcustomercb(pkg *packets.CustomerPacket, conn RPCConn) error {
	logs.Logger.Debugf("%s, CustomerPacket not support, MessageType: %d, Length: %d, Payload Length: %d.",
		conn.RemoteAddr(), pkg.MessageType, pkg.Length, len(pkg.Payload))
	return nil
}

func (ts *RPCUDPServer) defavcb(pkg *packets.AVPacket, conn RPCConn) error {
	logs.Logger.Debugf("%s, AVPacket not support, MessageType: %d, Payload Length: %d.",
		conn.RemoteAddr(), pkg.MessageType, len(pkg.Payload))
	return nil
}

// Count get connections count
func (ts *RPCUDPServer) Count() int32 {
	return atomic.LoadInt32(&ts.count)
}

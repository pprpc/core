package pprpc

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/golang/protobuf/proto"

	"github.com/pprpc/util/logs"
	"github.com/pprpc/packets"
	"github.com/pprpc/pptcp"
)

// RPCTCPServer TCP服务对象
type RPCTCPServer struct {
	//*RPCSess // 会话管理 这部分功能应该放到应用层更合适
	*pptcp.TCPServer
	//cv *sync.Cond
	*Service
	// 进行GO程处理报文
	RunGO bool
	// 定一个各个回调函数
	Attr attrDefine // 定义设置连接属性的回调
	//
	ConnectCB    pptcp.CloseCallback // 连接建立时的回调
	DisconnectCB pptcp.CloseCallback // 连接断开时的回调
	//
	PkgCB pkgCallBack // 所有数据报文的回调,将不会执行后续的其他回调
	// 在调用 HBCB, CmdCB, AVCB, CustomerCB 之前调用该接口
	// 如果该接口返回值 !=nil 则不执行后续回调(HBCB, CmdCB, AVCB, CustomerCB)
	PreHookCB prehookCallBack
	//
	HBCB       hbCallBack       // 心跳回调
	CmdCB      cmdCallBack      // 控制报文回调
	AVCB       avCallBack       // 音视频流回调
	CustomerCB customerCallBack // 自定义数据回调

	// ReadTimeout WriteTimeout
	ReadTimeout int
	//WriteTimeout int

	// count connect计数
	count int32
	// url
	lisURL *url.URL
}

// NewRPCTCPServer 创建RPC服务
func NewRPCTCPServer(uri *url.URL, tlsc *tls.Config) (*RPCTCPServer, error) {
	ts := new(RPCTCPServer)
	srv, err := newTCPServer(uri, tlsc)
	if err != nil {
		return nil, err
	}
	ts.TCPServer = srv
	ts.RunGO = false
	ts.ReadTimeout = 180
	ts.count = 0
	ts.lisURL = uri

	return ts, nil
}

func newTCPServer(uri *url.URL, tlsc *tls.Config) (srv *pptcp.TCPServer, err error) {
	switch uri.Scheme {
	case "tcp":
		srv, err = pptcp.NewTCPServer(uri.Host)
	case "tls":
		srv, err = pptcp.NewTLSTCPServer(uri.Host, tlsc)
	// case "quic":
	// 	srv, err = pptcp.NewQUICServer(uri.Host, tlsc)
	default:
		err = fmt.Errorf("not support: %s", uri.Scheme)
	}

	return
}

// Serve 启动服务
func (ts *RPCTCPServer) Serve() {
	for {
		conn, err := ts.TCPServer.Accept()
		if err != nil {
			logs.Logger.Warnf("srv.Accept(), error: %s.", err)
			continue
		}
		go ts.handleConnect(conn)
	}
}

// Stop 停止服务
func (ts *RPCTCPServer) Stop() {
	ts.TCPServer.Close()
}

// SetReadTimeout 设置连接读取超时时间.
func (ts *RPCTCPServer) SetReadTimeout(to int64) {
	ts.ReadTimeout = int(to)
}

func (ts *RPCTCPServer) handleConnect(conn *pptcp.Connection) {
	logs.Logger.Debugf("%s, handleConnect, new connect.", conn)

	atomic.AddInt32(&ts.count, 1)
	defer func() {
		atomic.AddInt32(&ts.count, -1)
	}()

	ci := conn.String()
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
			conn.SetReadDeadline(time.Now().Add(time.Duration(ts.ReadTimeout) * time.Second))
			pkg, e := packets.ReadTCPPacketAdv(conn, conn.AutoCrypt)
			if e != nil {
				err = fmt.Errorf("packets.ReadTCPPacket(), ts.ReadTimeout: %d, %s", ts.ReadTimeout, e)
				goto connEnd
			}

			if ts.RunGO {
				if ts.PkgCB == nil {
					go ts.handlePacket(pkg, conn)
				} else {
					go ts.PkgCB(pkg, conn)
				}
			} else {
				if ts.PkgCB == nil {
					ts.handlePacket(pkg, conn)
				} else {
					ts.PkgCB(pkg, conn)
				}
			}
		}
	}
connEnd:
	if err != nil {
		logs.Logger.Warnf("%s, handleConnect exit, error: %s.", ci, err)
	}

	// if ts.DisconnectCB != nil {
	// 	ts.DisconnectCB(conn)
	// }
	if conn != nil {
		conn.Close()
	}
}

func (ts *RPCTCPServer) handlePacket(pkg packets.PPPacket, conn *pptcp.Connection) {
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

func (ts *RPCTCPServer) defhbcb(pkg *packets.HBPacket, conn RPCConn) error {
	logs.Logger.Debugf("%s, HBPacket, MessageType: %d.", conn.RemoteAddr(), pkg.FixHeader.MessageType)
	_, err := pkg.Write(conn)
	return err
}

func (ts *RPCTCPServer) defcmdcb(pkg *packets.CmdPacket, conn RPCConn) error {
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

func (ts *RPCTCPServer) defcustomercb(pkg *packets.CustomerPacket, conn RPCConn) error {
	logs.Logger.Debugf("%s, CustomerPacket not support, MessageType: %d, Length: %d, Payload Length: %d.",
		conn.RemoteAddr(), pkg.MessageType, pkg.Length, len(pkg.Payload))
	return nil
}

func (ts *RPCTCPServer) defavcb(pkg *packets.AVPacket, conn RPCConn) error {
	logs.Logger.Debugf("%s, AVPacket not support, MessageType: %d, Payload Length: %d.",
		conn.RemoteAddr(), pkg.MessageType, len(pkg.Payload))
	return nil
}

// Count get connections count
func (ts *RPCTCPServer) Count() int32 {
	return atomic.LoadInt32(&ts.count)
}

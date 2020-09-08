package pprpc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	"xcthings.com/hjyz/logs"
	"xcthings.com/pprpc/packets"
	"xcthings.com/pprpc/ppudp"
)

// UDPCliConn PPRPC TCP 连接.
type UDPCliConn struct {
	ctx                context.Context
	ctxCancel          context.CancelFunc
	hbSec              int
	SyncWriteTimeoutMs int

	*ppudp.ClientConn
	*Service

	firstChan chan error

	asyncChans *sync.Map
	// 加密类型
	cryptType   uint8
	messageType uint8
	// 如果该接口返回值 !=nil 则不执行后续回调(HBCB, CmdCB, AVCB, CustomerCB)
	PreHookCB pkgCallBack

	HBCB       hbCallBack       // 心跳回调
	CmdCB      cmdCallBack      // 控制报文回调
	AVCB       avCallBack       // 音视频流回调
	CustomerCB customerCallBack // 自定义数据回调
	autohb     bool
}

// DailUDP 建立PPRPC的连接(udp)
func DailUDP(addr string, si *Service, readTimeout int64, fn udpCliCallBack) (tcc *UDPCliConn, err error) {
	tcc = new(UDPCliConn)
	tcc.ctx, tcc.ctxCancel = context.WithCancel(context.Background())
	tcc.Service = si

	tcc.asyncChans = new(sync.Map)

	tcc.ClientConn = ppudp.NewClientConn(addr, readTimeout)
	tcc.hbSec = 10
	tcc.SyncWriteTimeoutMs = 3000

	tcc.messageType = packets.TYPEPBBIN
	tcc.cryptType = packets.AES256CFB
	tcc.firstChan = make(chan error, 2)
	tcc.autohb = true

	go tcc.run(fn)
	err = <-tcc.firstChan
	return
}

func (tcc *UDPCliConn) run(fn udpCliCallBack) {
	cli := tcc.ClientConn
	var err error
	err = cli.Connect()
	if err != nil {
		logs.Logger.Errorf("cli.Connect(), waiting 3sec reconnect, error: %s.", err)
		tcc.firstChan <- err
	}
	tcc.firstChan <- nil
	// 连接建立回调
	if fn != nil {
		go fn(tcc)
	}
	// read
	go func() {
		var err error
		defer func() {
			logs.Logger.Debug("read exit.")
		}()

		for {
			select {
			case <-cli.Ctx.Done():
				logs.Logger.Warn("cli.Ctx.Done(), read exit.")
				return
			case <-tcc.ctx.Done():
				logs.Logger.Warn("tcc.ctx.Done(), HB exit.")
				return
			default:
				pkg, err := packets.ReadUDPPacket(cli)
				if err == io.EOF {
					err = nil
					goto connEnd
				} else if err != nil {
					logs.Logger.Errorf("packets.ReadUDPPacket(), error: %s.", err)
					break
				}
				go tcc.handlePacket(pkg)
			}
		}
	connEnd:
		if err != nil {
			logs.Logger.Warnf("handleConnect exit, error: %s.", err)
		}
	}()

	// hb
	for {
		select {
		case <-cli.Ctx.Done():
			logs.Logger.Warn("cli.Ctx.Done(), HB exit.")
			goto Stop
		case <-tcc.ctx.Done():
			logs.Logger.Warn("tcc.ctx.Done(), HB exit.")
			goto Stop
		case <-time.After(time.Second * time.Duration(tcc.hbSec)):
			if tcc.autohb {
				hb := packets.NewHBPacket()
				hb.FixHeader.SetProtocol(packets.PROTOUDP)

				_, err = hb.Write(cli)
				if err != nil {
					logs.Logger.Errorf("hb.Write(), error: %s.", err)
					goto Stop
				}
			}
		}
	}
Stop:
	cli.Close()
	logs.Logger.Warnf("Close UDPCliConn.")
}

// Invoke 调用Service,同步
func (tcc *UDPCliConn) Invoke(ctx context.Context, cmdid uint64, req interface{}) (pkg *packets.CmdPacket, resp interface{}, err error) {
	// 检查连接状态
	_s, _ := tcc.GetState()
	if _s != ppudp.StateConnected {
		err = fmt.Errorf("connect status: %d, not Invoke", _s)
		return
	}

	// 构造 CmdHeader.
	seq := GetSeqID()
	cmd := packets.NewCmdPacket(tcc.messageType)
	cmd.FixHeader.SetProtocol(packets.PROTOUDP)
	cmd.CmdSeq = seq
	cmd.CmdID = cmdid
	cmd.EncType = tcc.cryptType
	cmd.RPCType = packets.RPCREQ

	if tcc.messageType == packets.TYPEPBBIN {
		cmd.Payload, err = proto.Marshal(req.(proto.Message))
	} else if tcc.messageType == packets.TYPEPBJSON {
		cmd.Payload, err = proto.MarshalMessageSetJSON(req)
	}
	if err != nil {
		return
	}

	// 构造返回请求.
	ansQueue := make(chan *packets.CmdPacket, 2)

	tcc.asyncChans.Store(seq, ansQueue)
	defer func() {
		tcc.asyncChans.Delete(seqID)
		close(ansQueue)
	}()
	// Write
	_, err = cmd.Write(tcc.ClientConn)
	if err != nil {
		return
	}
	// Read
	select {
	case <-time.After(time.Millisecond * time.Duration(tcc.SyncWriteTimeoutMs)):
		err = fmt.Errorf("write cmdid: %d timeout", cmdid)
	case <-ctx.Done():
		err = fmt.Errorf("ctx.Done()")
		return
	case pkg = <-ansQueue:
		resp, err = tcc.decodeCmd(pkg, tcc.ClientConn)
	}

	return
}

// InvokeAsync 调用Service,异步
func (tcc *UDPCliConn) InvokeAsync(ctx context.Context, cmdid uint64, req interface{}) (err error) {
	// 检查连接状态
	_s, _ := tcc.GetState()
	if _s != ppudp.StateConnected {
		err = fmt.Errorf("connect status: %d, not Invoke", _s)
		return
	}
	// 构造 CmdHeader.
	seq := GetSeqID()
	cmd := packets.NewCmdPacket(tcc.messageType)
	cmd.FixHeader.SetProtocol(packets.PROTOUDP)
	cmd.CmdSeq = seq
	cmd.CmdID = cmdid
	cmd.EncType = tcc.cryptType
	cmd.RPCType = packets.RPCREQ

	if tcc.messageType == packets.TYPEPBBIN {
		cmd.Payload, err = proto.Marshal(req.(proto.Message))
	} else if tcc.messageType == packets.TYPEPBJSON {
		cmd.Payload, err = proto.MarshalMessageSetJSON(req)
	}
	if err != nil {
		return
	}

	_, err = cmd.Write(tcc.ClientConn)
	if err != nil {
		return
	}
	return
}

// Close 关闭连接,退出重连
func (tcc *UDPCliConn) Close() (err error) {
	tcc.ctxCancel()
	return
}

// SetHB 设置心跳间隔时间，单位: 秒
func (tcc *UDPCliConn) SetHB(sec int) {
	if sec < 10 {
		return
	}
	tcc.hbSec = sec
}

// GetHB 获取心跳间隔时间，单位: 秒
func (tcc *UDPCliConn) GetHB() int {
	return tcc.hbSec

}

// SetAutoHB 是否自动HB
func (tcc *UDPCliConn) SetAutoHB(b bool) {
	tcc.autohb = b
}

// SetCrypt 设置加密类型
func (tcc *UDPCliConn) SetCrypt(v uint8) {
	tcc.cryptType = v
}

func (tcc *UDPCliConn) handlePacket(pkg packets.PPPacket) {
	var err error
	if tcc.PreHookCB != nil {
		err = tcc.PreHookCB(pkg, tcc.ClientConn)
		if err != nil {
			return
		}
	}
	switch pkg.(type) {
	case *packets.HBPacket:
		if tcc.HBCB != nil {
			err = tcc.HBCB(pkg.(*packets.HBPacket), tcc.ClientConn)
		} else {
			err = tcc.defhbcb(pkg.(*packets.HBPacket), tcc.ClientConn)
		}
	case *packets.CmdPacket:
		cmd := pkg.(*packets.CmdPacket)
		v, ok := tcc.asyncChans.Load(cmd.CmdSeq)
		if ok {
			v.(chan *packets.CmdPacket) <- cmd
			tcc.asyncChans.Delete(cmd.CmdSeq)
		} else {
			if tcc.CmdCB != nil {
				err = tcc.CmdCB(cmd, tcc.ClientConn)
			} else {
				err = tcc.defcmdcb(cmd, tcc.ClientConn)
			}
		}
	case *packets.CustomerPacket:
		cus := pkg.(*packets.CustomerPacket)
		if tcc.CustomerCB != nil {
			err = tcc.CustomerCB(cus, tcc.ClientConn)
		} else {
			err = tcc.defcustomercb(cus, tcc.ClientConn)
		}
	case *packets.AVPacket:
		av := pkg.(*packets.AVPacket)
		if tcc.AVCB != nil {
			err = tcc.AVCB(av, tcc.ClientConn)
		} else {
			err = tcc.defavcb(av, tcc.ClientConn)
		}
	default:
		err = errors.New("not support pkg.(type)")
	}
	if err != nil {
		logs.Logger.Errorf("%s, handlePacket, error: %s.", tcc.ClientConn.RemoteAddr(), err)
	}
}

func (tcc *UDPCliConn) defhbcb(pkg *packets.HBPacket, conn RPCConn) error {
	logs.Logger.Debugf("%s, HBPacket, MessageType: %d.", conn.RemoteAddr(), pkg.FixHeader.MessageType)
	return nil
}

func (tcc *UDPCliConn) decodeCmd(pkg *packets.CmdPacket, conn RPCConn) (dobj interface{}, err error) {
	v := tcc.GetService(pkg.CmdID)
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
			dobj, err = v.ReqHandler(v.Hanlder, conn, pkg, false, dec)
		} else if pkg.RPCType == packets.RPCRESP {
			dobj, err = v.RespHandler(v.Hanlder, conn, pkg, false, dec)
		} else {
			err = fmt.Errorf("CmdId: %d, Name: %s, pkg.RPCType: %d, not support",
				v.CmdID, v.CmdName, pkg.RPCType)
		}
		if err != nil {
			err = fmt.Errorf("%s, CmdId: %d, Name: %s, call Handler error: %s",
				conn.RemoteAddr(), v.CmdID, v.CmdName, err)
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
	return
}

func (tcc *UDPCliConn) defcmdcb(pkg *packets.CmdPacket, conn RPCConn) error {
	var err error
	v := tcc.GetService(pkg.CmdID)
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
			err = fmt.Errorf("%s, CmdId: %d, Name: %s, call Handler error: %s",
				conn.RemoteAddr(), v.CmdID, v.CmdName, err)
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

func (tcc *UDPCliConn) defcustomercb(pkg *packets.CustomerPacket, conn RPCConn) error {
	logs.Logger.Debugf("%s, CustomerPacket not support, MessageType: %d, Length: %d, Payload Length: %d.",
		conn.RemoteAddr(), pkg.MessageType, pkg.Length, len(pkg.Payload))
	return nil
}

func (tcc *UDPCliConn) defavcb(pkg *packets.AVPacket, conn RPCConn) error {
	logs.Logger.Debugf("%s, AVPacket not support, MessageType: %d, Payload Length: %d.",
		conn.RemoteAddr(), pkg.MessageType, len(pkg.Payload))
	return nil
}

/*

go func() {
		var err error
		defer func() {
			once.Do(cliClose)
			logs.Logger.Debug("write exit.")
		}()

		for {
			select {
			case <-cli.Ctx.Done():
				logs.Logger.Warn("cli.Ctx.Done(), write exit.")
				return
			//default:
			case <-time.After(time.Second * 5):
				cus := packets.NewCustomerPacket()
				cus.Payload = []byte("Hello, ppudp.")
				_, err = cus.Write(cli)
				if err != nil {
					logs.Logger.Errorf("cus.Write(), error: %s.", err)

					return
				}
				//common.Sleep(5)
			}
		}
    }()
*/

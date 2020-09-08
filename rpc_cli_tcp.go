package pprpc

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/url"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/pprpc/util/common"
	"github.com/pprpc/util/logs"
	"github.com/pprpc/packets"
	"github.com/pprpc/pptcp"
)

// TCPCliConn PPRPC TCP 连接.
type TCPCliConn struct {
	ctx                context.Context
	ctxCancel          context.CancelFunc
	hbSec              int
	intervalSec        int
	SyncWriteTimeoutMs int

	*pptcp.ClientConn
	*Service

	firstChan chan error
	isFirst   bool

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

	autohb   bool
	stopDail bool
}

// Dail 建立PPRPC的连接(tcp,tls,quic)
func Dail(uri *url.URL, tlsc *tls.Config, si *Service, dialTimeout time.Duration, fn tcpCliCallBack) (tcc *TCPCliConn, err error) {
	tcc = new(TCPCliConn)
	tcc.ctx, tcc.ctxCancel = context.WithCancel(context.Background())
	tcc.Service = si

	tcc.firstChan = make(chan error, 2)
	tcc.isFirst = true
	tcc.asyncChans = new(sync.Map)

	tcc.ClientConn = pptcp.NewClientConn(uri, tlsc, dialTimeout) //pptcp.NewClientConn(u, nil, 5*time.Second)
	tcc.intervalSec = 3
	tcc.hbSec = 180
	tcc.SyncWriteTimeoutMs = 3000

	tcc.messageType = packets.TYPEPBBIN
	tcc.cryptType = packets.AES256CFB

	tcc.autohb = true
	tcc.stopDail = false

	go tcc.run(fn)
	err = <-tcc.firstChan

	return
}

func (tcc *TCPCliConn) run(fn tcpCliCallBack) {
	cli := tcc.ClientConn
	var err error
	for {
	Connect:
		err = cli.Connect()
		if err != nil {
			logs.Logger.Errorf("cli.Connect(), waiting 3sec reconnect, error: %s.", err)
			if tcc.isFirst {
				tcc.firstChan <- err
			}
			tcc.isFirst = false
			common.Sleep(tcc.intervalSec)
			if tcc.stopDail == false {
				goto Connect
			} else {
				goto Stop
			}
		} else {
			if tcc.isFirst {
				tcc.firstChan <- nil
			}
			tcc.isFirst = false
		}
		var once sync.Once
		cliClose := func() {
			logs.Logger.Warnf("%s, disconnect.", cli.RemoteAddr())
			cli.Close()
		}

		defer func() {
			once.Do(cliClose)
			logs.Logger.Debug("read exit.")
		}()
		// 每次连接都会消耗，需要重新处理.
		cli.Ctx, cli.CtxCancel = context.WithCancel(context.Background())

		go func() {
			var err error
			defer func() {
				once.Do(cliClose)
				logs.Logger.Debug("read exit.")
			}()

			for {
				select {
				case <-cli.Ctx.Done():
					logs.Logger.Warn("cli.Ctx.Done(), read exit.")
					return
				case <-tcc.ctx.Done():
					logs.Logger.Warn("tcc.ctx.Done(), read exit.")
					return
				default:
					cli.SetReadDeadline(time.Now().Add(time.Duration(tcc.hbSec+10) * time.Second))
					pkg, err := packets.ReadTCPPacket(cli)
					if err == io.EOF {
						err = nil
						goto connEnd
					} else if err != nil {
						//goto connEnd
						logs.Logger.Errorf("packets.ReadTCPPacket(), error: %s.", err)
						goto connEnd
					}
					go tcc.handlePacket(pkg)
				}
			}
		connEnd:
			if err != nil {
				logs.Logger.Warnf("handleConnect exit, error: %s.", err)
			}
		}()

		// 呼叫连接建立回调
		if fn != nil {
			go fn(tcc)
		}
		// HB
		for {
			select {
			case <-cli.Ctx.Done():
				logs.Logger.Warn("cli.Ctx.Done(), HB exit.")
				goto loopEnd
			case <-tcc.ctx.Done():
				logs.Logger.Warn("tcc.ctx.Done(), read exit.")
				goto Stop
			case <-time.After(time.Second * time.Duration(tcc.hbSec)):
				if tcc.autohb {
					hb := packets.NewHBPacket()
					_, err = hb.Write(cli)
					if err != nil {
						logs.Logger.Errorf("hb.Write(), error: %s.", err)
						goto loopEnd
					}
				}
			}
		}
	loopEnd:
		//once.Do(cliClose)
		logs.Logger.Debugf("loopEnd, goto Connect.")
		common.Sleep(tcc.intervalSec)
	}
Stop:
	logs.Logger.Warnf("Close TCPCliConn.")
}

// Invoke 调用Service,同步
func (tcc *TCPCliConn) Invoke(ctx context.Context, cmdid uint64, req interface{}) (pkg *packets.CmdPacket, resp interface{}, err error) {
	// 检查连接状态
	_s, _ := tcc.GetState()
	if _s != pptcp.StateConnected {
		err = fmt.Errorf("connect status: %d, not Invoke", _s)
		return
	}

	// 构造 CmdHeader.
	seq := GetSeqID()
	cmd := packets.NewCmdPacket(tcc.messageType)
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
		tcc.asyncChans.Delete(seq)
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
		err = fmt.Errorf("cli: %s, seq: %d, write cmdid: %d timeout, close tcc.ClientConn", tcc.ClientConn, seq, cmdid)
		tcc.ClientConn.Close()

	case <-ctx.Done():
		err = fmt.Errorf("ctx.Done()")
		return
	case pkg = <-ansQueue:
		resp, err = tcc.decodeCmd(pkg, tcc.ClientConn)
	}

	return
}

// InvokeAsync 调用Service,异步
func (tcc *TCPCliConn) InvokeAsync(ctx context.Context, cmdid uint64, req interface{}) (err error) {
	// 检查连接状态
	_s, e := tcc.GetState()
	if e != nil {
		err = fmt.Errorf("tcc.GetState, %s", e)
		return
	}
	if _s != pptcp.StateConnected {
		err = fmt.Errorf("connect status: %d, not Invoke", _s)
		return
	}
	// 构造 CmdHeader.
	seq := GetSeqID()
	cmd := packets.NewCmdPacket(tcc.messageType)
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
func (tcc *TCPCliConn) Close() (err error) {
	tcc.stopDail = true
	tcc.ctxCancel()
	return
}

// SetHB 设置心跳间隔时间，单位: 秒
func (tcc *TCPCliConn) SetHB(sec int) {
	if sec < 10 {
		return
	}
	tcc.hbSec = sec
}

// GetHB 获取心跳间隔时间，单位: 秒
func (tcc *TCPCliConn) GetHB() int {
	return tcc.hbSec
}

// SetAutoHB 是否自动HB
func (tcc *TCPCliConn) SetAutoHB(b bool) {
	tcc.autohb = b
}

// SetInterval 设置重新连接的间隔时间, 单位: 秒
func (tcc *TCPCliConn) SetInterval(sec int) {
	if sec < 1 {
		return
	}
	tcc.intervalSec = sec
}

// GetInterval 获取重新连接的间隔时间, 单位: 秒
func (tcc *TCPCliConn) GetInterval() int {
	return tcc.intervalSec
}

// SetCrypt 设置加密类型
func (tcc *TCPCliConn) SetCrypt(v uint8) {
	tcc.cryptType = v
}

func (tcc *TCPCliConn) handlePacket(pkg packets.PPPacket) {
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

func (tcc *TCPCliConn) defhbcb(pkg *packets.HBPacket, conn RPCConn) error {
	logs.Logger.Debugf("%s, HBPacket, MessageType: %d.", conn, pkg.FixHeader.MessageType)
	return nil
}

func (tcc *TCPCliConn) decodeCmd(pkg *packets.CmdPacket, conn RPCConn) (dobj interface{}, err error) {
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

func (tcc *TCPCliConn) defcmdcb(pkg *packets.CmdPacket, conn RPCConn) error {
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

func (tcc *TCPCliConn) defcustomercb(pkg *packets.CustomerPacket, conn RPCConn) error {
	logs.Logger.Debugf("%s, CustomerPacket not support, MessageType: %d, Length: %d, Payload Length: %d.",
		conn.RemoteAddr(), pkg.MessageType, pkg.Length, len(pkg.Payload))
	return nil
}

func (tcc *TCPCliConn) defavcb(pkg *packets.AVPacket, conn RPCConn) error {
	logs.Logger.Debugf("%s, AVPacket not support, MessageType: %d, Payload Length: %d.",
		conn.RemoteAddr(), pkg.MessageType, len(pkg.Payload))
	return nil
}

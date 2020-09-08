package pprpc

import (
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/pprpc/packets"
	"github.com/pprpc/ppudp"
)

type pkgCallBack func(packets.PPPacket, RPCConn) error
type hbCallBack func(*packets.HBPacket, RPCConn) error
type cmdCallBack func(*packets.CmdPacket, RPCConn) error
type avCallBack func(*packets.AVPacket, RPCConn) error
type customerCallBack func(*packets.CustomerPacket, RPCConn) error
type attrDefine func() interface{}

type prehookCallBack func(packets.PPPacket, RPCConn) bool

type connectCallBack func(RPCConn)

//type tcpCliCallBack func(*TCPCliConn)
type tcpCliCallBack func(RPCCliConn)

//type udpCliCallBack func(*UDPCliConn)
type udpCliCallBack func(RPCCliConn)

// WriteResp 写入RESP
func WriteResp(c RPCConn, pkg *packets.CmdPacket, resp interface{}) (n int64, err error) {
	var b []byte
	if resp != nil && pkg.Code == 0 {
		if pkg.MessageType == packets.TYPEPBBIN {
			b, err = proto.Marshal(resp.(proto.Message))
		} else if pkg.MessageType == packets.TYPEPBJSON {
			b, err = proto.MarshalMessageSetJSON(resp)
		} else {
			err = fmt.Errorf("not support MessageType: %d", pkg.MessageType)
		}
		if err != nil {
			return
		}
	}
	pkg.Payload = b
	pkg.RPCType = packets.RPCRESP
	n, err = pkg.Write(c)

	return
}

// InvokeAsync 执行远程调用(异步).
func InvokeAsync(c RPCConn, cmdid uint64, req interface{}, mt, crypt uint8) (err error) {
	seq := GetSeqID()
	cmd := packets.NewCmdPacket(mt)

	switch c.(type) {
	case *ppudp.Connection:
		cmd.FixHeader.SetProtocol(packets.PROTOUDP)
	}

	cmd.CmdSeq = seq
	cmd.CmdID = cmdid
	cmd.EncType = crypt
	cmd.RPCType = packets.RPCREQ

	if mt == packets.TYPEPBBIN {
		cmd.Payload, err = proto.Marshal(req.(proto.Message))
	} else if mt == packets.TYPEPBJSON {
		cmd.Payload, err = proto.MarshalMessageSetJSON(req)
	}
	if err != nil {
		return
	}

	_, err = cmd.Write(c)
	if err != nil {
		return
	}
	return
}

// // Invoke 执行远程调用(同步)[FIXME].
// func Invoke(c RPCConn, cmdid uint64, req interface{}, mt, crypt uint8) (pkg *packets.CmdPacket, resp interface{}, err error) {
// 	return
// }

// DecodePkg .
func DecodePkg(pkg *packets.CmdPacket, s *Service) (obj interface{}, err error) {
	v := s.GetService(pkg.CmdID)
	if v == nil {
		err = fmt.Errorf("CmdID: %d, not register", pkg.CmdID)
		return
	}
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
		obj, err = v.ReqHandler(v.Hanlder, nil, pkg, false, dec)
	} else if pkg.RPCType == packets.RPCRESP {
		obj, err = v.RespHandler(v.Hanlder, nil, pkg, false, dec)
	} else {
		err = fmt.Errorf("CmdId: %d, Name: %s, pkg.RPCType: %d, not support",
			v.CmdID, v.CmdName, pkg.RPCType)
	}
	if err != nil {
		err = fmt.Errorf("CmdId: %d, Name: %s, call Handler error: %s",
			v.CmdID, v.CmdName, err)
	}

	return
}

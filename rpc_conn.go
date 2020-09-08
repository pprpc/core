package pprpc

import (
	"context"
	"net"
	"time"

	"xcthings.com/pprpc/packets"
)

// RPCConn 定义RPC连接
type RPCConn interface {
	SetAttr(attr interface{}) (err error)
	GetAttr() (interface{}, error)
	IsClose() bool
	SetState(v uint32) (err error)
	GetState() (v uint32, err error)
	Write(b []byte) (n int, err error)
	Read(b []byte) (n int, err error)
	Close() (err error)
	SetDeadline(t time.Time) error
	SetReadDeadline(t time.Time) error
	SetWriteDeadline(t time.Time) error
	LocalAddr() net.Addr
	RemoteAddr() net.Addr
	LogPre() string
	LogPreShort() string
	HandleClose() context.Context
	String() string
	SetAutoCrypt(bool)
	Type() string // T=TCP； S=TLS； Q=QUIC; M = mqtt; U=UDP
}

// RPCCliConn 定义RPC Client 连接.
type RPCCliConn interface {
	SetAttr(attr interface{}) (err error)
	GetAttr() (interface{}, error)
	IsClose() bool
	SetState(v uint32) (err error)
	GetState() (v uint32, err error)
	Write(b []byte) (n int, err error)
	Read(b []byte) (n int, err error)
	Close() (err error)
	SetDeadline(t time.Time) error
	SetReadDeadline(t time.Time) error
	SetWriteDeadline(t time.Time) error
	LocalAddr() net.Addr
	RemoteAddr() net.Addr
	LogPre() string
	LogPreShort() string
	String() string
	Type() string // T=TCP； S=TLS； Q=QUIC; M = mqtt; U=UDP
	SetAutoHB(b bool)
	// 增加两个方法调用
	Invoke(ctx context.Context, cmdid uint64, req interface{}) (pkg *packets.CmdPacket, resp interface{}, err error)
	InvokeAsync(ctx context.Context, cmdid uint64, req interface{}) (err error)
}

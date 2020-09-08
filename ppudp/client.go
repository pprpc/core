package ppudp

import (
	"context"
	"net"
)

// ClientConn Client连接结构体.
type ClientConn struct {
	*Connection
	ctx         context.Context
	ctxCancel   context.CancelFunc
	addr        string
	readTimeout int64
}

// NewClientConn .
func NewClientConn(addr string, readTimeout int64) *ClientConn {
	c := new(ClientConn)
	c.addr = addr
	c.readTimeout = readTimeout
	c.ctx, c.ctxCancel = context.WithCancel(context.Background())
	return c
}

// Connect 发起连接.
func (c *ClientConn) Connect() error {
	conn, err := openClientConn(c.addr)
	if err != nil {
		return err
	}
	c.Connection = NewConnection(c.ctx, conn, nil, c.readTimeout, nil, nil)
	return nil
}

// Disconnect 断开连接.
func (c *ClientConn) Disconnect() error {
	return c.Connection.Close()
}

// Reconnect 重新连接
func (c *ClientConn) Reconnect() error {
	if c.Connection != nil {
		c.Connection.Close()
		return c.Connect()
	}
	return c.Connect()
}

// openClientConn 建立连接
func openClientConn(addr string) (*net.UDPConn, error) {
	dstAddr, err := net.ResolveUDPAddr("udp4", addr)
	if err != nil {
		return nil, err
	}
	conn, err := net.DialUDP("udp", nil, dstAddr)
	return conn, err
}

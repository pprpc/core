package pptcp

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/url"
	"sync"
	"time"

	"github.com/pprpc/util/common"
)

// ClientConn Client连接结构体.
type ClientConn struct {
	*Connection
	mu          sync.Mutex
	uri         *url.URL
	tlsc        *tls.Config
	dialTimeout time.Duration
}

// NewClientConn .
func NewClientConn(uri *url.URL, tlsc *tls.Config, dialTimeout time.Duration) *ClientConn {
	c := new(ClientConn)
	c.uri = uri
	c.tlsc = tlsc
	c.dialTimeout = dialTimeout
	c.mu = sync.Mutex{}

	return c
}

// Connect 发起连接.
func (c *ClientConn) Connect() error {
	s, _ := c.GetState()
	if s != StateDisconnected {
		return fmt.Errorf("conn status: %d, not %d", s, StateDisconnected)
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	conn, err := openClientConn(c.uri, c.tlsc, c.dialTimeout)
	if err != nil {
		return err
	}
	var ct string
	switch c.uri.Scheme {
	case "tcp":
		ct = "T"
	case "tls":
		ct = "S"
	case "quic":
		ct = "Q"
	}
	c.Connection = NewConnection(conn, ct)
	c.SetState(StateConnected)
	return nil
}

// Disconnect 断开连接.
func (c *ClientConn) Disconnect() error {
	c.SetState(StateDisconnected)
	return c.Connection.Close()
}

// Reconnect 重新连接
func (c *ClientConn) Reconnect(sleepSec int) error {
	if c.Connection != nil {
		c.Disconnect()
		if sleepSec > 0 {
			common.Sleep(sleepSec)
		}
		return c.Connect()
	}
	return c.Connect()
}

// openClientConn 建立连接
func openClientConn(uri *url.URL, tlsc *tls.Config, timeout time.Duration) (net.Conn, error) {
	switch uri.Scheme {
	case "tcp":
		conn, err := net.DialTimeout("tcp", uri.Host, timeout)
		if err != nil {
			return nil, err
		}
		return conn, nil
	case "tls":
		conn, err := tls.DialWithDialer(&net.Dialer{Timeout: timeout}, "tcp", uri.Host, tlsc)
		if err != nil {
			return nil, err
		}
		return conn, nil
		// case "quic":
		// 	conn, err := quicconn.Dial(uri.Host, tlsc)
		// 	if err != nil {
		// 		return nil, err
		// 	}
		// 	return conn, nil
	}

	return nil, fmt.Errorf("Unknown protocol: %s", uri.Scheme)
}

package pptcp

import (
	"crypto/tls"
	"net"
)

// TCPServer TCP服务结构体
type TCPServer struct {
	lis net.Listener
	SSL bool
	ct  string
}

// NewTCPServer 创建TCP服务
func NewTCPServer(addr string) (ts *TCPServer, err error) {
	var lis net.Listener
	lis, err = net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}

	ts = new(TCPServer)
	ts.lis = lis
	ts.SSL = false
	ts.ct = "T"
	return
}

// NewTLSTCPServer 创建安全TCP服务(TLS)
func NewTLSTCPServer(addr string, config *tls.Config) (ts *TCPServer, err error) {
	var lis net.Listener
	lis, err = tls.Listen("tcp", addr, config)
	if err != nil {
		return nil, err
	}

	ts = new(TCPServer)
	ts.lis = lis
	ts.SSL = true
	ts.ct = "S"
	return
}

// // NewQUICServer 创建QUIC服务
// func NewQUICServer(addr string, config *tls.Config) (ts *TCPServer, err error) {
// 	var lis net.Listener
// 	lis, err = quicconn.Listen("udp", addr, config)
// 	if err != nil {
// 		return nil, err
// 	}

// 	ts = new(TCPServer)
// 	ts.lis = lis
// 	ts.SSL = true
// 	ts.ct = "Q"
// 	return
// }

// Accept 允许连接接入.
func (ts *TCPServer) Accept() (*Connection, error) {
	conn, err := ts.lis.Accept()
	if err != nil {
		return nil, err
	}

	return NewConnection(conn, ts.ct), nil
}

// Close 关闭服务.
func (ts *TCPServer) Close() error {
	err := ts.lis.Close()
	if err != nil {
		return err
	}

	return nil
}

// Addr 获取监听地址.
func (ts *TCPServer) Addr() net.Addr {
	return ts.lis.Addr()
}

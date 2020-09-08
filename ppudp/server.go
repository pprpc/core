package ppudp

import (
	"context"
	"errors"
	"fmt"
	"net"

	"github.com/pprpc/sess"

	"github.com/pprpc/util/logs"
)

type sendPkg struct {
	data []byte
	addr *net.UDPAddr
}

// UDPServer UDP服务结构体
type UDPServer struct {
	//sync.RWMutex

	ctx       context.Context
	ctxCancel context.CancelFunc
	conn      *net.UDPConn
	conns     *sess.Sessions // 存放所有连接,无序

	newConn     chan *Connection
	closeConn   chan string
	readTimeout int64

	sendChan chan sendPkg
	//bufPool  sync.Pool
}

// NewUDPServer 创建UDP服务
func NewUDPServer(ip string, port, maxSess int) (ts *UDPServer, err error) {
	if maxSess < 0 {
		maxSess = 100
	} else if maxSess == 0 {
		maxSess = 10000000
	}
	ts = new(UDPServer)
	addr := &net.UDPAddr{IP: net.ParseIP(ip), Port: port}
	ts.conn, err = net.ListenUDP("udp", addr)
	if err != nil {
		return nil, err
	}
	//ts.RWMutex = sync.RWMutex{}
	ts.ctx, ts.ctxCancel = context.WithCancel(context.Background())
	ts.conns = sess.NewSessions(int32(maxSess))
	ts.newConn = make(chan *Connection, 1024)
	ts.sendChan = make(chan sendPkg, 2048)
	ts.closeConn = make(chan string, 2048)
	ts.readTimeout = 45
	// ts.bufPool = sync.Pool{
	// 	New: func() interface{} {
	// 		b := make([]byte, 1500)
	// 		return &b
	// 	},
	// }
	go ts.run()
	return
}

// Accept 允许连接接入.
func (ts *UDPServer) Accept() (*Connection, error) {
	select {
	case c := <-ts.newConn:
		return c, nil
	case <-ts.ctx.Done():
		return nil, errors.New("ctx.Donw(), exit Accept")
	}
}

// Close 关闭服务.
func (ts *UDPServer) Close() error {
	err := ts.conn.Close()
	if err != nil {
		return err
	}

	ts.ctxCancel()
	return nil
}

// Addr 获取监听地址.
func (ts *UDPServer) Addr() net.Addr {
	return ts.conn.LocalAddr()
}

// SetReadTimeout 设置读取数据超时时间.
func (ts *UDPServer) SetReadTimeout(to int64) {
	ts.readTimeout = to
}

func (ts *UDPServer) run() {
	go ts.readLoop()
	go ts.writeLoop()
	go ts.closeLoop()
}

func (ts *UDPServer) readLoop() {
	data := make([]byte, MAXPKGSIZE)
	// read
	for {
		select {
		case <-ts.ctx.Done():
			logs.Logger.Warn("ts.ctx.Donw(), exit readLoop.")
			return
		default:
			n, remoteAddr, err := ts.conn.ReadFromUDP(data)
			if err != nil {
				logs.Logger.Debugf("ts.conn.ReadFromUDP(data), error: %s.", err)
				continue
			}
			v, err := ts.conns.Get(remoteAddr.String())
			if err != nil {
				c := NewConnection(ts.ctx, ts.conn, remoteAddr, ts.readTimeout, ts.sendChan, ts.closeConn)
				_, err := ts.conns.Push(remoteAddr.String(), c)
				if err != nil {
					logs.Logger.Debugf("ts.conns.Push(remoteAddr.String(), c), error: %s.", err)
					continue
				}
				ts.newConn <- c
				in := make([]byte, n)
				copy(in, data[:n])
				c.recvChan <- in
			} else {
				in := make([]byte, n)
				copy(in, data[:n])
				v.(*Connection).recvChan <- in
			}
		}
	}
}

func (ts *UDPServer) writeLoop() {
	var err error
	for {
		select {
		case <-ts.ctx.Done():
			logs.Logger.Warn("ts.ctx.Done(), exit writeLoop.")
			return
		case s := <-ts.sendChan:
			_, err = ts.conn.WriteToUDP(s.data, s.addr)
			if err != nil {
				logs.Logger.Warnf("ts.conn.WriteToUDP(s.data, s.addr), error: %s.", err)
				return
			}
		}
	}
}

func (ts *UDPServer) closeLoop() {
	for {
		select {
		case <-ts.ctx.Done():
			logs.Logger.Warn("ts.ctx.Done(), exit writeLoop.")
			return
		case s := <-ts.closeConn:
			ts.conns.Remove(s)
		}
	}
}

// GetUDPConn 输入对端地址，返回一条连接；不会进入到Accept中
func (ts *UDPServer) GetUDPConn(rAddr *net.UDPAddr) (c *Connection, err error) {
	c = NewConnection(ts.ctx, ts.conn, rAddr, ts.readTimeout, ts.sendChan, ts.closeConn)
	_, err = ts.conns.Push(rAddr.String(), c)
	if err != nil {
		logs.Logger.Debugf("ts.conns.Push(remoteAddr.String(), c), error: %s.", err)
		return
	}
	//ts.newConn <- c
	return
}

// WriteToUDP .
func (ts *UDPServer) WriteToUDP(b []byte, raddr *net.UDPAddr) (n int, err error) {
	ts.sendChan <- sendPkg{b, raddr}
	return
}

// WriteToAddr write data to addr.
func (ts *UDPServer) WriteToAddr(b []byte, addr string) (n int, err error) {
	var raddr *net.UDPAddr
	raddr, err = net.ResolveUDPAddr("udp4", addr)
	if err != nil {
		err = fmt.Errorf("net.ResolveUDPAddr(), %s", err)
		return
	}
	ts.sendChan <- sendPkg{b, raddr}
	return
}

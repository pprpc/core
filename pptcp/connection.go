package pptcp

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"

	"github.com/pprpc/util/common"
)

// CloseCallback 连接断开回调定义
type CloseCallback func(*Connection)

// Connection 连接结构体
type Connection struct {
	sync.RWMutex
	net.Conn

	ct string // T=TCP； S=TLS； Q=QUIC; M = mqtt
	// 传入用户自定义的结构体.
	attr interface{}
	// 连接状态:
	state uint32
	// 连接建立时间(ms)
	//CreateTime int64

	Ctx       context.Context
	CtxCancel context.CancelFunc
	// 连接断开回调
	closecb CloseCallback
	//
	AutoCrypt bool
}

// NewConnection 创建连接
func NewConnection(c net.Conn, ct string) *Connection {
	retConn := new(Connection)
	retConn.Conn = c
	retConn.state = StateConnected
	retConn.RWMutex = sync.RWMutex{}
	//retConn.CreateTime = common.GetUTCTimeMs()
	retConn.Ctx, retConn.CtxCancel = context.WithCancel(context.Background())
	retConn.ct = ct
	retConn.closecb = nil
	retConn.AutoCrypt = true

	return retConn
}

// SetCloseCB 设置连接断开回调
func (c *Connection) SetCloseCB(fn CloseCallback) (err error) {
	if c == nil {
		err = fmt.Errorf("not init Connection")
		return
	}
	c.closecb = fn
	return
}

// SetAutoCrypt .
func (c *Connection) SetAutoCrypt(b bool) {
	c.AutoCrypt = b
}

// HandleClose 监视连接断开通知
func (c *Connection) HandleClose() context.Context {
	return c.Ctx
}

// SetAttr 设置连接属性
func (c *Connection) SetAttr(attr interface{}) (err error) {
	if c == nil {
		err = fmt.Errorf("not init Connection")
		return
	}
	c.attr = attr
	return
}

// SetCT set ct
func (c *Connection) SetCT(ct string) (err error) {
	if c == nil {
		err = fmt.Errorf("not init Connection")
		return
	}
	c.ct = ct
	return
}

// GetAttr 获取连接属性
func (c *Connection) GetAttr() (interface{}, error) {
	if c == nil {
		return nil, fmt.Errorf("not init Connection")
	}
	if c.attr == nil {
		return nil, fmt.Errorf("not set attr")
	}
	return c.attr, nil
}

//IsClose 连接是否断开； true 断开： false 连接正常
func (c *Connection) IsClose() bool {
	if c == nil {
		return true
	}

	if atomic.LoadUint32(&c.state) == StateDisconnected {
		return true
	}
	return false
}

// SetState 设置当前状态
func (c *Connection) SetState(v uint32) (err error) {
	if c == nil {
		err = fmt.Errorf("not init Connection")
		return
	}
	atomic.StoreUint32(&c.state, v)
	return
}

// GetState 获取当前连接状态
func (c *Connection) GetState() (v uint32, err error) {
	if c == nil {
		err = fmt.Errorf("not init Connection")
		return
	}

	v = atomic.LoadUint32(&c.state)
	return
}

func (c *Connection) Write(b []byte) (n int, err error) {
	if c == nil {
		return 0, fmt.Errorf("not init Connection")
	}
	if c.IsClose() {
		return 0, fmt.Errorf("use close connection")
	}

	c.Lock()
	defer c.Unlock()
	n, err = c.Conn.Write(b)
	return
}

func (c *Connection) Read(b []byte) (n int, err error) {
	if c == nil {
		return 0, fmt.Errorf("not init Connection")
	}
	if c.IsClose() {
		return 0, fmt.Errorf("use close connection")
	}

	//c.RLock() // RLock 会与 Lock 形成竞争
	//defer c.RUnlock()
	n, err = c.Conn.Read(b)
	return
}

// LogPre 日志前缀
func (c *Connection) LogPre() string {
	return fmt.Sprintf("%s-%s-%s", c.ct, c.LocalAddr(), c.RemoteAddr())
}

// LogPreShort 日志前缀(远端)
func (c *Connection) LogPreShort() string {
	return fmt.Sprintf("%s-%s", c.ct, c.RemoteAddr())
}

func (c *Connection) String() string {
	return fmt.Sprintf("%s-%s-%s", c.ct, common.GetPort(c.LocalAddr()), c.RemoteAddr())
}

// Type 连接类型
func (c *Connection) Type() string {
	return c.ct
}

// Close 关闭连接
func (c *Connection) Close() (err error) {
	if c == nil {
		err = fmt.Errorf("not init Connection")
		return
	}
	c.Lock()
	defer c.Unlock()
	atomic.StoreUint32(&c.state, StateDisconnected)
	//logs.Logger.Debugf("%s", common.CallStack("pptcp.Connection.Close()", 2))
	c.CtxCancel()
	if c.Conn != nil {
		if c.closecb != nil {
			c.closecb(c)
		}
		err = c.Conn.Close()
	}
	if err != nil {
		return
	}
	//c.Conn = nil
	//c = nil
	return
}

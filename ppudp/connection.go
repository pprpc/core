package ppudp

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pprpc/util/common"
)

// CloseCallback 连接端口回调定义
type CloseCallback func(*Connection)

// Connection 连接结构体
type Connection struct {
	pctx      context.Context
	Ctx       context.Context
	CtxCancel context.CancelFunc
	sync.RWMutex
	//
	conn       *net.UDPConn
	remoteAddr *net.UDPAddr
	// Listener 发送数据
	recvChan chan []byte
	//recvErr  chan error

	//需要写入的数据
	sendChan chan sendPkg
	// 关闭连接
	closeChan chan string
	//sendErr  chan error

	readTimeout  time.Time
	writeTimeout time.Time

	// 传入用户自定义的结构体.
	attr interface{}
	// 连接状态:
	state uint32
	// 连接建立时间(ms)
	//CreateTime     int64
	//LastTime       int64
	readTimeoutSec int64

	closecb   CloseCallback
	AutoCrypt bool
}

// NewConnection 创建连接
func NewConnection(ctx context.Context, c *net.UDPConn, remoteAddr *net.UDPAddr, readTimeoutSec int64, sc chan sendPkg, closeChan chan string) *Connection {
	retConn := new(Connection)
	retConn.pctx = ctx
	retConn.Ctx, retConn.CtxCancel = context.WithCancel(ctx)
	retConn.conn = c
	retConn.state = StateConnected
	retConn.RWMutex = sync.RWMutex{}
	//retConn.CreateTime = common.GetUTCTimeMs()
	retConn.recvChan = make(chan []byte, 1024)
	retConn.sendChan = sc
	retConn.remoteAddr = remoteAddr
	retConn.readTimeoutSec = readTimeoutSec
	retConn.closeChan = closeChan
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

// HandleClose 监视连接断开通知
func (c *Connection) HandleClose() context.Context {
	return c.Ctx
}

// SetAutoCrypt .
func (c *Connection) SetAutoCrypt(b bool) {
	c.AutoCrypt = b
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
	if c.remoteAddr != nil {
		c.sendChan <- sendPkg{b, c.remoteAddr}
		n = len(b)
	} else {
		c.Lock()
		defer c.Unlock()

		n, err = c.conn.Write(b)
	}
	return
}

// LogPre 日志前缀
func (c *Connection) LogPre() string {
	return fmt.Sprintf("U-%s-%s", c.LocalAddr(), c.RemoteAddr())
}

// LogPreShort 日志前缀(远端)
func (c *Connection) LogPreShort() string {
	return fmt.Sprintf("U-%s", c.RemoteAddr())
}

func (c *Connection) String() string {
	return fmt.Sprintf("U-%s-%s", common.GetPort(c.LocalAddr()), c.RemoteAddr())
}

// Type 连接类型
func (c *Connection) Type() string {
	return "U"
}

func (c *Connection) Read(b []byte) (n int, err error) {
	if c == nil {
		return 0, fmt.Errorf("not init Connection")
	}
	if c.IsClose() {
		return 0, fmt.Errorf("use close connection")
	}

	if c.remoteAddr != nil {
		select {
		case <-time.After(time.Second * time.Duration(c.readTimeoutSec)):
			err = fmt.Errorf("read timeout %d, %s", c.readTimeoutSec, c.remoteAddr)
			return 0, err
		case data := <-c.recvChan:
			copy(b, data)
			return len(data), nil
		case <-c.Ctx.Done():
			return 0, fmt.Errorf("ctx.Done()")
		}
	} else {
		c.conn.SetReadDeadline(time.Now().Add(time.Duration(c.readTimeoutSec) * time.Second))
		n, err = c.conn.Read(b)
	}
	return
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
	//logs.Logger.Debugf("%s", common.CallStack("ppudp.Connection.Close()", 2))

	c.CtxCancel()
	if c.closecb != nil {
		c.closecb(c)
	}
	if c.remoteAddr != nil {
		c.closeChan <- c.remoteAddr.String()
	} else {
		err = c.conn.Close()
	}

	//c = nil
	return
}

// Connected 判断连接时服务端还是客户端连接
func (c *Connection) Connected() bool { return c.remoteAddr == nil }

func (c *Connection) SetDeadline(t time.Time) error {
	c.readTimeout = t
	c.writeTimeout = t
	return nil
}
func (c *Connection) SetReadDeadline(t time.Time) error {
	c.readTimeout = t
	return nil
}
func (c *Connection) SetWriteDeadline(t time.Time) error {
	c.writeTimeout = t
	return nil
}
func (c *Connection) LocalAddr() net.Addr { return c.conn.LocalAddr() }
func (c *Connection) RemoteAddr() net.Addr {
	if c.remoteAddr != nil {
		return c.remoteAddr
	}
	return c.conn.RemoteAddr()
}

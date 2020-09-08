// Package sess 会话管理
package sess

import (
	"fmt"
	"sync"
	"sync/atomic"
)

// Sessions 会话管理
type Sessions struct {
	count int32
	max   int32
	Conns *sync.Map // 存放所有连接,无序
}

// RangeCall 遍历所有Session回调
type RangeCall func(k, v interface{}) bool

// NewSessions 创建会话, max=0 不做做大限制
func NewSessions(max int32) *Sessions {
	sess := new(Sessions)
	sess.max = max
	sess.count = 0
	sess.Conns = new(sync.Map)
	return sess
}

// Remove 删除连接
func (sess *Sessions) Remove(connid string) {
	_, err := sess.Get(connid)
	if err != nil {
		return
	}

	atomic.AddInt32(&sess.count, -1)
	sess.Conns.Delete(connid)
}

// Push 增加连接
func (sess *Sessions) Push(connid string, v interface{}) (c int32, err error) {
	c = atomic.LoadInt32(&sess.count)
	_, err = sess.Get(connid)
	if err != nil {
		err = nil
		if c >= sess.max && sess.max != 0 {
			err = fmt.Errorf("sess, max connect over flow(%d)", sess.max)
			return
		}
		atomic.AddInt32(&sess.count, 1)
		sess.Conns.Store(connid, v)
		c = c + 1
	} else {
		sess.Conns.Store(connid, v)
	}
	return
}

// Get 获得一条连接
func (sess *Sessions) Get(connid string) (v interface{}, err error) {
	var ok bool
	v, ok = sess.Conns.Load(connid)
	if !ok {
		err = fmt.Errorf("sess, not find: %s", connid)
	}
	return
}

// Range 获取所有连接，通过回调
func (sess *Sessions) Range(fn RangeCall) {
	sess.Conns.Range(fn)
}

// Len 当前连接数
func (sess *Sessions) Len() int32 {
	return atomic.LoadInt32(&sess.count)
}

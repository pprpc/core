package pprpc

import (
	"crypto/rand"
	"crypto/tls"
	"sync/atomic"
	"time"
)

// error
const (
	// CmdIDNotReg 命令ID没有注册，不支持
	CmdIDNotReg uint64 = 1
)

var seqID uint64 // 全局唯一ID

// GetSeqID 获得一个全局的唯一ID（在一定时间范围内,但数字大于 4294836215 会从1开始计数.）
func GetSeqID() uint64 {
	if seqID > 4294836215 {
		seqID = 0
	}
	return atomic.AddUint64(&seqID, 1)
}

// GetTLSConfig 传入TLS Key相关文件，返回配置对象.
func GetTLSConfig(tlsCrt, tlsKey string) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(tlsCrt, tlsKey)
	if err != nil {
		return nil, err
	}
	tlsc := &tls.Config{Certificates: []tls.Certificate{cert}}
	tlsc.Time = time.Now
	tlsc.Rand = rand.Reader

	return tlsc, nil
}

// GetMaxSeqID 设置最大的seqid
func GetMaxSeqID(max uint64) uint64 {
	if seqID > max {
		seqID = 0
	}
	return atomic.AddUint64(&seqID, 1)
}

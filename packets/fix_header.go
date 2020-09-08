package packets

import (
	"bytes"
	"fmt"
	"io"

	"github.com/golang/protobuf/proto"
	"github.com/pprpc/util/common"
)

// FixHeader 固定报头
type FixHeader struct {
	protoType   uint8  // 协议类型
	MessageType uint8  // 协议类型
	Flag        uint8  // 协议标志位
	Length      uint64 // 后续数据长度
	RawHeader   []byte
}

// String 输出FixHeader调试信息
func (fh *FixHeader) String() string {
	return fmt.Sprintf("types: %d, flag: %d, length: %d", fh.MessageType, fh.Flag, fh.Length)
}

// Pack 编码
func (fh *FixHeader) Pack() (header bytes.Buffer, err error) {
	if fh.MessageType < 3 || fh.MessageType > 8 {
		err = fmt.Errorf("types: %d, not support", fh.MessageType)
		return
	}
	if fh.Flag != 8 {
		err = fmt.Errorf("flag: %d, not support", fh.Flag)
		return
	}
	if fh.Length > maxpkg {
		err = fmt.Errorf("Length(%d) over %d", fh.Length, maxpkg)
		return
	}

	fh.RawHeader = []byte{}
	if fh.protoType == PROTOUDP {
		//header.Write(udpPreHeader)
		fh.RawHeader = udpPreHeader
	}
	fh.RawHeader = append(fh.RawHeader, fh.MessageType<<4|fh.Flag)
	fh.RawHeader = append(fh.RawHeader, proto.EncodeVarint(fh.Length)...)
	header.Write(fh.RawHeader)
	return
}

// Unpack 解码
func (fh *FixHeader) Unpack(typeAndFlag byte, r io.Reader) (err error) {
	var _t, _f uint8
	var decLength []byte
	_t, _f, err = checkFirstByte(typeAndFlag)
	if err != nil {
		return
	}

	fh.MessageType = _t
	fh.Flag = _f
	fh.Length, decLength = decodeLength(r)
	fh.RawHeader = append(fh.RawHeader, typeAndFlag)
	fh.RawHeader = append(fh.RawHeader, decLength...)
	return
}

// SetProtocol 设置固定报头的协议类型
func (fh *FixHeader) SetProtocol(p uint8) error {
	if p != PROTOTCP && p != PROTOUDP {
		return fmt.Errorf("Protocol type: %d, not support", p)
	}
	fh.protoType = p
	return nil
}

func (fh *FixHeader) Debug() string {
	return fmt.Sprintf("types: %d, flag: %d, length: %d, Bytes: [%s].",
		fh.MessageType, fh.Flag, fh.Length, common.ByteConvertString(fh.RawHeader))
}

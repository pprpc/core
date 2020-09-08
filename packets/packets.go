// Package packets 处理pprpc协议头信息
package packets

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"github.com/pprpc/util/common"
)

// PPPacket .
type PPPacket interface {
	Write(io.Writer) (int64, error)
	Pack() (packet bytes.Buffer, err error)
	Unpack(io.Reader) error
	String() string
	Debug() string
}

// ReadTCPPacket 读取一个完整的Packet(TCP).
func ReadTCPPacket(r io.Reader) (pp PPPacket, err error) {
	var fh FixHeader
	fh.protoType = PROTOTCP
	b := make([]byte, 1) // sync.Pool

	_, err = io.ReadFull(r, b)
	if err != nil {
		return nil, err
	}
	err = fh.Unpack(b[0], r)
	if err != nil {
		return
	}

	pp = NewPPPacketWithHeader(fh)
	if pp == nil {
		return nil, errors.New("Bad data from client")
	}
	packetBytes := make([]byte, fh.Length)
	_, err = io.ReadFull(r, packetBytes)
	if err != nil {
		return nil, err
	}
	err = pp.Unpack(bytes.NewBuffer(packetBytes))
	if err != nil {
		err = fmt.Errorf("%s, Data: %s", err, common.ByteConvertString(append(fh.RawHeader, packetBytes...)))
	}
	return pp, err
}

// ReadTCPPacketAdv 读取一个完整的Packet(TCP).
func ReadTCPPacketAdv(r io.Reader, ac bool) (pp PPPacket, err error) {
	var fh FixHeader
	fh.protoType = PROTOTCP
	b := make([]byte, 1) // sync.Pool

	_, err = io.ReadFull(r, b)
	if err != nil {
		return nil, err
	}
	err = fh.Unpack(b[0], r)
	if err != nil {
		return
	}

	pp = NewPPPacketWithHeaderV2(fh, ac)
	if pp == nil {
		return nil, errors.New("Bad data from client")
	}
	packetBytes := make([]byte, fh.Length)
	_, err = io.ReadFull(r, packetBytes)
	if err != nil {
		return nil, err
	}
	err = pp.Unpack(bytes.NewBuffer(packetBytes))
	if err != nil {
		err = fmt.Errorf("%s, Data: %s", err, common.ByteConvertString(append(fh.RawHeader, packetBytes...)))
	}
	return pp, err
}

// ReadUDPPacket 读取一个完整的Packet(UDP).
func ReadUDPPacket(r io.Reader) (pp PPPacket, err error) {
	// UDP 必须先读取整个报文
	fb := make([]byte, MAXUDPBUFSIZE)
	n, err := r.Read(fb)
	if err != nil {
		return
	}
	rb := bytes.NewBuffer(fb[:n])

	var fh FixHeader
	fh.protoType = PROTOUDP
	b := make([]byte, 3)
	_, err = io.ReadFull(rb, b)
	if err != nil {
		return nil, err
	}
	if b[0] != udpPreHeader[0] || b[1] != udpPreHeader[1] {
		return nil, fmt.Errorf("not match PreHeader: [%s][%s]",
			common.ByteConvertString(b[:2]), common.ByteConvertString(udpPreHeader))
	}
	err = fh.Unpack(b[2], rb)
	if err != nil {
		return
	}
	// 添加固定报头
	fh.RawHeader = append(udpPreHeader, fh.RawHeader...)

	fh.SetProtocol(PROTOUDP) // 必须设定该值(header自动添加UDP固定报头)
	pp = NewPPPacketWithHeader(fh)
	if pp == nil {
		return nil, errors.New("Bad data from client")
	}
	packetBytes := make([]byte, fh.Length)
	_, err = io.ReadFull(rb, packetBytes)
	if err != nil {
		return nil, err
	}
	err = pp.Unpack(bytes.NewBuffer(packetBytes))
	if err != nil {
		err = fmt.Errorf("%s.\nData: \n%s", err, common.ByteConvertString(append(fh.RawHeader, packetBytes...)))
	}
	return pp, err
}

// NewPPPacketWithHeader  创建PPPacket
func NewPPPacketWithHeader(fh FixHeader) (pp PPPacket) {
	switch fh.MessageType {
	case TYPEHB:
		pp = &HBPacket{FixHeader: fh}
	case TYPEPBBIN:
		_t := NewCmdPacket(TYPEPBBIN)
		_t.FixHeader = fh
		pp = _t
	case TYPEPBJSON:
		_t := NewCmdPacket(TYPEPBJSON)
		_t.FixHeader = fh
		pp = _t
	case TYPEAV:
		_t := NewAVPacket()
		_t.FixHeader = fh
		pp = _t
	case TYPECUSTOMER:
		_t := NewCustomerPacket()
		_t.FixHeader = fh
		pp = _t
	case TYPEFILE:
		_t := NewFilePacket()
		_t.FixHeader = fh
		pp = _t
	default:
		return nil
	}
	return pp
}

// NewPPPacketWithHeaderV2  创建PPPacket
func NewPPPacketWithHeaderV2(fh FixHeader, b bool) (pp PPPacket) {
	switch fh.MessageType {
	case TYPEHB:
		pp = &HBPacket{FixHeader: fh}
	case TYPEPBBIN:
		_t := NewCmdPacket(TYPEPBBIN)
		_t.FixHeader = fh
		_t.AutoCrypt = b
		pp = _t
	case TYPEPBJSON:
		_t := NewCmdPacket(TYPEPBJSON)
		_t.AutoCrypt = b
		_t.FixHeader = fh
		pp = _t
	case TYPEAV:
		_t := NewAVPacket()
		//_t.AutoCrypt = b
		_t.FixHeader = fh
		pp = _t
	case TYPECUSTOMER:
		_t := NewCustomerPacket()
		_t.FixHeader = fh
		pp = _t
	case TYPEFILE:
		_t := NewFilePacket()
		_t.FixHeader = fh
		pp = _t
	default:
		return nil
	}
	return pp
}

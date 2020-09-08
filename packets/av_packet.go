package packets

import (
	"bytes"
	"fmt"
	"io"

	"github.com/golang/protobuf/proto"
	ppcrypto "xcthings.com/hjyz/crypto"
)

// AVPacket 流媒体报文
type AVPacket struct {
	FixHeader
	AutoCrypt  bool   // false: 不自动处理加解密; true: 自动处理加解密
	AVIFrame   uint8  // 是否I帧: 1是；0 否
	AVFormat   uint8  // 流媒体格式
	EncType    uint8  // 流媒体加密类型
	AVChannel  uint64 // 通道编号
	AVSeq      uint64 // 流媒体序列号报文
	Timestamp  uint64 // 媒体包时间戳
	EncLength  uint64 // 加密数据的长度
	VarHeader  []byte
	Key        []byte // 加密用Key,固定部分
	Md5Byte    []byte
	EnKey      []byte
	Payload    []byte // 存放协议载荷
	RAWPayload []byte // 存放原始数据
}

// NewAVPacket  creates a new AVPacket.
func NewAVPacket() (av *AVPacket) {
	av = new(AVPacket)
	av.MessageType = TYPEAV
	av.Flag = FLAGAV
	av.AutoCrypt = false
	av.Key = AESKEYPREFIX

	return
}

func (av *AVPacket) Write(w io.Writer) (int64, error) {
	packet, err := av.Pack()
	if err != nil {
		return 0, err
	}
	n, err := packet.WriteTo(w)
	return n, err
}

// Pack 编码数据包，根据 AutoCrypt 自动处理加密.
// FIXME: 处理指定长度的加密
func (av *AVPacket) Pack() (packet bytes.Buffer, err error) {
	if av.AVIFrame != 0 && av.AVIFrame != 1 {
		err = fmt.Errorf("AvIFrame not support: %d", av.AVIFrame)
		return
	}
	// AVFormat FIXME.
	if aesValidityCheck(av.EncType) == false {
		err = fmt.Errorf("Crypt type not support: %d", av.EncType)
		return
	}
	if av.AVChannel > defmaxValue || av.AVSeq > defmaxValue || av.EncLength > defmaxValue {
		err = fmt.Errorf("AVChannel(%d),AVSeq(%d),EncLength(%d); Overflow(%d)",
			av.AVChannel, av.AVSeq, av.EncLength, defmaxValue)
		return
	}

	// VarHeader
	av.VarHeader = []byte{}
	av.VarHeader = append(av.VarHeader, uint8(av.AVIFrame<<7|av.AVFormat))
	av.VarHeader = append(av.VarHeader, av.EncType)
	av.VarHeader = append(av.VarHeader, proto.EncodeVarint(av.AVChannel)...)
	av.VarHeader = append(av.VarHeader, proto.EncodeVarint(av.AVSeq)...)
	av.VarHeader = append(av.VarHeader, proto.EncodeVarint(av.Timestamp)...)
	av.VarHeader = append(av.VarHeader, proto.EncodeVarint(av.EncLength)...)

	av.GetCryptoKey()
	if len(av.Payload) > 0 && av.AutoCrypt {
		av.Payload, err = Encrypt(av.EncType, av.EnKey, av.EnKey, av.Payload)
		if err != nil {
			return
		}
	}

	av.Length = uint64(len(av.Payload) + len(av.VarHeader))
	packet, err = av.FixHeader.Pack() // FixHeader
	if err != nil {
		return
	}
	packet.Write(av.VarHeader) // VarHeader
	packet.Write(av.Payload)   // Payload

	return
}

// Unpack 解码数据包，根据 AutoCrypt 自动处理解密.
// FIXME: 处理指定长度的解密
func (av *AVPacket) Unpack(r io.Reader) error {
	var varHeader []byte
	var payloadLength = av.FixHeader.Length
	var err error

	av.AVIFrame, av.AVFormat, varHeader, err = decodeAVFormat(r)
	if err != nil {
		return err
	}
	av.VarHeader = append(av.VarHeader, varHeader...)

	av.EncType = decodeUint8(r)
	av.VarHeader = append(av.VarHeader, av.EncType)

	varHeader = []byte{}
	av.AVChannel, varHeader = decodeVarintDef(r)
	av.VarHeader = append(av.VarHeader, varHeader...)

	varHeader = []byte{}
	av.AVSeq, varHeader = decodeVarintDef(r)
	av.VarHeader = append(av.VarHeader, varHeader...)

	varHeader = []byte{}
	av.Timestamp, varHeader = decodeTimestamp(r)
	av.VarHeader = append(av.VarHeader, varHeader...)

	varHeader = []byte{}
	av.EncLength, varHeader = decodeVarintDef(r)
	av.VarHeader = append(av.VarHeader, varHeader...)

	payloadLength = payloadLength - uint64(len(av.VarHeader))
	av.RAWPayload = make([]byte, payloadLength)
	_, err = r.Read(av.RAWPayload)
	if err != nil {
		return err
	}

	if len(av.RAWPayload) > 0 && av.AutoCrypt {
		av.GetCryptoKey()
		av.Payload, err = Decrypt(av.EncType, av.EnKey, av.EnKey, av.RAWPayload)
	} else if len(av.RAWPayload) > 0 && av.AutoCrypt == false {
		av.Payload = av.RAWPayload
	}
	return nil
}

func (av *AVPacket) String() string {
	return fmt.Sprintf("types: %d, flag: %d, length: %d", av.MessageType, av.Flag, av.Length)
}

// Debug debug packet.
func (av *AVPacket) Debug() string {
	return fmt.Sprintf("types: %d, flag: %d, length: %d", av.MessageType, av.Flag, av.Length)
}

// GetCryptoKey get cryptoKey.
func (av *AVPacket) GetCryptoKey() {
	// MD5(fmt.Sprintf("%s,AVSeq:%d-TT:%d-AVChannel:%d", "P2p0r1p8c0622",av.AVSeq, av.Timestamp, av.AVChannel))
	_t := fmt.Sprintf(",AVSeq:%d-TT:%d-AVChannel:%d", av.AVSeq, av.Timestamp, av.AVChannel)
	av.Md5Byte = []byte{}
	av.Md5Byte = append(av.Md5Byte, av.Key...)
	av.Md5Byte = append(av.Md5Byte, []byte(_t)...)
	av.EnKey = []byte(ppcrypto.MD5(av.Md5Byte))
}

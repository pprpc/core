package packets

import (
	"fmt"
	"io"

	"github.com/golang/protobuf/proto"
)

// 结构体
var udpPreHeader []byte

// 协议类型
const (
	// TYPEHB 心跳报文
	TYPEHB uint8 = 3
	// TYPEPBBIN 控制报文，二进制PB编码
	TYPEPBBIN uint8 = 4
	// TYPEPBJSON 控制报文，JSON编码
	TYPEPBJSON uint8 = 5
	// TYPEAV 流媒体报文
	TYPEAV uint8 = 6
	// TYPECUSTOMER 用户自定义报文，透传
	TYPECUSTOMER uint8 = 7
	// TYPEFILE 传输文件报文
	TYPEFILE uint8 = 8
)

// 音视频变量
const (
	// 帧数据类型
	FRAMEI    uint8 = 1 // I帧时该值位1
	FRAMENONE uint8 = 0 // P,B帧或者Audio数据帧

	// Video
	AVH264  uint8 = 1
	AVH265  uint8 = 2
	AVMPEG  uint8 = 3
	AVMJPEG uint8 = 4
	// Audio
	AVG711A uint8 = 21
	AVULAW  uint8 = 31
	AVG711U uint8 = 41
	AVOPUS  uint8 = 51
	AVADPCM uint8 = 61
	AVG721  uint8 = 71
	AVG723  uint8 = 81
	AVG726  uint8 = 91
	AVAAC   uint8 = 101
	AVSPEEX uint8 = 111
	AVPCM   uint8 = 121
)

// AVALL 所有AV格式
var AVALL []uint8 = []uint8{AVH264, AVH265, AVMPEG, AVMJPEG,
	AVG711A, AVULAW, AVG711U, AVPCM, AVADPCM, AVG721, AVG723, AVG726,
	AVAAC, AVSPEEX, AVOPUS}

// 加密类型
const (
	AESNONE uint8 = 0
	// CBC 加密后的数据是16的整数倍
	AES128CBC uint8 = 1
	AES192CBC uint8 = 2
	AES256CBC uint8 = 3
	// CFB 加密前后数据长度不变
	AES128CFB uint8 = 4
	AES192CFB uint8 = 5
	AES256CFB uint8 = 6

	AES128ECB uint8 = 7
	AES192ECB uint8 = 8
	AES256ECB uint8 = 9

	AES128OFB uint8 = 10
	AES192OFB uint8 = 11
	AES256OFB uint8 = 12

	AES128CTR uint8 = 13
	AES192CTR uint8 = 14
	AES256CTR uint8 = 15

/*
	系统目前支持的加密类型：
	0，1，2，3, 4, 5, 6
*/
)

// RPC 类型
const (
	RPCREQ  uint8 = 0
	RPCRESP uint8 = 1
)

// 协议标志位
const (
	FLAGHB       uint8 = 8
	FLAGPBBIN    uint8 = 8
	FLAGPBJSON   uint8 = 8
	FLAGAV       uint8 = 8
	FLAGCUSTOMER uint8 = 8
	FLAGFILE     uint8 = 8
)

// 错误代码
const (
	RETOK  uint8 = 0
	RETERR uint8 = 99
)

const (
	maxpkg      uint64 = 268435455
	defmaxValue uint64 = 268435455
	// PROTOTCP TCP 协议
	PROTOTCP uint8 = 1
	// PROTOUDP UDP协议
	PROTOUDP uint8 = 2

	// MAXUDPBUFSIZE UDP最大报大小
	MAXUDPBUFSIZE int = 1500
)

//AESKEYPREFIX 加密Key前导
var AESKEYPREFIX []byte

func init() {
	udpPreHeader = []byte{0x51, 0x70}
	AESKEYPREFIX = []byte("P2p0r1p8c0622")
}

//aesValidityCheck AES值合法性检查()
func aesValidityCheck(v uint8) bool {
	if v < AESNONE || v > AES256CFB {
		return false
	}
	return true
}

//rpcTypeValidityCheck 检查控制报文类型
func rpcTypeValidityCheck(v uint8) bool {
	if v != RPCREQ && v != RPCRESP {
		return false
	}
	return true
}

//frameValidityCheck 检查帧参数
func frameValidityCheck(v uint8) bool {
	if v != FRAMEI && v != FRAMENONE {
		return false
	}
	return true
}

// avValidityCheck 检查AV类型是否正确
func avValidityCheck(inv uint8) bool {
	for _, v := range AVALL {
		if inv == v {
			return true
		}
	}
	return false
}

func decodeUint8(b io.Reader) uint8 {
	num := make([]byte, 1)
	b.Read(num)
	return uint8(num[0])
}

func decodeLength(r io.Reader) (l uint64, decLength []byte) {
	l, decLength = decodeVarint(r, 4)
	return
}
func decodeTimestamp(r io.Reader) (l uint64, decLength []byte) {
	l, decLength = decodeVarint(r, 9)
	return
}

func decodeUint64(r io.Reader) (l uint64, decLength []byte) {
	l, decLength = decodeVarint(r, 9)
	return
}

func decodeVarintDef(r io.Reader) (l uint64, decLength []byte) {
	l, decLength = decodeVarint(r, 4)
	return
}

func decodeVarint(r io.Reader, bytelen int) (l uint64, decLength []byte) {
	b := make([]byte, 1)
	for i := 0; i < bytelen; i++ {
		io.ReadFull(r, b)
		if uint8(b[0]) > 127 {
			decLength = append(decLength, b...)
		} else {
			decLength = append(decLength, b...)
			break
		}
	}
	l, _ = proto.DecodeVarint(decLength)
	return
}

// checkFirstByte 检查协议的第一位
func checkFirstByte(in byte) (types, flag uint8, err error) {
	types = uint8(in & 0xf0 >> 4)
	if types < 3 || types > 8 {
		err = fmt.Errorf("types: %d, not support", types)
		return
	}

	flag = uint8(in & 0x0f)
	if flag != 8 {
		err = fmt.Errorf("flag: %d, not support", flag)
	}
	return
}

func decodeCMDFlag(r io.Reader) (encType, rpcType uint8, rb byte, err error) {
	b := make([]byte, 1)
	_, err = io.ReadFull(r, b)
	if err != nil {
		return
	}
	rb = b[0]
	encType = uint8(b[0] >> 2)
	rpcType = uint8(b[0] & 0x03)
	return
}

func decodeAVFormat(r io.Reader) (iFrame, format uint8, b []byte, err error) {
	b = make([]byte, 1)
	_, err = io.ReadFull(r, b)
	if err != nil {
		return
	}
	iFrame = uint8(b[0] >> 7)
	format = uint8(b[0] & 0x7f)
	return
}

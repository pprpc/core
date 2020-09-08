package packets

import (
	"bytes"
	"fmt"
	"io"

	"github.com/golang/protobuf/proto"
	"github.com/pprpc/util/common"
)

/*
fileid|Varint|文件标识ID，用于唯一标识一个文件.
offset|Varint| 后续内容相对于文件开始的偏移量.
EncryptType|uint8|文件流数据加密类型,具体参见[9. 加密类型](#9-加密类型)详细定义
EncryptLength|Varint|加密数据的长度,对于文件流加密的数据长度;0,表示Payload数据全部加密;最大值: 268435455
*/

// FilePacket 自定义报文
type FilePacket struct {
	FixHeader
	FileID        uint64
	Offset        uint64
	EncryptType   uint8
	EncryptLength uint64
	VarHeader     []byte
	Payload       []byte
}

// NewFilePacket  creates a new FilePacket.
func NewFilePacket() (fp *FilePacket) {
	fp = new(FilePacket)
	fp.MessageType = TYPEFILE
	fp.Flag = FLAGFILE

	return
}

func (fp *FilePacket) Write(w io.Writer) (int64, error) {
	packet, err := fp.Pack()
	if err != nil {
		return 0, err
	}
	n, err := packet.WriteTo(w)
	return n, err
}

// Pack encode packet.
func (fp *FilePacket) Pack() (packet bytes.Buffer, err error) {
	fp.VarHeader = []byte{}
	fp.VarHeader = append(fp.VarHeader, proto.EncodeVarint(fp.FileID)...)
	fp.VarHeader = append(fp.VarHeader, proto.EncodeVarint(fp.Offset)...)
	fp.VarHeader = append(fp.VarHeader, fp.EncryptType)
	fp.VarHeader = append(fp.VarHeader, proto.EncodeVarint(fp.EncryptLength)...)

	fp.Length = uint64(len(fp.Payload) + len(fp.VarHeader))
	packet, err = fp.FixHeader.Pack() // FixHeader
	if err != nil {
		return
	}
	packet.Write(fp.VarHeader) // VarHeader
	packet.Write(fp.Payload)   // Payload

	return
}

// Unpack decode packet.
func (fp *FilePacket) Unpack(r io.Reader) error {
	var varHeader []byte
	var payloadLength = fp.FixHeader.Length
	var err error

	// fileid
	varHeader = []byte{}
	fp.FileID, varHeader = decodeUint64(r)
	fp.VarHeader = append(fp.VarHeader, varHeader...)
	// offset
	varHeader = []byte{}
	fp.Offset, varHeader = decodeUint64(r)
	fp.VarHeader = append(fp.VarHeader, varHeader...)
	// encrypt type
	fp.EncryptType = decodeUint8(r)
	fp.VarHeader = append(fp.VarHeader, fp.EncryptType)
	// encrypt length
	varHeader = []byte{}
	fp.EncryptLength, varHeader = decodeUint64(r)
	fp.VarHeader = append(fp.VarHeader, varHeader...)
	// Payload
	payloadLength = payloadLength - uint64(len(fp.VarHeader))
	fp.Payload = make([]byte, payloadLength)
	_, err = r.Read(fp.Payload)
	if err != nil {
		return err
	}

	return nil
}

func (fp *FilePacket) String() string {
	return fmt.Sprintf("types: %d, flag: %d, length: %d", fp.MessageType, fp.Flag, fp.Length)
}

// Debug debug packet.
func (fp *FilePacket) Debug() string {
	return fmt.Sprintf("types: %d, flag: %d, length: %d,RawHeader: %s, Payload: [%s].",
		fp.MessageType, fp.Flag, fp.Length, common.ByteConvertString(fp.RawHeader), fp.Payload)
}

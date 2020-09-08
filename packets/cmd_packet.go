package packets

import (
	"bytes"
	"fmt"
	"io"

	"github.com/golang/protobuf/proto"
	ppcrypto "github.com/pprpc/util/crypto"
)

// CmdPacket 控制报文
type CmdPacket struct {
	FixHeader
	AutoCrypt bool   // false: 不自动处理加解密; true: 自动处理加解密
	CmdSeq    uint64 // 命令序列号
	CmdID     uint64 // 命令ID
	CmdName   string // 命令ID对应的名称
	EncType   uint8  // 加密类型
	RPCType   uint8  // 控制报文类型: 0 请求; 1 应答
	Code      uint64 // RPCType == 1 存在该值
	VarHeader []byte
	Key       []byte // 加密用Key,固定部分
	Md5Byte   []byte // 加密使用的key
	EnKey     []byte // 加密最后使用的Key
	//CipherPyaload []byte // 加密协议载荷
	//TextPayload   []byte // 明文协议载荷
	Payload    []byte // 存放协议载荷
	RAWPayload []byte // 存放原始数据
}

// NewCmdPacket  creates a new CmdPacket.
func NewCmdPacket(t uint8) (cmd *CmdPacket) {
	cmd = new(CmdPacket)
	if t == TYPEPBJSON {
		cmd.MessageType = TYPEPBJSON
		cmd.Flag = FLAGPBJSON
	} else if t == TYPEPBBIN {
		cmd.MessageType = TYPEPBBIN
		cmd.Flag = FLAGPBBIN
	}
	cmd.AutoCrypt = true
	cmd.Key = AESKEYPREFIX

	return
}

func (cmd *CmdPacket) Write(w io.Writer) (int64, error) {
	packet, err := cmd.Pack()
	if err != nil {
		return 0, err
	}
	n, err := packet.WriteTo(w)
	return n, err
}

// Pack 编码数据包，根据 AutoCrypt 自动处理加密.
func (cmd *CmdPacket) Pack() (packet bytes.Buffer, err error) {
	if aesValidityCheck(cmd.EncType) == false {
		err = fmt.Errorf("Crypt type not support: %d", cmd.EncType)
		return
	}
	if rpcTypeValidityCheck(cmd.RPCType) == false {
		err = fmt.Errorf("RPCType not support: %d", cmd.RPCType)
		return
	}
	if cmd.CmdID > defmaxValue || cmd.CmdSeq > defmaxValue || cmd.Code > defmaxValue {
		err = fmt.Errorf("CmdID(%d),CmdSeq(%d),Code(%d); Overflow(%d)", cmd.CmdID, cmd.CmdSeq, cmd.Code, defmaxValue)
		return
	}

	// VarHeader
	cmd.VarHeader = []byte{}
	cmd.VarHeader = append(cmd.VarHeader, proto.EncodeVarint(cmd.CmdSeq)...)
	cmd.VarHeader = append(cmd.VarHeader, proto.EncodeVarint(cmd.CmdID)...)
	cmd.VarHeader = append(cmd.VarHeader, uint8(cmd.EncType<<2|cmd.RPCType))
	if cmd.RPCType == RPCRESP {
		cmd.VarHeader = append(cmd.VarHeader, proto.EncodeVarint(cmd.Code)...)
	}

	if len(cmd.Payload) > 0 && cmd.AutoCrypt {
		cmd.GetCryptoKey()
		cmd.Payload, err = Encrypt(cmd.EncType, cmd.EnKey, cmd.EnKey, cmd.Payload)
		if err != nil {
			return
		}
	}

	cmd.Length = uint64(len(cmd.Payload) + len(cmd.VarHeader))
	packet, err = cmd.FixHeader.Pack() // FixHeader
	if err != nil {
		return
	}
	packet.Write(cmd.VarHeader) // VarHeader
	packet.Write(cmd.Payload)   // Payload
	return
}

// Unpack 解码数据包，根据 AutoCrypt 自动处理解密.
func (cmd *CmdPacket) Unpack(r io.Reader) error {
	var varHeader []byte
	var payloadLength = cmd.FixHeader.Length
	var b byte
	var err error

	cmd.CmdSeq, varHeader = decodeVarintDef(r)
	cmd.VarHeader = append(cmd.VarHeader, varHeader...)
	cmd.CmdID, varHeader = decodeVarintDef(r)
	cmd.VarHeader = append(cmd.VarHeader, varHeader...)
	cmd.EncType, cmd.RPCType, b, err = decodeCMDFlag(r)
	if err != nil {
		return err
	}
	cmd.VarHeader = append(cmd.VarHeader, b)
	if aesValidityCheck(cmd.EncType) == false {
		return fmt.Errorf("Crypt type not support: %d", cmd.EncType)
	}
	if rpcTypeValidityCheck(cmd.RPCType) == false {
		return fmt.Errorf("RPCType not support: %d", cmd.RPCType)
	}
	if cmd.RPCType == RPCRESP {
		cmd.Code, varHeader = decodeVarintDef(r)
		cmd.VarHeader = append(cmd.VarHeader, varHeader...)
	}
	payloadLength = payloadLength - uint64(len(cmd.VarHeader))
	cmd.RAWPayload = make([]byte, payloadLength)
	_, err = r.Read(cmd.RAWPayload)
	if err != nil {
		return err
	}

	if len(cmd.RAWPayload) > 0 && cmd.AutoCrypt {
		cmd.GetCryptoKey()
		cmd.Payload, err = Decrypt(cmd.EncType, cmd.EnKey, cmd.EnKey, cmd.RAWPayload)
	} else if len(cmd.RAWPayload) > 0 && cmd.AutoCrypt == false {
		cmd.Payload = cmd.RAWPayload
	}
	return err
}

func (cmd *CmdPacket) String() string {
	return fmt.Sprintf("types: %d, flag: %d, length: %d", cmd.MessageType, cmd.Flag, cmd.Length)
}

// Debug debug packet.
func (cmd *CmdPacket) Debug() string {
	return fmt.Sprintf("types: %d, flag: %d, length: %d", cmd.MessageType, cmd.Flag, cmd.Length)
}

// GetCryptoKey get cryptoKey.
func (cmd *CmdPacket) GetCryptoKey() {
	//md5(fmt.Sprintf("%s,ID:%d-SEQ:%d-RPC:%d", "P2p0r1p8c0622",cmd.CmdID, cmd.CmdSeq, cmd.RPCType))
	_t := fmt.Sprintf(",ID:%d-SEQ:%d-RPC:%d", cmd.CmdID, cmd.CmdSeq, cmd.RPCType)
	cmd.Md5Byte = []byte{}
	cmd.Md5Byte = append(cmd.Md5Byte, cmd.Key...)
	cmd.Md5Byte = append(cmd.Md5Byte, []byte(_t)...)
	cmd.EnKey = []byte(ppcrypto.MD5(cmd.Md5Byte))
}

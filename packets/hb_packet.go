package packets

import (
	"bytes"
	"fmt"
	"io"

	"xcthings.com/hjyz/common"
)

// HBPacket 心跳报文结构体
type HBPacket struct {
	FixHeader
}

// NewHBPacket  creates a new HBPacket.
func NewHBPacket() (hbp *HBPacket) {
	hbp = new(HBPacket)
	hbp.MessageType = TYPEHB
	hbp.Flag = FLAGHB
	hbp.Length = 0

	return
}

func (hb *HBPacket) Write(w io.Writer) (int64, error) {
	packet, err := hb.FixHeader.Pack()
	if err != nil {
		return 0, err
	}
	n, err := packet.WriteTo(w)
	return n, err
}

// Pack encode packet.
func (hb *HBPacket) Pack() (header bytes.Buffer, err error) {
	header, err = hb.FixHeader.Pack()
	return
}

// Unpack decode packet.
func (hb *HBPacket) Unpack(b io.Reader) error {
	return nil
}

func (hb *HBPacket) String() string {
	return fmt.Sprintf("types: %d, flag: %d, length: %d", hb.MessageType, hb.Flag, hb.Length)
}

// Debug debug packet.
func (hb *HBPacket) Debug() string {
	return fmt.Sprintf("Hex: %s", common.ByteConvertString(hb.RawHeader))
}

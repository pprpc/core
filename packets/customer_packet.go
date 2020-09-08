package packets

import (
	"bytes"
	"fmt"
	"io"

	"xcthings.com/hjyz/common"
)

// CustomerPacket 自定义报文
type CustomerPacket struct {
	FixHeader
	Payload []byte
}

// NewCustomerPacket  creates a new CustomerPacket.
func NewCustomerPacket() (cus *CustomerPacket) {
	cus = new(CustomerPacket)
	cus.MessageType = TYPECUSTOMER
	cus.Flag = FLAGCUSTOMER

	return
}

func (cus *CustomerPacket) Write(w io.Writer) (int64, error) {
	packet, err := cus.Pack()
	if err != nil {
		return 0, err
	}
	n, err := packet.WriteTo(w)
	return n, err
}

// Pack encode packet.
func (cus *CustomerPacket) Pack() (packet bytes.Buffer, err error) {
	cus.Length = uint64(len(cus.Payload))
	packet, err = cus.FixHeader.Pack()
	packet.Write(cus.Payload)
	return
}

// Unpack decode packet.
func (cus *CustomerPacket) Unpack(r io.Reader) error {
	cus.Payload = make([]byte, cus.Length)
	_, err := r.Read(cus.Payload)
	if err != nil {
		return err
	}
	return nil
}

func (cus *CustomerPacket) String() string {
	return fmt.Sprintf("types: %d, flag: %d, length: %d", cus.MessageType, cus.Flag, cus.Length)
}

// Debug debug packet.
func (cus *CustomerPacket) Debug() string {
	return fmt.Sprintf("types: %d, flag: %d, length: %d,RawHeader: %s, Payload: [%s].",
		cus.MessageType, cus.Flag, cus.Length, common.ByteConvertString(cus.RawHeader), cus.Payload)
}

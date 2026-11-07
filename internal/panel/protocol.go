// Package panel speaks the bulk USB protocol of Thermalright "USBDISPLAY" panels
// (vendor 0x0416, product 0x5408, e.g. the Trofeo Vision 9.16, 1920x462).
//
// Worked out from observed USB traffic; see docs/PROTOCOL.md.
//
//	Handshake  OUT 2048 bytes: 02 ff 00 00 00 00 00 00 01, rest zero
//	           IN  512 bytes:  03 ff ... 01 ... then u32 device id @16, u32 width @24, u32 model @32
//	Frame      a baseline JPEG split into 512-byte packets:
//	             0  01 ff
//	             2  u32 LE total JPEG length
//	             6  u16 LE payload bytes in this packet (<= 496)
//	             8  01
//	             9  u16 LE packet count
//	            11  u32 LE packet index
//	            15  00
//	            16  payload
//	           packets are written 8 at a time (4096-byte bulk writes); the final write is
//	           padded with all-zero packets to a whole number of 2048 bytes; then a 512-byte
//	           status read (03 ff ...). The panel's decoder needs baseline 4:4:4 JPEG.
package panel

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	PacketSize    = 512
	HeaderSize    = 16
	PayloadSize   = PacketSize - HeaderSize // 496
	PacketsPerUSB = 8
	HandshakeSize = 2048
)

// HandshakeRequest is the 2048-byte hello the host sends first.
func HandshakeRequest() []byte {
	b := make([]byte, HandshakeSize)
	b[0], b[1], b[8] = 0x02, 0xff, 0x01
	return b
}

// Info is what the panel reports in its handshake reply.
type Info struct {
	DeviceID uint32
	Width    int
	Model    int
	Raw      []byte
}

// ParseHandshake reads the panel's reply.
func ParseHandshake(b []byte) (Info, error) {
	if len(b) < 48 || b[0] != 0x03 || b[1] != 0xff {
		return Info{}, fmt.Errorf("unexpected handshake reply (% x)", head(b, 16))
	}
	return Info{
		DeviceID: binary.LittleEndian.Uint32(b[16:]),
		Width:    int(binary.LittleEndian.Uint32(b[24:])),
		Model:    int(binary.LittleEndian.Uint32(b[32:])),
		Raw:      append([]byte(nil), b...),
	}, nil
}

// Packets splits a JPEG into the panel's 512-byte packets, grouped into bulk writes.
func Packets(jpeg []byte) ([][]byte, error) {
	if len(jpeg) < 4 || jpeg[0] != 0xff || jpeg[1] != 0xd8 {
		return nil, errors.New("panel: frame is not a JPEG")
	}
	count := (len(jpeg) + PayloadSize - 1) / PayloadSize
	if count > 0xffff {
		return nil, fmt.Errorf("panel: frame too large (%d bytes)", len(jpeg))
	}
	var writes [][]byte
	var cur []byte
	for i := 0; i < count; i++ {
		start := i * PayloadSize
		end := min(start+PayloadSize, len(jpeg))
		p := make([]byte, PacketSize)
		p[0], p[1] = 0x01, 0xff
		binary.LittleEndian.PutUint32(p[2:], uint32(len(jpeg)))
		binary.LittleEndian.PutUint16(p[6:], uint16(end-start))
		p[8] = 0x01
		binary.LittleEndian.PutUint16(p[9:], uint16(count))
		binary.LittleEndian.PutUint32(p[11:], uint32(i))
		copy(p[HeaderSize:], jpeg[start:end])
		cur = append(cur, p...)
		if len(cur) == PacketSize*PacketsPerUSB {
			writes = append(writes, cur)
			cur = nil
		}
	}
	if len(cur) > 0 {
		// The final write must be a whole number of 2048 bytes (4 packets); the panel ignores
		// frames whose last write is 1024 or 3072 bytes. Pad with all-zero packets.
		if pad := len(cur) % 2048; pad != 0 {
			cur = append(cur, make([]byte, 2048-pad)...)
		}
		writes = append(writes, cur)
	}
	return writes, nil
}

func head(b []byte, n int) []byte {
	if len(b) < n {
		return b
	}
	return b[:n]
}

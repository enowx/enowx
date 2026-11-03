// Package proto defines the wire protocol between the enowX app and the
// enowxd daemon.
//
// A connection carries frames in both directions. Each frame has a type, a
// channel and a payload:
//
//	+--------+--------+-----------+----------+
//	| length | type   | channel   | payload  |
//	| u32 BE | u8     | u32 BE    | length-5 |
//	+--------+--------+-----------+----------+
//
// Control frames (Hello, Request, Response, Event) use channel 0 and carry
// JSON. Stream frames (Output, Resize, Input) use the channel of a stream
// returned by session.attach and carry raw bytes.
package proto

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// Version is the protocol version: the framing and handshake, which change
// only incompatibly. Methods, events and fields are added within a version
// (docs/protocol.md). A connection speaks the lower of the two sides'
// versions, which must be MinVersion or later.
const (
	Version    = 1
	MinVersion = 1
)

// Negotiate is the version a side speaking Version speaks with one that
// said theirs in its Hello, and whether it can.
func Negotiate(theirs int) (int, bool) {
	v := min(theirs, Version)
	return v, v >= MinVersion
}

// MaxPayload bounds a frame's payload so a corrupt length can't exhaust memory.
const MaxPayload = 16 << 20

// Type is the kind of a frame.
type Type uint8

const (
	// TypeHello opens a connection in both directions (JSON Hello).
	TypeHello Type = 1
	// TypeRequest is a client call (JSON Request).
	TypeRequest Type = 2
	// TypeResponse answers a request (JSON Response).
	TypeResponse Type = 3
	// TypeEvent is pushed by the daemon (JSON Event).
	TypeEvent Type = 4
	// TypeOutput carries terminal frames from the daemon to a stream.
	TypeOutput Type = 5
	// TypeResize tells a stream the daemon's screen size before the
	// frames drawn at it (payload: u16 cols, u16 rows, big endian).
	TypeResize Type = 6
	// TypeInput carries typed bytes from a stream to the daemon.
	TypeInput Type = 7
	// TypeTunnelData carries bytes of a tunnel (tunnel.open), both ways.
	TypeTunnelData Type = 8
	// TypeTunnelClose ends a tunnel, from either side.
	TypeTunnelClose Type = 9
	// TypeTunnelAck gives the other side back credit for the bytes it
	// sent, once taken (payload: u32 count, big endian).
	TypeTunnelAck Type = 10
)

func (t Type) String() string {
	switch t {
	case TypeHello:
		return "hello"
	case TypeRequest:
		return "request"
	case TypeResponse:
		return "response"
	case TypeEvent:
		return "event"
	case TypeOutput:
		return "output"
	case TypeResize:
		return "resize"
	case TypeInput:
		return "input"
	case TypeTunnelData:
		return "tunnel-data"
	case TypeTunnelClose:
		return "tunnel-close"
	case TypeTunnelAck:
		return "tunnel-ack"
	case TypePart:
		return "part"
	}
	return fmt.Sprintf("type(%d)", uint8(t))
}

// Frame is one unit on the wire.
type Frame struct {
	Type    Type
	Channel uint32
	Payload []byte
}

const headerSize = 4 + 1 + 4

// WriteFrame writes f to w in one call, so writers serialized by a mutex
// never interleave frames.
func WriteFrame(w io.Writer, f Frame) error {
	if len(f.Payload) > MaxPayload {
		return fmt.Errorf("proto: payload of %d bytes exceeds limit", len(f.Payload))
	}
	buf := make([]byte, headerSize+len(f.Payload))
	binary.BigEndian.PutUint32(buf[0:4], uint32(1+4+len(f.Payload)))
	buf[4] = byte(f.Type)
	binary.BigEndian.PutUint32(buf[5:9], f.Channel)
	copy(buf[headerSize:], f.Payload)
	_, err := w.Write(buf)
	return err
}

// ReadFrame reads the next frame from r.
func ReadFrame(r io.Reader) (Frame, error) { return ReadFrameMax(r, MaxPayload) }

// ReadFrameMax is ReadFrame refusing a payload over max bytes, read from
// the header before anything is allocated for it: a connection not yet
// trusted gets no room to make the reader hold much.
func ReadFrameMax(r io.Reader, max int) (Frame, error) {
	var hdr [headerSize]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return Frame{}, err
	}
	n := binary.BigEndian.Uint32(hdr[0:4])
	if n < 5 {
		return Frame{}, errors.New("proto: frame too short")
	}
	size := int(n) - 5
	if size > min(max, MaxPayload) {
		return Frame{}, fmt.Errorf("proto: payload of %d bytes exceeds limit", size)
	}
	f := Frame{Type: Type(hdr[4]), Channel: binary.BigEndian.Uint32(hdr[5:9])}
	if size > 0 {
		f.Payload = make([]byte, size)
		if _, err := io.ReadFull(r, f.Payload); err != nil {
			return Frame{}, err
		}
	}
	return f, nil
}

// EncodeSize is the payload of a TypeResize frame.
func EncodeSize(cols, rows int) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint16(b[0:2], uint16(cols))
	binary.BigEndian.PutUint16(b[2:4], uint16(rows))
	return b
}

// DecodeSize reads the payload of a TypeResize frame.
func DecodeSize(b []byte) (cols, rows int, err error) {
	if len(b) != 4 {
		return 0, 0, errors.New("proto: bad resize payload")
	}
	return int(binary.BigEndian.Uint16(b[0:2])), int(binary.BigEndian.Uint16(b[2:4])), nil
}

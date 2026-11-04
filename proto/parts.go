package proto

import (
	"encoding/binary"
	"errors"
	"fmt"
	"iter"
)

// A frame too large for MaxPayload goes as parts, between sides that both said
// Hello.Parts: a canvas's page with its pictures written in, or a design's
// picture. Each part is a frame of its own, so what else the connection carries
// goes on between them, and parts of different messages may interleave.
//
//	+------------+---------------+---------+---------+------+
//	| inner type | inner channel | index   | total   | data |
//	| u8         | u32 BE        | u32 BE  | u32 BE  |      |
//	+------------+---------------+---------+---------+------+
//
// The part frame's own channel is the message's number, the sender's to choose.
const (
	// TypePart is a piece of a larger frame.
	TypePart Type = 11
	// MaxMessage bounds a frame sent as parts, what a receiver holds of one.
	MaxMessage = 256 << 20
	// partData is how much of the message a part holds.
	partData = 8 << 20
	// partHeader is the size of what a part says of its message.
	partHeader = 1 + 4 + 4 + 4
)

// ErrTooLarge is a frame this connection can't carry: larger than MaxMessage,
// or than MaxPayload to a side that takes no parts.
var ErrTooLarge = errors.New("proto: frame too large for this connection")

// FitsParts says whether a frame of payload size goes as parts.
func FitsParts(size int) bool { return size <= MaxMessage }

// Parts are the frames that carry f as message number id.
func Parts(f Frame, id uint32) iter.Seq[Frame] {
	total := max((len(f.Payload)+partData-1)/partData, 1)
	return func(yield func(Frame) bool) {
		for i := range total {
			data := f.Payload[i*partData : min((i+1)*partData, len(f.Payload))]
			payload := make([]byte, partHeader+len(data))
			payload[0] = byte(f.Type)
			binary.BigEndian.PutUint32(payload[1:5], f.Channel)
			binary.BigEndian.PutUint32(payload[5:9], uint32(i))
			binary.BigEndian.PutUint32(payload[9:13], uint32(total))
			copy(payload[partHeader:], data)
			if !yield(Frame{Type: TypePart, Channel: id, Payload: payload}) {
				return
			}
		}
	}
}

// Assembler puts frames sent as parts back together. It belongs to the
// goroutine reading the connection.
type Assembler struct {
	open map[uint32]*partial
	held int
}

type partial struct {
	f     Frame
	next  uint32
	total uint32
}

// Take adds part f, and gives the whole frame once its last part came. An error
// is a side not following the protocol, which ends the connection.
func (a *Assembler) Take(f Frame) (Frame, bool, error) {
	if len(f.Payload) < partHeader {
		return Frame{}, false, errors.New("proto: part too short")
	}
	inner := Type(f.Payload[0])
	if inner == TypePart || inner == TypeHello {
		return Frame{}, false, fmt.Errorf("proto: %s frame in parts", inner)
	}
	index := binary.BigEndian.Uint32(f.Payload[5:9])
	total := binary.BigEndian.Uint32(f.Payload[9:13])
	data := f.Payload[partHeader:]
	p := a.open[f.Channel]
	if p == nil {
		if a.open == nil {
			a.open = map[uint32]*partial{}
		}
		p = &partial{f: Frame{Type: inner, Channel: binary.BigEndian.Uint32(f.Payload[1:5])}, total: total}
		a.open[f.Channel] = p
	}
	if index != p.next || total != p.total || total == 0 {
		return Frame{}, false, fmt.Errorf("proto: part %d of %d out of order in message %d", index, total, f.Channel)
	}
	if a.held+len(data) > MaxMessage {
		return Frame{}, false, fmt.Errorf("proto: parts of over %d bytes", MaxMessage)
	}
	p.f.Payload = append(p.f.Payload, data...)
	p.next++
	a.held += len(data)
	if p.next < p.total {
		return Frame{}, false, nil
	}
	delete(a.open, f.Channel)
	a.held -= len(p.f.Payload)
	return p.f, true, nil
}

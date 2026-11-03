package proto

import (
	"bytes"
	"slices"
	"testing"
)

func TestPartsMakeTheFrameAgain(t *testing.T) {
	big := bytes.Repeat([]byte("abc"), 10<<20)
	other := []byte("short")
	a, b := slices.Collect(Parts(Frame{Type: TypeEvent, Payload: big}, 1)), slices.Collect(Parts(Frame{Type: TypeResponse, Channel: 7, Payload: other}, 2))
	if len(a) < 2 || len(b) != 1 {
		t.Fatalf("%d and %d parts", len(a), len(b))
	}
	var asm Assembler
	// Another message's part between them, as the connection may carry it.
	f, done, err := asm.Take(a[0])
	if done || err != nil {
		t.Fatal(done, err)
	}
	f, done, err = asm.Take(b[0])
	if !done || err != nil || f.Type != TypeResponse || f.Channel != 7 || !bytes.Equal(f.Payload, other) {
		t.Fatalf("short message: %v %v %+v", done, err, f.Type)
	}
	for _, p := range a[1:] {
		if len(p.Payload) > MaxPayload {
			t.Fatalf("a part of %d bytes", len(p.Payload))
		}
		if f, done, err = asm.Take(p); err != nil {
			t.Fatal(err)
		}
	}
	if !done || f.Type != TypeEvent || !bytes.Equal(f.Payload, big) {
		t.Fatalf("whole message: done %v, %d bytes", done, len(f.Payload))
	}
	if asm.held != 0 || len(asm.open) != 0 {
		t.Fatalf("still held: %d bytes, %d messages", asm.held, len(asm.open))
	}
}

func TestAssemblerRefusesWhatDoesntFollow(t *testing.T) {
	parts := slices.Collect(Parts(Frame{Type: TypeEvent, Payload: make([]byte, 20<<20)}, 1))
	var asm Assembler
	if _, _, err := asm.Take(parts[1]); err == nil {
		t.Fatal("a part out of order was taken")
	}
	if _, _, err := new(Assembler).Take(Frame{Type: TypePart, Payload: []byte{1}}); err == nil {
		t.Fatal("a part too short was taken")
	}
	nested := slices.Collect(Parts(Frame{Type: TypePart, Payload: []byte("x")}, 1))
	if _, _, err := new(Assembler).Take(nested[0]); err == nil {
		t.Fatal("a part in parts was taken")
	}
	// A peer can't make the reader hold more than a message's limit.
	var greedy Assembler
	greedy.held = MaxMessage
	if _, _, err := greedy.Take(parts[0]); err == nil {
		t.Fatal("over the limit was taken")
	}
}

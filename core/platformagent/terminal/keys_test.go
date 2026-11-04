package terminal

import (
	"reflect"
	"testing"
)

func kindsOf(keys []Key) []KeyKind {
	var out []KeyKind
	for _, k := range keys {
		out = append(out, k.Kind)
	}
	return out
}

// A paste is one key wherever reads cut it, its line breaks kept.
func TestPasteSplitAcrossReads(t *testing.T) {
	data := []byte("\x1b[200~first\r\nsecond é\nthird\x1b[31m\x1b[201~\r")
	for cut := 1; cut < len(data); cut++ {
		var p Parser
		keys := append(p.Feed(data[:cut]), p.Feed(data[cut:])...)
		if len(keys) != 2 || keys[0].Kind != KeyPaste || keys[1].Kind != KeyEnter {
			t.Fatalf("cut at %d: %+v", cut, keys)
		}
		if want := "first\nsecond é\nthird[31m"; keys[0].Text != want {
			t.Fatalf("cut at %d: pasted %q, want %q", cut, keys[0].Text, want)
		}
	}
}

func TestLoneEscWaitsToBeToldFromASequence(t *testing.T) {
	var p Parser
	if keys := p.Feed([]byte("\x1b")); len(keys) != 0 || !p.Pending() {
		t.Fatalf("keys %v, pending %v", keys, p.Pending())
	}
	if keys := p.Flush(); !reflect.DeepEqual(kindsOf(keys), []KeyKind{KeyEsc}) {
		t.Fatalf("flush: %v", keys)
	}
	if p.Pending() {
		t.Fatal("still pending")
	}

	// The same bytes, the rest in time: Shift+Tab, however they're cut.
	data := []byte("\x1b[Z")
	for cut := 1; cut < len(data); cut++ {
		var p Parser
		keys := append(p.Feed(data[:cut]), p.Feed(data[cut:])...)
		if !reflect.DeepEqual(kindsOf(keys), []KeyKind{KeyShiftTab}) {
			t.Fatalf("cut at %d: %v", cut, keys)
		}
	}

	// A cut sequence flushed is nothing, not an Esc.
	var q Parser
	q.Feed([]byte("\x1b["))
	if keys := q.Flush(); len(keys) != 0 {
		t.Fatalf("cut sequence: %v", keys)
	}
}

func TestEscThenTextInALaterRead(t *testing.T) {
	var p Parser
	keys := append(p.Feed([]byte("\x1b")), p.Feed([]byte("hi"))...)
	if !reflect.DeepEqual(kindsOf(keys), []KeyKind{KeyEsc, KeyRune, KeyRune}) {
		t.Fatalf("%+v", keys)
	}
	// In one read it is Alt+h, which terminals send so, and nothing here.
	var q Parser
	if keys := q.Feed([]byte("\x1bh")); len(keys) != 0 {
		t.Fatalf("Alt+h: %+v", keys)
	}
	// Esc, then at once a sequence.
	var r Parser
	if keys := r.Feed([]byte("\x1b\x1b[Z")); !reflect.DeepEqual(kindsOf(keys), []KeyKind{KeyEsc, KeyShiftTab}) {
		t.Fatalf("%+v", keys)
	}
}

func TestKeysOfTheEditor(t *testing.T) {
	var p Parser
	got := kindsOf(p.Feed([]byte("\x1b[A\x1b[B\x1b[C\x1b[D\x1b[H\x1b[F\x1b[3~\x1bOA\x7f\x03\x04\x15\x0b\x17")))
	want := []KeyKind{KeyUp, KeyDown, KeyRight, KeyLeft, KeyHome, KeyEnd, KeyDelete, KeyUp, KeyBackspace, KeyCtrlC, KeyCtrlD, KeyKillLine, KeyKillEnd, KeyKillWord}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestRuneSplitAcrossReads(t *testing.T) {
	data := []byte("日本")
	for cut := 1; cut < len(data); cut++ {
		var p Parser
		keys := append(p.Feed(data[:cut]), p.Feed(data[cut:])...)
		if len(keys) != 2 || keys[0].Rune != '日' || keys[1].Rune != '本' {
			t.Fatalf("cut at %d: %+v", cut, keys)
		}
	}
}

func TestLinesEndEitherWay(t *testing.T) {
	var p Parser
	if got := kindsOf(p.Feed([]byte("a\r\nb\nc\r"))); !reflect.DeepEqual(got, []KeyKind{KeyRune, KeyEnter, KeyRune, KeyEnter, KeyRune, KeyEnter}) {
		t.Fatalf("%v", got)
	}
}

func TestEditor(t *testing.T) {
	var e editor
	for _, k := range []Key{{Kind: KeyPaste, Text: "hello wide"}, {Kind: KeyHome}, {Kind: KeyRight}, {Kind: KeyDelete}, {Kind: KeyEnd}, {Kind: KeyKillWord}} {
		e.apply(k)
	}
	if e.String() != "hllo " {
		t.Fatalf("%q", e.String())
	}
	e.remember()
	e.insert("draft")
	e.apply(Key{Kind: KeyUp})
	if e.String() != "hllo " {
		t.Fatalf("up: %q", e.String())
	}
	e.apply(Key{Kind: KeyDown})
	if e.String() != "draft" {
		t.Fatalf("down: %q", e.String())
	}
}

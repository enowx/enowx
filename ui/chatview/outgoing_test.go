package chatview

import (
	"errors"
	"testing"
	"time"

	"github.com/enowx/enowx/proto"
)

func userEntry(i int, text string) proto.TranscriptEntry {
	return proto.TranscriptEntry{Index: i, Kind: proto.TranscriptUser, Text: text}
}

// A message shows until the agent records it, once for each time it was sent.
func TestOutgoingSettles(t *testing.T) {
	ch := New()
	o := Options{Send: func(string, []string) {}}
	ch.apply(proto.Transcript{Supported: true, Rev: 1, Entries: []proto.TranscriptEntry{userEntry(0, "same"), {Index: 1, Kind: proto.TranscriptAssistant, Text: "ok"}}})
	ch.outgoingSend(o, "same", nil)
	ch.outgoingSend(o, "same", nil)
	if len(ch.out) != 2 {
		t.Fatalf("an older message of the same words took one: %d left", len(ch.out))
	}
	ch.apply(proto.Transcript{Supported: true, Rev: 2, Entries: []proto.TranscriptEntry{userEntry(2, "same")}})
	if len(ch.out) != 1 {
		t.Fatalf("after the first: %d left", len(ch.out))
	}
	ch.apply(proto.Transcript{Supported: true, Rev: 3, Entries: []proto.TranscriptEntry{{Index: 3, Kind: proto.TranscriptAssistant, Text: "ok"}}})
	if len(ch.out) != 1 {
		t.Fatalf("the same entry took the second: %d left", len(ch.out))
	}
	ch.apply(proto.Transcript{Supported: true, Rev: 4, Entries: []proto.TranscriptEntry{userEntry(4, "same")}})
	if len(ch.out) != 0 {
		t.Fatalf("after the second: %d left", len(ch.out))
	}
}

// A command to the agent shows no bubble: nothing would replace it.
func TestCommandShowsNoBubble(t *testing.T) {
	ch := New()
	sent := ""
	ch.outgoingSend(Options{Send: func(text string, _ []string) { sent = text }}, "/model opus", nil)
	if sent != "/model opus" || len(ch.out) != 0 {
		t.Fatalf("sent %q, %d shown", sent, len(ch.out))
	}
}

// A send that fails keeps the message, to retry or take back with its files.
func TestOutgoingFails(t *testing.T) {
	ch := New()
	var done func(error)
	o := Options{Deliver: func(_ string, _ []string, d func(error)) { done = d }}
	ch.outgoingSend(o, "hello", []string{"/tmp/a.png"})
	done(errors.New("no connection"))
	m := ch.out[0]
	if m.failed == "" || ch.outgoingFlying() {
		t.Fatalf("not marked: %+v", m)
	}
	ch.outgoingGo(o, m)
	if m.failed != "" || !ch.outgoingFlying() {
		t.Fatalf("not on its way again: %+v", m)
	}
	ch.outgoingBack(m)
	if len(ch.out) != 0 || ch.input != "hello" || len(ch.attachments) != 1 || ch.attachments[0].path != "/tmp/a.png" {
		t.Fatalf("not back in the composer: %q, %d attached", ch.input, len(ch.attachments))
	}
}

// Only with the agent free does a message count as not taken.
func TestOutgoingNotTaken(t *testing.T) {
	ch := New()
	o := Options{Send: func(string, []string) {}}
	ch.outgoingSend(o, "hello", nil)
	later := time.Now().Add(2 * outgoingLost)
	busy := o
	busy.Working = true
	ch.outgoingLook(busy, later)
	if ch.out[0].failed != "" {
		t.Fatal("marked while the agent was busy")
	}
	ch.outgoingLook(o, later.Add(2*outgoingLost))
	if ch.out[0].failed == "" {
		t.Fatal("not marked")
	}
}

// The message on its way is keyed as its entry will be, and what runs stays the
// agent's own turn while it works.
func TestOutgoingTurns(t *testing.T) {
	ch := New()
	ch.apply(proto.Transcript{Supported: true, Rev: 1, Entries: []proto.TranscriptEntry{userEntry(0, "first")}})
	ch.outgoingSend(Options{Send: func(string, []string) {}}, "next", nil)
	turns := ch.withOutgoing(ch.turns, "")
	if len(turns) != 2 || turns[1].start != 1 {
		t.Fatalf("turns: %+v", turns)
	}
	if got := runningTurn(turns, true); got != 0 {
		t.Errorf("working: %d", got)
	}
	if got := runningTurn(turns, false); got != 1 {
		t.Errorf("free: %d", got)
	}
}

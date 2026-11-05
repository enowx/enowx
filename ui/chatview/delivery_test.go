package chatview

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/enowx/enowx/proto"
)

func TestDeliveryRetryKeepsKeyAndWaitsForAcknowledgement(t *testing.T) {
	ch := New()
	ch.apply(proto.Transcript{Supported: true, Rev: 1})
	var keys []string
	var done func(error)
	o := Options{Run: 7, Deliver: func(key, text string, files []string, d func(error)) { keys = append(keys, key); done = d }}
	ch.outgoingSend(o, "hello", nil)
	m := ch.out[0]
	ch.outgoingGo(o, m) // clicks while the request is in flight
	if len(keys) != 1 {
		t.Fatal(keys)
	}
	done(errors.New("response lost"))
	ch.outgoingGo(o, m)
	if len(keys) != 2 || keys[0] == "" || keys[0] != keys[1] {
		t.Fatal(keys)
	}
	done(nil)
	if len(ch.out) != 1 {
		t.Fatal("transport success acknowledged a user message")
	}
	ch.outgoingLook(o, time.Now().Add(2*outgoingLost))
	for range 5 {
		ch.outgoingGo(o, m)
	}
	if len(keys) != 2 {
		t.Fatal("unconfirmed terminal input was sent again", keys)
	}
	ch.apply(proto.Transcript{Supported: true, Rev: 1, Entries: []proto.TranscriptEntry{userEntry(0, "hello")}})
	if len(ch.out) != 0 {
		t.Fatal("late acknowledgement was lost")
	}
}

func TestDeliveryQueuesWithoutTouchingBusyOrWaitingTerminal(t *testing.T) {
	for _, busy := range []Options{{Working: true}, {Waiting: true}} {
		ch := New()
		sent := 0
		busy.Send = func(string, []string) { sent++ }
		ch.outgoingSend(busy, "one", nil)
		ch.outgoingSend(busy, "two", nil)
		ch.outgoingLook(busy, time.Now().Add(10*outgoingLost))
		if sent != 0 || ch.out[0].failed != "" {
			t.Fatal("touched a busy/menu terminal")
		}
		free := busy
		free.Working, free.Waiting = false, false
		ch.outgoingLook(free, time.Now())
		if sent != 1 {
			t.Fatal("didn't send the oldest once", sent)
		}
		ch.apply(proto.Transcript{Supported: true, Rev: 1, Entries: []proto.TranscriptEntry{userEntry(0, "one")}})
		ch.outgoingLook(free, time.Now())
		if sent != 2 {
			t.Fatal("didn't release the next message after acknowledgement", sent)
		}
	}
}

func TestDeliveryAcknowledgementRequiresWholeTextAndFiles(t *testing.T) {
	for _, text := range []string{strings.Repeat("a", 250) + " end", ""} {
		ch := New()
		o := Options{Send: func(string, []string) {}}
		ch.outgoingSend(o, text, []string{"/fixture/image.png"})
		ch.apply(proto.Transcript{Supported: true, Rev: 1, Entries: []proto.TranscriptEntry{userEntry(0, strings.Repeat("a", 250)+" other")}})
		if len(ch.out) != 1 {
			t.Fatal("unrelated user message acknowledged the send")
		}
		ch.apply(proto.Transcript{Supported: true, Rev: 2, Entries: []proto.TranscriptEntry{userEntry(1, ch.out[0].shown())}})
		if len(ch.out) != 0 {
			t.Fatal("whole message didn't acknowledge the send")
		}
	}
}

func TestDeliveryDoesNotMoveAcrossRunsOrConversations(t *testing.T) {
	for _, reset := range []bool{false, true} {
		ch := New()
		calls := 0
		o := Options{Run: 1, Deliver: func(_, _ string, _ []string, done func(error)) { calls++; done(errors.New("uncertain")) }}
		ch.apply(proto.Transcript{Supported: true, Rev: 1})
		ch.outgoingSend(o, "hello", nil)
		m := ch.out[0]
		if reset {
			ch.apply(proto.Transcript{Supported: true, Reset: true, Rev: 2, Entries: []proto.TranscriptEntry{userEntry(0, "hello")}})
		} else {
			o.Run = 2
			ch.outgoingLook(o, time.Now())
		}
		ch.outgoingGo(o, m)
		if len(ch.out) != 1 || !m.changed || calls != 1 {
			t.Fatalf("reused a delivery across identity change: %+v, calls %d", m, calls)
		}
	}
}

func TestInterruptedDeliveryRequiresTerminalReview(t *testing.T) {
	ch := New()
	ch.apply(proto.Transcript{Supported: true, Rev: 1})
	calls := 0
	o := Options{Deliver: func(_, _ string, _ []string, done func(error)) { calls++; done(ErrDeliveryInterrupted) }}
	ch.outgoingSend(o, "hello", nil)
	m := ch.out[0]
	for range 5 {
		ch.outgoingGo(o, m)
	}
	if calls != 1 || !m.changed || m.failed == "" {
		t.Fatal("interrupted input was replayed")
	}
}

func TestDeliveryWaitsForTranscriptBaseline(t *testing.T) {
	ch := New()
	calls := 0
	o := Options{Deliver: func(_, _ string, _ []string, done func(error)) { calls++; done(nil) }}
	ch.outgoingSend(o, "same", nil)
	if calls != 0 {
		t.Fatal("sent before reading the acknowledgement baseline")
	}
	ch.apply(proto.Transcript{Supported: true, Rev: 1, Reset: true, Entries: []proto.TranscriptEntry{userEntry(0, "same")}})
	ch.outgoingLook(o, time.Now())
	ch.outgoingSettle()
	if calls != 1 || len(ch.out) != 1 {
		t.Fatal("an existing message acknowledged the new send")
	}
	ch.apply(proto.Transcript{Supported: true, Rev: 2, Entries: []proto.TranscriptEntry{userEntry(1, "same")}})
	if len(ch.out) != 0 {
		t.Fatal("new acknowledgement didn't settle")
	}
}

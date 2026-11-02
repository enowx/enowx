package daemon

import (
	"slices"
	"testing"
	"time"

	"github.com/enowx/enowx/core/session"
	"github.com/enowx/enowx/proto"
)

// Which sessions end, and when: never one working, waiting or possibly either,
// never what isn't a helper.
func TestHelperEnds(t *testing.T) {
	review := proto.SessionInfo{Group: "review:x", State: proto.StateDone}
	page := proto.SessionInfo{CanvasFor: "c", State: proto.StateDone}
	for _, c := range []struct {
		name     string
		info     proto.SessionInfo
		finished bool
		quiet    time.Duration
		want     bool
	}{
		{"a reviewer, verdict in", review, true, time.Minute, true},
		{"a reviewer, verdict just in", review, true, time.Second, false},
		{"a reviewer, no verdict yet", review, false, time.Minute, false},
		{"a reviewer left long", review, false, time.Hour, true},
		{"a page's agent, just answered", page, false, time.Minute, false},
		{"a page's agent left long", page, false, time.Hour, true},
		{"at work", proto.SessionInfo{Group: "review:x", State: proto.StateWorking}, true, time.Hour, false},
		{"waiting on the user", proto.SessionInfo{CanvasFor: "c", State: proto.StateWaiting}, true, time.Hour, false},
		{"hooks gone quiet", proto.SessionInfo{NotesFor: "n", StateStale: true}, true, time.Hour, false},
		{"ended", proto.SessionInfo{Group: "unslop:f", Exited: true}, false, 0, true},
		{"the user's", proto.SessionInfo{State: proto.StateDone}, true, time.Hour, false},
		{"a fan-out", proto.SessionInfo{Group: "fanout:1", State: proto.StateDone}, true, time.Hour, false},
		{"GOA", proto.SessionInfo{GOA: true, CanvasFor: "c", State: proto.StateDone}, true, time.Hour, false},
		{"GOA's worker", proto.SessionInfo{GOAWorker: "g", Group: "review:x", State: proto.StateDone}, true, time.Hour, false},
		{"a teammate's", proto.SessionInfo{Member: &proto.SessionMember{Name: "Rina"}, NotesFor: "n", State: proto.StateDone}, true, time.Hour, false},
		{"a tab", proto.SessionInfo{Parent: "p", CanvasFor: "c", State: proto.StateDone}, true, time.Hour, false},
	} {
		if got := helperEnds(c.info, c.finished, c.quiet); got != c.want {
			t.Errorf("%s: %v", c.name, got)
		}
	}
}

// A reviewer and a Canvas agent go once finished, and the one at work stays; the
// list says so; a page's conversation goes on with its ID afterwards.
func TestHelpersPassEndsThemAndKeepsTheRest(t *testing.T) {
	srv := startServer(t)
	make := func(meta session.Meta) *session.Session {
		t.Helper()
		sess, err := srv.sessions.Create(session.Spec{Command: []string{"cat"}, Dir: t.TempDir(), Cols: 80, Rows: 24, Meta: meta})
		if err != nil {
			t.Fatal(err)
		}
		return sess
	}
	reviewer := make(session.Meta{Agent: "claude", Group: "review:x"})
	canvasAgent := make(session.Meta{Agent: "claude", CanvasFor: "c1", Conversation: "conv-1"})
	atWork := make(session.Meta{Agent: "claude", Group: "review:y"})
	users := make(session.Meta{Agent: "claude"})
	for _, sess := range []*session.Session{reviewer, canvasAgent, users} {
		if err := srv.sessions.SetState(sess, proto.StateDone); err != nil {
			t.Fatal(err)
		}
	}
	if err := srv.sessions.SetState(atWork, proto.StateWorking); err != nil {
		t.Fatal(err)
	}
	srv.helperFinished(reviewer.ID())
	srv.helperFinished(atWork.ID())
	ended := srv.helpersPass(time.Now().Add(2 * helperIdle))
	for _, want := range []string{reviewer.ID(), canvasAgent.ID()} {
		if !slices.Contains(ended, want) {
			t.Errorf("%s still here: %v", want, ended)
		}
	}
	for _, kept := range []*session.Session{atWork, users} {
		if _, err := srv.sessions.Get(kept.ID()); err != nil {
			t.Errorf("%s was ended", kept.ID())
		}
	}
	p := proto.SessionCreateParams{Agent: "claude", CanvasFor: "c1"}
	if got := srv.takeHelperConvo(p); got != "conv-1" {
		t.Fatalf("its conversation: %q", got)
	}
	if got := srv.takeHelperConvo(p); got != "" {
		t.Fatalf("taken twice: %q", got)
	}
}

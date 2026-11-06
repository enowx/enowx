package daemon

import (
	"runtime"
	"strings"
	"testing"

	"github.com/enowx/enowx/core/session"
	"github.com/enowx/enowx/proto"
)

// A GOA session that ended is history: a new GOA starts beside it, and is the one the
// host's GOA is, while the old one can't come back as a second. One running, or asleep
// and able to come back, still keeps a second from starting (issue #49).
func TestGOAStartsBesideAnEndedOne(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	fakeAgents(t, "claude")
	srv := startServer(t)
	c := connect(t, srv, nil)
	old, err := srv.sessions.Create(session.Spec{Command: []string{"sh", "-c", "exit 0"}, Dir: srv.goaDir(), Cols: 80, Rows: 24,
		Exited: true, Meta: session.Meta{Agent: "claude", GOA: true, Relaunch: true}})
	if err != nil {
		t.Fatal(err)
	}
	g, err := c.CreateSession(proto.SessionCreateParams{Agent: "claude", GOA: true, Cols: 80, Rows: 24})
	if err != nil {
		t.Fatalf("a GOA beside one that ended: %v", err)
	}
	if now, ok := srv.goaSession(); !ok || now.ID != g.ID {
		t.Fatalf("the host's GOA is %s, want %s", now.ID, g.ID)
	}
	if _, err := c.ContinueSession(old.ID()); err == nil || !strings.Contains(err.Error(), "another GOD of Agents") {
		t.Fatalf("the old one came back beside it: %v", err)
	}
	if _, err := c.CreateSession(proto.SessionCreateParams{Agent: "claude", GOA: true}); err == nil || !strings.Contains(err.Error(), "already runs") {
		t.Fatalf("a second GOA beside one running: %v", err)
	}
}

// A GOA asleep comes back on its conversation: a new one waits until it is woken or
// closed.
func TestGOAAsleepKeepsASecondFromStarting(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	fakeAgents(t, "claude")
	srv := startServer(t)
	c := connect(t, srv, nil)
	asleep, err := srv.sessions.Create(session.Spec{Command: []string{"sh", "-c", "exit 0"}, Dir: srv.goaDir(), Cols: 80, Rows: 24,
		Exited: true, Meta: session.Meta{Agent: "claude", GOA: true, Relaunch: true, Hibernated: true}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.CreateSession(proto.SessionCreateParams{Agent: "claude", GOA: true})
	if err == nil || !strings.Contains(err.Error(), "asleep") || !strings.Contains(err.Error(), asleep.ID()) {
		t.Fatalf("a GOA beside one asleep: %v", err)
	}
}

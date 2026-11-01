package daemon

import (
	"runtime"
	"strings"
	"testing"

	"github.com/enowx/enowx/core/session"
	"github.com/enowx/enowx/proto"
)

// A worker stopped on a prompt no hook tells of, in a program with no hooks,
// goes to waiting: GOA is told what's asked and the time it gives, session_ask
// says the same, and GOA hears when the timed prompt goes.
func TestGOATold(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	srv, _ := hookServer(t)
	c := connect(t, srv, nil)
	g, err := srv.sessions.Create(session.Spec{Command: []string{"sh", "-c", "cat"}, Dir: t.TempDir(), Cols: 100, Rows: 24, Meta: session.Meta{GOA: true, Agent: "claude"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.sessions.SetState(g, proto.StateDone); err != nil {
		t.Fatal(err)
	}
	script := `read task; printf 'rm -rf x\nDo you want to proceed?\n > 1. Yes\n   2. No\nIt will block this automatically in 1:51\n'; read answer; printf '\033[2J\033[Hbusy again\n'; sleep 60`
	var w proto.SessionInfo
	if err := c.Call(proto.MethodSessionCreate, proto.SessionCreateParams{Command: []string{"sh", "-c", script}, Dir: t.TempDir(), Cols: 100, Rows: 24}, &w); err != nil {
		t.Fatal(err)
	}
	var task proto.GOATask
	if err := c.Call(proto.MethodGOADelegate, proto.GOADelegateParams{Session: w.ID, Prompt: "go", NoRoute: true}, &task); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the task waiting", func() bool {
		for _, x := range goaBoardOf(t, c).Tasks {
			if x.ID == task.ID && x.Status == proto.GOATaskWaiting {
				return true
			}
		}
		return false
	})
	waitScreen(t, c, g.ID(), "Do you want to proceed?")
	waitScreen(t, c, g.ID(), "1:51")
	var ask proto.SessionAsk
	if err := c.Call(proto.MethodSessionAsk, proto.SessionRef{ID: w.ID}, &ask); err != nil {
		t.Fatal(err)
	}
	if !ask.Waiting || !strings.Contains(ask.Prompt, "proceed") || ask.Deadline != "1:51" {
		t.Fatalf("ask: %+v", ask)
	}
	if err := c.Input(w.ID, "1", false, true); err != nil {
		t.Fatal(err)
	}
	waitScreen(t, c, g.ID(), "no longer shows")
}

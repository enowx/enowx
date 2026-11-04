package cli

import (
	"bytes"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/daemon"
	"github.com/enowx/enowx/internal/rpccount"
	"github.com/enowx/enowx/proto"
)

// A wait follows the session's events: it hears the end at once, and the list of
// sessions, which an agent waiting on another read twice a second, isn't asked again.
func TestWaitFollowsEventsNotTheList(t *testing.T) {
	srv, err := daemon.NewServer(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	a, b := net.Pipe()
	go srv.ServeConn(a)
	counted := rpccount.Wrap(b)
	c, err := client.New(counted, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	s, err := c.CreateSession(proto.SessionCreateParams{Command: []string{"sh"}, Dir: t.TempDir(), Cols: 80, Rows: 10})
	if err != nil {
		t.Fatal(err)
	}
	setState := func(state string) {
		t.Helper()
		if err := c.Call(proto.MethodSessionState, proto.SessionStateParams{ID: s.ID, State: state}, nil); err != nil {
			t.Fatal(err)
		}
	}
	setState(proto.StateWorking)
	var out bytes.Buffer
	done := make(chan error, 1)
	began := time.Now()
	go func() { done <- runner{c: c, out: &out}.wait([]string{s.ID, "--timeout", "1m"}) }()
	time.Sleep(4 * time.Second)
	said := time.Now()
	setState(proto.StateDone)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("wait didn't hear the session end")
	}
	took, heard := time.Since(began), time.Since(said)
	if !strings.Contains(out.String(), proto.StateDone) {
		t.Fatalf("said %q", out.String())
	}
	lists := counted.Calls(proto.MethodSessionList)
	t.Logf("wait: %d session.list in %s, %.0f a minute; heard the end after %s", lists, took.Round(time.Millisecond), float64(lists)/took.Minutes(), heard.Round(time.Millisecond))
	if lists > 1 {
		t.Fatalf("session.list called %d times", lists)
	}
	if heard > 2*time.Second {
		t.Fatalf("heard the end only after %s", heard)
	}
}

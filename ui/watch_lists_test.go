//go:build !windows

package ui

import (
	"net"
	"testing"
	"time"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/hosts"
	"github.com/enowx/enowx/daemon"
	"github.com/enowx/enowx/internal/rpccount"
	"github.com/enowx/enowx/proto"
)

// watchedApp is an app whose main thread is posted, and run by the returned function.
func watchedApp(t *testing.T) (*App, func(ok func() bool, what string, within time.Duration)) {
	t.Helper()
	t.Setenv("ENOWX_HOME", t.TempDir())
	a := New(Options{})
	posted := make(chan func(), 1024)
	a.onMain = func(fn func()) { posted <- fn }
	return a, func(ok func() bool, what string, within time.Duration) {
		t.Helper()
		deadline := time.Now().Add(within)
		for !ok() {
			select {
			case fn := <-posted:
				fn()
			case <-time.After(10 * time.Millisecond):
			}
			if time.Now().After(deadline) {
				t.Fatalf("never: %s", what)
			}
		}
	}
}

// A watched host's sessions follow its events: a session made and closed shows without
// the list being asked for again each time the host is asked how it is.
func TestWatchFollowsSessionsByEvents(t *testing.T) {
	srv, err := daemon.NewServer(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	every := watchEvery
	watchEvery = 100 * time.Millisecond
	t.Cleanup(func() { watchEvery = every })
	a, run := watchedApp(t)
	w := &hostWatch{host: hosts.Host{ID: "h", Name: "h"}, stop: make(chan struct{})}
	var counted *rpccount.Conn
	a.watchDial = func(hosts.Host) (*client.Client, error) {
		x, y := net.Pipe()
		go srv.ServeConn(x)
		counted = rpccount.Wrap(y)
		return client.New(counted, func(ev proto.Event) { a.watchEvent(w, ev) })
	}
	go a.watch(w)
	defer close(w.stop)
	run(func() bool { return w.status.state == hostOnline }, "connected", 10*time.Second)

	x, y := net.Pipe()
	go srv.ServeConn(x)
	other, err := client.New(y, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	s, err := other.CreateSession(proto.SessionCreateParams{Command: []string{"sh"}, Dir: t.TempDir(), Cols: 80, Rows: 10})
	if err != nil {
		t.Fatal(err)
	}
	has := func() bool {
		for _, info := range w.status.sessions {
			if info.ID == s.ID && !info.Exited {
				return true
			}
		}
		return false
	}
	run(has, "a new session shown", 5*time.Second)
	if err := other.Call(proto.MethodSessionKill, proto.SessionRef{ID: s.ID}, nil); err != nil {
		t.Fatal(err)
	}
	run(func() bool { return !has() }, "a closed session gone", 5*time.Second)

	began := time.Now()
	run(func() bool { return time.Since(began) >= 3*time.Second }, "polls", 6*time.Second)
	info, lists := counted.Calls(proto.MethodDaemonInfo), counted.Calls(proto.MethodSessionList)
	t.Logf("%d polls, %d session.list: at one poll every 15 s, %.1f session.list a minute", info, lists, 4*float64(lists)/float64(info))
	if info < 10 || lists != 1 {
		t.Fatalf("%d polls, %d session.list", info, lists)
	}
}

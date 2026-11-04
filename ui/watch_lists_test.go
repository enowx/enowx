//go:build !windows

package ui

import (
	"net"
	"sync/atomic"
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

// A host whose connection drops as it comes up isn't connected to again at once: that
// went on without a pause, its whole state read each time.
func TestWatchBacksOffWhenConnectionsDrop(t *testing.T) {
	srv, err := daemon.NewServer(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	a, run := watchedApp(t)
	var dials atomic.Int64
	a.watchDial = func(hosts.Host) (*client.Client, error) {
		dials.Add(1)
		x, y := net.Pipe()
		go srv.ServeConn(x)
		cl, err := client.New(y, nil)
		if err != nil {
			return nil, err
		}
		cl.Close() // gone as soon as it's there
		return cl, nil
	}
	w := &hostWatch{host: hosts.Host{ID: "h", Name: "h"}, stop: make(chan struct{}), wake: make(chan struct{}, 1)}
	go a.watch(w)
	defer close(w.stop)
	const window = 4 * time.Second
	began := time.Now()
	run(func() bool { return time.Since(began) >= window }, "the time to pass", 2*window)
	got := dials.Load()
	t.Logf("a connection dropping as it comes up: %d connections in %s, %.0f a minute", got, window, float64(got)/window.Minutes())
	if got > 1 {
		t.Fatalf("%d connections", got)
	}
	// The user asking doesn't wait.
	w.retryNow()
	run(func() bool { return dials.Load() == got+1 }, "a try when asked", 3*time.Second)
}

func TestRetryWaitGrowsUntilAConnectionHolds(t *testing.T) {
	r := retryWait{first: 15 * time.Second, most: time.Minute}
	for i, want := range []time.Duration{15 * time.Second, 30 * time.Second, time.Minute, time.Minute} {
		if got := r.failed(); got != want {
			t.Fatalf("try %d: waits %s, want %s", i, got, want)
		}
	}
	r.held()
	if got := r.failed(); got != 15*time.Second {
		t.Fatalf("after holding: waits %s", got)
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
	w := &hostWatch{host: hosts.Host{ID: "h", Name: "h"}, stop: make(chan struct{}), wake: make(chan struct{}, 1)}
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

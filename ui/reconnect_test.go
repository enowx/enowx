//go:build !windows

package ui

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/enowx/enowx/daemon"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/transport"
)

// When the daemon's connection drops, the app connects again by itself
// once it answers, its sessions back in view.
func TestReconnectsByItself(t *testing.T) {
	t.Setenv("ENOWX_HOME", t.TempDir())
	// A host with Xpra asks the screen's scale, which waits for a window's loop this has none of.
	scale := screenScale
	screenScale = func() int { return 1 }
	t.Cleanup(func() { screenScale = scale })
	path, err := transport.SocketPath()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(filepath.Dir(path)) })
	state := t.TempDir()
	var l net.Listener
	serve := func() *daemon.Server {
		t.Helper()
		var err error
		if l, err = transport.Listen(path); err != nil {
			t.Fatal(err)
		}
		srv, err := daemon.NewServer(state)
		if err != nil {
			t.Fatal(err)
		}
		go srv.Serve(l)
		return srv
	}
	srv := serve()
	cl, err := localClient()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cl.CreateSession(proto.SessionCreateParams{Command: []string{"sh"}, Dir: t.TempDir(), Name: "kept", Cols: 80, Rows: 10}); err != nil {
		t.Fatal(err)
	}
	cl.Close()

	a := New(Options{})
	posted := make(chan func(), 256)
	a.onMain = func(fn func()) { posted <- fn }
	until := func(ok func() bool, what string, within time.Duration) {
		t.Helper()
		deadline := time.Now().Add(within)
		for !ok() {
			select {
			case fn := <-posted:
				fn()
			case <-time.After(20 * time.Millisecond):
			}
			if time.Now().After(deadline) {
				t.Fatalf("never: %s (status %q, error %q)", what, a.status, a.connErr)
			}
		}
	}
	a.connect(LocalHost())
	until(func() bool { return a.client != nil && len(a.sessions) == 1 }, "connected", 10*time.Second)

	// The daemon can't be reached, and the connection drops, as when the
	// network goes.
	l.Close()
	srv.Close()
	a.client.Close()
	until(func() bool { return a.client == nil && a.retrying > 0 }, "noticed", 10*time.Second)
	time.Sleep(1500 * time.Millisecond) // past the first try, which fails
	if a.retrying < 2 {
		until(func() bool { return a.retrying >= 2 }, "a failed try, then another", 5*time.Second)
	}
	srv = serve() // and comes back
	defer srv.Close()
	until(func() bool { return a.client != nil && a.retrying == 0 && len(a.sessions) == 1 }, "connected again", 15*time.Second)
	if a.sessions[0].Name != "kept" {
		t.Fatalf("sessions %+v", a.sessions)
	}
	a.disconnect()
}

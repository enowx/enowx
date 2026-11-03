//go:build !windows

package ui

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/hosts"
	"github.com/enowx/enowx/daemon"
	"github.com/enowx/enowx/proto"
)

// goaHost is a daemon of its own, connected as the app connects, its
// events to onEvent.
func goaHost(t *testing.T, onEvent func(proto.Event)) *client.Client {
	t.Helper()
	srv, err := daemon.NewServer(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	x, y := net.Pipe()
	go srv.ServeConn(x)
	cl, err := client.New(y, onEvent)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cl.Close() })
	return cl
}

// GOA on host A hands a task to a new session on host B, which only the
// app reaches: B's worker shows in the Sessions tool with its task, and
// when its turn ends on B, the app's relay tells A, whose board says done
// and whose GOA is told in its own conversation.
func TestGOAFollowsAWorkerOnAnotherHost(t *testing.T) {
	bin := t.TempDir()
	os.WriteFile(filepath.Join(bin, "claude"), []byte("#!/bin/sh\nwhile read -r line; do echo \"heard: $line\"; done\n"), 0o755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	app := New(Options{})
	posted := make(chan func(), 256)
	app.onMain = func(fn func()) { posted <- fn }
	w := &hostWatch{host: hosts.Host{ID: "b", Name: "Server B"}, stop: make(chan struct{})}
	// The app misses B's events while away, and the missed worker's
	// (missed) ever after.
	var away atomic.Bool
	var missed atomic.Value
	missed.Store("")
	clB := goaHost(t, func(ev proto.Event) {
		var info proto.SessionInfo
		if ev.Name == proto.EventSessionUpdated && json.Unmarshal(ev.Data, &info) == nil && info.ID == missed.Load() {
			return
		}
		if !away.Load() {
			app.watchEvent(w, ev)
		}
	})
	clA := goaHost(t, func(ev proto.Event) { app.handleEvent(app.gen, ev) })
	infoA, err := clA.Info()
	if err != nil {
		t.Fatal(err)
	}
	infoB, err := clB.Info()
	if err != nil {
		t.Fatal(err)
	}
	sessB, err := clB.Sessions()
	if err != nil {
		t.Fatal(err)
	}
	w.cl, w.info, w.status = clB, infoB, hostStatus{state: hostOnline, sessions: sessB}
	app.client, app.daemonInfo, app.gen = clA, infoA, 1
	app.host = hosts.Host{ID: "a", Name: "Mac A"}
	app.watches = map[string]*hostWatch{"b": w}
	go func() { _ = clA.ServeApp() }() // A's host.call comes to the app

	g, err := clA.CreateSession(proto.SessionCreateParams{Agent: "claude", GOA: true, Cols: 100, Rows: 30})
	if err != nil {
		t.Fatal(err)
	}
	tt := ui.NewTester(func(c *ui.Context) { app.view(c) }, 1400, 850)
	frames := func(what string, until func() bool) {
		t.Helper()
		deadline := time.Now().Add(20 * time.Second)
		for !until() {
			select {
			case fn := <-posted:
				fn()
			case <-time.After(20 * time.Millisecond):
			}
			tt.Advance(50 * time.Millisecond)
			tt.Frame()
			if time.Now().After(deadline) {
				t.Fatalf("never: %s: %q", what, tt.Texts())
			}
		}
	}
	frames("GOA's board loaded", func() bool { return app.goa.host == "a" && app.goa.board.Session == g.ID })
	app.enterGOA()

	var task proto.GOATask
	delegated := make(chan error, 1)
	go func() {
		delegated <- clA.Call(proto.MethodGOADelegate, proto.GOADelegateParams{From: g.ID, Host: "Server B", Agent: "claude",
			Project: t.TempDir(), Title: "Fix the totals", Prompt: "Fix the rounding of the totals", NoRoute: true}, &task)
	}()
	var derr error
	frames("the task delegated, through the app", func() bool {
		select {
		case derr = <-delegated:
			return true
		default:
			return false
		}
	})
	if derr != nil {
		t.Fatal(derr)
	}
	frames("the task on the app's board", func() bool { return app.goaTaskOf(task.Session) != nil })
	frames("B's worker in Sessions, with its task", func() bool {
		return tt.HasText("Server B") && tt.HasText("Fix the totals · working")
	})

	// Its turn ends on B (its agent's turn.done hook); the app tells A. A turn's end in
	// a task's first seconds is what it was doing before, so this one comes after them.
	time.Sleep(3100 * time.Millisecond)
	if err := clB.Call(proto.MethodSessionState, proto.SessionStateParams{ID: task.Session, State: proto.StateDone}, nil); err != nil {
		t.Fatal(err)
	}
	frames("the task done on A's board", func() bool {
		var b proto.GOABoard
		if clA.Call(proto.MethodGOABoard, nil, &b) != nil {
			return false
		}
		return len(b.Tasks) == 1 && b.Tasks[0].Status == proto.GOATaskTurnEnded
	})
	// This stand-in for its agent shows no empty composer, so the line waits for its next turn.
	frames("GOA told, in its conversation", func() bool {
		var b proto.GOABoard
		return clA.Call(proto.MethodGOABoard, nil, &b) == nil && strings.Contains(strings.Join(b.Waiting, "\n"), "[enowX GOA] Task "+task.ID)
	})
	frames("the Sessions tool says its turn ended", func() bool { return tt.HasText("Fix the totals · turn-ended") })
	shot(t, tt, "goa-remote-worker.png")

	// The app misses B's updates (closed, or B's link down): another
	// worker there finishes meanwhile. When the app is back, its watch's
	// poll of B tells A.
	away.Store(true)
	var task2 proto.GOATask
	go func() {
		delegated <- clA.Call(proto.MethodGOADelegate, proto.GOADelegateParams{From: g.ID, Host: "Server B", Agent: "claude",
			Project: t.TempDir(), Title: "Add the tests", Prompt: "Add tests for the totals", NoRoute: true}, &task2)
	}()
	frames("the second task delegated", func() bool {
		select {
		case derr = <-delegated:
			return true
		default:
			return false
		}
	})
	if derr != nil {
		t.Fatal(derr)
	}
	missed.Store(task2.Session)
	away.Store(false)
	frames("the second task on the app's board", func() bool { return app.goaTaskOf(task2.Session) != nil })
	if err := clB.Call(proto.MethodSessionState, proto.SessionStateParams{ID: task2.Session, State: proto.StateDone}, nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { close(w.stop) })
	go app.poll(w, clB)
	frames("the second task done on A's board", func() bool {
		var b proto.GOABoard
		if clA.Call(proto.MethodGOABoard, nil, &b) != nil {
			return false
		}
		return len(b.Tasks) == 2 && b.Tasks[1].Status == proto.GOATaskTurnEnded
	})
	frames("GOA told of the second", func() bool {
		var b proto.GOABoard
		return clA.Call(proto.MethodGOABoard, nil, &b) == nil && strings.Contains(strings.Join(b.Waiting, "\n"), "[enowX GOA] Task "+task2.ID)
	})
}

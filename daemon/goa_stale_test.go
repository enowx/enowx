package daemon

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/enowx/enowx/proto"
)

// An idle worker with no hooks whose earlier task the board still has at work (as GOA
// set it, or its turn's end went unheard) takes the next task now, in the same session:
// that older task is marked turn-ended and GOA is told (issue #45).
func TestGOADeliversToAnIdleWorkerWithAStaleTask(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	bin := t.TempDir()
	script := "#!/bin/sh\n[ \"$1\" = --version ] && { echo 1.18.35; exit 0; }\necho '> '\nwhile read -r line; do echo \"heard: $line\"; done\n"
	if err := os.WriteFile(filepath.Join(bin, "opencode"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte("#!/bin/sh\nwhile read -r line; do :; done\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	srv := startServer(t)
	c := connect(t, srv, nil)
	if err := c.Call(proto.MethodGOAStallSave, proto.GOAStall{Blank: 60, Idle: 1, Stuck: 60}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := c.CreateSession(proto.SessionCreateParams{Agent: "claude", GOA: true, Cols: 80, Rows: 24}); err != nil {
		t.Fatal(err) // GOA, to be told
	}
	var info proto.SessionInfo
	if err := c.Call(proto.MethodSessionCreate, proto.SessionCreateParams{Agent: "opencode", Project: t.TempDir(), BranchMode: proto.BranchNone, Cols: 80, Rows: 24}, &info); err != nil {
		t.Fatal(err)
	}
	sess, err := srv.sessions.Get(info.ID)
	if err != nil {
		t.Fatal(err)
	}
	old, err := srv.goa.board.Add(proto.GOATask{Title: "earlier", Session: info.ID, Prompt: "earlier", Status: proto.GOATaskWorking})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "its screen to settle past the idle limit", func() bool {
		i := sess.Info()
		return !i.Active && i.FinishedAt > 0 && time.Since(time.UnixMilli(i.FinishedAt)) > 1500*time.Millisecond
	})
	before := len(srv.sessions.List())

	var task proto.GOATask
	if err := c.Call(proto.MethodGOADelegate, proto.GOADelegateParams{Session: info.ID, Prompt: "the next task", NoRoute: true}, &task); err != nil {
		t.Fatal(err)
	}
	if task.Status == proto.GOATaskQueued || task.Session != info.ID {
		t.Fatalf("queued, or elsewhere: %+v", task)
	}
	if got := taskStatus(t, srv, old.ID).Status; got != proto.GOATaskTurnEnded {
		t.Fatalf("the earlier task: %s", got)
	}
	waitFor(t, "the task reaching it", func() bool { return heardTimes(sess, "the next task") == 1 })
	if n := len(srv.sessions.List()); n != before {
		t.Fatalf("%d sessions, was %d", n, before)
	}
	if !strings.Contains(strings.Join(srv.goaWaitingLines(), "\n"), old.ID) {
		t.Fatalf("GOA wasn't told of %s: %q", old.ID, srv.goaWaitingLines())
	}
}

// A session really at work still queues the task, its reason naming what holds it.
func TestGOAQueuesForABusySessionNamingWhatHoldsIt(t *testing.T) {
	queueServerAgent(t)
	srv := startServer(t)
	c := connect(t, srv, nil)
	sess := busySession(t, srv, c, t.TempDir())
	old, err := srv.goa.board.Add(proto.GOATask{Title: "at work", Session: sess.ID(), Prompt: "at work", Status: proto.GOATaskWorking})
	if err != nil {
		t.Fatal(err)
	}
	task := queuedTask(t, c, proto.GOADelegateParams{Session: sess.ID(), Prompt: "after it"})
	if !strings.Contains(task.Why, old.ID) || !strings.Contains(task.Why, "goa task "+old.ID+" --status done") {
		t.Fatalf("why: %q", task.Why)
	}
	if got := taskStatus(t, srv, old.ID).Status; got != proto.GOATaskWorking {
		t.Fatalf("the task at work: %s", got)
	}
}

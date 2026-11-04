package daemon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/session"
	"github.com/enowx/enowx/proto"
)

// queueServerAgent is a stand-in agent that says what it hears, each line once.
func queueServerAgent(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	bin := t.TempDir()
	script := "#!/bin/sh\necho '❯ '\nwhile read -r line; do echo \"heard: $line\"; done\n"
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func heardTimes(sess *session.Session, what string) int {
	return strings.Count(sess.Terminal().Text(), "heard: "+what)
}

func queuedTask(t *testing.T, c *client.Client, p proto.GOADelegateParams) proto.GOATask {
	t.Helper()
	p.NoRoute = true
	var task proto.GOATask
	if err := c.Call(proto.MethodGOADelegate, p, &task); err != nil {
		t.Fatal(err)
	}
	if task.Status != proto.GOATaskQueued {
		t.Fatalf("not queued: %+v", task)
	}
	return task
}

func taskStatus(t *testing.T, srv *Server, id string) proto.GOATask {
	t.Helper()
	tasks := srv.goa.board.List()
	i := slices.IndexFunc(tasks, func(x proto.GOATask) bool { return x.ID == id })
	if i < 0 {
		t.Fatalf("no task %s", id)
	}
	return tasks[i]
}

// busySession starts a claude session in project, at work.
func busySession(t *testing.T, srv *Server, c *client.Client, project string) *session.Session {
	t.Helper()
	var info proto.SessionInfo
	if err := c.Call(proto.MethodSessionCreate, proto.SessionCreateParams{Agent: "claude", Project: project, BranchMode: proto.BranchNone, Cols: 80, Rows: 24}, &info); err != nil {
		t.Fatal(err)
	}
	sess, err := srv.sessions.Get(info.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.sessions.SetState(sess, proto.StateWorking); err != nil {
		t.Fatal(err)
	}
	return sess
}

// GOA on host A, its worker on host B: the app telling A of B's turn ending is the
// only word A gets, and has to give it its queued task, once, the next one waiting
// for its turn. This is what never reached a session on another host.
func TestGOAQueuedTaskReachesAnotherHost(t *testing.T) {
	queueServerAgent(t)
	a, b := startServer(t), startServer(t)
	standInApp(t, a, map[string]*client.Client{b.daemonID: connect(t, b, nil), a.daemonID: connect(t, a, nil)}, func(proto.HostCallParams) bool { return true })
	ca, cb := connect(t, a, nil), connect(t, b, nil)
	project := t.TempDir()
	sess := busySession(t, b, cb, project)
	p := proto.GOADelegateParams{Host: b.daemonID, Session: sess.ID(), Title: "One"}
	p.Prompt = "first thing"
	first := queuedTask(t, ca, p)
	p.Prompt, p.Title = "second thing", "Two"
	second := queuedTask(t, ca, p)

	done := func() {
		t.Helper()
		if err := b.sessions.SetState(sess, proto.StateDone); err != nil {
			t.Fatal(err)
		}
		if err := ca.Call(proto.MethodGOAWorker, proto.GOAWorkerParams{Host: b.daemonID, Session: sess.Info()}, nil); err != nil {
			t.Fatal(err)
		}
	}
	done()
	waitFor(t, "the first went in", func() bool { return heardTimes(sess, "first thing") == 1 })
	if got := taskStatus(t, a, first.ID).Status; got != proto.GOATaskWorking {
		t.Fatalf("the first: %s", got)
	}
	// Told again of the turn before, as polls and other changes do: nothing more goes in.
	done()
	time.Sleep(500 * time.Millisecond)
	if heardTimes(sess, "first thing") != 1 || heardTimes(sess, "second thing") != 0 {
		t.Fatalf("delivered on top of a turn: %q", sess.Terminal().Text())
	}
	time.Sleep(goaTurnLeast) // what is told from here on is the first's own turn ending
	done()
	waitFor(t, "the second went in", func() bool { return heardTimes(sess, "second thing") == 1 })
	if heardTimes(sess, "first thing") != 1 || taskStatus(t, a, second.ID).Status != proto.GOATaskWorking {
		t.Fatalf("%q, %+v", sess.Terminal().Text(), taskStatus(t, a, second.ID))
	}
}

// Many changes of a session still done at once give it one task, not all of them.
func TestGOAQueuedTasksGoOneAtATime(t *testing.T) {
	queueServerAgent(t)
	srv := startServer(t)
	c := connect(t, srv, nil)
	sess := busySession(t, srv, c, t.TempDir())
	for _, prompt := range []string{"one", "two", "three"} {
		queuedTask(t, c, proto.GOADelegateParams{Session: sess.ID(), Prompt: prompt})
	}
	if err := srv.sessions.SetState(sess, proto.StateDone); err != nil {
		t.Fatal(err)
	}
	info := sess.Info()
	for range 8 {
		go srv.goaDeliver(info)
	}
	waitFor(t, "the first went in", func() bool { return heardTimes(sess, "one") == 1 })
	time.Sleep(500 * time.Millisecond)
	if heardTimes(sess, "two") != 0 || heardTimes(sess, "three") != 0 {
		t.Fatalf("more than one went in: %q", sess.Terminal().Text())
	}
}

// A session done before its task was queued hears no turn's end again; the watch
// gives it the task once the session has been quiet, once.
func TestGOAQueuedTaskReachesASessionAlreadyDone(t *testing.T) {
	queueServerAgent(t)
	srv := startServer(t)
	c := connect(t, srv, nil)
	sess := busySession(t, srv, c, t.TempDir())
	if err := srv.sessions.SetState(sess, proto.StateDone); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond) // its turn's end has been heard, with nothing queued
	task, err := srv.goa.board.Add(proto.GOATask{Session: sess.ID(), Prompt: "after the turn", Status: proto.GOATaskQueued})
	if err != nil {
		t.Fatal(err)
	}
	queuedPassUntil(t, srv, func() bool { return heardTimes(sess, "after the turn") == 1 })
	if got := taskStatus(t, srv, task.ID).Status; got != proto.GOATaskWorking {
		t.Fatalf("the task: %s", got)
	}
}

// queuedPassUntil runs the stall watch's passes, its queued tasks' screens long
// quiet, until ok.
func queuedPassUntil(t *testing.T, srv *Server, ok func() bool) {
	t.Helper()
	w := &stallWatch{follows: map[string]*stallFollow{}, sampled: map[string]bool{}}
	deadline := time.Now().Add(20 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatal("never")
		}
		srv.stallPass(w)
		for _, f := range w.follows {
			f.since = time.Now().Add(-2 * queuedQuiet)
		}
		time.Sleep(minSample)
	}
}

// A queued task survives a daemon's restart, whole, and goes in once to the session
// brought back, which has no state to say its turn ended.
func TestGOAQueuedTaskSurvivesARestart(t *testing.T) {
	queueServerAgent(t)
	t.Setenv("SHELL", "/bin/sh")
	dir := t.TempDir()
	srv, err := NewServer(dir)
	if err != nil {
		t.Fatal(err)
	}
	c := connect(t, srv, nil)
	project := t.TempDir()
	sess := busySession(t, srv, c, project)
	id := sess.ID()
	prompt := "all of this " + strings.Repeat("word ", 40) + "end"
	task := queuedTask(t, c, proto.GOADelegateParams{Session: id, Prompt: prompt})
	srv.Close()

	again, err := NewServer(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(again.Close)
	kept := taskStatus(t, again, task.ID)
	if kept.Status != proto.GOATaskQueued || !strings.HasPrefix(kept.Prompt, prompt) {
		t.Fatalf("after the restart: %+v", kept)
	}
	back, err := again.sessions.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	queuedPassUntil(t, again, func() bool {
		return strings.Contains(strings.Join(strings.Fields(back.Terminal().Text()), ""), "heard:allofthis")
	})
	if got := taskStatus(t, again, task.ID).Status; got != proto.GOATaskWorking {
		t.Fatalf("the task: %s", got)
	}
	// Later passes and turn's-end words don't give it again.
	w := &stallWatch{follows: map[string]*stallFollow{}, sampled: map[string]bool{}}
	again.stallPass(w)
	again.stallPass(w)
	again.goaDeliver(back.Info())
	time.Sleep(200 * time.Millisecond)
	if n := strings.Count(strings.Join(strings.Fields(back.Terminal().Text()), ""), "heard:allofthis"); n != 1 {
		t.Fatalf("went in %d times: %q", n, back.Terminal().Text())
	}
}

// What can't go in says so, on the board and to GOA, rather than stay queued.
func TestGOAQueuedTaskThatCantGoIn(t *testing.T) {
	queueServerAgent(t)
	srv := startServer(t)
	c := connect(t, srv, nil)
	g, err := c.CreateSession(proto.SessionCreateParams{Agent: "claude", GOA: true, Cols: 100, Rows: 30})
	if err != nil {
		t.Fatal(err)
	}
	// Told: waiting for it, or typed in already, as its stand-in's prompt is free.
	told := func(what string) bool {
		goa, err := srv.sessions.Get(g.ID)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(strings.Join(strings.Fields(goa.Terminal().Text()), ""), strings.Join(strings.Fields(what), "")) {
			return true
		}
		return slices.ContainsFunc(srv.goaWaitingLines(), func(l string) bool { return strings.Contains(l, what) })
	}

	// Its session is closed: told as it ends, or found gone by the watch.
	gone := busySession(t, srv, c, t.TempDir())
	first := queuedTask(t, c, proto.GOADelegateParams{From: g.ID, Session: gone.ID(), Prompt: "lost"})
	if err := srv.deleteSession(gone.ID()); err != nil {
		t.Fatal(err)
	}
	queuedPassUntil(t, srv, func() bool { return taskStatus(t, srv, first.ID).Status == proto.GOATaskFailed })
	if !strings.Contains(taskStatus(t, srv, first.ID).Result, "its session") {
		t.Fatalf("%+v", taskStatus(t, srv, first.ID))
	}
	waitFor(t, "GOA told of the first", func() bool { return told("Task " + first.ID) })

	// Typing it in fails: its host's app no longer answers, say.
	busy := busySession(t, srv, c, t.TempDir())
	second := queuedTask(t, c, proto.GOADelegateParams{From: g.ID, Session: busy.ID(), Prompt: "undelivered"})
	_, _ = srv.goa.board.Update(second.ID, func(t *proto.GOATask) { t.Host = "no-such-host" })
	if err := srv.sessions.SetState(busy, proto.StateDone); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "failed", func() bool { return taskStatus(t, srv, second.ID).Status == proto.GOATaskFailed })
	waitFor(t, "GOA told of the second", func() bool { return told("Task " + second.ID) })
	if heardTimes(busy, "undelivered") != 0 {
		t.Fatal("it went in")
	}
	raw, _ := json.Marshal(taskStatus(t, srv, second.ID))
	if !strings.Contains(string(raw), "couldn't be delivered") {
		t.Fatalf("%s", raw)
	}
}

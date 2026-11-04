package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
)

// wakeAgent is a claude that keeps each start's arguments, and each line it takes, in
// files; it prints on starting, so its screen shows it's back.
func wakeAgent(t *testing.T) (starts, lines string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	dir := t.TempDir()
	starts, lines = filepath.Join(dir, "starts"), filepath.Join(dir, "lines")
	script := fmt.Sprintf("#!/bin/sh\necho \"$@\" >> %q\necho started $$\nwhile read -r line; do printf '%%s\\n' \"$line\" >> %q; done\n", starts, lines)
	if err := os.WriteFile(filepath.Join(dir, "claude"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return starts, lines
}

func linesOf(file string) []string {
	data, _ := os.ReadFile(file)
	var out []string
	for _, l := range strings.Split(string(data), "\n") {
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

// asleepAgent is a session of wakeAgent's, started and hibernated.
func asleepAgent(t *testing.T, srv *Server, c *client.Client, starts string) proto.SessionInfo {
	t.Helper()
	info, err := c.CreateSession(proto.SessionCreateParams{Agent: "claude", Dir: t.TempDir(), Cols: 80, Rows: 10})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "its agent started", func() bool { return len(linesOf(starts)) == 1 })
	if res, err := c.HibernateSessions(proto.SessionHibernateParams{IDs: []string{info.ID}, Force: true}); err != nil || len(res.Hibernated) != 1 {
		t.Fatalf("hibernate: %+v, %v", res, err)
	}
	return info
}

func sendTo(c *client.Client, id, text string) (proto.SessionInputResult, error) {
	var res proto.SessionInputResult
	err := c.Call(proto.MethodSessionInput, proto.SessionInputParams{ID: id, Text: text, Enter: true}, &res)
	return res, err
}

// What session send answered "Done" to went to a terminal nothing read. Now it brings
// the session back and says so, its message arrives once, and it runs as it last ran.
func TestSendToAHibernatedSessionWakesItAndArrivesOnce(t *testing.T) {
	starts, lines := wakeAgent(t)
	srv := startServer(t)
	c := connect(t, srv, nil)
	info := asleepAgent(t, srv, c, starts)
	sess, _ := srv.sessions.Get(info.ID)
	srv.sessions.SetModel(sess, "claude-opus-5-5", "high")
	srv.sessions.SetMode(sess, "plan")

	res, err := sendTo(c, info.ID, "the first")
	if err != nil || !res.Sent || !res.Woken {
		t.Fatalf("sent to one asleep: %+v, %v", res, err)
	}
	waitFor(t, "the message to arrive", func() bool { return len(linesOf(lines)) > 0 })
	if got := linesOf(lines); len(got) != 1 || got[0] != "the first" {
		t.Fatalf("what arrived: %q", got)
	}
	all := linesOf(starts)
	if len(all) != 2 {
		t.Fatalf("started %d times: %q", len(all), all)
	}
	for _, want := range []string{"--model claude-opus-5-5", "--effort high", "--permission-mode plan"} {
		if !strings.Contains(all[1], want) {
			t.Errorf("woken without %s: %s", want, all[1])
		}
	}
	list, _ := c.Sessions()
	if list[0].Hibernated || list[0].Exited {
		t.Fatalf("after: %+v", list[0])
	}
	// Awake now: it goes straight in.
	if res, err = sendTo(c, info.ID, "the second"); err != nil || !res.Sent || res.Woken {
		t.Fatalf("sent to one awake: %+v, %v", res, err)
	}
	waitFor(t, "the second message", func() bool { return len(linesOf(lines)) == 2 })
}

// Several at once bring it back once, and each goes in once.
func TestSendsAtOnceToOneAsleepWakeItOnce(t *testing.T) {
	starts, lines := wakeAgent(t)
	srv := startServer(t)
	c := connect(t, srv, nil)
	info := asleepAgent(t, srv, c, starts)
	var wg sync.WaitGroup
	errs := make([]error, 4)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = sendTo(connect(t, srv, nil), info.ID, fmt.Sprintf("message %d", i))
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	waitFor(t, "every message", func() bool { return len(linesOf(lines)) >= 4 })
	seen := map[string]int{}
	for _, l := range linesOf(lines) {
		seen[l]++
	}
	if len(seen) != 4 || len(linesOf(starts)) != 2 {
		t.Fatalf("arrived %v, started %d times", seen, len(linesOf(starts)))
	}
	for l, n := range seen {
		if n != 1 {
			t.Errorf("%q arrived %d times", l, n)
		}
	}
}

// One that ended on its own comes back too.
func TestSendToAnEndedAgentWakesIt(t *testing.T) {
	starts, lines := wakeAgent(t)
	srv := startServer(t)
	c := connect(t, srv, nil)
	info, err := c.CreateSession(proto.SessionCreateParams{Agent: "claude", Dir: t.TempDir(), Cols: 80, Rows: 10})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "its agent started", func() bool { return len(linesOf(starts)) == 1 })
	sess, _ := srv.sessions.Get(info.ID)
	srv.sessions.Input(sess, []byte("\x04")) // end of input: it ends
	waitFor(t, "it ended", func() bool {
		srv.sessions.Input(sess, []byte("exit\r")) // its shell afterwards, if it has one
		return sess.Info().Exited
	})
	res, err := sendTo(c, info.ID, "back again")
	if err != nil || !res.Woken {
		t.Fatalf("sent to one ended: %+v, %v", res, err)
	}
	waitFor(t, "the message", func() bool {
		return strings.Contains(strings.Join(linesOf(lines), "\n"), "back again")
	})
}

// A plain shell has no conversation to come back on: it says so, at once, and nothing
// starts.
func TestSendToAnEndedShellSaysItCantComeBack(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	srv := startServer(t)
	c := connect(t, srv, nil)
	info, err := c.CreateSession(proto.SessionCreateParams{Dir: t.TempDir(), Command: []string{"sh", "-c", "exit 0"}, Cols: 80, Rows: 10})
	if err != nil {
		t.Fatal(err)
	}
	sess, _ := srv.sessions.Get(info.ID)
	waitFor(t, "it ended", func() bool { return sess.Info().Exited })
	res, err := sendTo(c, info.ID, "anyone?")
	if err == nil || !strings.Contains(err.Error(), "can't come back") || !strings.Contains(err.Error(), "nothing was sent") {
		t.Fatalf("to an ended shell: %+v, %v", res, err)
	}
	if !sess.Info().Exited {
		t.Fatal("it started again")
	}
}

// An answer to one asleep brings it back; what it asked went with it, so the answer
// goes in as a message, once. Nonsense doesn't wake it.
func TestAnswerToAHibernatedSessionWakesIt(t *testing.T) {
	starts, lines := wakeAgent(t)
	srv := startServer(t)
	c := connect(t, srv, nil)
	info := asleepAgent(t, srv, c, starts)
	if _, err := c.AnswerSession(info.ID, "maybe"); err == nil {
		t.Fatal("answered maybe")
	}
	if list, _ := c.Sessions(); !list[0].Hibernated {
		t.Fatal("a bad answer woke it")
	}
	res, err := c.AnswerSession(info.ID, "allow")
	if err != nil || !res.Woken || !res.Told {
		t.Fatalf("answered: %+v, %v", res, err)
	}
	waitFor(t, "the answer", func() bool { return len(linesOf(lines)) > 0 })
	if got := linesOf(lines); len(got) != 1 || !strings.Contains(got[0], "given to it: allow") {
		t.Fatalf("what arrived: %q", got)
	}
}

// A session woken by hand and sent to at once: what's sent waits for its agent.
func TestSendWhileWakingWaitsForTheAgent(t *testing.T) {
	starts, lines := wakeAgent(t)
	srv := startServer(t)
	c := connect(t, srv, nil)
	info := asleepAgent(t, srv, c, starts)
	if _, err := c.WakeSession(info.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := sendTo(c, info.ID, "right after"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the message", func() bool { return len(linesOf(lines)) == 1 })
	if got := linesOf(starts); len(got) != 2 {
		t.Fatalf("started %d times", len(got))
	}
}

// endAgent ends session id's agent on its own, and the shell after it if it has one.
func endAgent(t *testing.T, srv *Server, id string) {
	t.Helper()
	sess, err := srv.sessions.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	srv.sessions.Input(sess, []byte("\x04"))
	waitFor(t, "it ended", func() bool {
		srv.sessions.Input(sess, []byte("exit\r"))
		return sess.Info().Exited
	})
}

// A task queued for a session whose agent then ends brings it back, and goes in once,
// rather than failing with it.
func TestGOAQueuedTaskBringsBackAnEndedSession(t *testing.T) {
	queueServerAgent(t)
	srv := startServer(t)
	c := connect(t, srv, nil)
	sess := busySession(t, srv, c, t.TempDir())
	task := queuedTask(t, c, proto.GOADelegateParams{Session: sess.ID(), Prompt: "after it ended"})
	endAgent(t, srv, sess.ID())
	waitFor(t, "it took the task", func() bool {
		now, err := srv.sessions.Get(sess.ID())
		return err == nil && heardTimes(now, "after it ended") > 0
	})
	now, _ := srv.sessions.Get(sess.ID())
	if n := heardTimes(now, "after it ended"); n != 1 {
		t.Fatalf("heard %d times", n)
	}
	if st := taskStatus(t, srv, task.ID).Status; st == proto.GOATaskFailed || st == proto.GOATaskQueued {
		t.Fatalf("task: %s", st)
	}
}

// A task for what can't come back says so: to GOA delegating, and, queued before it
// ended, on the board and in a line to GOA.
func TestGOATaskForAnEndedShellSaysItCantComeBack(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	queueServerAgent(t)
	srv := startServer(t)
	c := connect(t, srv, nil)
	g, err := c.CreateSession(proto.SessionCreateParams{Agent: "claude", GOA: true, Cols: 100, Rows: 30})
	if err != nil {
		t.Fatal(err)
	}
	shell, err := c.CreateSession(proto.SessionCreateParams{Dir: t.TempDir(), Command: []string{"sh", "-c", "read x"}, Cols: 80, Rows: 10})
	if err != nil {
		t.Fatal(err)
	}
	sess, _ := srv.sessions.Get(shell.ID)
	if err := srv.sessions.SetState(sess, proto.StateWorking); err != nil {
		t.Fatal(err)
	}
	task := queuedTask(t, c, proto.GOADelegateParams{From: g.ID, Session: shell.ID, Prompt: "for a shell"})
	srv.sessions.Input(sess, []byte("go\r"))
	waitFor(t, "it ended", func() bool { return sess.Info().Exited })
	waitFor(t, "the task failed", func() bool { return taskStatus(t, srv, task.ID).Status == proto.GOATaskFailed })
	if r := taskStatus(t, srv, task.ID).Result; !strings.Contains(r, "can't come back") {
		t.Fatalf("result: %s", r)
	}
	gs, _ := srv.sessions.Get(g.ID)
	waitFor(t, "GOA told", func() bool {
		said := "Task " + task.ID
		if strings.Contains(strings.Join(strings.Fields(gs.Terminal().Text()), ""), strings.Join(strings.Fields(said), "")) {
			return true
		}
		return slices.ContainsFunc(srv.goaWaitingLines(), func(l string) bool { return strings.Contains(l, said) })
	})
	var again proto.GOATask
	err = c.Call(proto.MethodGOADelegate, proto.GOADelegateParams{From: g.ID, Session: shell.ID, Prompt: "again", NoRoute: true}, &again)
	if err == nil || !strings.Contains(err.Error(), "can't come back") {
		t.Fatalf("delegated: %+v, %v", again, err)
	}
	if !sess.Info().Exited {
		t.Fatal("the shell started again")
	}
}

package daemon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
)

// The stand-in agent: what it does depends on words in its first prompt. It
// doesn't echo what's typed, so a screen stays blank until it means otherwise,
// and writes down what it's sent, a line of GOA's wrapping on no screen. Started on
// none of those prompts, it's GOD of Agents, showing a composer with nothing in it: lines
// are typed only into one. Its loops of work end on their own, should anything outlive a test.
const stallAgent = `#!/bin/sh
stty -echo 2>/dev/null
work() { i=0; while [ $i -lt 300000000 ]; do i=$((i+1)); done; }
case "$*" in
*STANDIN-SILENT*) ;;
*STANDIN-READY*) echo "ready for it" ;;
*STANDIN-FROZEN*) echo "compiling the thing" ;;
*STANDIN-BUSYBLANK*) work ;;
*STANDIN-BUILD*) echo "building"; work ;;
*) composer=yes; echo ">" ;;
esac
while read -r line; do
	echo "heard: $line"
	echo "$line" >>"$STALL_HEARD"
	[ -z "$composer" ] || echo ">"
done
`

// stallStandIn puts the stand-in agent first on PATH, as claude, and says the
// file it writes what it hears in.
func stallStandIn(t *testing.T) string {
	t.Helper()
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte(stallAgent), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	heard := filepath.Join(t.TempDir(), "heard")
	t.Setenv("STALL_HEARD", heard)
	return heard
}

// stallLinesOf are the lines of GOA's heard so far about task id.
func stallLinesOf(heard, id string) []string {
	data, _ := os.ReadFile(heard)
	var out []string
	for _, line := range strings.Split(string(data), "\n") {
		if strings.Contains(line, "[enowX GOA] Task "+id) {
			out = append(out, line)
		}
	}
	return out
}

// stallHookFile has a hook on session.stalled write each event to the file it
// says.
func stallHookFile(t *testing.T, srv *Server) string {
	t.Helper()
	file := filepath.Join(t.TempDir(), "fired")
	if _, err := srv.saveHostHooks([]proto.Hook{{Name: "Stalled", On: true, Event: proto.HookStalled, Action: proto.HookActionRun,
		Command: "cat >>" + file + "; echo >>" + file}}); err != nil {
		t.Fatal(err)
	}
	return file
}

// stallFired are the session.stalled events of session id the hook wrote.
func stallFired(t *testing.T, file, id string) []proto.HookEvent {
	t.Helper()
	data, _ := os.ReadFile(file)
	var out []proto.HookEvent
	for _, line := range strings.Split(string(data), "\n") {
		var ev proto.HookEvent
		if json.Unmarshal([]byte(line), &ev) == nil && ev.Session == id {
			out = append(out, ev)
		}
	}
	return out
}

func stallDelegate(t *testing.T, c *client.Client, p proto.GOADelegateParams) proto.GOATask {
	t.Helper()
	p.NoRoute, p.Agent = true, "claude"
	if p.Project == "" {
		p.Project = t.TempDir()
	}
	var task proto.GOATask
	if err := c.Call(proto.MethodGOADelegate, p, &task); err != nil {
		t.Fatal(err)
	}
	return task
}

func stallStatus(t *testing.T, c *client.Client, id string) string {
	t.Helper()
	for _, task := range goaBoardOf(t, c).Tasks {
		if task.ID == id {
			return task.Status
		}
	}
	t.Fatalf("no task %s", id)
	return ""
}

// Each way of not moving, end to end with the daemon's own loop: the task shows
// as stalled, GOA hears one line naming the case, how long and the screen, the
// hook fires once, a further line comes only after three times the limit, and
// the task goes back to working when its screen moves. A process at work, screen
// blank or still, is left alone.
func TestGOAStalledTasksEndToEnd(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	heard := stallStandIn(t)
	srv := startServer(t)
	c := connect(t, srv, nil)
	fired := stallHookFile(t, srv)
	lim := proto.GOAStall{Blank: 2, Idle: 3, Stuck: 4}
	if err := c.Call(proto.MethodGOAStallSave, lim, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := c.CreateSession(proto.SessionCreateParams{Agent: "claude", GOA: true, Cols: 200, Rows: 40}); err != nil {
		t.Fatal(err)
	}

	type stalled struct {
		name, prompt, shows string
		limit               int
		task                proto.GOATask
		first, second       time.Time
	}
	cases := []*stalled{
		{name: "never started", prompt: "STANDIN-SILENT", shows: "Its screen is blank.", limit: lim.Blank},
		{name: "idle", prompt: "STANDIN-READY", shows: "Its screen ends: ready for it.", limit: lim.Idle},
		{name: "stuck", prompt: "STANDIN-FROZEN", shows: "Its screen ends: compiling the thing.", limit: lim.Stuck},
	}
	for _, sc := range cases {
		sc.task = stallDelegate(t, c, proto.GOADelegateParams{Title: sc.name, Prompt: sc.prompt})
	}
	frozen := cases[2]
	if err := c.Call(proto.MethodSessionState, proto.SessionStateParams{ID: frozen.task.Session, State: proto.StateWorking}, nil); err != nil {
		t.Fatal(err)
	}
	working := []proto.GOATask{
		stallDelegate(t, c, proto.GOADelegateParams{Title: "busy, blank", Prompt: "STANDIN-BUSYBLANK"}),
		stallDelegate(t, c, proto.GOADelegateParams{Title: "a long build", Prompt: "STANDIN-BUILD"}),
	}
	if err := c.Call(proto.MethodSessionState, proto.SessionStateParams{ID: working[1].Session, State: proto.StateWorking}, nil); err != nil {
		t.Fatal(err)
	}

	waitFor2 := time.Now().Add(90 * time.Second)
	for {
		done := true
		now := time.Now()
		for _, sc := range cases {
			lines := stallLinesOf(heard, sc.task.ID)
			if len(lines) >= 1 && sc.first.IsZero() {
				sc.first = now
				if st := stallStatus(t, c, sc.task.ID); st != proto.GOATaskStalled {
					t.Errorf("%s: told, and the board says %s", sc.name, st)
				}
				for _, want := range []string{"isn't moving: " + sc.name, ", for ", sc.shows} {
					if !strings.Contains(lines[0], want) {
						t.Errorf("%s: no %q in its line: %s", sc.name, want, lines[0])
					}
				}
			}
			if len(lines) >= 2 && sc.second.IsZero() {
				sc.second = now
				if gap := sc.second.Sub(sc.first); gap < time.Duration(3*sc.limit)*time.Second-time.Second {
					t.Errorf("%s: a further line after only %s (its limit %ds)", sc.name, gap, sc.limit)
				}
			}
			done = done && !sc.second.IsZero()
			// What the session says of itself, as a worker elsewhere's goes
			// through the app, doesn't take it for moving.
			if !sc.first.IsZero() && sc == frozen {
				if err := c.Call(proto.MethodGOAWorker, proto.GOAWorkerParams{Session: srvInfo(t, c, sc.task.Session)}, nil); err != nil {
					t.Fatal(err)
				}
				time.Sleep(50 * time.Millisecond)
				if st := stallStatus(t, c, sc.task.ID); st != proto.GOATaskStalled {
					t.Fatalf("%s: the session said working again, and the board says %s", sc.name, st)
				}
			}
		}
		for _, task := range working {
			if st := stallStatus(t, c, task.ID); st != proto.GOATaskWorking {
				t.Fatalf("%q was taken for stalled: %s: %q", task.Title, st, stallLinesOf(heard, task.ID))
			}
		}
		if done {
			break
		}
		if now.After(waitFor2) {
			t.Fatalf("no further line in time: %q", mustRead(heard))
		}
		time.Sleep(100 * time.Millisecond)
	}

	// However many lines: one hook, naming the case; then the screen moves.
	for _, sc := range cases {
		waitFor(t, sc.name+"'s hook", func() bool { return len(stallFired(t, fired, sc.task.Session)) > 0 })
		evs := stallFired(t, fired, sc.task.Session)
		if len(evs) != 1 || !strings.HasPrefix(evs[0].Text, sc.name+":") || evs[0].Event != proto.HookStalled {
			t.Errorf("%s: the hook: %+v", sc.name, evs)
		}
		if err := c.Input(sc.task.Session, "go on", false, true); err != nil {
			t.Fatal(err)
		}
		waitFor(t, sc.name+" working again", func() bool { return stallStatus(t, c, sc.task.ID) == proto.GOATaskWorking })
	}
}

// srvInfo is session id as the host lists it.
func srvInfo(t *testing.T, c *client.Client, id string) proto.SessionInfo {
	t.Helper()
	list, err := c.Sessions()
	if err != nil {
		t.Fatal(err)
	}
	for _, info := range list {
		if info.ID == id {
			return info
		}
	}
	t.Fatalf("no session %s", id)
	return proto.SessionInfo{}
}

func mustRead(path string) string {
	data, _ := os.ReadFile(path)
	return string(data)
}

// A task on another host is watched the same, through the app: stalled on GOA's
// board, GOA told where and the hook fired on GOA's host, back to working when the
// screen there moves, a process at work there left alone. Those looks stay out of
// the log of what sessions did. With the host out of reach, nothing is concluded.
func TestGOAStalledTaskOnAnotherHost(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	heard := stallStandIn(t)
	a, b := startServer(t), startServer(t)
	ca, cb := connect(t, a, nil), connect(t, b, nil)
	standInApp(t, a, map[string]*client.Client{b.daemonID: cb}, func(proto.HostCallParams) bool { return true })
	fired := stallHookFile(t, a)
	if err := ca.Call(proto.MethodGOAStallSave, proto.GOAStall{Blank: 2, Idle: 2, Stuck: 2}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := ca.CreateSession(proto.SessionCreateParams{Agent: "claude", GOA: true, Cols: 200, Rows: 40}); err != nil {
		t.Fatal(err)
	}
	blank := stallDelegate(t, ca, proto.GOADelegateParams{Host: b.daemonID, Title: "far", Prompt: "STANDIN-SILENT"})
	busy := stallDelegate(t, ca, proto.GOADelegateParams{Host: b.daemonID, Title: "far build", Prompt: "STANDIN-BUSYBLANK"})
	if _, err := a.sessions.Get(blank.Session); err == nil {
		t.Fatal("the worker is on A")
	}
	unreached, err := a.goa.board.Add(proto.GOATask{Prompt: "out of reach", Host: "nowhere", Session: "abc"})
	if err != nil {
		t.Fatal(err)
	}

	waitFor(t, "told", func() bool { return len(stallLinesOf(heard, blank.ID)) == 1 })
	line := stallLinesOf(heard, blank.ID)[0]
	for _, want := range []string{"on " + b.daemonID, "never started", "Its screen is blank.", "--host " + b.daemonID} {
		if !strings.Contains(line, want) {
			t.Errorf("no %q in: %s", want, line)
		}
	}
	if st := stallStatus(t, ca, blank.ID); st != proto.GOATaskStalled {
		t.Fatalf("told, and the board says %s", st)
	}
	waitFor(t, "the hook", func() bool { return len(stallFired(t, fired, blank.Session)) == 1 })
	if st := stallStatus(t, ca, busy.ID); st != proto.GOATaskWorking {
		t.Fatalf("a process at work there was taken for stalled: %s", st)
	}
	// A host out of reach says nothing of its session: what GOA is told says that, once the
	// limit has passed, and nothing calls it not moving.
	waitFor(t, "told of the host out of reach", func() bool { return len(stallLinesOf(heard, unreached.ID)) == 1 })
	if line := stallLinesOf(heard, unreached.ID)[0]; !strings.Contains(line, "can't be reached") || strings.Contains(line, "never started") {
		t.Errorf("a host out of reach read as a stalled task: %s", line)
	}
	log, err := ca.ControlLog()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range log {
		if e.Method == proto.MethodSessionScreen || e.Method == proto.MethodDaemonStats {
			t.Fatalf("the daemon's looks in the log: %+v", e)
		}
	}
	if err := cb.Input(blank.Session, "go on", false, true); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "working again", func() bool { return stallStatus(t, ca, blank.ID) == proto.GOATaskWorking })
}

// A hibernated session and one gone both count as idle, each told once with
// what became of it, and the hook has what it knows of a session gone.
func TestGOAStalledSessionHibernatedOrGone(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	heard := stallStandIn(t)
	srv := startServer(t)
	c := connect(t, srv, nil)
	fired := stallHookFile(t, srv)
	if err := c.Call(proto.MethodGOAStallSave, proto.GOAStall{Blank: 60, Idle: 2, Stuck: 60}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := c.CreateSession(proto.SessionCreateParams{Agent: "claude", GOA: true, Cols: 200, Rows: 40}); err != nil {
		t.Fatal(err)
	}
	asleep := stallDelegate(t, c, proto.GOADelegateParams{Title: "asleep", Prompt: "STANDIN-READY"})
	gone := stallDelegate(t, c, proto.GOADelegateParams{Title: "gone", Prompt: "STANDIN-READY"})
	waitScreen(t, c, asleep.Session, "ready for it")
	// A program that only just printed counts as working, and isn't hibernated.
	waitFor(t, "hibernated", func() bool {
		res, err := c.HibernateSessions(proto.SessionHibernateParams{IDs: []string{asleep.Session}})
		return err == nil && len(res.Hibernated) == 1
	})
	if err := srv.deleteSession(gone.Session); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		task proto.GOATask
		says string
	}{{asleep, "idle: its session is hibernated"}, {gone, "idle: its session is gone"}} {
		waitFor(t, c.says, func() bool { return len(stallLinesOf(heard, c.task.ID)) == 1 })
		if line := stallLinesOf(heard, c.task.ID)[0]; !strings.Contains(line, c.says) {
			t.Errorf("its line: %s", line)
		}
		waitFor(t, c.says+": the hook", func() bool { return len(stallFired(t, fired, c.task.Session)) == 1 })
	}
}

// Settled while stalled, a task stays settled and nothing more is said of it.
func TestGOASettlingAStalledTask(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	heard := stallStandIn(t)
	srv := startServer(t)
	c := connect(t, srv, nil)
	if err := c.Call(proto.MethodGOAStallSave, proto.GOAStall{Blank: 1, Idle: 1, Stuck: 1}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := c.CreateSession(proto.SessionCreateParams{Agent: "claude", GOA: true, Cols: 200, Rows: 40}); err != nil {
		t.Fatal(err)
	}
	task := stallDelegate(t, c, proto.GOADelegateParams{Prompt: "STANDIN-SILENT"})
	waitFor(t, "told", func() bool { return len(stallLinesOf(heard, task.ID)) == 1 })
	if err := c.Call(proto.MethodGOATask, proto.GOATaskParams{ID: task.ID, Status: proto.GOATaskFailed, Result: "gave up"}, nil); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Second) // past a further line's time
	if st := stallStatus(t, c, task.ID); st != proto.GOATaskFailed {
		t.Fatalf("settled, then %s", st)
	}
	if lines := stallLinesOf(heard, task.ID); len(lines) != 1 {
		t.Fatalf("still told of it: %q", lines)
	}
}

// A settled task raises no stall line: not one looked at from before it was
// settled, nor one waiting for GOA to be idle when it was.
func TestGOASettledTaskRaisesNoStallLine(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	heard := stallStandIn(t)
	srv := startServer(t)
	c := connect(t, srv, nil)
	if _, err := c.CreateSession(proto.SessionCreateParams{Agent: "claude", GOA: true, Cols: 200, Rows: 40}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := srv.goa.board.Add(proto.GOATask{Title: "settled", Session: "gone-session", Status: proto.GOATaskStalled})
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Created = time.Now().Add(-time.Hour) // long enough ago to count as not moving
	if err := c.Call(proto.MethodGOATask, proto.GOATaskParams{ID: snapshot.ID, Status: proto.GOATaskDone}, nil); err != nil {
		t.Fatal(err)
	}
	w := &stallWatch{follows: map[string]*stallFollow{}, sampled: map[string]bool{}}
	srv.stallCheck(w, snapshot, stallHost{infos: map[string]proto.SessionInfo{}}, proto.GOAStall{Blank: 1, Idle: 1, Stuck: 1})
	srv.goa.mu.Lock()
	srv.goa.tell = append(srv.goa.tell, goaLine{Text: "moot", Task: snapshot.ID}, goaLine{Text: "still true"})
	srv.goa.mu.Unlock()
	if got := srv.goaLinesToSay(); len(got) != 1 || got[0].Text != "still true" {
		t.Fatalf("to say: %+v", got)
	}
	time.Sleep(2 * time.Second)
	if lines := stallLinesOf(heard, snapshot.ID); len(lines) != 0 {
		t.Fatalf("told of a settled task: %q", lines)
	}
}

// Over a restart, a stalled task still shows so; GOA, whose queue went with the
// daemon, hears again, and the hook isn't fired a second time for the same stall.
// A session that ended meanwhile settles its task.
func TestGOAStallAcrossARestart(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	heard := stallStandIn(t)
	dir := t.TempDir()
	srv, err := NewServer(dir)
	if err != nil {
		t.Fatal(err)
	}
	c := connect(t, srv, nil)
	fired := stallHookFile(t, srv)
	if err := c.Call(proto.MethodGOAStallSave, proto.GOAStall{Blank: 2, Idle: 2, Stuck: 2}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := c.CreateSession(proto.SessionCreateParams{Agent: "claude", GOA: true, Cols: 200, Rows: 40}); err != nil {
		t.Fatal(err)
	}
	task := stallDelegate(t, c, proto.GOADelegateParams{Prompt: "STANDIN-SILENT"})
	waitFor(t, "told", func() bool { return len(stallLinesOf(heard, task.ID)) == 1 })
	waitFor(t, "the hook", func() bool { return len(stallFired(t, fired, task.Session)) == 1 })
	srv.Close()

	again, err := NewServer(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	c2 := connect(t, again, nil)
	deadline := time.Now().Add(30 * time.Second)
	for len(stallLinesOf(heard, task.ID)) < 2 {
		if st := stallStatus(t, c2, task.ID); st != proto.GOATaskStalled {
			t.Fatalf("after a restart the board says %s", st)
		}
		if time.Now().After(deadline) {
			t.Fatalf("GOA wasn't told again: %q", mustRead(heard))
		}
		time.Sleep(100 * time.Millisecond)
	}
	if evs := stallFired(t, fired, task.Session); len(evs) != 1 {
		t.Fatalf("the hook fired again: %+v", evs)
	}
}

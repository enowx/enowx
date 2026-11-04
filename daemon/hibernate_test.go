package daemon

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"syscall"
	"testing"
	"time"

	"github.com/enowx/enowx/proto"
)

// Hibernating by hand: an agent working is planned, not stopped, unless
// forced or once done; one done stops at once, its program gone, and
// wakes on its conversation; a session kept from hibernating on its own
// stays so.
func TestHibernateByHandAndWake(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	fakeAgent(t)
	srv := startServer(t)
	c := connect(t, srv, nil)
	info, err := c.CreateSession(proto.SessionCreateParams{Agent: "claude", Dir: t.TempDir(), Cols: 80, Rows: 10})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Call(proto.MethodSessionState, proto.SessionStateParams{ID: info.ID, State: proto.StateWorking}, nil); err != nil {
		t.Fatal(err)
	}
	res, err := c.HibernateSessions(proto.SessionHibernateParams{IDs: []string{info.ID}})
	if err != nil || len(res.Hibernated) != 0 || len(res.Plans) != 1 || !res.Plans[0].Working {
		t.Fatalf("a working agent: %+v, %v", res, err)
	}
	// Its program may still be starting; once it runs, there is memory to free.
	if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		waitFor(t, "memory to free planned", func() bool {
			res, err = c.HibernateSessions(proto.SessionHibernateParams{IDs: []string{info.ID}})
			return err == nil && len(res.Plans) == 1 && res.Plans[0].RSS != 0
		})
	}
	// Once done.
	res, err = c.HibernateSessions(proto.SessionHibernateParams{IDs: []string{info.ID}, WhenDone: true})
	if err != nil || !slices.Equal(res.Waiting, []string{info.ID}) {
		t.Fatalf("when done: %+v, %v", res, err)
	}
	srv.hibernate()
	if list, _ := c.Sessions(); list[0].Hibernated {
		t.Fatal("hibernated while working")
	}
	sess, _ := srv.sessions.Get(info.ID)
	var pids map[int]bool
	waitFor(t, "a program found to stop", func() bool {
		pids = srv.sessionProgram(info.ID)
		return len(pids) > 0
	})
	if err := c.Call(proto.MethodSessionState, proto.SessionStateParams{ID: info.ID, State: proto.StateDone}, nil); err != nil {
		t.Fatal(err)
	}
	var list []proto.SessionInfo
	waitFor(t, "hibernated after its turn", func() bool {
		srv.hibernate()
		list, _ = c.Sessions()
		return list[0].Hibernated
	})
	if list[0].HibernatedHow == nil || !list[0].HibernatedHow.Manual {
		t.Fatalf("after its turn: %+v", list[0].HibernatedHow)
	}
	_ = sess
	waitFor(t, "its program gone", func() bool {
		for pid := range pids {
			if p, err := os.FindProcess(pid); err == nil && p.Signal(syscall.Signal(0)) == nil {
				return false
			}
		}
		return true
	})
	woke, err := c.WakeSession(info.ID)
	if err != nil || woke.Hibernated || woke.Exited {
		t.Fatalf("woken: %+v, %v", woke, err)
	}

	// Kept from hibernating on its own: the daemon leaves it.
	if err := c.SetAutoHibernate(proto.SessionAutoHibernateParams{ID: info.ID, Never: true}); err != nil {
		t.Fatal(err)
	}
	hibernateUnit = time.Millisecond
	defer func() { hibernateUnit = time.Minute }()
	if _, err := c.SetSettings(proto.DaemonSettings{HibernateAfter: 1}); err != nil {
		t.Fatal(err)
	}
	if err := c.Call(proto.MethodSessionState, proto.SessionStateParams{ID: info.ID, State: proto.StateDone}, nil); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	srv.hibernate()
	if list, _ := c.Sessions(); list[0].Hibernated || !list[0].NoAutoHibernate {
		t.Fatalf("kept from hibernating on its own: %+v", list[0])
	}
	// Its preference is in the synced settings; so is the default.
	if err := c.SetAutoHibernate(proto.SessionAutoHibernateParams{Default: true, Never: true}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(srv.dataDir, "settings", "hibernate.json"))
	prefs := srv.hibernatePrefs()
	if !prefs.NeverByDefault || !prefs.Sessions[info.ID] {
		t.Fatalf("settings/hibernate.json: %s", data)
	}
	next, err := c.CreateSession(proto.SessionCreateParams{Agent: "claude", Dir: t.TempDir(), Cols: 80, Rows: 10})
	if err != nil || !next.NoAutoHibernate {
		t.Fatalf("a new session with the default: %+v, %v", next, err)
	}

	// Hibernate all idle: the done one goes, a shell stays (it couldn't
	// come back).
	shell, _ := c.CreateSession(proto.SessionCreateParams{Agent: "shell", Dir: t.TempDir(), Cols: 80, Rows: 10})
	c.Call(proto.MethodSessionState, proto.SessionStateParams{ID: next.ID, State: proto.StateDone}, nil)
	res, err = c.HibernateSessions(proto.SessionHibernateParams{Idle: true})
	if err != nil || !slices.Contains(res.Hibernated, next.ID) || slices.Contains(res.Hibernated, shell.ID) {
		t.Fatalf("all idle: %+v, %v", res, err)
	}
}

// A session with a task under way stays, asked to hibernate or left idle, until forced;
// once its task's turn has ended it goes.
func TestHibernateLeavesATaskUnderWay(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	fakeAgent(t)
	srv := startServer(t)
	c := connect(t, srv, nil)
	info, err := c.CreateSession(proto.SessionCreateParams{Agent: "claude", Dir: t.TempDir(), Cols: 80, Rows: 10})
	if err != nil {
		t.Fatal(err)
	}
	task, err := srv.goa.board.Add(proto.GOATask{Title: "x", Session: info.ID, Status: proto.GOATaskWorking})
	if err != nil {
		t.Fatal(err)
	}
	res, err := c.HibernateSessions(proto.SessionHibernateParams{IDs: []string{info.ID}})
	if err != nil || len(res.Hibernated) != 0 || len(res.Plans) != 1 || res.Plans[0].Task != task.ID {
		t.Fatalf("asked: %+v, %v", res, err)
	}
	if res, err = c.HibernateSessions(proto.SessionHibernateParams{Idle: true}); err != nil || len(res.Hibernated) != 0 {
		t.Fatalf("all idle: %+v, %v", res, err)
	}
	// Not on its own either, however long it has been done.
	hibernateUnit = time.Millisecond
	defer func() { hibernateUnit = time.Minute }()
	if _, err := c.SetSettings(proto.DaemonSettings{HibernateAfter: 1}); err != nil {
		t.Fatal(err)
	}
	sess, _ := srv.sessions.Get(info.ID)
	if err := srv.sessions.SetState(sess, proto.StateDone); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	srv.hibernate()
	if list, _ := c.Sessions(); list[0].Hibernated {
		t.Fatal("hibernated on its own, its task under way")
	}
	if _, err := srv.goa.board.Update(task.ID, func(x *proto.GOATask) { x.Status = proto.GOATaskTurnEnded }); err != nil {
		t.Fatal(err)
	}
	if plan := srv.hibernatePlan(sess, nil); plan.Task != "" {
		t.Fatalf("its turn ended: %+v", plan)
	}
	if _, err := srv.goa.board.Update(task.ID, func(x *proto.GOATask) { x.Status = proto.GOATaskWorking }); err != nil {
		t.Fatal(err)
	}
	res, err = c.HibernateSessions(proto.SessionHibernateParams{IDs: []string{info.ID}, Force: true})
	if err != nil || !slices.Equal(res.Hibernated, []string{info.ID}) {
		t.Fatalf("forced: %+v, %v", res, err)
	}
}

// Hibernating names the agents in a session's background as work that may be lost, and doesn't
// happen on its own while they run.
func TestHibernateNamesBackgroundAgents(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	fakeAgent(t)
	srv := startServer(t)
	c := connect(t, srv, nil)
	info, err := c.CreateSession(proto.SessionCreateParams{Agent: "claude", Dir: t.TempDir(), Cols: 80, Rows: 10})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := srv.sessions.Get(info.ID)
	if err != nil {
		t.Fatal(err)
	}
	srv.sessions.SetBackground(sess, []proto.BackgroundAgent{{PID: 4242, Start: "s", Name: "codex"}})
	res, err := c.HibernateSessions(proto.SessionHibernateParams{IDs: []string{info.ID}})
	if err != nil || len(res.Hibernated) != 0 || len(res.Plans) != 1 || !slices.Equal(res.Plans[0].Background, []string{"codex"}) || !res.Plans[0].Busy() {
		t.Fatalf("asked: %+v, %v", res, err)
	}
}

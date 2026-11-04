package ui

import (
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/hosts"
	"github.com/enowx/enowx/daemon"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
)

// New session's first field lists the project's sessions and its agents'
// past conversations: picking a running one opens it and sends it the
// message; picking a past conversation resumes it.
func TestNewSessionGoesOnWithASessionOfTheProject(t *testing.T) {
	srv, err := daemon.NewServer(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	pa, pb := net.Pipe()
	go srv.ServeConn(pa)
	cl, err := client.New(pb, func(proto.Event) {})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cl.Close() })
	dir := t.TempDir()
	if _, err := cl.AddProject(dir); err != nil {
		t.Fatal(err)
	}
	sh, err := cl.CreateSession(proto.SessionCreateParams{Project: dir, Agent: "shell", Name: "build shell", Cols: 80, Rows: 20})
	if err != nil {
		t.Fatal(err)
	}
	app := New(Options{})
	var later mainQueue
	app.onMain = later.post
	app.client, app.host = cl, hosts.Host{ID: hosts.LocalID, Name: "This Mac"}
	app.agents = []proto.AgentInfo{{ID: "shell", Name: "Shell", Installed: true}, {ID: "claude", Name: "Claude Code", Command: "claude", Installed: true}}
	app.sessions = []proto.SessionInfo{sh}
	app.openNewSession(proto.ProjectInfo{Name: "proj", Path: dir})
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(theme.Window())
		ui.Column(c).FillWidth().FillHeight().Background(theme.Background).Children(func() { app.newSessionDialog(c) })
	}, 1000, 900)
	settle := func(what string, done func() bool) {
		t.Helper()
		for deadline := time.Now().Add(8 * time.Second); ; {
			tt.Frame()
			later.run()
			if done() {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s: never; shows %q", what, tt.Texts())
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatalf("%v; shows %q", err, tt.Texts())
		}
		tt.Frame()
	}
	// Loaded first: the dialog grows as the project and agents come, which
	// moves its controls under a click aimed before.
	settle("the dialog", func() bool { return app.newSession.loaded && tt.HasText("Session") })
	tt.Frame()
	quiet := time.Now().Add(500 * time.Millisecond) // the host's conversations read
	settle("a moment", func() bool { return time.Now().After(quiet) })
	app.newSession.convs = []proto.Conversation{{Agent: "claude", ID: "conv-9", Title: "Fix the login bug", Dir: dir, Updated: time.Now().Add(-time.Hour)}}
	tt.Frame()
	click("Session: New session")
	settle("the list", func() bool { return tt.HasText("Fix the login bug") && tt.HasText("build shell") })
	shot(t, tt, "new-session-continue.png")

	click("build shell")
	settle("open it", func() bool { return tt.HasText("Open session") })
	m, _ := tt.Find("Message (optional)")
	tt.ClickAt(m.X+10, m.Y+35)
	tt.Type("echo picked-from-new-session")
	click("Open session")
	settle("opened", func() bool { return !app.newSession.open && app.selected == sh.ID })
	var screen string
	for deadline := time.Now().Add(8 * time.Second); time.Now().Before(deadline) && !strings.Contains(screen, "picked-from-new-session"); time.Sleep(50 * time.Millisecond) {
		screen, _ = cl.Screen(sh.ID)
	}
	if !strings.Contains(screen, "picked-from-new-session") {
		t.Fatalf("the message isn't in the session: %q", screen)
	}

	app.openNewSession(proto.ProjectInfo{Name: "proj", Path: dir})
	settle("the dialog again", func() bool { return app.newSession.loaded && tt.HasText("Session") })
	quiet = time.Now().Add(500 * time.Millisecond)
	settle("a moment", func() bool { return time.Now().After(quiet) })
	app.newSession.convs = []proto.Conversation{{Agent: "claude", ID: "conv-9", Title: "Fix the login bug", Dir: dir, Updated: time.Now()}}
	tt.Frame()
	click("Session: New session")
	settle("the list", func() bool { return tt.HasText("Fix the login bug") })
	click("Fix the login bug")
	settle("resume it", func() bool { return tt.HasText("Resume") })
	click("Resume")
	settle("resumed", func() bool { return !app.newSession.open })
	list, _ := cl.Sessions()
	found := false
	for _, s := range list {
		found = found || s.Agent == "claude" && s.ID != sh.ID
	}
	if !found {
		t.Fatalf("no claude session resumed: %+v", list)
	}
}

// A hibernated session picked in New session wakes up, takes the message
// and shows.
func TestNewSessionWakesAHibernatedSession(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	bin := t.TempDir()
	// A stand-in for Claude Code that echoes what it is typed.
	script := "#!/bin/sh\necho ready\nwhile IFS= read -r line; do echo \"got: $line\"; done\n" // shows its input, as an agent does
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	srv, err := daemon.NewServer(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	pa, pb := net.Pipe()
	go srv.ServeConn(pa)
	cl, err := client.New(pb, func(proto.Event) {})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cl.Close() })
	dir := t.TempDir()
	if _, err := cl.AddProject(dir); err != nil {
		t.Fatal(err)
	}
	info, err := cl.CreateSession(proto.SessionCreateParams{Project: dir, Agent: "claude", Name: "sleepy agent", Cols: 80, Rows: 20})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cl.HibernateSessions(proto.SessionHibernateParams{IDs: []string{info.ID}, Force: true}); err != nil {
		t.Fatal(err)
	}
	var list []proto.SessionInfo
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(50 * time.Millisecond) {
		list, _ = cl.Sessions()
		if len(list) == 1 && list[0].Hibernated {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("never hibernated: %+v", list)
		}
	}
	app := New(Options{})
	var later mainQueue
	app.onMain = later.post
	app.client, app.host = cl, hosts.Host{ID: hosts.LocalID, Name: "This Mac"}
	app.agents = []proto.AgentInfo{{ID: "claude", Name: "Claude Code", Command: "claude", Installed: true}}
	app.sessions = list
	app.openNewSession(proto.ProjectInfo{Name: "proj", Path: dir})
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(theme.Window())
		ui.Column(c).FillWidth().FillHeight().Background(theme.Background).Children(func() { app.newSessionDialog(c) })
	}, 1000, 900)
	settle := func(what string, done func() bool) {
		t.Helper()
		for deadline := time.Now().Add(10 * time.Second); ; {
			tt.Frame()
			later.run()
			if done() {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s: never; shows %q", what, tt.Texts())
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatalf("%v; shows %q", err, tt.Texts())
		}
		tt.Frame()
	}
	settle("the dialog", func() bool { return app.newSession.loaded && tt.HasText("Session") })
	quiet := time.Now().Add(500 * time.Millisecond) // its motion in, done
	settle("a moment", func() bool { return time.Now().After(quiet) })
	click("Session: New session")
	settle("the hibernated session listed", func() bool { return tt.HasText("sleepy agent") })
	click("sleepy agent")
	settle("wake it", func() bool { return tt.HasText("Wake up") })
	m, _ := tt.Find("Message (optional)")
	tt.ClickAt(m.X+10, m.Y+35)
	tt.Type("hello again")
	click("Wake up")
	settle("woken and shown", func() bool { return !app.newSession.open && app.selected == info.ID })
	list, _ = cl.Sessions()
	if len(list) != 1 || list[0].Hibernated || list[0].Exited {
		t.Fatalf("not woken: %+v", list)
	}
	var screen string
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline) && !strings.Contains(screen, "hello again"); time.Sleep(50 * time.Millisecond) {
		screen, _ = cl.Screen(info.ID)
	}
	if !strings.Contains(screen, "hello again") {
		t.Fatalf("the message didn't reach it: %q", screen)
	}
}

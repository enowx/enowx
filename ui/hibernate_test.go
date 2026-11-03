//go:build !windows

package ui

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/daemon"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
)

// From a session's menu: one done hibernates at once and wakes; one
// working is asked about first, and hibernates once its agent is done.
func TestHibernateFromTheSessionMenu(t *testing.T) {
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte("#!/bin/sh\necho ready\nsleep 60\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	srv, err := daemon.NewServer(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	app := New(Options{})
	posted := make(chan func(), 256)
	app.onMain = func(fn func()) { posted <- fn }
	a1, b1 := net.Pipe()
	go srv.ServeConn(a1)
	cl, err := client.New(b1, func(ev proto.Event) { app.handleEvent(app.gen, ev) })
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	project := t.TempDir()
	if _, err := cl.AddProject(project); err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, name := range []string{"alpha", "beta"} {
		info, err := cl.CreateSession(proto.SessionCreateParams{Agent: "claude", Name: name, Project: project, Dir: project, Cols: 80, Rows: 10})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, info.ID)
	}
	setState := func(id, state string) {
		if err := cl.Call(proto.MethodSessionState, proto.SessionStateParams{ID: id, State: state}, nil); err != nil {
			t.Fatal(err)
		}
	}
	setState(ids[0], proto.StateDone)
	setState(ids[1], proto.StateWorking)
	projects, _ := cl.Projects()
	sessions, _ := cl.Sessions()
	app.client, app.agents = cl, agent.List()
	app.setProjects(projects)
	for _, s := range sessions {
		app.upsert(s)
	}
	app.project = project
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Width(1000).Height(700).Background(theme.Background).Children(func() {
			ui.Column(c).Width(300).FillHeight().Children(func() { app.sessionsBody(c) })
			app.hibernateDialog(c)
		})
	}, 1000, 700)
	frames := func(what string, until func() bool) {
		t.Helper()
		deadline := time.Now().Add(15 * time.Second)
		for !until() {
			select {
			case fn := <-posted:
				fn()
			case <-time.After(30 * time.Millisecond):
			}
			tt.Frame()
			if time.Now().After(deadline) {
				t.Fatalf("%s: never; shows %q", what, tt.Texts())
			}
		}
	}
	info := func(id string) proto.SessionInfo {
		s, _ := app.sessionByID(id)
		return s
	}
	menu := func(name, item string) {
		t.Helper()
		frames("the session "+name, func() bool { return tt.HasText(name) })
		if err := tt.RightClick(name); err != nil {
			t.Fatal(err)
		}
		frames("its menu", func() bool { return tt.HasText(item) })
		if err := tt.Click(item); err != nil {
			t.Fatal(err)
		}
	}

	menu("alpha", "Hibernate ("+modKey+"⇧H)")
	frames("alpha hibernated", func() bool {
		return info(ids[0]).Hibernated
	})

	menu("beta", "Hibernate ("+modKey+"⇧H)")
	frames("the question", func() bool { return tt.HasText("Its agent is working: it stops mid-task") && tt.HasText("Once done") })
	shot(t, tt, "hibernate-ask.png")
	if err := tt.Click("Once done"); err != nil {
		t.Fatal(err)
	}
	frames("the dialog closed", func() bool { return app.hibernating == nil })
	if info(ids[1]).Hibernated {
		t.Fatal("hibernated while working")
	}
	setState(ids[1], proto.StateDone)
	frames("beta hibernated once done", func() bool { return info(ids[1]).Hibernated })
	shot(t, tt, "hibernate-list.png")

	menu("alpha", "Wake")
	frames("alpha awake", func() bool { s := info(ids[0]); return !s.Hibernated && !s.Exited })
}

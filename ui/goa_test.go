//go:build !windows

package ui

import (
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/daemon"
	"github.com/enowx/enowx/proto"
)

// GOD of Agents mode: the title bar's chip starts GOA (its agent picked
// with how far it holds GOA shown), then the app shows GOA's session
// alone, the session list gone, a strip on top reaching Notes and the
// other pages, the side panel's Sessions and Tasks tools listing the
// sessions and the board; Leave goes back.
func TestGOAMode(t *testing.T) {
	bin := t.TempDir()
	os.WriteFile(filepath.Join(bin, "claude"), []byte("#!/bin/sh\nwhile read -r line; do echo \"heard: $line\"; done\n"), 0o755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	srv, err := daemon.NewServer(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	a1, b1 := net.Pipe()
	go srv.ServeConn(a1)
	app := New(Options{})
	posted := make(chan func(), 256)
	app.onMain = func(fn func()) { posted <- fn }
	cl, err := client.New(b1, func(ev proto.Event) {
		app.goaEvent(ev)
		if ev.Name == proto.EventSessionUpdated {
			app.handleEvent(app.gen, ev)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	app.client, app.host.ID, app.host.Name = cl, "local", "this Mac"
	tt := ui.NewTester(func(c *ui.Context) { app.view(c) }, 1400, 850)
	frames := func(what string, until func() bool) {
		t.Helper()
		deadline := time.Now().Add(15 * time.Second)
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
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatalf("click %q: %v (%q)", label, err, tt.Texts())
		}
		tt.Advance(time.Second)
		tt.Frame()
	}
	frames("the chip", func() bool { return tt.HasText("GOD of Agents") && app.goa.host == "local" })
	click("One session controlling every session on every host")
	frames("the agents, with their levels", func() bool { return tt.HasText("Start GOD of Agents") && tt.HasText("enforced") })
	shot(t, tt, "goa-start.png")
	click("Start GOA")
	frames("GOA mode", func() bool { return app.goaOn() })
	frames("GOA's session shown", func() bool {
		s, ok := app.goaSession()
		return ok && app.selected == s.ID && tt.HasText("Notes") && tt.HasText("Canvas") && tt.HasText("Sessions")
	})
	if app.navShown() && tt.HasText("Projects") {
		t.Fatal("the session list shows in GOA mode")
	}

	// A task on the board shows in Tasks; its worker in Sessions.
	var task proto.GOATask
	if err := cl.Call(proto.MethodGOADelegate, proto.GOADelegateParams{From: app.goa.board.Session, Agent: "claude", Project: t.TempDir(),
		Title: "Fix the totals", Prompt: "Fix the rounding of the totals", NoRoute: true}, &task); err != nil {
		t.Fatal(err)
	}
	frames("the worker listed", func() bool { return tt.HasText("Fix the totals · working") })

	// Dragged into GOA's chat, a session is a pointer in the composer.
	drag := func(label string, x, y float32) {
		t.Helper()
		from, ok := tt.Find(label)
		if !ok {
			t.Fatalf("no %q: %q", label, tt.Texts())
		}
		tt.Press(from.X+from.W/2, from.Y+from.H/2)
		tt.Move(from.X+from.W/2-30, from.Y+from.H/2)
		tt.Frame()
		tt.Move(x, y)
		tt.Frame()
		tt.Release(x, y)
		tt.Frame()
	}
	pointer := "[session " + task.Session + ": "
	frames("GOA on its chat", func() bool { s, ok := app.goaSession(); return ok && app.currentView(s) == viewChat })
	drag("Fix the totals · working", 400, 400)
	g0, _ := app.goaSession()
	frames("the pointer in the composer", func() bool { return strings.HasPrefix(app.chats[g0.ID].Draft(), pointer) })
	if !app.goaOn() || app.goa.detail != nil {
		t.Fatalf("the drag clicked: detail %+v", app.goa.detail)
	}
	shot(t, tt, "goa-pointer.png")
	// A worker's details: its buttons inside the dialog's frame.
	for _, s := range app.sessions {
		if s.ID == task.Session {
			app.goa.detail = &goaDetail{open: true, host: "this Mac", session: s}
		}
	}
	frames("the worker's details", func() bool { return tt.HasText("Open in a tab") })
	for range 6 {
		tt.Advance(100 * time.Millisecond)
		tt.Frame()
	}
	shot(t, tt, "goa-worker-details.png")
	const dialogRight = (1400 + 520) / 2
	for _, label := range []string{"Cancel", "Change model", "Open in a tab"} {
		if b, ok := tt.Find(label); ok && b.X+b.W > dialogRight {
			t.Errorf("%q ends at %.0f, past the dialog's %d", label, b.X+b.W, dialogRight)
		}
	}
	app.goa.detail = nil
	tt.Frame()

	// A click opens it in a tab: live, GOA still there; closing the tab
	// leaves it running, and it opens again.
	gBefore := app.selected
	row, _ := tt.Find("Fix the totals · working")
	tt.Press(row.X+10, row.Y+row.H/2)
	tt.Release(row.X+10, row.Y+row.H/2)
	frames("the tab", func() bool { return len(app.goa.tabs) == 1 && app.goa.tabs[0].term != nil })
	tab := app.goa.tabs[0]
	if !app.goaOn() || app.selected != gBefore || app.goa.shown != tab.key {
		t.Fatalf("opening a tab: goa %v selected %q shown %q", app.goaOn(), app.selected, app.goa.shown)
	}
	if err := cl.Input(task.Session, "seen in the tab", false, true); err != nil {
		t.Fatal(err)
	}
	frames("the tab live", func() bool { return strings.Contains(tab.term.Text(), "heard: seen in the tab") })
	for range 6 {
		tt.Advance(100 * time.Millisecond)
		tt.Frame()
	}
	shot(t, tt, "goa-tab.png")
	click("Close this tab: the session keeps running")
	if len(app.goa.tabs) != 0 || app.goa.shown != "" {
		t.Fatalf("the tab stayed: %+v", app.goa.tabs)
	}
	if err := cl.Input(task.Session, "still running", false, true); err != nil {
		t.Fatal(err)
	}
	frames("still running", func() bool {
		screen, _ := cl.Screen(task.Session)
		return strings.Contains(screen, "heard: still running")
	})
	for _, s := range app.sessions {
		if s.ID == task.Session {
			app.openGOATab(app.host.Name, cl, s)
		}
	}
	frames("opened again", func() bool { return len(app.goa.tabs) == 1 && app.goa.tabs[0].term != nil })
	app.goaShow("")
	tt.Frame()
	if app.currentView(g0) != viewChat || app.goaShownTab() != nil {
		t.Fatalf("back to GOA: view %q", app.currentView(g0))
	}
	app.closeGOATabs()
	// In its terminal, typed where the prompt is, not sent.
	app.showView(g0, viewTerminal)
	frames("GOA's terminal", func() bool { return app.panes[g0.ID] != nil })
	for range 4 {
		tt.Advance(100 * time.Millisecond)
		tt.Frame()
	}
	drag("Fix the totals · working", 400, 400)
	frames("the pointer in the terminal", func() bool {
		screen, _ := cl.Screen(g0.ID)
		return strings.Contains(screen, pointer)
	})
	if screen, _ := cl.Screen(g0.ID); strings.Contains(screen, "heard:") {
		t.Fatalf("the pointer was sent: %q", screen)
	}
	app.showView(g0, viewChat)
	click("Tasks")
	frames("the task on the board", func() bool { return tt.HasText("Fix the totals") && tt.HasText("its own · its own") })
	shot(t, tt, "goa-mode.png")

	// GOA opens on its chat, its agent having one; the strip's Browser shows
	// its browser in the main view, GOD of Agents its chat again.
	g, _ := app.goaSession()
	if app.currentView(g) != viewChat {
		t.Fatalf("GOA opened on %q, not its chat", app.currentView(g))
	}
	app.goaShow(goaBrowser)
	for range 10 {
		tt.Advance(100 * time.Millisecond) // the strip's colours ease over
		tt.Frame()
	}
	if app.page != "" || app.currentView(g) != viewBrowser {
		t.Fatalf("Browser from the strip: page %q view %q", app.page, app.currentView(g))
	}
	shot(t, tt, "goa-browser.png")
	if slices.Contains(app.sessionViews(g), viewBrowser) {
		t.Fatalf("Browser twice: in the strip and in GOA's header (%q)", app.sessionViews(g))
	}
	app.goaShow("")
	tt.Frame()
	if app.currentView(g) != viewChat {
		t.Fatalf("back to GOA: view %q", app.currentView(g))
	}

	// The strip reaches the pages, and back.
	click("Notes")
	if app.page != pageNotes || !app.goaOn() {
		t.Fatalf("Notes from GOA mode: page %q", app.page)
	}
	frames("the strip on the Notes page", func() bool { return tt.HasText("Leave") })
	// With no session list, Notes and Canvas show their own lists.
	if app.notesOptions().ListOutside {
		t.Fatal("Notes in GOA mode leaves its list to a sidebar that isn't there")
	}
	click("Canvas")
	if app.page != pageCanvas || app.canvasOptions().ListOutside {
		t.Fatalf("Canvas in GOA mode: page %q, its list left out", app.page)
	}
	// Their lists bring the choice of the user's own or a team's space.
	app.spacesOf().list = []proto.Space{{Team: "t1", Name: "Design"}}
	frames("the space to pick", func() bool { return tt.HasText("Personal") })
	shot(t, tt, "goa-canvas.png")
	app.spacesOf().list = nil
	click("Notes")
	click("GOD of Agents")
	if app.page != "" {
		t.Fatalf("back to GOA: page %q", app.page)
	}
	// Its rules: written in their dialog from the strip, saved on the host.
	click("Rules")
	frames("the rules dialog", func() bool {
		return app.goa.rules != nil && app.goa.rules.loaded && tt.HasText("Rules of GOD of Agents") && tt.HasText("Write with AI")
	})
	app.goa.rules.text = "- Ask me before merging"
	tt.Frame()
	shot(t, tt, "goa-rules.png")
	click("Save")
	frames("the rules saved", func() bool { return app.goa.rules == nil })
	var rules proto.GOARules
	if err := cl.Call(proto.MethodGOARules, nil, &rules); err != nil || rules.Rules != "- Ask me before merging" {
		t.Fatalf("the rules kept: %+v %v", rules, err)
	}

	click("Leave")
	if !app.goaOn() || app.confirming == nil {
		t.Fatal("left GOA mode without asking first")
	}
	click("Leave the mode")
	if app.goaOn() || app.state.GOA {
		t.Fatal("still in GOA mode")
	}
	frames("the chip again", func() bool { return tt.HasText("GOD of Agents") })
}

// A tab works on another host's connection the same, and brings a hibernated
// session back to show it.
func TestGOATabOnAnotherHost(t *testing.T) {
	bin := t.TempDir()
	os.WriteFile(filepath.Join(bin, "claude"), []byte("#!/bin/sh\necho up\nwhile read -r line; do echo \"heard: $line\"; done\n"), 0o755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	srv, err := daemon.NewServer(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	a1, b1 := net.Pipe()
	go srv.ServeConn(a1)
	cl, err := client.New(b1, func(proto.Event) {})
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	s, err := cl.CreateSession(proto.SessionCreateParams{Agent: "claude", Dir: t.TempDir(), Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	if err := cl.Call(proto.MethodSessionState, proto.SessionStateParams{ID: s.ID, State: proto.StateDone}, nil); err != nil {
		t.Fatal(err)
	}
	res, err := cl.HibernateSessions(proto.SessionHibernateParams{IDs: []string{s.ID}})
	if err != nil || len(res.Hibernated) != 1 {
		t.Fatalf("hibernating: %+v %v", res, err)
	}
	s.Exited, s.Hibernated = true, true
	app := New(Options{})
	posted := make(chan func(), 16)
	app.onMain = func(fn func()) { posted <- fn }
	app.openGOATab("enowxlabs", cl, s)
	tab := app.goa.tabs[0]
	deadline := time.After(15 * time.Second)
	for tab.term == nil && tab.err == "" {
		select {
		case fn := <-posted:
			fn()
		case <-deadline:
			t.Fatal("never attached")
		}
	}
	if tab.err != "" {
		t.Fatal(tab.err)
	}
	defer app.closeGOATabs()
	if err := cl.Input(s.ID, "over there", false, true); err != nil {
		t.Fatal(err)
	}
	for !strings.Contains(tab.term.Text(), "heard: over there") {
		select {
		case <-deadline:
			t.Fatalf("not live: %q", tab.term.Text())
		case <-time.After(20 * time.Millisecond):
		}
	}
}

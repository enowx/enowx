//go:build !windows

package ui

import (
	"net"
	"os"
	"path/filepath"
	"slices"
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

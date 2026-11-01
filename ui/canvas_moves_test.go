package ui

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
)

// What an agent asks to show (canvas open, goto, compare) leaves the user
// where they are, with what they were typing, and raises a card; only a click
// on the card goes there.
func TestAgentsDontMoveTheUser(t *testing.T) {
	h := newCanvasHarness(t)
	other, err := h.cl.CreateCanvas(proto.CanvasCreateParams{Title: "Other", Kind: proto.CanvasWireframe, Frames: []proto.CanvasFrame{{Name: "Solo"}}})
	if err != nil {
		t.Fatal(err)
	}
	cv, err := h.cl.CreateCanvas(proto.CanvasCreateParams{Title: "Landing", Kind: proto.CanvasWireframe,
		Frames: []proto.CanvasFrame{{Name: "One"}, {Name: "Two"}, {Name: "Three"}}})
	if err != nil {
		t.Fatal(err)
	}
	hc := h.app.canvasesOf()
	stays := func(what, page, open, frame string) {
		t.Helper()
		n := 0
		h.settle(what, func() bool { n++; return n > 10 })
		id, at := h.app.canvases.Showing()
		if h.app.page != page || open != "" && id != open || hc.open != "" && hc.open != open {
			t.Fatalf("%s moved the user: page %q, canvas %q (wanted %q), asked %q", what, h.app.page, id, open, hc.open)
		}
		if frame != "" && at != frame {
			t.Fatalf("%s moved the design to %s", what, at)
		}
	}
	show := func(id string) {
		h.app.page, hc.open, hc.frame = pageCanvas, id, ""
	}
	ask := func(c proto.CanvasControl) {
		t.Helper()
		if _, err := h.cl.ShowCanvas(c); err != nil {
			t.Fatal(err)
		}
	}

	// From another page altogether.
	h.app.page = pageSettings
	ask(proto.CanvasControl{ID: cv.ID, Action: proto.CanvasOpen})
	stays("canvas open", pageSettings, "", "")
	if len(h.app.sessionToasts) != 1 || h.app.sessionToasts[0].name != "Landing" {
		t.Fatalf("no card: %+v", h.app.sessionToasts)
	}
	ask(proto.CanvasControl{ID: cv.ID, Action: proto.CanvasCompare, Frames: []string{"1", "2"}})
	stays("canvas compare", pageSettings, "", "")
	if len(h.app.sessionToasts) != 1 {
		t.Fatalf("each ask added a card: %d", len(h.app.sessionToasts))
	}

	// On another canvas, and on this one at another design.
	show(other.ID)
	h.settle("the other canvas", func() bool { return h.tt.HasText("Frame Solo") })
	ask(proto.CanvasControl{ID: cv.ID, Action: proto.CanvasOpen, Frame: "3"})
	stays("canvas goto", pageCanvas, other.ID, "")
	if !h.tt.HasText("Frame Solo") {
		t.Fatalf("the canvas changed under the user: %q", h.tt.Texts())
	}

	show(cv.ID)
	h.settle("the canvas", func() bool { return h.tt.HasText("Frame One") })
	_, one := h.app.canvases.Showing()
	h.app.sessionToasts = nil
	ask(proto.CanvasControl{ID: cv.ID, Action: proto.CanvasOpen})
	stays("canvas open on the canvas shown", pageCanvas, cv.ID, one)
	if len(h.app.sessionToasts) != 0 {
		t.Fatalf("a card for what shows already: %+v", h.app.sessionToasts)
	}
	ask(proto.CanvasControl{ID: cv.ID, Action: proto.CanvasOpen, Frame: "3"})
	stays("canvas goto on the canvas shown", pageCanvas, cv.ID, one)

	// An agent's work on another design leaves the view too.
	el := proto.CanvasElement{ID: "x1", Type: proto.ShapeRect, X: 1, Y: 1, W: 9, H: 9, Text: "x1"}
	if _, err := h.cl.EditCanvas(proto.CanvasEdit{ID: cv.ID, Frame: cv.Frames[2].ID, Op: proto.CanvasOpElementSet, Element: &el, Session: "agent-1"}); err != nil {
		t.Fatal(err)
	}
	stays("an agent's change", pageCanvas, cv.ID, one)

	// The click on the card is the user's: it goes there.
	if len(h.app.sessionToasts) != 1 {
		t.Fatalf("no card for the design: %+v", h.app.sessionToasts)
	}
	h.app.openToast(h.app.sessionToasts[0])
	h.settle("the design asked for", func() bool { return h.tt.HasText("Frame Three") })
	if len(h.app.sessionToasts) != 0 {
		t.Fatal("the card stayed")
	}
}

// Typing in the agent's composer goes on through what agents ask to show:
// every character lands, and the composer keeps the keyboard.
func TestTypingSurvivesAgentsAsking(t *testing.T) {
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte("#!/bin/sh\nsleep 60\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	h := newCanvasHarness(t)
	h.app.agents = []proto.AgentInfo{{ID: "claude", Name: "Claude Code", Command: "claude", Installed: true}}
	cv, err := h.cl.CreateCanvas(proto.CanvasCreateParams{Title: "Login", Kind: proto.CanvasWireframe, Frames: []proto.CanvasFrame{{Name: "One"}, {Name: "Two"}}})
	if err != nil {
		t.Fatal(err)
	}
	other, err := h.cl.CreateCanvas(proto.CanvasCreateParams{Title: "Other", Kind: proto.CanvasWireframe, Frames: []proto.CanvasFrame{{Name: "Solo"}}})
	if err != nil {
		t.Fatal(err)
	}
	hero := proto.CanvasElement{ID: "hero", Type: proto.ShapeRect, X: 200, Y: 200, W: 400, H: 160, Text: "Welcome back"}
	if _, err := h.cl.EditCanvas(proto.CanvasEdit{ID: cv.ID, Frame: cv.Frames[0].ID, Op: proto.CanvasOpElementSet, Element: &hero}); err != nil {
		t.Fatal(err)
	}
	hc := h.app.canvasesOf()
	hc.open = cv.ID
	h.settle("the canvas open", func() bool { return h.tt.HasText("Welcome back") })
	if err := h.tt.Click("hero"); err != nil {
		t.Fatal(err)
	}
	h.tt.Advance(time.Second)
	h.settle("the inspector", func() bool { return h.tt.HasText("Ask agent") })
	if err := h.tt.Click("Ask agent about hero"); err != nil {
		t.Fatal(err)
	}
	h.settle("the agent's panel", func() bool { return h.tt.HasText("Design with Claude Code") })
	h.tt.Advance(time.Second)
	h.tt.Frame()
	if err := h.tt.Click("Message"); err != nil {
		t.Fatal(err)
	}
	h.tt.Frame()
	asks := []proto.CanvasControl{
		{ID: other.ID, Action: proto.CanvasOpen},
		{ID: cv.ID, Action: proto.CanvasOpen, Frame: "2"},
		{ID: cv.ID, Action: proto.CanvasCompare, Frames: []string{"1", "2"}},
	}
	want := ""
	for i, ask := range asks {
		piece := strings.Repeat(string(rune('a'+i)), 4)
		h.tt.Type(piece[:2])
		if _, err := h.cl.ShowCanvas(ask); err != nil {
			t.Fatal(err)
		}
		n := 0
		h.settle("the ask", func() bool { n++; return n > 8 })
		h.tt.Type(piece[2:])
		h.tt.Frame()
		want += piece
		if !h.tt.Focused("Message") {
			t.Fatalf("after ask %d the composer lost the keyboard", i)
		}
	}
	h.tt.Key(0, ui.KeyEnter)
	h.settle("what was typed sent whole", func() bool {
		list, _ := h.cl.Sessions()
		return slices.ContainsFunc(list, func(s proto.SessionInfo) bool { return strings.Contains(s.Prompt, want) })
	})
	if id, _ := h.app.canvases.Showing(); id != cv.ID || h.app.page != pageCanvas {
		t.Fatalf("moved to %s on %q", id, h.app.page)
	}
}

// A connection dropping and coming back leaves the user's page, and restoring
// the view on it neither leaves that page nor wakes a session.
func TestReconnectKeepsTheUserWhereTheyAre(t *testing.T) {
	h := newCanvasHarness(t)
	app := h.app
	app.host.ID, app.retryHost = "h1", "h1"
	s := proto.SessionInfo{ID: "s1", Project: "/p", Hibernated: true}
	app.sessions = []proto.SessionInfo{s}
	app.state.Sessions = map[string]string{"h1": "s1"}
	app.page = pageNotes
	app.restoredView = false
	app.restoreView()
	if app.page != pageNotes || app.selected != "s1" {
		t.Fatalf("page %q, session %q", app.page, app.selected)
	}
}

// An agent asking the user to log in raises a card; the click opens the page.
func TestAgentLoginAskIsACard(t *testing.T) {
	h := newCanvasHarness(t)
	h.app.page = pageSettings
	h.app.askVaultLogin("github")
	if h.app.page != pageSettings || len(h.app.sessionToasts) != 1 {
		t.Fatalf("page %q, cards %d", h.app.page, len(h.app.sessionToasts))
	}
	h.app.openToast(h.app.sessionToasts[0])
	if h.app.page != pageVault {
		t.Fatalf("the click went to %q", h.app.page)
	}
}

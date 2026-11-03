//go:build !windows

package ui

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
)

// The dock stays where it is on Notes and Canvas, and opens the panel for the
// session selected last with the page still open.
func TestDockStaysOnNotesAndCanvas(t *testing.T) {
	t.Setenv("ANDROID_HOME", stubAndroidSDK(t))
	h := newNotesHarness(t)
	seedAudit(t, h)
	h.app.selectSession(h.app.sessions[0])
	chosen := h.app.selected
	h.tt = ui.NewTester(func(c *ui.Context) { h.app.view(c) }, 1440, 900)
	draw := func() {
		for range 4 {
			h.settle("", func() bool { return true })
		}
	}
	draw()
	// More tools: the panel, once open, has a "Files" of its own.
	at, ok := h.tt.Find("More tools")
	if !ok {
		t.Fatalf("no dock beside the session: %q", h.tt.Texts())
	}
	for _, page := range []string{pageNotes, pageCanvas} {
		h.app.page = page
		draw()
		got, ok := h.tt.Find("More tools")
		if !ok || got != at {
			t.Fatalf("%s: dock's More tools at %+v, %v; on a session at %+v", page, got, ok, at)
		}
		if h.app.panelShown() {
			t.Fatalf("%s: the panel opened by itself", page)
		}
		if err := h.tt.Click("Files"); err != nil {
			t.Fatal(err)
		}
		draw()
		if h.app.page != page || !h.app.panelShows(panelFiles) || h.app.selected != chosen {
			t.Fatalf("%s: after Files page %q, panel %v, selected %q", page, h.app.page, h.app.panelShown(), h.app.selected)
		}
		if got, _ := h.tt.Find("More tools"); got != at {
			t.Fatalf("%s: the panel moved the dock to %+v", page, got)
		}
		h.app.toggleTool(panelFiles) // as the dock's button does: the panel's title reads the same
		draw()
		if h.app.page != page || h.app.panelShown() {
			t.Fatalf("%s: Files again leaves page %q, panel %v", page, h.app.page, h.app.panelShown())
		}
	}
}

// With no session the tools show but do nothing, in the dock and its menu.
func TestDockWithNoSessionDoesNothing(t *testing.T) {
	t.Setenv("ANDROID_HOME", stubAndroidSDK(t))
	h := newNotesHarness(t)
	seedAudit(t, h)
	h.app.page, h.app.selected = pageNotes, ""
	h.tt = ui.NewTester(func(c *ui.Context) { h.app.view(c) }, 1440, 900)
	draw := func() {
		for range 4 {
			h.settle("", func() bool { return true })
		}
	}
	draw()
	if _, ok := h.tt.Find("Files"); !ok {
		t.Fatalf("no dock: %q", h.tt.Texts())
	}
	_ = h.tt.Click("Files") // disabled: nothing to find a click on, or nothing happens
	draw()
	if err := h.tt.Click("More tools"); err != nil {
		t.Fatal(err)
	}
	draw()
	if _, ok := h.tt.Find("Ports"); !ok {
		t.Fatalf("Ports isn't in the menu: %q", h.tt.Texts())
	}
	_ = h.tt.Click("Ports")
	draw()
	if h.app.state.Panel.Open || h.app.page != pageNotes {
		t.Fatalf("panel %+v, page %q", h.app.state.Panel, h.app.page)
	}
	h.app.togglePanel() // the shortcut's way
	if h.app.state.Panel.Open {
		t.Fatal("the shortcut opened a session's tool with no session")
	}
}

// Everything side by side on Notes: the frame, the panel and the dock don't
// overlap at the smallest window, and the panel keeps its width.
func TestNotesPanelAgentAndDockSideBySide(t *testing.T) {
	t.Setenv("ANDROID_HOME", stubAndroidSDK(t))
	h := newNotesHarness(t)
	seedAudit(t, h)
	h.app.agents = []proto.AgentInfo{{ID: "claude", Name: "Claude Code", Command: "claude", Installed: true}}
	h.app.selectSession(h.app.sessions[0])
	h.app.page = pageNotes
	h.app.notes.Select(h.app.notesOptions(), h.app.noteList.list[0].ID)
	h.app.notes.SetAssistant(true)
	h.app.state.RailWide = true
	h.app.showTool(panelFiles)
	h.tt = ui.NewTester(func(c *ui.Context) { h.app.view(c) }, 1200, 720)
	h.settle("the note", func() bool { return h.app.notes.Opened() })
	h.settle("the note's agent", func() bool { return h.tt.HasText(notesSuggestions[0].title) })
	grip, ok := h.tt.Find("Resize the panel")
	dock, okDock := h.tt.Find("More tools")
	if !ok || !okDock {
		t.Fatalf("grip %v, dock %v", ok, okDock)
	}
	if dock.X-grip.X < defaultPanelWidth {
		t.Fatalf("panel %.0f wide, wanted it kept at %d", dock.X-grip.X, defaultPanelWidth)
	}
	if grip.X < navWidth+minNotesWidth {
		t.Fatalf("panel starts at %.0f, over the page", grip.X)
	}
	if ps := layoutProblems(h.tt, 1200, 720, false); len(ps) > 0 {
		t.Fatalf("%q", ps)
	}
}

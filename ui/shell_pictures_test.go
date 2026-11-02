package ui

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
)

// The window's menus and states the audit doesn't draw, to compare with the
// design; with SHOTS set it keeps their pictures. Every step's labels must show.
func TestShellMenusAndStates(t *testing.T) {
	h := newNotesHarness(t)
	seedAudit(t, h)
	a := h.app
	a.state.RailWide = true
	a.state.SessionsByProject = true
	a.page = ""
	if len(a.sessions) > 0 {
		a.selectSession(a.sessions[0])
	}
	tt := ui.NewTester(func(c *ui.Context) { a.view(c) }, 1280, 800)
	h.tt = tt
	settle := func() {
		for range 6 {
			h.settle("", func() bool { return true })
			tt.Advance(200 * time.Millisecond)
		}
	}
	settle()
	step := func(name string, do func() error, want ...string) {
		t.Helper()
		if err := do(); err != nil {
			t.Fatalf("%s: %v in %q", name, err, tt.Texts())
		}
		settle()
		shot(t, tt, filepath.Join("shell", name+".png"))
		for _, w := range want {
			if !tt.HasText(w) {
				t.Errorf("%s: no %q in %q", name, w, tt.Texts())
			}
		}
		for _, p := range outOfParents(tt) {
			t.Errorf("%s: %s", name, p)
		}
	}
	key := func() error { tt.Key(0, ui.KeyEscape); return nil }
	name := a.sessionName(a.sessions[0])
	step("more-menu", func() error { return tt.Click("More") }, "Credentials", "Hosts")
	step("more-closed", key)
	step("session-menu", func() error { return tt.RightClick(name) }, "Rename…", "Delete session…")
	step("session-closed", key)
	step("project-menu", func() error { tt.RightClickAt(80, 112); return nil }, "New session", "History…")
	step("project-closed", key)
	step("new-tab-menu", func() error { return tt.Click("New tab in this session's folder") })
	step("new-tab-closed", key)
	step("run-menu", func() error { return tt.Click("Run a command of this project") }, "Save a command…")
	step("run-closed", key)
	step("status-news", func() error { a.notify("Copied ui/goa.go"); return nil }, "Copied ui/goa.go")
	step("status-error", func() error { a.status, a.notice = "Couldn't reach the host", ""; return nil }, "Couldn't reach the host")
	a.status = ""
	// Empty states.
	sessions, projects := a.sessions, a.projects
	a.sessions, a.selected = nil, ""
	step("empty-project", func() error { return nil }, "No sessions in antares", "New session")
	a.projects, a.project = nil, ""
	step("empty-window", func() error { return nil }, "Open a folder to start")
	a.sessions, a.projects = sessions, projects
	a.project = projects[0].Path
	a.selectSession(sessions[0])
	// Two sessions side by side.
	second, err := h.cl.CreateSession(proto.SessionCreateParams{Agent: "shell", Project: projects[0].Path, Name: "a second shell with a rather long name to see it run in its pane"})
	if err != nil {
		t.Fatal(err)
	}
	a.upsert(second)
	step("split", func() error { a.setSplit(true); return nil }, "Side by side · 2 sessions", "Auto")
	a.setSplit(false)
	step("split-closed", func() error { return nil })
	// A session in a viewer, the switch unlit, and back.
	step("banner", func() error {
		s := sessions[0]
		s.SkipPermissions = true
		a.upsert(proto.SessionInfo(s))
		return nil
	}, "Skips permission prompts")
}

// A note open in the window, its list in the sidebar's place, to compare with
// the design's Notes page.
func TestNotesPageInTheWindow(t *testing.T) {
	h := newNotesHarness(t)
	seedAudit(t, h)
	a := h.app
	a.state.RailWide = true
	a.page = pageNotes
	a.notes.SetAssistant(false)
	a.notes.Select(a.notesOptions(), a.noteList.list[0].ID)
	tt := ui.NewTester(func(c *ui.Context) { a.view(c) }, 1440, 900)
	h.tt = tt
	title := a.noteList.list[0].Title
	h.settle("the note open", func() bool { return tt.HasText("one") && tt.HasText(title) })
	for range 6 {
		tt.Advance(200 * time.Millisecond)
		h.settle("", func() bool { return true })
	}
	shot(t, tt, filepath.Join("shell", "notes-in-window.png"))
	for _, p := range outOfParents(tt) {
		t.Error(p)
	}
}

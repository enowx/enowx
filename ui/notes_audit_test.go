package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/notesview"
)

// noteAuditBody has every kind of block a note shows.
const noteAuditBody = "# Heading\n\nSome text, long enough to reach the end of the line in a narrow frame and go on to another one.\n\n## To-do\n\n- [ ] one\n- [x] two\n- a bullet\n1. a number\n\n> A quote.\n\n" +
	":::callout warning Waiting on the owner\nThe choice isn't made yet.\n:::\n\n:::toggle Where it lives\nIn the project.\n:::\n\n" +
	"| Area | Surfaces |\n|---|---|\n| Notes | 68 |\n| Canvas | 61 |\n\n```sh\nenowx-app notes check NOTE --item \"shell.title-bar\" --and-a-long-flag-to-see-it-wrap-inside-its-frame-whatever-its-width\n```\n\n" +
	"| Area | Surfaces |\n|---|---|\n| Shell | 12 |\n| Sessions | 34 |\n| Tools | 56 |\n| Hosts | 7 |\n| Vault | 8 |\n| GOA | 9 |\n\n```svg\n<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 40 20\"><rect width=\"40\" height=\"20\" fill=\"#0090FF\"/></svg>\n```\n\n```svg\nnot a drawing\n```\n\n" +
	":::callout tip A tip\nA line.\n:::\n\n:::callout danger A danger\nA line.\n:::\n\n::spacer\n\n::embed\n\n" +
	":::tabs\n::tab Summary\nIts summary.\n::tab Details\nIts details.\n:::\n\n:::columns\n::col Left\nOne side.\n::col Right\nThe other.\n:::\n\n:::cards\n::card First\nA card.\n::card Second\nAnother.\n:::\n\n::toc\n\n::progress\n\n---\n\n::embed canvas:abc\n"

// The notes' surfaces in every state the layout audit should see: a note
// with each kind of block, in each mode, nested, with the agent's panel, and
// the dialogs and menus reached from it.
func TestLayoutAuditNotes(t *testing.T) {
	h := newNotesHarness(t)
	seedAudit(t, h)
	rich, err := h.cl.SaveNote(proto.Note{NoteInfo: proto.NoteInfo{Title: "A note with every block and a title so long it has to run inside the header of the frame around it"}, Body: noteAuditBody})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.cl.SaveNoteGroup("", proto.NoteGroup{Name: "Plans with a long name that runs"}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.cl.SetNoteGroup(rich.ID, "Plans with a long name that runs"); err != nil {
		t.Fatal(err)
	}
	// A second save, for the history to hold an earlier version.
	if _, err := h.cl.SaveNote(proto.Note{NoteInfo: rich, Body: noteAuditBody + "\nOne more line.\n"}); err != nil {
		t.Fatal(err)
	}
	h.app.agents = []proto.AgentInfo{{ID: "claude", Name: "Claude Code", Command: "claude", Installed: true}, {ID: "codex", Name: "Codex", Command: "codex", Installed: true}}
	child, err := h.cl.SaveNote(proto.Note{NoteInfo: proto.NoteInfo{Title: "Under it", Parent: rich.ID}, Body: "Text under it."})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.cl.AttachNote(rich.ID, []proto.NoteProject{{Ref: "gone", Name: "a project with a rather long name", Host: "elsewhere", HostName: "a host", Path: "/some/path"}}); err != nil {
		t.Fatal(err)
	}
	click := func(labels ...string) func() {
		return func() {
			for _, l := range labels {
				h.settle(l, func() bool { return h.tt.HasText(l) })
				if err := h.tt.Click(l); err != nil {
					t.Error(l, err)
				}
				// What opens slides in: done before the next click.
				h.settle("", func() bool { return true })
				h.tt.Advance(time.Second)
			}
		}
	}
	rightClick := func(label string, then ...string) func() {
		return func() {
			h.settle(label, func() bool { _, ok := h.tt.Find(label); return ok })
			if err := h.tt.RightClick(label); err != nil {
				t.Error(label, err)
			}
			h.settle("", func() bool { return true })
			click(then...)()
		}
	}
	const group = "Group Plans with a long name that runs"
	states := []struct {
		name string
		note string
		then func()
	}{
		{name: "empty"},
		{name: "edit", note: rich.ID},
		{name: "read", note: rich.ID, then: click("Read")},
		{name: "markdown", note: rich.ID, then: click("Markdown")},
		{name: "nested", note: child.ID},
		{name: "agent", note: rich.ID, then: click("Note agent")},
		{name: "page-menu", note: rich.ID, then: click("More for this note")},
		{name: "io-menu", then: click("Import and export")},
		{name: "new-group", then: click("Import and export", "New group…")},
		{name: "icon-picker", note: child.ID, then: click("Page icon")},
		{name: "history", note: rich.ID, then: click("More for this note", "Version history…")},
		{name: "attach", note: rich.ID, then: click("More for this note", "Attach to projects…")},
		{name: "export", note: rich.ID, then: click("More for this note", "Export…")},
		{name: "import", then: click("Import and export", "Import…")},
		{name: "jump", note: rich.ID, then: func() { h.app.notes.OpenJump() }},
		{name: "delete", note: rich.ID, then: click("More for this note", "Delete…")},
		{name: "row-menu", then: rightClick("Release plan")},
		{name: "group-menu", then: rightClick(group)},
		{name: "edit-group", then: rightClick(group, "Edit group…")},
		{name: "delete-group", then: rightClick(group, "Delete group…")},
		{name: "new-group-error", then: click("Import and export", "New group…", "Create")},
		{name: "new-group-icon", then: click("Import and export", "New group…", "Group icon")},
		{name: "group-filter", then: click("Show: All groups")},
		{name: "project-filter", then: click("Projects: all pages")},
		{name: "by-project", then: click("Projects: all pages", "Grouped by project")},
		{name: "agent-picker", note: rich.ID, then: click("Note agent", "Agent")},
		{name: "outline-hidden", note: rich.ID, then: click("Outline · ⌘K jumps to a section")},
		{name: "icon-custom", note: child.ID, then: click("Page icon", "Custom")},
		{name: "slash", note: child.ID, then: func() {
			click("Type below")()
			h.tt.Type("/")
		}},
		{name: "block-source", note: rich.ID, then: click("Waiting on the owner")},
		{name: "filter-none", then: func() {
			click("Filter notes")()
			h.tt.Type("nothing is called this")
		}},
		{name: "scrolled", note: rich.ID, then: func() {
			h.tt.Wheel(600, 400, 0, 1500)
		}},
		{name: "earlier-chats", note: rich.ID, then: func() {
			h.app.noteChatLog(rich.ID) // reads it, then what is said is put in its place
			h.settle("the log", func() bool { return !h.app.noteChatsRead.loading[rich.ID] })
			h.app.noteChatsRead.logs[rich.ID] = proto.NoteChatLog{Note: rich.ID, Messages: []proto.NoteChatMessage{
				{Role: proto.TranscriptUser, Text: "Make it a checklist", Time: time.Now().Add(-time.Hour)},
				{Role: proto.TranscriptAssistant, Agent: "claude", Host: "this Mac", Text: strings.Repeat("A long answer that wraps and stops at four lines. ", 12), Time: time.Now()},
			}}
			click("Note agent")()
		}},
		{name: "import-notion", then: click("Import and export", "Import…", "Notion (API)")},
	}
	shots := os.Getenv("SHOTS")
	var problems []string
	sizes := drawSizes(auditSizes)
	for _, st := range states {
		if !auditWanted("notes-" + st.name) {
			continue
		}
		for _, size := range sizes {
			closeAudit(h.app)
			h.app.page = pageNotes
			h.app.state.RailWide = true
			// A fresh view: a menu or mode the last state left would turn its
			// button's click into a close.
			h.app.notes = notesview.View{}
			h.tt = auditTester(h.app, size[0], size[1])
			h.settle("", func() bool { return true })
			if st.note != "" {
				h.app.notes.Select(h.app.notesOptions(), st.note)
				h.settle("the note", func() bool { return h.app.notes.Opened() })
			} else {
				h.app.notes.CloseNote()
			}
			h.settle("", func() bool { return true })
			if st.then != nil {
				st.then()
			}
			// Calls to the host come back, then what opens and fades ends.
			waited := 0
			h.settle("", func() bool { waited++; return waited > 12 && !h.tt.HasText("Reading…") })
			h.tt.Advance(time.Second)
			h.tt.Frame()
			name := fmt.Sprintf("notes-%s-%dx%d", st.name, size[0], size[1])
			if shots != "" {
				shot(t, h.tt, filepath.Join("audit", name+".png"))
			}
			for _, pr := range layoutProblems(h.tt, float32(size[0]), float32(size[1]), false) {
				problems = append(problems, name+": "+pr)
			}
		}
	}
	if shots != "" {
		// The whole note on one picture, every block in each mode.
		h.tt = ui.NewTester(func(c *ui.Context) { h.app.view(c) }, 1280, 2800)
		closeAudit(h.app)
		h.app.page = pageNotes
		h.app.notes = notesview.View{}
		h.settle("", func() bool { return true })
		h.app.notes.Select(h.app.notesOptions(), rich.ID)
		h.settle("the note", func() bool { return h.app.notes.Opened() })
		for mode, name := range []string{"edit", "read"} {
			h.app.notes.SetMode(mode)
			waited := 0
			h.settle("", func() bool { waited++; return waited > 2 })
			h.tt.Advance(time.Second)
			h.tt.Frame()
			shot(t, h.tt, filepath.Join("audit", "notes-tall-"+name+".png"))
		}
	}
	if len(problems) > 0 {
		t.Errorf("%d layout problems:\n%s", len(problems), strings.Join(problems, "\n"))
	}
}

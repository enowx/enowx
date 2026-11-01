package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
)

// noteAuditBody has every kind of block a note shows.
const noteAuditBody = "# Heading\n\nSome text, long enough to reach the end of the line in a narrow frame and go on to another one.\n\n## To-do\n\n- [ ] one\n- [x] two\n- a bullet\n1. a number\n\n> A quote.\n\n" +
	":::callout warning Waiting on the owner\nThe choice isn't made yet.\n:::\n\n:::toggle Where it lives\nIn the project.\n:::\n\n" +
	"| Area | Surfaces |\n|---|---|\n| Notes | 68 |\n| Canvas | 61 |\n\n```sh\nenowx-app notes check NOTE --item \"shell.title-bar\" --and-a-long-flag-to-see-it-wrap-inside-its-frame-whatever-its-width\n```\n\n" +
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
				h.settle("", func() bool { return true })
			}
		}
	}
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
	}
	shots := os.Getenv("SHOTS")
	only := os.Getenv("AUDIT")
	var problems []string
	for _, st := range states {
		if only != "" && !strings.Contains("notes-"+st.name, only) {
			continue
		}
		for _, size := range auditSizes {
			closeAudit(h.app)
			h.app.page = pageNotes
			h.app.state.RailWide = true
			h.app.notes.SetMode(0)
			h.tt = ui.NewTester(func(c *ui.Context) { h.app.view(c) }, size[0], size[1])
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
			for range 4 {
				h.settle("", func() bool { return true })
			}
			name := fmt.Sprintf("notes-%s-%dx%d", st.name, size[0], size[1])
			if shots != "" {
				shot(t, h.tt, filepath.Join("audit", name+".png"))
			}
			for _, pr := range layoutProblems(h.tt, float32(size[0]), float32(size[1]), false) {
				problems = append(problems, name+": "+pr)
			}
		}
	}
	if len(problems) > 0 {
		t.Errorf("%d layout problems:\n%s", len(problems), strings.Join(problems, "\n"))
	}
}

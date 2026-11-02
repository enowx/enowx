package ui

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/notesview"
)

// The import dialog's steps past its first and the note agent's panel with no
// agent or a terminal, in the whole window, each saying what it should.
func TestNotesImportAndAgentStates(t *testing.T) {
	fakeKeychain(t)
	h := newNotesHarness(t)
	seedAudit(t, h)
	entries := []proto.NoteImportEntry{
		{Title: "Roadmap", Group: "Plans", Action: "new"},
		{Title: "A page with a title so long that it has to run inside its row of the list, whatever the width", Action: "new"},
		{Title: "Meeting notes", Action: proto.ImportOverwrite},
		{Title: "Old page", Action: proto.ImportSkip},
	}
	job := func(st proto.NoteImportStatus) func(a *App) {
		return func(a *App) {
			a.page = pageNotes
			a.openNoteImport(nil)
			a.noteImport.job = &st
		}
	}
	notion := func(a *App, f func(*noteImportForm)) {
		a.page = pageNotes
		a.openNoteImport(nil)
		a.noteImport.notionAPI = true
		if f != nil {
			f(a.noteImport)
		}
	}
	states := []struct {
		name  string
		open  func(a *App)
		wants []string
	}{
		{"import-preview", job(proto.NoteImportStatus{State: proto.ImportDone, DryRun: true, Preview: &proto.NoteImportPreview{From: proto.ImportNotion, Notes: entries, Assets: 2, Conflicts: []string{"Roadmap", "Meeting notes"}}}),
			[]string{"3 notes and 2 files to import, read as a Notion export.", "One was imported before and stays as it is.", "Titles there already, kept both: Roadmap, Meeting notes", "Import 3 notes"}},
		{"import-preview-none", job(proto.NoteImportStatus{State: proto.ImportDone, DryRun: true, Preview: &proto.NoteImportPreview{From: proto.ImportMarkdown, Notes: entries[3:]}}),
			[]string{"0 notes to import, read as Markdown.", "Import 0 notes"}},
		{"import-progress", job(proto.NoteImportStatus{State: proto.ImportRunning, Phase: "Writing", Done: 3, Total: 12}),
			[]string{"Writing 3 of 12", "Stop"}},
		{"import-report", job(proto.NoteImportStatus{State: proto.ImportDone, Report: &proto.NoteImportReport{Imported: 12, Overwritten: 2,
			Skipped:     []proto.NoteImportIssue{{Where: "Export/old.bin", What: "not a note"}},
			Unconverted: []proto.NoteImportIssue{{Where: "Roadmap", What: "an embed, kept as text"}, {Where: "Maths", What: "an equation, kept as text"}}}}),
			[]string{"2 imported before written again.", "Skipped", "Kept as text or left out", "Export/old.bin: not a note"}},
		{"notion-locked", func(a *App) {
			auditCredentials(t, a)
			a.vaultLock()
			notion(a, nil)
		}, []string{"Unlock the vault (Credentials) to use a token kept there, or to import again on a schedule."}},
		{"notion-keep", func(a *App) {
			auditCredentials(t, a)
			notion(a, func(f *noteImportForm) { f.keep = true })
		}, []string{"Keep the token in the vault", "Import again", "Every 6 hours"}},
		{"notion-syncs", func(a *App) {
			notion(a, nil)
			a.state.NotionSyncs = []notionSync{
				{ID: "s1", HostName: "this Mac", Hours: 24, Group: "Plans", Last: time.Now().Add(-2 * time.Hour)},
				{ID: "s2", HostName: "enowxlabs", Hours: 6, Err: "the integration lost its access to these pages"},
			}
		}, []string{"Imported again on their own"}},
	}
	for _, st := range states {
		t.Run(st.name, func(t *testing.T) {
			closeAudit(h.app)
			h.app.notes = notesview.View{}
			h.app.state.NotionSyncs = nil
			st.open(h.app)
			tt := ui.NewTester(func(c *ui.Context) { h.app.view(c) }, 1280, 800)
			h.tt = tt
			for range 6 {
				h.settle("", func() bool { return true })
			}
			tt.Advance(time.Second)
			tt.Frame()
			shows := strings.Join(tt.Texts(), "\n")
			for _, want := range st.wants {
				if !strings.Contains(shows, want) {
					t.Errorf("no %q in %q", want, tt.Texts())
				}
			}
			shot(t, tt, filepath.Join("notes", st.name+".png"))
			for _, pr := range layoutProblems(tt, 1280, 800, false) {
				t.Error(pr)
			}
		})
	}
	closeAudit(h.app)
	h.app.state.NotionSyncs = nil

	// The note's agent panel: no agent, then an agent with no chat, in a terminal.
	h.app.page = pageNotes
	h.app.notes = notesview.View{}
	h.app.agents = nil
	h.tt = ui.NewTester(func(c *ui.Context) { h.app.view(c) }, 1280, 800)
	h.settle("", func() bool { return true })
	h.app.notes.Select(h.app.notesOptions(), h.app.noteList.list[0].ID)
	h.settle("the note", func() bool { return h.app.notes.Opened() })
	h.app.notes.SetAssistant(true)
	want := "No agent is installed on " + h.app.host.Name + "."
	h.settle(want, func() bool { return h.tt.HasText(want) })
	h.tt.Advance(time.Second)
	h.tt.Frame()
	shot(t, h.tt, filepath.Join("notes", "agent-none.png"))
}

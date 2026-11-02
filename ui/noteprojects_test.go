package ui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
)

// A page attached from its menu to a project of the host shows its badge,
// and the tree grouped by project lists it there; a project gone from its
// host is marked so.
func TestNoteAttachedToAProject(t *testing.T) {
	h := newNotesHarness(t)
	dir := filepath.Join(t.TempDir(), "antares")
	os.MkdirAll(dir, 0o755)
	if _, err := h.cl.AddProject(dir); err != nil {
		t.Fatal(err)
	}
	info, _ := h.cl.Info()
	h.app.daemonInfo = info
	n, _ := h.cl.SaveNote(proto.Note{NoteInfo: proto.NoteInfo{Title: "Launch plan"}, Body: "Ship it.\n"})
	h.settle("the catalog", func() bool {
		cats, _ := h.cl.Catalogs()
		return len(cats) == 1 && len(cats[0].Projects) == 1
	})
	h.settle("the page", func() bool { return h.tt.HasText("Launch plan") })
	if err := h.tt.Click("Launch plan"); err != nil {
		t.Fatal(err)
	}
	h.settle("the page open", func() bool { return h.tt.HasText("Attach to a project") })
	if err := h.tt.Click("Attach to a project"); err != nil {
		t.Fatal(err)
	}
	h.settle("the dialog", func() bool { return h.tt.HasText("antares") && h.app.attaching != nil })
	if err := h.tt.Click("antares"); err != nil {
		t.Fatal(err)
	}
	if err := h.tt.Click("Done"); err != nil {
		t.Fatal(err)
	}
	h.settle("attached", func() bool {
		got, err := h.cl.Note(n.ID)
		return err == nil && len(got.Projects) == 1 && h.tt.HasText("antares · ")
	})
	h.tt.Advance(time.Second)
	shot(t, h.tt, "note-projects.png")

	h.app.notes.ShowProject(byProjectRef(h))
	h.settle("the filter", func() bool { return h.tt.HasText("Launch plan") })

	// Gone from its host: marked, offering to attach it again.
	got, _ := h.cl.Note(n.ID)
	h.cl.RemoveProject(dir)
	h.settle("broken", func() bool {
		h.app.catalogs.read = time.Time{}
		h.app.loadCatalogs()
		return h.app.projectState(got.Projects[0]) == "broken" && h.tt.HasText("broken")
	})
}

func byProjectRef(h *notesHarness) string {
	for _, n := range h.app.noteList.list {
		if len(n.Projects) > 0 {
			return n.Projects[0].Ref
		}
	}
	return ""
}

var _ = ui.Center
var _ = theme.Border

// The notes attached to a project fold into one row under its sessions,
// which unfolds into them.
func TestProjectNotesFoldIntoOneRow(t *testing.T) {
	h := newNotesHarness(t)
	dir := filepath.Join(t.TempDir(), "antares")
	os.MkdirAll(dir, 0o755)
	if _, err := h.cl.AddProject(dir); err != nil {
		t.Fatal(err)
	}
	info, _ := h.cl.Info()
	h.app.daemonInfo = info
	for _, title := range []string{"Launch plan", "Roadmap", "Bugs"} {
		n, _ := h.cl.SaveNote(proto.Note{NoteInfo: proto.NoteInfo{Title: title}})
		if _, err := h.cl.AttachNote(n.ID, []proto.NoteProject{{Name: "antares", Host: info.ID, Path: dir}}); err != nil {
			t.Fatal(err)
		}
	}
	p := proto.ProjectInfo{Name: "antares", Path: dir}
	h.tt = ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Width(260).Height(300).Background(theme.Background).Children(func() { h.app.projectNotesRow(c, p) })
	}, 260, 300)
	h.settle("the row", func() bool { return h.tt.HasText("3 notes") })
	if h.tt.HasText("Roadmap") {
		t.Fatalf("listed before unfolding: %q", h.tt.Texts())
	}
	if err := h.tt.Click("Notes of antares (3)"); err != nil {
		t.Fatal(err)
	}
	h.settle("unfolded", func() bool { return h.tt.HasText("Launch plan") && h.tt.HasText("Roadmap") && h.tt.HasText("Bugs") })
}

// Each project's notes row is its own: unfolding one leaves the others.
func TestProjectNotesRowsAreEachTheirOwn(t *testing.T) {
	h := newNotesHarness(t)
	info, _ := h.cl.Info()
	h.app.daemonInfo = info
	var ps []proto.ProjectInfo
	for _, name := range []string{"antares", "mastumbas"} {
		dir := filepath.Join(t.TempDir(), name)
		os.MkdirAll(dir, 0o755)
		if _, err := h.cl.AddProject(dir); err != nil {
			t.Fatal(err)
		}
		n, _ := h.cl.SaveNote(proto.Note{NoteInfo: proto.NoteInfo{Title: name + " plan"}})
		if _, err := h.cl.AttachNote(n.ID, []proto.NoteProject{{Name: name, Host: info.ID, Path: dir}}); err != nil {
			t.Fatal(err)
		}
		ps = append(ps, proto.ProjectInfo{Name: name, Path: dir})
	}
	h.tt = ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Width(260).Height(300).Background(theme.Background).Children(func() {
			for _, p := range ps {
				h.app.projectNotesRow(c, p)
			}
		})
	}, 260, 300)
	h.settle("the rows", func() bool { return h.tt.HasText("1 note") })
	if err := h.tt.Click("Notes of mastumbas (1)"); err != nil {
		t.Fatal(err)
	}
	h.settle("mastumbas unfolded", func() bool { return h.tt.HasText("mastumbas plan") })
	if h.tt.HasText("antares plan") {
		t.Fatalf("unfolding mastumbas unfolded antares too: %q", h.tt.Texts())
	}
}

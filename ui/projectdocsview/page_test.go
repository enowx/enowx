package projectdocsview

import (
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/projectdocs"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/notesview"
	"github.com/enowx/enowx/ui/theme"
)

// shot saves what tt shows when SHOTS names a folder, to look at.
func shot(t *testing.T, tt *ui.Tester, name string) {
	t.Helper()
	dir := os.Getenv("SHOTS")
	if dir == "" {
		return
	}
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	_ = png.Encode(f, tt.Image())
}

// realPage draws the real Notes page with the panel in its hook, as the app does.
type realPage struct {
	s     *store
	notes notesview.View
}

func (r *realPage) options() notesview.Options {
	o := r.s.options()
	return notesview.Options{
		Host: o.Host, Notes: o.Notes, Loaded: true, ListOutside: true,
		Load: o.Load, Save: o.Save, Delete: o.Delete,
		DocsPanel: func(c *ui.Context, p notesview.DocsPage) {
			r.s.view.Panel(c, r.panelOptions(), Page{Note: p.Note, Unsaved: p.Unsaved, Flush: p.Flush, Rewrite: p.Rewrite, Proposals: p.Proposals})
		},
	}
}

func (r *realPage) panelOptions() Options {
	o := r.s.options()
	o.Open = func(id string) { r.notes.Select(r.options(), id) }
	return o
}

func (r *realPage) tester(width, height int) *ui.Tester {
	r.s.view.now = func() time.Time { return r.s.now }
	return ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Width(float32(width)).Height(float32(height)).Background(theme.Background).Children(func() {
			r.notes.Draw(c, r.options())
			r.s.view.Dialogs(c, r.panelOptions())
		})
	}, width, height)
}

// In the real page, at every width, the summary and the strip stay inside the
// page, and what they hold inside them.
func TestPanelsFitInTheRealPage(t *testing.T) {
	for _, width := range testWidths {
		s := newStore()
		root := s.project(t, longTitle, projectdocs.SizeFull)
		spec := s.docNamed(t, "Technical Spec")
		s.retitle(spec, longTitle+" spec")
		s.setBody(spec, func(b string) string {
			first, rest, _ := strings.Cut(b, "\n")
			return first + "\n\nBased on: [[PRD#Features and priorities]] @00000000\n" + rest
		})
		r := &realPage{s: s}
		tt := r.tester(width, 2600)
		r.notes.Select(r.options(), root.ID)
		s.settle(t, tt, "the summary", func() bool { return tt.HasText("Blocked from building") })
		s.frames(tt, 3)
		inside(t, tt, "Project summary", width)
		shot(t, tt, "summary-"+itoaW(width)+".png")

		r.notes.Select(r.options(), spec)
		s.settle(t, tt, "the strip", func() bool { return tt.HasText("changed since this was based on it") })
		s.frames(tt, 3)
		inside(t, tt, "Document strip", width)
		apart(t, tt, "Status: Draft", "Approve")
		if a, ok := tt.Find("Open " + projectdocs.AreaArch); !ok || a.W < 40 {
			t.Errorf("width %d: long titles squeeze the area's crumb: %+v", width, a)
		}
		if tt.HasText(projectdocs.Marker) {
			t.Errorf("width %d: the first line shows on the page", width)
		}
		shot(t, tt, "strip-"+itoaW(width)+".png")
	}
}

func itoaW(w int) string {
	return map[int]string{320: "320", 560: "560", 1000: "1000"}[w]
}

// Through the real page: an edit made just before approving is saved and is
// what's approved; the page then shows the status, and nothing is overwritten.
func TestApproveOnTheRealPage(t *testing.T) {
	s := newStore()
	s.project(t, "Shop", projectdocs.SizeCore)
	prd := s.docNamed(t, "PRD")
	r := &realPage{s: s}
	tt := r.tester(1000, 1000)
	r.notes.Select(r.options(), prd)
	s.settle(t, tt, "the strip", func() bool { return tt.HasText(projectdocs.StatusDraft) })
	click(t, tt, "Approve")
	s.frames(tt, 8)
	shot(t, tt, "approve-confirm.png")
	click(t, tt, "Yes, approve")
	s.settle(t, tt, "approved", func() bool {
		m, _ := projectdocs.Parse(s.body(prd))
		return m.Status == projectdocs.StatusApproved
	})
	s.frames(tt, 3)
	body := s.body(prd)
	if projectdocs.ChangedSinceApproval(body) {
		t.Errorf("approved, then changed by saving:\n%s", body)
	}
	if _, ok := tt.Find("Status: " + projectdocs.StatusApproved); !ok {
		t.Errorf("the strip after approving: %q", tt.Texts())
	}
	if !tt.HasText("Reopen") {
		t.Errorf("no Reopen: %q", tt.Texts())
	}
	shot(t, tt, "approved.png")
}

// Sheets fit the window at every width.
func TestNewFormShot(t *testing.T) {
	for _, width := range testWidths {
		s := newStore()
		tn, _ := s.put(proto.Note{NoteInfo: proto.NoteInfo{Title: templatesNote}})
		s.put(proto.Note{NoteInfo: proto.NoteInfo{Title: longTitle, Parent: tn.ID}})
		s.view.OpenNew()
		tt := s.tester(width, 900)
		s.frames(tt, 8)
		shot(t, tt, "new-"+itoaW(width)+".png")
		if r, ok := tt.Find(longTitle); !ok || r.X+r.W > float32(width) {
			t.Errorf("width %d: a template's long title sticks out: %+v", width, r)
		}
	}
}

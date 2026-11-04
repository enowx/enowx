package projectdocsview

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/projectdocs"
)

var testWidths = []int{320, 560, 1000}

const slack = 0.51

// inside says every text the frame shows lies inside frame, horizontally: a row
// may only give way by running.
func inside(t *testing.T, tt *ui.Tester, label string, width int) ui.Rect {
	t.Helper()
	frame, ok := tt.Find(label)
	if !ok {
		t.Fatalf("no %q: %q", label, tt.Texts())
	}
	if frame.X < -slack || frame.X+frame.W > float32(width)+slack {
		t.Errorf("width %d: %s sticks out of the window: %+v", width, label, frame)
	}
	for _, text := range tt.Texts() {
		if strings.HasPrefix(text, "Section ") || text == "On this page" || strings.HasPrefix(text, "Outline to level") ||
			text == "1" || text == "2" || text == "3" || text == "All" {
			continue // the page's outline, beside it
		}
		r, ok := tt.Find(text)
		if !ok || r.Y < frame.Y-slack || r.Y > frame.Y+frame.H {
			continue // of something else on the window
		}
		if r.X < frame.X-slack || r.X+r.W > frame.X+frame.W+slack {
			t.Errorf("width %d: %q sticks out of %s: %+v in %+v", width, cut(text), label, r, frame)
		}
	}
	return frame
}

func cut(s string) string {
	if len(s) > 40 {
		return s[:40] + "…"
	}
	return s
}

func apart(t *testing.T, tt *ui.Tester, a, b string) {
	t.Helper()
	x, ok1 := tt.Find(a)
	y, ok2 := tt.Find(b)
	if !ok1 || !ok2 {
		t.Fatalf("no %q or %q: %q", cut(a), cut(b), tt.Texts())
	}
	if x.X < y.X+y.W-slack && y.X < x.X+x.W-slack && x.Y < y.Y+y.H-slack && y.Y < x.Y+x.H-slack {
		t.Errorf("%q overlaps %q: %+v %+v", cut(a), cut(b), x, y)
	}
}

// A project's summary fits at every width with very long titles, and a long
// title moves nothing: the frame and the rows are as with a short one.
func TestSummaryFitsAndTitlesChangeNothing(t *testing.T) {
	for _, width := range testWidths {
		s := newStore()
		root := s.project(t, "Shop", projectdocs.SizeCore)
		prd := s.docNamed(t, "PRD")
		s.open = root.ID
		tt := s.tester(width, 2400)
		s.settle(t, tt, "the summary", func() bool { return tt.HasText("Blocked from building") })
		s.frames(tt, 2)
		frame := inside(t, tt, "Project summary", width)
		row, ok := tt.Find("Open PRD")
		if !ok {
			t.Fatalf("no PRD row: %q", tt.Texts())
		}
		if row.H != rowHeight {
			t.Errorf("width %d: a row's height: %v", width, row.H)
		}

		s.retitle(prd, longTitle)
		s.now = s.now.Add(10 * time.Second)
		s.settle(t, tt, "the long title", func() bool { return tt.HasText(longTitle) })
		s.frames(tt, 2)
		after := inside(t, tt, "Project summary", width)
		if after.W != frame.W {
			t.Errorf("width %d: a long title changed the frame's width: %v to %v", width, frame.W, after.W)
		}
		// The gate names it too, and each is one row as before.
		if after.H != frame.H {
			t.Errorf("width %d: a long title changed the frame's height: %v to %v", width, frame.H, after.H)
		}
		long, ok := tt.Find("Open " + longTitle)
		if !ok || long != row {
			t.Errorf("width %d: a long title moved its row: %+v to %+v", width, row, long)
		}
		if width >= narrowWidth+100 {
			apart(t, tt, longTitle, projectdocs.StatusDraft)
		}

		// Under the pointer it runs, and nothing around it moves.
		tt.Move(long.X+long.W/2, long.Y+long.H/2)
		for range 12 {
			tt.Advance(300 * time.Millisecond)
			tt.Frame()
			if r, _ := tt.Find("Open " + longTitle); r != long {
				t.Fatalf("width %d: running moved its row: %+v to %+v", width, long, r)
			}
			if f, _ := tt.Find("Project summary"); f != after {
				t.Fatalf("width %d: running moved the frame: %+v to %+v", width, after, f)
			}
		}
	}
}

// A document's strip fits at every width, with long titles, links and warnings.
func TestStripFits(t *testing.T) {
	for _, width := range testWidths {
		s := newStore()
		s.project(t, "Shop", projectdocs.SizeFull)
		spec := s.docNamed(t, "Technical Spec")
		prd := s.docNamed(t, "PRD")
		s.setBody(spec, func(b string) string {
			first, rest, _ := strings.Cut(b, "\n")
			return first + "\n\nBased on: [[PRD#Features and priorities]] @00000000, [[" + longTitle + "]] @11111111\n" + rest
		})
		s.retitle(s.docNamed(t, projectdocs.AreaArch), longTitle+" area")
		s.retitle(prd, "PRD")
		s.open = spec
		tt := s.tester(width, 1400)
		s.settle(t, tt, "the strip", func() bool { return tt.HasText("changed since this was based on it") })
		s.frames(tt, 2)
		inside(t, tt, "Document strip", width)
		for _, label := range []string{"Status: Draft"} {
			r, ok := tt.Find(label)
			if !ok {
				t.Fatalf("no %q: %q", label, tt.Texts())
			}
			if r.H != statusHeight {
				t.Errorf("width %d: %s's height: %v", width, label, r.H)
			}
		}
		if r, _ := tt.Find("Status: Draft"); r.W != statusWidth {
			t.Errorf("width %d: the status's width: %v", width, r.W)
		}
		apart(t, tt, "Status: Draft", "Approve")
		for _, want := range []string{"Open Shop", "Open " + longTitle + " area", "Open Technical Spec"} {
			if _, ok := tt.Find(want); !ok {
				t.Errorf("width %d: the breadcrumb has no %q: %q", width, cut(want), tt.Texts())
			}
		}
		if _, ok := tt.Find("Open source"); !ok {
			t.Errorf("width %d: no Open source", width)
		}
		if _, ok := tt.Find("Mark reviewed"); !ok {
			t.Errorf("width %d: no Mark reviewed", width)
		}
	}
}

// The form keeps its frames: picking the bigger pack, a long title and the
// progress of making change none of them.
func TestNewFormKeepsItsFrames(t *testing.T) {
	for _, width := range testWidths {
		s := newStore()
		s.view.OpenNew()
		tt := s.tester(width, 900)
		s.frames(tt, 8)
		core, _ := tt.Find("Core pack frame")
		full, _ := tt.Find("Full pack frame")
		if core.W != full.W || core.H != full.H {
			t.Errorf("width %d: the cards differ: %+v %+v", width, core, full)
		}
		if core.X+core.W > full.X+slack && core.Y+core.H > full.Y+slack {
			t.Errorf("width %d: the cards overlap: %+v %+v", width, core, full)
		}
		for _, r := range []ui.Rect{core, full} {
			if r.X < -slack || r.X+r.W > float32(width)+slack {
				t.Errorf("width %d: a card sticks out of the window: %+v", width, r)
			}
		}
		for _, name := range []string{"12 documents", "Brief", "User Stories and Acceptance Criteria"} {
			if r, ok := tt.Find(name); !ok || r.X < core.X || r.X+r.W > core.X+core.W+slack {
				t.Errorf("width %d: %q isn't inside its card: %+v in %+v", width, name, r, core)
			}
		}
		if tt.HasText(projectdocs.Catalog[0].Name) == false {
			t.Errorf("width %d: no document listed: %q", width, tt.Texts())
		}
		progress, _ := tt.Find("Making progress")
		click(t, tt, "Full pack")
		s.frames(tt, 2)
		if s.view.form.size != projectdocs.SizeFull {
			t.Fatalf("width %d: Full pack wasn't picked", width)
		}
		s.view.form.err = longTitle
		s.frames(tt, 2)
		c2, _ := tt.Find("Core pack frame")
		p2, _ := tt.Find("Making progress")
		if c2 != core || p2 != progress {
			t.Errorf("width %d: a long error moved the form: %+v to %+v, %+v to %+v", width, core, c2, progress, p2)
		}
		inside(t, tt, "Making progress", width)
	}
}

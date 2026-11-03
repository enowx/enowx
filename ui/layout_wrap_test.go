package ui

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/widget"
	"github.com/enowx/enowx/ui/wrapcheck"
)

// outOfParents are the elements sticking out of the one that wraps them.
func outOfParents(tt *ui.Tester) []string { return wrapcheck.OutOfParents(tt) }

// The check sees a button sticking out of its row, and a row that wraps it.
func TestOutOfParentsFindsWhatSticksOut(t *testing.T) {
	out := ui.NewTester(func(c *ui.Context) {
		ui.Row(c).Width(100).Height(40).Children(func() {
			ui.Box(c).Width(200).Height(20).Shrink(0).Label("too wide")
		})
	}, 400, 100)
	out.Frame()
	if got := outOfParents(out); len(got) != 1 {
		t.Fatalf("found %q", got)
	}
	fits := ui.NewTester(func(c *ui.Context) {
		ui.Row(c).Width(100).Height(40).Children(func() {
			ui.Box(c).Width(80).Height(20).Shrink(0).Label("fits")
		})
	}, 400, 100)
	fits.Frame()
	if got := outOfParents(fits); len(got) != 0 {
		t.Fatalf("found %q", got)
	}
}

// The check sees a row under a scroller's bar, and one that keeps the gutter.
func TestScrollbarOverlapsFindsWhatLiesUnderTheBar(t *testing.T) {
	draw := func(scroll func(c *ui.Context) ui.Element) *ui.Tester {
		tt := ui.NewTester(func(c *ui.Context) {
			scroll(c).Width(200).Height(100).Children(func() {
				ui.Text(c, "a row").FillWidth()
			})
		}, 300, 200)
		tt.Frame()
		return tt
	}
	if out := wrapcheck.ScrollbarOverlaps(draw(ui.Scroll)); len(out) == 0 {
		t.Fatal("a row to the scroller's edge wasn't found")
	}
	if out := wrapcheck.ScrollbarOverlaps(draw(widget.Scroll)); len(out) != 0 {
		t.Fatalf("a row keeping the gutter: %q", out)
	}
}

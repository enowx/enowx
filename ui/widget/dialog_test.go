package widget

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/theme"
)

// A sheet's buttons stay inside it, however many there are: what doesn't
// fit goes to another line, its last button still at the right.
func TestSheetFooterStaysInside(t *testing.T) {
	open := true
	const window, width = 800, 320
	labels := []string{"Change the model", "Open the whole session", "Do something else"}
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(theme.Window())
		ui.Column(c).Fill().Children(func() {
			FormSheet(c, &open, width, "A sheet", "", func() { ui.Text(c, "Its body") }, func() {
				for _, l := range labels {
					Button(c.Key(l), Secondary, nil, l, "")
				}
			})
		})
	}, window, 600)
	for range 8 {
		tt.Advance(100 * time.Millisecond)
		tt.Frame()
	}
	left, right := float32(window-width)/2, float32(window+width)/2
	for _, l := range append(labels, "Cancel") {
		b, ok := tt.Find(l)
		if !ok {
			t.Fatalf("no %q: %q", l, tt.Texts())
		}
		if b.X < left || b.X+b.W > right {
			t.Errorf("%q spans %.0f to %.0f, outside %.0f to %.0f", l, b.X, b.X+b.W, left, right)
		}
	}
	first, _ := tt.Find("Cancel")
	last, _ := tt.Find(labels[2])
	if last.Y <= first.Y {
		t.Errorf("all on one line: Cancel at %.0f, the last at %.0f", first.Y, last.Y)
	}
	if last.X+last.W < right-60 {
		t.Errorf("the last button isn't at the right: it ends at %.0f of %.0f", last.X+last.W, right)
	}
}

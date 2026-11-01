package ui

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/theme"
)

// However narrow the window, the title bar's chips stay whole inside it:
// the search gives way.
func TestTitleBarChipsStayWhole(t *testing.T) {
	for _, width := range []int{1600, 1300, 1200} {
		a := New(Options{})
		a.winCtl = &fakeWindow{}
		var last ui.Rect
		tt := ui.NewTester(func(c *ui.Context) {
			ui.Column(c).Width(float32(width)).Height(60).Background(theme.Ground).Children(func() {
				a.titleBarWith(c, func() {
					for _, w := range []float32{120, 40, 40, 40} {
						last = ui.Box(c).Size(w, 32).Shrink(0).Background(theme.Done).Bounds()
					}
				})
			})
		}, width, 60)
		for range 4 {
			tt.Frame()
		}
		if right := last.X + last.W; right > float32(width) {
			t.Errorf("at %d wide, the last chip ends at %.0f", width, right)
		}
	}
}

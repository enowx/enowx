package widget

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// A frame keeps its width whatever the text, which is cut until it runs.
func TestRunningTextKeepsItsFrame(t *testing.T) {
	long := "a very long session name that could never fit in this little frame at all"
	var w float32
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Row(c).Width(120).Height(30).Children(func() {
			w = Running(c, long, RunStyle{Always: true}).LayoutBounds().W
		})
	}, 400, 100)
	for range 3 {
		tt.Frame()
	}
	if w > 120 || w <= 0 {
		t.Fatalf("the frame is %v wide", w)
	}
	if !tt.HasText(long) {
		t.Fatal("the text is gone")
	}
}

func TestRunAtRestsAtBothEnds(t *testing.T) {
	for _, tc := range []struct{ p, want float32 }{{0, 0}, {0.05, 0}, {0.25, 0.5}, {0.5, 1}, {0.75, 0.5}, {1, 0}} {
		if got := runAt(tc.p, 0.2); got < tc.want-0.01 || got > tc.want+0.01 {
			t.Errorf("runAt(%v) = %v, want %v", tc.p, got, tc.want)
		}
	}
}

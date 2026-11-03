package diffview

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
)

// A changed line of a megabyte, side by side, costs what a short one does.
func TestLongDiffLineStaysQuick(t *testing.T) {
	lines := []proto.DiffLine{{Text: strings.Repeat("y", 1<<20), New: 1, Old: 1}}
	tt := ui.NewTester(func(c *ui.Context) {
		c.After(time.Millisecond)
		ui.Row(c).Width(1000).Height(40).Children(func() { pairLine(c, lines, 0, true) })
	}, 1000, 100)
	tt.Image()
	began := time.Now()
	const builds = 16
	tt.Advance(builds * time.Millisecond)
	tt.Image()
	if took := time.Since(began) / builds; took > 16*time.Millisecond {
		t.Errorf("%v a frame", took)
	}
}

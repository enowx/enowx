package androidview

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
)

// A log line of a megabyte costs a frame what a short one does.
func TestLongLogLineStaysQuick(t *testing.T) {
	line := proto.AndroidLogLine{Level: "I", Tag: "app", Message: strings.Repeat("y", 1<<20)}
	tt := ui.NewTester(func(c *ui.Context) {
		c.After(time.Millisecond)
		ui.Column(c).Width(1000).Children(func() { logLine(c, line) })
	}, 1000, 700)
	tt.Image()
	began := time.Now()
	const builds = 16
	tt.Advance(builds * time.Millisecond)
	tt.Image()
	if took := time.Since(began) / builds; took > 16*time.Millisecond {
		t.Errorf("%v a frame", took)
	}
}

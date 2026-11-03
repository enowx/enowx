package checkview

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/framecheck"
)

// A check's output of ten megabytes, or a line of one, costs a frame what a short one does,
// the judge's word on it included.
func TestLongOutputStaysQuick(t *testing.T) {
	for name, output := range map[string]string{"10 MB of lines": framecheck.Lines(), "a 1 MB line": framecheck.Line()} {
		v := New()
		v.loaded = true
		v.info = proto.ChecksInfo{
			Checks: []proto.Check{{Name: "test", Command: "make test"}},
			Runs: []proto.CheckRun{{Name: "test", State: proto.CheckFailed, ExitCode: 1, Output: output,
				Judged: proto.VerdictFlaky, JudgedWhy: framecheck.Line()}},
		}
		queue := make(chan func(), 16)
		o := Options{Update: func(fn func()) { queue <- fn }}
		tt := framecheck.New(t, name, func(c *ui.Context) {
			c.After(time.Millisecond)
			ui.Column(c).Width(1000).Height(700).Children(func() { v.DrawOutput(c, o, "test") })
		}, 1000, 700)
		tt.Image()
		select {
		case fn := <-queue:
			fn()
		case <-time.After(30 * time.Second):
			t.Fatalf("%s: never prepared", name)
		}
		framecheck.Quick(t, name, tt)
	}
}

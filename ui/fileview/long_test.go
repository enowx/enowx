package fileview

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
)

// A file of ten megabytes, and one with a line of a megabyte, read as any other.
func TestLongFilesStayQuick(t *testing.T) {
	for name, text := range map[string]string{
		"10 MB of lines": strings.Repeat(strings.Repeat("x", 79)+"\n", 128<<10),
		"a 1 MB line":    strings.Repeat("y", 1<<20),
	} {
		queue := make(chan func(), 16)
		d := NewDoc("big.txt")
		d.content, d.lines = &proto.FileContent{Path: "big.txt", Text: text}, splitLines(text)
		o := Options{Update: func(fn func()) { queue <- fn }}
		tt := ui.NewTester(func(c *ui.Context) {
			c.After(time.Millisecond)
			ui.Column(c).Width(1000).Height(700).Children(func() { d.text(c, o) })
		}, 1000, 700)
		tt.Image()
		select {
		case fn := <-queue:
			fn()
		case <-time.After(10 * time.Second):
			t.Fatalf("%s: never prepared", name)
		}
		began := time.Now()
		const builds = 16
		tt.Advance(builds * time.Millisecond)
		tt.Image()
		took := time.Since(began) / builds
		t.Logf("%s: %v a frame", name, took)
		if took > 16*time.Millisecond {
			t.Errorf("%s: %v a frame", name, took)
		}
	}
}

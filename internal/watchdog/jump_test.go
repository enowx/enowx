package watchdog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMemoryJumpIsMarkedWithWhatWasHappening(t *testing.T) {
	for _, c := range []struct {
		before, now uint64
		want        bool
	}{
		{0, 2 << 30, false},           // the first look has nothing to compare
		{500 << 20, 900 << 20, true},  // grew much, by much
		{500 << 20, 600 << 20, false}, // not much
		{2 << 30, 2300 << 20, false},  // much, but not by much
		{2 << 30, 1 << 30, false},     // less
	} {
		if got := heapJumped(c.before, c.now); got != c.want {
			t.Errorf("%d to %d: %v", c.before, c.now, got)
		}
	}
	dir := t.TempDir()
	w := &Watch{dir: dir, name: "app", stopCh: make(chan struct{})}
	w.shown.Store("page=canvas")
	w.SetDoing(func() string { return "page.change,canvas.load" })
	w.line(w.jumpMark(500<<20, 900<<20))
	data, err := os.ReadFile(filepath.Join(dir, "app.log"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"mark memory jump", "heap=900MB", "from=500MB", "page.change,canvas.load", "page=canvas"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("no %q in %s", want, data)
		}
	}
}

// Package framecheck is for tests: the worst of long content, and whether drawing
// it costs a frame more than the eye notices. Nothing but tests may import it.
package framecheck

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

const (
	// budget is the most a frame may take, drawn again and again.
	budget = 16 * time.Millisecond
	// firstMost is the most the first frame may take, what's done once for the text
	// included. A text laid out whole takes far longer: New stops there, instead of
	// drawing such frames until the tests run out of time.
	firstMost = 5 * time.Second
)

// Lines is ten megabytes of lines.
var Lines = sync.OnceValue(func() string { return strings.Repeat(strings.Repeat("x", 79)+"\n", 128<<10) })

// Line is one line of a megabyte.
var Line = sync.OnceValue(func() string { return strings.Repeat("y", 1<<20) })

// New is a Tester of view, which draws its first frame: t stops when that takes too long.
func New(t *testing.T, name string, view func(c *ui.Context), width, height int) *ui.Tester {
	t.Helper()
	began := time.Now()
	tt := ui.NewTester(view, width, height)
	if first := time.Since(began); first > firstMost {
		t.Fatalf("%s: the first frame took %v", name, first)
	}
	return tt
}

// Quick fails t when what tt shows costs too much to draw. tt's view asks for its next
// frame itself (c.After(time.Millisecond)), so that nothing is skipped.
func Quick(t *testing.T, name string, tt *ui.Tester) {
	t.Helper()
	began := time.Now()
	const builds = 16
	tt.Advance(builds * time.Millisecond)
	tt.Image()
	took := time.Since(began) / builds
	t.Logf("%s: %v a frame", name, took)
	if took > budget {
		t.Errorf("%s: %v a frame", name, took)
	}
}

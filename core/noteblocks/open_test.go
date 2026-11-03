package noteblocks

import (
	"slices"
	"testing"
)

// Open says which of a task's to-dos aren't ticked, one not there included.
func TestOpenItems(t *testing.T) {
	md := "- [x] Build the thing\n- [ ] Write the docs\n"
	got := Open(md, []string{"Build the thing", "Write the docs", "Not a to-do here"})
	if !slices.Equal(got, []string{"Write the docs", "Not a to-do here"}) {
		t.Fatalf("%q", got)
	}
}

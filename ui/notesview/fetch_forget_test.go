package notesview

import (
	"testing"
	"time"
)

// Past the limit the links least recently asked for go, none still on its way.
func TestForgetRemotes(t *testing.T) {
	now := time.Now()
	m := map[string]*remote{
		"old":    {done: true, used: now.Add(-time.Hour)},
		"coming": {used: now.Add(-2 * time.Hour)},
		"new":    {done: true, used: now},
	}
	forgetRemotes(m, 2)
	if m["old"] != nil || m["coming"] == nil || m["new"] == nil {
		t.Fatalf("left: %v", m)
	}
}

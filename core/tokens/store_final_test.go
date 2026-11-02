package tokens

import (
	"testing"
	"time"

	"github.com/enowx/enowx/proto"
)

// A call written over several records keeps each field's largest, whichever record
// says it, over a reopen too; records that say no more change nothing.
func TestStoreKeepsEachCallsFinalCounts(t *testing.T) {
	dir := t.TempDir()
	at := time.Now().Add(-time.Hour)
	s := Open(dir)
	first := Call{ID: "claude:m1", Time: at, Input: 3, CacheRead: 100, CacheWrite: 10, Output: 1}
	if got, err := s.Add([]Call{first}); err != nil || len(got) != 1 {
		t.Fatalf("%v %v", got, err)
	}
	grown := first
	grown.Output = 500
	if got, _ := s.Add([]Call{grown, first}); len(got) != 1 || got[0].Output != 500 {
		t.Fatalf("grown: %+v", got)
	}
	smaller := first
	smaller.Output, smaller.CacheRead = 2, 120
	if got, _ := s.Add([]Call{smaller}); len(got) != 1 || got[0].Output != 500 || got[0].CacheRead != 120 {
		t.Fatalf("each field apart: %+v", got)
	}
	if got, _ := s.Add([]Call{grown}); len(got) != 0 {
		t.Fatalf("nothing new: %+v", got)
	}
	again := Open(dir).Calls(at.Add(-time.Minute), at.Add(time.Minute))
	if len(again) != 1 || again[0].Output != 500 || again[0].CacheRead != 120 {
		t.Fatalf("reopened: %+v", again)
	}
}

// Tokens are input, output and cache write: cache read is beside them.
func TestTotalLeavesOutCacheReads(t *testing.T) {
	c := proto.TokenCounts{Input: 1, Output: 2, CacheWrite: 4, CacheRead: 1000}
	if c.Total() != 7 {
		t.Fatalf("%d", c.Total())
	}
}

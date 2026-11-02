package ui

import (
	"slices"
	"testing"

	"github.com/enowx/enowx/proto"
)

// GOA's list: what is at work on top, then what finished, the latest first.
func TestGOAOrder(t *testing.T) {
	list := []proto.SessionInfo{
		{ID: "old", State: proto.StateDone, FinishedAt: 100},
		{ID: "never"},
		{ID: "ended", Exited: true, ExitCode: 1, FinishedAt: 300},
		{ID: "working", State: proto.StateWorking, FinishedAt: 50},
		{ID: "just", State: proto.StateDone, FinishedAt: 400},
		{ID: "asks", State: proto.StateWaiting},
		{ID: "queued", Queued: true},
	}
	slices.SortStableFunc(list, goaCompare)
	want := []string{"asks", "working", "queued", "just", "ended", "old", "never"}
	for i, s := range list {
		if s.ID != want[i] {
			t.Fatalf("at %d: %s, want %v", i, s.ID, want)
		}
	}
}

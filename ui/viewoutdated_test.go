package ui

import (
	"testing"

	"github.com/enowx/enowx/proto"
)

// A view goes out of date when its session's program starts again, even if the stop
// between was never seen; never for what is still stopped, or the same run.
func TestViewOutdated(t *testing.T) {
	for name, c := range map[string]struct {
		old, now proto.SessionInfo
		want     bool
	}{
		"the same run":          {proto.SessionInfo{Run: 1}, proto.SessionInfo{Run: 1}, false},
		"woken, stop unseen":    {proto.SessionInfo{Run: 1}, proto.SessionInfo{Run: 2}, true},
		"woken, stop seen":      {proto.SessionInfo{Run: 1, Exited: true}, proto.SessionInfo{Run: 2}, true},
		"hibernating":           {proto.SessionInfo{Run: 1}, proto.SessionInfo{Run: 2, Exited: true}, false},
		"an older daemon, woke": {proto.SessionInfo{Exited: true}, proto.SessionInfo{}, true},
		"an older daemon, same": {proto.SessionInfo{}, proto.SessionInfo{}, false},
		"a queued one starting": {proto.SessionInfo{Queued: true}, proto.SessionInfo{}, true},
	} {
		if got := viewOutdated(c.old, c.now); got != c.want {
			t.Errorf("%s: %v", name, got)
		}
	}
}

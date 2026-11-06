package cli

import (
	"strings"
	"testing"
)

// GOA settles a task; it doesn't set one at work by hand (issue #45).
func TestGOATaskTakesOnlySettlingStatuses(t *testing.T) {
	for _, status := range []string{"working", "waiting", "queued", "turn-ended"} {
		err := runner{}.goaTask([]string{"t1", "--status", status})
		if err == nil || !strings.Contains(err.Error(), "done, failed or stopped") {
			t.Errorf("%s: %v", status, err)
		}
	}
}

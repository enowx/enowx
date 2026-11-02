package daemon

import (
	"testing"
	"time"

	"github.com/enowx/enowx/proto"
)

// A finished turn on a screen no hook speaks for is a report, not a stall; a
// session only quiet stays one.
func TestTurnOverOnScreen(t *testing.T) {
	const over = "The report is in.\n✻ Baked for 27s\n"
	still := 30 * time.Second
	for name, c := range map[string]struct {
		look   stallLook
		screen string
		want   bool
	}{
		"in a shell tab":   {stallLook{Still: still}, over, true},
		"its hooks quiet":  {stallLook{Info: proto.SessionInfo{StateStale: true}, Still: still}, over, true},
		"its hooks at it":  {stallLook{Info: proto.SessionInfo{State: proto.StateWorking}, Still: still}, over, false},
		"too soon":         {stallLook{Still: time.Second}, over, false},
		"something runs":   {stallLook{Still: still, CPU: 80}, over, false},
		"only quiet":       {stallLook{Still: still}, "compiling the thing\n", false},
		"cannot see a CPU": {stallLook{Still: still, NoCPU: true, CPU: 80}, over, true},
	} {
		if got := turnOverOnScreen(c.look, c.screen); got != c.want {
			t.Errorf("%s: %v", name, got)
		}
	}
}

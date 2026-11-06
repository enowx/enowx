package hostconn

import (
	"testing"

	"github.com/enowx/enowx/proto"
)

// A newer app replaces an older daemon, and another build of its own version; an older
// app leaves a newer daemon alone, so two versions left running can't take the daemon
// from each other in turn (issue #49).
func TestDaemonFitsTheApp(t *testing.T) {
	for _, c := range []struct {
		name          string
		daemon, build string
		want          int
	}{
		{"same", "0.7.1", "b1", daemonSame},
		{"older daemon, newer app", "0.7.0", "b0", daemonOther},
		{"another build of this version", "0.7.1", "b2", daemonOther},
		{"newer daemon, older app", "0.7.2", "b3", daemonNewer},
		{"a daemon that says no version", "", "", daemonOther},
	} {
		if got := daemonFits(proto.DaemonInfo{Version: c.daemon, Build: c.build}, "0.7.1", "b1"); got != c.want {
			t.Errorf("%s: %d, want %d", c.name, got, c.want)
		}
	}
	// The other side of the same pair: the 0.7.0 app leaves the 0.7.1 daemon.
	if got := daemonFits(proto.DaemonInfo{Version: "0.7.1", Build: "b1"}, "0.7.0", "b0"); got != daemonNewer {
		t.Errorf("an older app took a newer daemon: %d", got)
	}
}

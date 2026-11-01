package daemon

import (
	"strings"
	"testing"
	"time"

	"github.com/enowx/enowx/proto"
)

// A task not moving is named by how, and long work isn't taken for it.
func TestStallCases(t *testing.T) {
	working := proto.SessionInfo{State: proto.StateWorking}
	for _, c := range []struct {
		name string
		look stallLook
		want string
	}{
		{"blank long after", stallLook{Blank: true, Age: 2 * time.Minute, Info: working}, "never started"},
		{"blank at first", stallLook{Blank: true, Age: 10 * time.Second, Info: working}, ""},
		{"a long build", stallLook{Info: working, Still: time.Hour, CPU: 180}, ""},
		{"a slow reply, screen moving", stallLook{Info: working, Still: time.Minute}, ""},
		{"frozen", stallLook{Info: working, Still: 11 * time.Minute}, "stuck"},
		{"hibernated", stallLook{Info: proto.SessionInfo{Exited: true, Hibernated: true}}, "idle"},
		{"no state, nothing runs", stallLook{Still: 3 * time.Minute}, "idle"},
		{"no state, something runs", stallLook{Still: 3 * time.Minute, CPU: 50}, ""},
		{"ended", stallLook{Info: proto.SessionInfo{Exited: true}}, ""},
		{"its turn ended", stallLook{Info: proto.SessionInfo{State: proto.StateDone}, Still: time.Hour}, ""},
	} {
		got, _ := stallCase(c.look)
		if c.want == "" && got != "" || c.want != "" && !strings.HasPrefix(got, c.want) {
			t.Errorf("%s: %q", c.name, got)
		}
	}
}

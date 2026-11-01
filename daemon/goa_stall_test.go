package daemon

import (
	"strings"
	"testing"
	"time"

	"github.com/enowx/enowx/core/goa"
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
		got, _ := stallCase(c.look, goa.StallOr(proto.GOAStall{}))
		if c.want == "" && got != "" || c.want != "" && !strings.HasPrefix(got, c.want) {
			t.Errorf("%s: %q", c.name, got)
		}
	}
}

// The limits are the host's, kept over a restart, each back to its default at
// 0; what can't be a limit is refused and changes nothing.
func TestGOAStallLimits(t *testing.T) {
	dir := t.TempDir()
	srv, err := NewServer(dir)
	if err != nil {
		t.Fatal(err)
	}
	c := connect(t, srv, nil)
	var got proto.GOAStall
	if err := c.Call(proto.MethodGOAStall, nil, &got); err != nil || got != goa.StallOr(proto.GOAStall{}) {
		t.Fatalf("the defaults: %+v %v", got, err)
	}
	if err := c.Call(proto.MethodGOAStallSave, proto.GOAStall{Blank: 30, Idle: 300}, &got); err != nil {
		t.Fatal(err)
	}
	want := proto.GOAStall{Blank: 30, Idle: 300, Stuck: goa.StallStuck}
	if got != want {
		t.Fatalf("saved: %+v", got)
	}
	if err := c.Call(proto.MethodGOAStallSave, proto.GOAStall{Blank: 2 * 24 * 60 * 60}, nil); err == nil {
		t.Fatal("two days taken")
	}
	srv.Close()
	again, err := NewServer(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	c = connect(t, again, nil)
	if err := c.Call(proto.MethodGOAStall, nil, &got); err != nil || got != want {
		t.Fatalf("after a restart: %+v %v", got, err)
	}
	if err := c.Call(proto.MethodGOAStallSave, proto.GOAStall{Idle: 300}, &got); err != nil || got.Blank != goa.StallBlank {
		t.Fatalf("back to its default: %+v %v", got, err)
	}
}

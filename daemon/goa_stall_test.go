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
		{"blank, something runs", stallLook{Blank: true, Age: time.Hour, CPU: 80}, ""},
		{"a long build", stallLook{Info: working, Still: time.Hour, CPU: 180}, ""},
		{"a slow reply, screen moving", stallLook{Info: working, Still: time.Minute}, ""},
		{"frozen", stallLook{Info: working, Still: 11 * time.Minute}, "stuck"},
		{"frozen, its CPU unknown", stallLook{Info: working, Still: 11 * time.Minute, CPU: 50, NoCPU: true}, "stuck"},
		{"frozen, not yet", stallLook{Info: proto.SessionInfo{StateStale: true}, Still: 3 * time.Minute}, ""},
		{"frozen, so long its state went stale", stallLook{Info: proto.SessionInfo{StateStale: true}, Still: 11 * time.Minute}, "stuck"},
		{"waiting for its turn to start", stallLook{Info: proto.SessionInfo{Queued: true}, Blank: true, Age: time.Hour}, ""},
		{"hibernated", stallLook{Info: proto.SessionInfo{Exited: true, Hibernated: true}, Still: 3 * time.Minute}, "idle"},
		{"hibernated just now", stallLook{Info: proto.SessionInfo{Exited: true, Hibernated: true}}, ""},
		{"gone", stallLook{Gone: true, Still: 3 * time.Minute}, "idle"},
		{"no state, nothing runs", stallLook{Still: 3 * time.Minute}, "idle"},
		{"no state, something runs", stallLook{Still: 3 * time.Minute, CPU: 50}, ""},
		{"ended", stallLook{Info: proto.SessionInfo{Exited: true}}, ""},
		{"its turn ended before the task", stallLook{Info: proto.SessionInfo{State: proto.StateDone}, Still: time.Hour}, "idle"},
		{"it asks", stallLook{Info: proto.SessionInfo{State: proto.StateWaiting}, Still: time.Hour}, ""},
	} {
		got, _ := stallCase(c.look, goa.StallOr(proto.GOAStall{}))
		if c.want == "" && got != "" || c.want != "" && !strings.HasPrefix(got, c.want) {
			t.Errorf("%s: %q", c.name, got)
		}
	}
}

// A queued task goes in when its session's prompt is really free, its turn's
// end unheard; not while it works, asks, runs something, or may be a shell.
func TestQueuedCanGo(t *testing.T) {
	stale := proto.SessionInfo{StateStale: true}
	const free = "  ⎿  Interrupted · What should Claude do instead?\n\n❯ \n  ⏵⏵ bypass permissions on\n"
	for _, c := range []struct {
		name   string
		look   stallLook
		screen string
		want   bool
	}{
		{"after an interrupt and /model", stallLook{Info: stale, Still: time.Minute}, free, true},
		{"still too shortly", stallLook{Info: stale, Still: time.Second}, free, false},
		{"something runs", stallLook{Info: stale, Still: time.Minute, CPU: 50}, free, false},
		{"it says it works", stallLook{Info: stale, Still: time.Minute}, "✶ Thinking… (esc to interrupt)\n", false},
		{"it asks", stallLook{Info: stale, Still: time.Minute}, "Do you want to proceed?\n ❯ 1. Yes\n   2. No\n", false},
		{"a blank screen", stallLook{Info: stale, Still: time.Minute}, " \n", false},
		{"no state: maybe a shell", stallLook{Still: time.Minute}, free, false},
		{"really working", stallLook{Info: proto.SessionInfo{State: proto.StateWorking}, Still: time.Minute}, free, false},
		{"its own input queued", stallLook{Info: proto.SessionInfo{StateStale: true, Queued: true}, Still: time.Minute}, free, false},
		{"ended", stallLook{Info: proto.SessionInfo{StateStale: true, Exited: true}, Still: time.Minute}, free, false},
	} {
		if got := queuedCanGo(c.look, c.screen); got != c.want {
			t.Errorf("%s: %v", c.name, got)
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

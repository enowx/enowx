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

// A task carrying its note's to-dos shows them to the worker, and settled done
// says which aren't ticked.
func TestGOATaskCarriesItsToDos(t *testing.T) {
	p := proto.GOADelegateParams{Note: "n1", Items: []string{"Build it", "Write docs"}}
	lines := goaToDoLines(p)
	if !strings.Contains(lines, "notes check n1") || !strings.Contains(lines, "- Write docs") {
		t.Fatalf("%q", lines)
	}
	if goaToDoLines(proto.GOADelegateParams{}) != "" {
		t.Fatal("lines with no note")
	}
	srv := startServer(t)
	n, err := srv.notesFor("").Save(proto.Note{NoteInfo: proto.NoteInfo{Title: "Plan"}, Body: "- [x] Build it\n- [ ] Write docs\n"})
	if err != nil {
		t.Fatal(err)
	}
	open := srv.goaOpenItems(proto.GOATask{Note: n.ID, Items: p.Items})
	if len(open) != 1 || open[0] != "Write docs" {
		t.Fatalf("open: %q", open)
	}
	if got := srv.goaOpenItems(proto.GOATask{Note: "gone", Items: p.Items}); len(got) != 2 {
		t.Fatalf("a note gone: %q", got)
	}
}

// Who ticked what is kept per note; a turn with a commit reminds once.
func TestTicksAreRecorded(t *testing.T) {
	srv := startServer(t)
	for _, tick := range []proto.NoteTick{{Note: "n1", Item: "one", Session: "gone"}, {Note: "n2", Item: "other"}} {
		if err := srv.recordTick(tick); err != nil {
			t.Fatal(err)
		}
	}
	got := srv.ticksOf("n1")
	if len(got) != 1 || got[0].Item != "one" || got[0].Session != "gone" || got[0].Time.IsZero() {
		t.Fatalf("%+v", got)
	}
	if none := srv.ticksOf("n3"); len(none) != 0 {
		t.Fatalf("%+v", none)
	}
	if notes := srv.notesWithOpenToDos(""); notes != nil {
		t.Fatalf("no project: %q", notes)
	}
}

// A further line comes after three times the limit, and not before.
func TestStallTellsAgainAfterThreeLimits(t *testing.T) {
	srv := startServer(t)
	task, err := srv.goa.board.Add(proto.GOATask{Title: "quiet", Session: "s1", Status: proto.GOATaskWorking, Created: time.Now().Add(-time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	lim := proto.GOAStall{Blank: 100, Idle: 100, Stuck: 100}
	limit := 100 * time.Second
	screen := "something\n"
	h := stallHost{infos: map[string]proto.SessionInfo{"s1": {ID: "s1", Agent: "some-agent"}}, cpuKnown: true,
		cpuOf:  func(string) float64 { return 0 },
		screen: func(string) (string, error) { return screen, nil }}
	f := &stallFollow{screen: screen, since: time.Now().Add(-time.Hour)}
	w := &stallWatch{follows: map[string]*stallFollow{task.ID: f}, sampled: map[string]bool{}}
	srv.stallCheck(w, task, h, lim)
	if f.told.IsZero() {
		t.Fatal("a stalled task's first line never came")
	}
	task.Status = proto.GOATaskStalled
	early := time.Now().Add(-3*limit + time.Minute)
	f.told = early
	srv.stallCheck(w, task, h, lim)
	if !f.told.Equal(early) {
		t.Fatal("a further line came before three times the limit")
	}
	late := time.Now().Add(-3*limit - time.Minute)
	f.told = late
	srv.stallCheck(w, task, h, lim)
	if f.told.Equal(late) {
		t.Fatal("no further line after three times the limit")
	}
}

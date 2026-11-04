package daemon

import (
	"strings"
	"testing"
	"time"

	"github.com/enowx/enowx/proto"
)

// A stall waits in Tasks without a hook, reaches the next hook once, and can
// remind again after three limits without ever entering the user's draft.
func TestGOAStallReportsUseHooksAndTasks(t *testing.T) {
	srv := startServer(t)
	c := connect(t, srv, nil)
	g := goaComposerFixture(t, srv, "› unfinished draft\r\n")
	before := g.Terminal().Text()
	task, err := srv.goa.board.Add(proto.GOATask{Title: "quiet", Session: "worker", Status: proto.GOATaskWorking,
		Created: time.Now().Add(-time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	const screen = "compiling the thing\n"
	h := stallHost{infos: map[string]proto.SessionInfo{"worker": {ID: "worker", State: proto.StateWorking}}, cpuKnown: true,
		cpuOf: func(string) float64 { return 0 }, screen: func(string) (string, error) { return screen, nil }}
	f := &stallFollow{screen: screen, since: time.Now().Add(-time.Hour)}
	w := &stallWatch{follows: map[string]*stallFollow{task.ID: f}}
	lim := proto.GOAStall{Blank: 100, Idle: 100, Stuck: 100}
	srv.stallCheck(w, task, h, lim)
	lines := stallWaitingOf(t, c, task.ID)
	if len(lines) != 1 || !strings.Contains(lines[0], "stuck:") || stallStatus(t, c, task.ID) != proto.GOATaskStalled {
		t.Fatalf("stall missing from Tasks: %q", lines)
	}
	take := func(event string) {
		t.Helper()
		var d proto.HookDecision
		if err := c.Call(proto.MethodSessionHook, proto.SessionHookParams{ID: g.ID(), Event: proto.HookEvent{Event: event}}, &d); err != nil {
			t.Fatal(err)
		}
		if strings.Count(d.Context, "[enowX GOA] Task "+task.ID) != 1 || !strings.Contains(d.Context, screen[:len(screen)-1]) {
			t.Fatalf("hook did not receive the stall once: %+v", d)
		}
		if got := stallWaitingOf(t, c, task.ID); len(got) != 0 {
			t.Fatalf("hook left delivered lines in Tasks: %q", got)
		}
		var next proto.HookDecision
		if err := c.Call(proto.MethodSessionHook, proto.SessionHookParams{ID: g.ID(), Event: proto.HookEvent{Event: event}}, &next); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(next.Context, "[enowX GOA] Task "+task.ID) {
			t.Fatalf("hook received the stall twice: %+v", next)
		}
	}
	take(proto.HookPromptStart)
	task.Status = proto.GOATaskStalled
	f.told = time.Now().Add(-240 * time.Second)
	srv.stallCheck(w, task, h, lim)
	if got := stallWaitingOf(t, c, task.ID); len(got) != 0 {
		t.Fatalf("reminded before three limits: %q", got)
	}
	f.told = time.Now().Add(-360 * time.Second)
	srv.stallCheck(w, task, h, lim)
	if got := stallWaitingOf(t, c, task.ID); len(got) != 1 {
		t.Fatalf("no reminder after three limits: %q", got)
	}
	take(proto.HookToolAfter)
	if got := g.Terminal().Text(); got != before || g.Info().Prompt != "" {
		t.Fatalf("stall delivery changed the draft: %q", got)
	}
}

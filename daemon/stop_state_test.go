package daemon

import (
	"testing"

	"github.com/enowx/enowx/core/session"
	"github.com/enowx/enowx/proto"
)

// A Stop with tasks running behind it isn't the turn's end, nor is one whose hooks give
// the agent more to do; a plain one is.
func TestDoneOnlyWhenNothingGoesOn(t *testing.T) {
	srv := startServer(t)
	sess, err := srv.sessions.Create(session.Spec{Command: []string{"sh", "-c", "cat"}, Dir: t.TempDir(), Cols: 80, Rows: 24, Meta: session.Meta{Agent: "claude"}})
	if err != nil {
		t.Fatal(err)
	}
	stop := func(background int) {
		t.Helper()
		ev := proto.HookEvent{Event: proto.HookTurnDone, Background: background}
		if _, err := srv.sessionHook(proto.SessionHookParams{ID: sess.ID(), State: proto.StateDone, Event: ev}); err != nil {
			t.Fatal(err)
		}
	}
	stop(2)
	if got := sess.Info().State; got != proto.StateWorking {
		t.Fatalf("with tasks running: %q", got)
	}
	stop(0)
	if got := sess.Info().State; got != proto.StateDone {
		t.Fatalf("at its end: %q", got)
	}
	if !goesOn(proto.HookEvent{}, proto.HookDecision{Continue: "more"}) {
		t.Error("a hook's more isn't going on")
	}
}

// What Claude Code's Stop says of its background tasks is read.
func TestStopReadsBackgroundTasks(t *testing.T) {
	for in, want := range map[string]int{
		`{"background_tasks":[{"id":"a"},{"id":"b"}]}`: 2,
		`{"background_tasks":[]}`:                      0,
		`{}`:                                           0,
	} {
		if got := agentEvent(proto.HookTurnDone, []byte(in)).Background; got != want {
			t.Errorf("%s: %d", in, got)
		}
	}
}

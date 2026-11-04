package daemon

import (
	"testing"

	"github.com/enowx/enowx/proto"
)

// A turn that ends with Claude Code's background tasks still running isn't the session done;
// its turn with none left is.
func TestTurnWithBackgroundTasksKeepsWorking(t *testing.T) {
	srv := startServer(t)
	c := connect(t, srv, nil)
	created, err := c.CreateSession(proto.SessionCreateParams{Command: []string{"cat"}, Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	turn := func(payload string) string {
		t.Helper()
		ev := agentEvent(proto.HookTurnDone, []byte(payload))
		if err := c.Call(proto.MethodSessionHook, proto.SessionHookParams{ID: created.ID, State: proto.StateDone, Event: ev}, nil); err != nil {
			t.Fatal(err)
		}
		sess, err := srv.sessions.Get(created.ID)
		if err != nil {
			t.Fatal(err)
		}
		return sess.Info().State
	}
	if got := turn(`{"hook_event_name":"Stop","background_tasks":[{"id":"bash_1","type":"local_bash"}]}`); got != proto.StateWorking {
		t.Fatalf("with one running: %q", got)
	}
	if got := turn(`{"hook_event_name":"Stop","background_tasks":[]}`); got != proto.StateDone {
		t.Fatalf("with none left: %q", got)
	}
}

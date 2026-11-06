package daemon

import (
	"testing"
	"time"

	"github.com/enowx/enowx/core/session"
)

// OpenCode's database is read only while an OpenCode session runs, and once just after:
// other agents' records always (issue #47).
func TestOpenCodeRecordsReadOnlyWhileItRuns(t *testing.T) {
	srv := startServer(t)
	srv.tok.scanMu.Lock()
	defer srv.tok.scanMu.Unlock()
	now := time.Now()
	live := srv.tokenAgentsLive(now)
	if live("opencode") || live("kilo") || !live("claude") || !live("codex") {
		t.Fatal("with no session running")
	}
	sess, err := srv.sessions.Create(session.Spec{Command: []string{"sh", "-c", "cat"}, Dir: t.TempDir(), Cols: 80, Rows: 24, Meta: session.Meta{Agent: "opencode"}})
	if err != nil {
		t.Fatal(err)
	}
	if !srv.tokenAgentsLive(now)("opencode") {
		t.Fatal("with an OpenCode session running")
	}
	if err := srv.sessions.Kill(sess.ID()); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "it ended", func() bool {
		_, err := srv.sessions.Get(sess.ID())
		return err != nil || sess.Info().Exited
	})
	if !srv.tokenAgentsLive(now.Add(scanEvery))("opencode") {
		t.Fatal("the scan just after it stopped")
	}
	if srv.tokenAgentsLive(now.Add(3 * scanEvery))("opencode") {
		t.Fatal("long after it stopped")
	}
}

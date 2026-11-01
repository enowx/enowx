package daemon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/enowx/enowx/proto"
)

func TestSessionsSurviveADaemonRestart(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	dir := t.TempDir()
	work := t.TempDir()

	first, err := NewServer(dir)
	if err != nil {
		t.Fatal(err)
	}
	c := connect(t, first, nil)
	shell, err := c.CreateSession(proto.SessionCreateParams{Agent: "shell", Dir: work, Cols: 80, Rows: 24, Env: []string{"PS1=$ "}})
	if err != nil {
		t.Fatal(err)
	}
	custom, err := c.CreateSession(proto.SessionCreateParams{Agent: "custom", Command: []string{"sh", "-c", "echo custom-output; sleep 60"}, Dir: work, Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	st, err := c.Attach(shell.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Write([]byte("echo before-restart\r")); err != nil {
		t.Fatal(err)
	}
	watch(st).waitFor(t, "before-restart")
	cst, err := c.Attach(custom.ID)
	if err != nil {
		t.Fatal(err)
	}
	watch(cst).waitFor(t, "custom-output")
	first.Close() // what SIGTERM does: save, then stop

	second, err := NewServer(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(second.Close)
	c2 := connect(t, second, nil)
	list, err := c2.Sessions()
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]proto.SessionInfo{}
	for _, s := range list {
		byID[s.ID] = s
	}
	if s := byID[shell.ID]; !s.Restored || s.Exited {
		t.Fatalf("shell after restart = %+v, want restored and running", s)
	}
	if s := byID[custom.ID]; !s.Restored || !s.Exited {
		t.Fatalf("custom command after restart = %+v, want restored but not rerun", s)
	}
	// The relaunched shell shows what it showed before, and runs again.
	st2, err := c2.Attach(shell.ID)
	if err != nil {
		t.Fatal(err)
	}
	w := watch(st2)
	w.waitFor(t, "before-restart")
	w.waitFor(t, "restarted this session")
	if _, err := st2.Write([]byte("echo after-restart\r")); err != nil {
		t.Fatal(err)
	}
	w.waitFor(t, "after-restart")
	cst2, err := c2.Attach(custom.ID)
	if err != nil {
		t.Fatal(err)
	}
	watch(cst2).waitFor(t, "custom-output")

	// Deleting forgets the session for the next daemon too.
	if err := c2.KillSession(custom.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(second.persist.historyFile(custom.ID)); !os.IsNotExist(err) {
		t.Fatalf("history file kept after delete: %v", err)
	}
	for _, r := range second.persist.saved() {
		if r.ID == custom.ID {
			t.Fatal("record kept after delete")
		}
	}
}

// An agent continued after an upgrade starts with this daemon's hooks and
// guide, not those of the daemon that saved it.
func TestRestoredAgentGetsCurrentArguments(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	dir := t.TempDir()
	saved := `[{"id":"a1","dir":"` + t.TempDir() + `","agent":"claude","relaunch":true,"restart":["/bin/sh","-c","exit 0"]}]`
	if err := os.WriteFile(filepath.Join(dir, "sessions.json"), []byte(saved), 0o600); err != nil {
		t.Fatal(err)
	}
	srv, err := NewServer(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	sess, err := srv.sessions.Get("a1")
	if err != nil {
		t.Fatal(err)
	}
	if cmd := sess.Info().Command; !slices.Contains(cmd, "--append-system-prompt") {
		t.Fatalf("restored with %q", cmd)
	}
}

// Sessions brought back by a restart get ENOWX_BIN, which agents' hooks
// and `enowx display` need, as new ones do.
func TestRestoredSessionKnowsTheDaemon(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	dir := t.TempDir()
	saved := `[{"id":"s1","dir":"` + t.TempDir() + `","agent":"shell","relaunch":true}]`
	if err := os.WriteFile(filepath.Join(dir, "sessions.json"), []byte(saved), 0o600); err != nil {
		t.Fatal(err)
	}
	srv, err := newServer(dir, "/opt/enowx-test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	st, err := connect(t, srv, nil).Attach("s1")
	if err != nil {
		t.Fatal(err)
	}
	w := watch(st)
	w.waitFor(t, "$ ") // the shell's prompt: typing sooner may be dropped
	if _, err := st.Write([]byte("echo bin=$ENOWX_BIN\r")); err != nil {
		t.Fatal(err)
	}
	w.waitFor(t, "bin=/opt/enowx-test")
}

// An ended session continues in place: same session, its screen kept, its
// program running again.
func TestContinueAnEndedSession(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	srv := startServer(t)
	exited := make(chan string, 4)
	c := connect(t, srv, func(e proto.Event) {
		var info proto.SessionInfo
		if e.Name == proto.EventSessionUpdated && json.Unmarshal(e.Data, &info) == nil && info.Exited {
			exited <- info.ID
		}
	})
	info, err := c.CreateSession(proto.SessionCreateParams{Agent: "shell", Dir: t.TempDir(), Env: []string{"PS1=$ "}})
	if err != nil {
		t.Fatal(err)
	}
	st, err := c.Attach(info.ID)
	if err != nil {
		t.Fatal(err)
	}
	w := watch(st)
	w.waitFor(t, "$ ") // the shell's prompt: typing sooner may be dropped
	if _, err := st.Write([]byte("echo before-exit; exit 3\r")); err != nil {
		t.Fatal(err)
	}
	w.waitFor(t, "before-exit")
	select {
	case <-exited:
	case <-time.After(5 * time.Second):
		t.Fatal("the shell never exited")
	}
	again, err := c.ContinueSession(info.ID)
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != info.ID || again.Exited {
		t.Fatalf("continued: %+v", again)
	}
	st2, err := c.Attach(info.ID)
	if err != nil {
		t.Fatal(err)
	}
	w2 := watch(st2)
	w2.waitFor(t, "before-exit")
	if _, err := st2.Write([]byte("echo after-continue\r")); err != nil {
		t.Fatal(err)
	}
	w2.waitFor(t, "after-continue")
	if _, err := c.ContinueSession(info.ID); err == nil {
		t.Fatal("continued a running session")
	}
}

// A session's note comes back with it after a restart.
func TestNotesSurviveARestart(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	dir := t.TempDir()
	srv, err := NewServer(dir)
	if err != nil {
		t.Fatal(err)
	}
	c := connect(t, srv, nil)
	info, err := c.CreateSession(proto.SessionCreateParams{Command: []string{"sh", "-c", "sleep 30"}, Dir: t.TempDir(), Cols: 80, Rows: 10})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.SetNote(info.ID, "  reproduced the bug  "); err != nil {
		t.Fatal(err)
	}
	srv.Close()
	again, err := NewServer(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	list, _ := connect(t, again, nil).Sessions()
	if len(list) != 1 || list[0].Note != "reproduced the bug" {
		t.Fatalf("after the restart: %+v", list)
	}
}

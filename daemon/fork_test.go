package daemon

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"testing"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/transcript"
	"github.com/enowx/enowx/proto"
)

// forkHarness has a fake Claude Code that keeps a conversation as the real
// one does: resumed with --fork-session it makes a new file, and /compact
// records a summary in the file it is on, or fails when told.
func forkHarness(t *testing.T) (*Server, *client.Client, string, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	bin := t.TempDir()
	script := `#!/bin/sh
id=$(od -An -N8 -tx1 /dev/urandom | tr -d ' \n')
for a in "$@"; do
  [ "$prev" = "--resume" ] && resumed=$a
  [ "$a" = "--fork-session" ] && fork=1
  prev=$a
done
printf '%s\n' "$*" >> "$HOME/args"
dir="$HOME/.claude/projects/$(pwd | sed 's/[^A-Za-z0-9]/-/g')"
mkdir -p "$dir"
if [ -n "$resumed" ] && [ -z "$fork" ]; then id=$resumed; fi
file="$dir/$id.jsonl"
if [ -n "$resumed" ]; then
  [ -n "$fork" ] && cp "$dir/$resumed.jsonl" "$file"
else
  echo '{"type":"user","uuid":"u1","message":{"content":"remember the word"}}' > "$file"
fi
echo "❯ ready $id"
while read -r line; do
  case "$line" in
  */compact*)
    if [ -f "$HOME/fail" ]; then echo "Error during compaction: no"; else
      echo '{"type":"user","uuid":"u2","isCompactSummary":true,"message":{"content":"summary"}}' >> "$file"
      echo "compacted"
    fi ;;
  *) echo "heard: $line" ;;
  esac
done
`
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	srv := startServer(t)
	c := connect(t, srv, nil)
	dir := gitRepo(t)
	if real, err := filepath.EvalSymlinks(dir); err == nil {
		dir = real
	}
	if _, err := c.AddProject(dir); err != nil {
		t.Fatal(err)
	}
	return srv, c, home, dir
}

// A fork goes on from a compacted copy in a new session named after the old,
// in its folder and branch; the old one's conversation isn't touched. What
// fails leaves the old one and no new one, and says what; another agent is
// told it isn't supported; a stopped session comes back first; one working
// waits.
func TestForkSession(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	srv, c, home, dir := forkHarness(t)
	old, err := c.CreateSession(proto.SessionCreateParams{Agent: "claude", Project: dir, Name: "Long work", Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	waitScreen(t, c, old.ID, "ready")
	oldFile := ""
	waitFor(t, "its conversation", func() bool { oldFile = srv.transcriptPath(old.ID); return oldFile != "" })
	before, _ := os.ReadFile(oldFile)

	// Working, it waits.
	if err := c.Call(proto.MethodSessionState, proto.SessionStateParams{ID: old.ID, State: proto.StateWorking}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ForkSession(old.ID); err == nil || !strings.Contains(err.Error(), "working") {
		t.Fatalf("forked while working: %v", err)
	}
	if err := c.Call(proto.MethodSessionState, proto.SessionStateParams{ID: old.ID, State: proto.StateDone}, nil); err != nil {
		t.Fatal(err)
	}

	// It fails: only the old one stays, and it says why.
	os.WriteFile(filepath.Join(home, "fail"), nil, 0o600)
	if _, err := c.ForkSession(old.ID); err == nil || !strings.Contains(err.Error(), "Long work is as it was") || !strings.Contains(err.Error(), "compact") {
		t.Fatalf("a failed compaction: %v", err)
	}
	if list, _ := c.Sessions(); len(list) != 1 || list[0].ID != old.ID {
		t.Fatalf("after failing: %+v", list)
	}
	os.Remove(filepath.Join(home, "fail"))

	fork, err := c.ForkSession(old.ID)
	if err != nil {
		t.Fatal(err)
	}
	if fork.Name != "Long work (fork)" || fork.Dir != old.Dir || fork.Branch != old.Branch || fork.ID == old.ID || fork.Worktree {
		t.Fatalf("the fork: %+v", fork)
	}
	args, _ := os.ReadFile(filepath.Join(home, "args"))
	want := "--resume " + strings.TrimSuffix(filepath.Base(oldFile), ".jsonl") + " --fork-session"
	if !strings.Contains(string(args), want) {
		t.Fatalf("started with %q, not %q", args, want)
	}
	newFile := srv.transcriptPath(fork.ID)
	if newFile == "" || newFile == oldFile || !transcript.Compacted("claude", newFile) {
		t.Fatalf("the fork's conversation: %q, of %q", newFile, oldFile)
	}
	if after, _ := os.ReadFile(oldFile); string(after) != string(before) {
		t.Fatalf("the old conversation changed: %s", after)
	}
	sess, _ := srv.sessions.Get(old.ID)
	if sess.Info().Exited {
		t.Fatal("the old session stopped")
	}
	if err := c.Input(fork.ID, "go on", false, true); err != nil {
		t.Fatal(err)
	}
	waitScreen(t, c, fork.ID, "heard: go on")

	// Hibernated, it comes back first, with a prompt for the fork to take.
	if res, err := c.HibernateSessions(proto.SessionHibernateParams{IDs: []string{old.ID}, Force: true}); err != nil || len(res.Hibernated) != 1 {
		t.Fatalf("hibernating: %+v %v", res, err)
	}
	var second proto.SessionInfo
	if err := c.Call(proto.MethodSessionFork, proto.SessionForkParams{ID: old.ID, Prompt: "next step"}, &second); err != nil {
		t.Fatal(err)
	}
	if sess, _ := srv.sessions.Get(old.ID); sess.Info().Exited {
		t.Fatal("the hibernated session didn't come back")
	}
	waitScreen(t, c, second.ID, "next step")

	// Others aren't supported, and nothing starts.
	n := len(srv.sessions.List())
	shell, err := c.CreateSession(proto.SessionCreateParams{Command: []string{"cat"}, Dir: t.TempDir(), Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.ForkSession(shell.ID); err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("a program: %v", err)
	}
	if got := len(srv.sessions.List()); got != n+1 {
		t.Fatalf("%d sessions, was %d and one more", got, n)
	}
}

// A session that keeps ending as it starts says why, and isn't continued in a
// tight loop.
func TestSessionEndingAtOnceStops(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	srv := startServer(t)
	c := connect(t, srv, nil)
	info, err := c.CreateSession(proto.SessionCreateParams{Command: []string{"sh", "-c", "exit 129"}, Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := srv.sessions.Get(info.ID)
	if err != nil {
		t.Fatal(err)
	}
	for sess.QuickExits() < 1 {
		time.Sleep(10 * time.Millisecond)
	}
	if why := sess.Info().ExitWhy; !strings.Contains(why, "hang-up") || !strings.Contains(why, "never got going") {
		t.Fatalf("why: %q", why)
	}
}

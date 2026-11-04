package daemon

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/enowx/enowx/proto"
)

// A session says when its agent moved to a model of its own accord, and a restart goes back to
// the one it was given; /model is the user's, and becomes the session's.
func TestSessionSaysItsModelMoved(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	bin := t.TempDir()
	// Each line typed is a record: a model's answer, or /model's.
	script := `#!/bin/sh
dir="$HOME/.claude/projects/$(pwd | sed 's/[^A-Za-z0-9]/-/g')"
mkdir -p "$dir"
while read -r kind model; do case "$kind" in
answer) echo "{\"type\":\"assistant\",\"message\":{\"role\":\"assistant\",\"model\":\"$model\",\"content\":[{\"type\":\"text\",\"text\":\"hi\"}]}}" >> "$dir/s.jsonl" ;;
choose) echo "{\"type\":\"user\",\"message\":{\"role\":\"user\",\"content\":\"<local-command-stdout>Set model to $model</local-command-stdout>\"}}" >> "$dir/s.jsonl" ;;
esac; done
`
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	srv := startServer(t)
	c := connect(t, srv, nil)
	created, err := c.CreateSession(proto.SessionCreateParams{Agent: "claude", Project: gitRepo(t), Model: "claude-opus-5-5"})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := srv.sessions.Get(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.sessions.SetState(sess, proto.StateWorking); err != nil {
		t.Fatal(err)
	}
	wait := func(what, line, model, started string) {
		t.Helper()
		if err := c.Input(created.ID, line, false, true); err != nil {
			t.Fatal(err)
		}
		for deadline := time.Now().Add(30 * time.Second); ; time.Sleep(100 * time.Millisecond) {
			info := sess.Info()
			if info.Model == model && info.ModelStarted == started {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s: model %q, started on %q", what, info.Model, info.ModelStarted)
			}
		}
	}
	wait("as asked", "answer claude-opus-5-5-20261101", "claude-opus-5-5-20261101", "")
	wait("a fallback", "answer claude-sonnet-5-5", "claude-sonnet-5-5", "claude-opus-5-5-20261101")
	if args := keptArgs(sess.Meta()); !slices.Contains(args, "claude-opus-5-5-20261101") || slices.Contains(args, "claude-sonnet-5-5") {
		t.Fatalf("a restart would stay on the fallback: %v", args)
	}
	wait("/model", "choose claude-haiku-5-5", "claude-haiku-5-5", "")
	if args := keptArgs(sess.Meta()); !slices.Contains(args, "claude-haiku-5-5") {
		t.Fatalf("a restart would drop the user's /model: %v", args)
	}
}

package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/enowx/enowx/proto"
)

// The owner's bug: a switch of one session typed /model, and Claude Code saved it in
// ~/.claude/settings.json for every session. The fake does what the real one does with what is
// typed, in a real file: switching by session.settings, a task's model and the board's Change
// model leave it as it was, and each lands on the agent's command line.
func TestSwitchLeavesTheUsersDefaultAlone(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses bash")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	settings := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settings), 0o700); err != nil {
		t.Fatal(err)
	}
	const mine = `{"model":"opus"}`
	if err := os.WriteFile(settings, []byte(mine), 0o600); err != nil {
		t.Fatal(err)
	}
	starts := filepath.Join(home, "starts")
	bin := t.TempDir()
	// Bash reads and writes by itself: a program it ran would count as what stopping it loses.
	script := fmt.Sprintf(`#!/usr/bin/env bash
printf '%%s\n' "$*" >> %q
echo started $$
while IFS= read -r line; do
  case "$line" in
  "/model "*) printf '{"model":"%%s"}' "${line#/model }" > %q ;;
  "/effort "*) printf '{"effortLevel":"%%s"}' "${line#/effort }" > %q ;;
  esac
done
`, starts, settings, settings)
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	srv := startServer(t)
	c := connect(t, srv, nil)
	s, err := c.CreateSession(proto.SessionCreateParams{Agent: "claude", Dir: t.TempDir(), Cols: 100, Rows: 30})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "its agent started", func() bool { return len(linesOf(starts)) == 1 })
	// Its hooks would say so: at rest between turns.
	rest := func() {
		t.Helper()
		if err := c.Call(proto.MethodSessionState, proto.SessionStateParams{ID: s.ID, State: proto.StateDone}, nil); err != nil {
			t.Fatal(err)
		}
	}
	kept := func(what string, want ...string) {
		t.Helper()
		if got, _ := os.ReadFile(settings); string(got) != mine {
			t.Fatalf("%s changed the user's settings: %s", what, got)
		}
		all := linesOf(starts)
		for _, w := range want {
			if !strings.Contains(all[len(all)-1], w) {
				t.Fatalf("%s: started without %s: %q", what, w, all)
			}
		}
	}

	rest()
	if err := c.SwitchSettings(proto.SessionSettingsParams{ID: s.ID, Model: "claude-sonnet-5-5", Effort: "low"}); err != nil {
		t.Fatal(err)
	}
	kept("a switch", "--model claude-sonnet-5-5", "--effort low")
	sess, err := srv.sessions.Get(s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if info := sess.Info(); info.Model != "claude-sonnet-5-5" || info.Effort != "low" {
		t.Fatalf("the session says %q, %q", info.Model, info.Effort)
	}

	// A task handed to it by hand on a model, as goa delegate does.
	rest()
	var task proto.GOATask
	if err := c.Call(proto.MethodGOADelegate, proto.GOADelegateParams{Session: s.ID, Prompt: "fix the totals", Model: "claude-fable-5-1", Effort: "max"}, &task); err != nil {
		t.Fatal(err)
	}
	if task.Model != "claude-fable-5-1" {
		t.Fatalf("the task: %+v", task)
	}
	kept("a task's model", "--model claude-fable-5-1", "--effort max")

	// The board's Change model, the worker's turn over.
	rest()
	waitFor(t, "its turn ended on the board", func() bool {
		tasks := goaBoardOf(t, c).Tasks
		return len(tasks) == 1 && tasks[0].Status == proto.GOATaskTurnEnded
	})
	if err := c.Call(proto.MethodGOAOverride, proto.GOAOverrideParams{ID: task.ID, Model: "claude-haiku-5-5"}, nil); err != nil {
		t.Fatal(err)
	}
	kept("the board's Change model", "--model claude-haiku-5-5")
	if got := goaBoardOf(t, c).Tasks[0].Status; got != proto.GOATaskTurnEnded {
		t.Fatalf("starting again counted as its task's end: %s", got)
	}

	// At work it isn't stopped, and says so.
	if err := c.Call(proto.MethodSessionState, proto.SessionStateParams{ID: s.ID, State: proto.StateWorking}, nil); err != nil {
		t.Fatal(err)
	}
	n := len(linesOf(starts))
	if err := c.SwitchSettings(proto.SessionSettingsParams{ID: s.ID, Model: "claude-opus-5-5"}); err == nil || !strings.Contains(err.Error(), "turn's end") {
		t.Fatalf("switched at work: %v", err)
	}
	if len(linesOf(starts)) != n {
		t.Fatal("stopped at work")
	}

	// What this test watches for: the fake saves what is typed, as the real one does.
	if err := c.Input(s.ID, "/model typed", false, true); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the typed /model to reach the file", func() bool {
		got, _ := os.ReadFile(settings)
		return strings.Contains(string(got), "typed")
	})
}

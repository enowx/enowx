package daemon

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/enowx/enowx/proto"
)

// An agent that exits at once leaves a shell, and what was to be typed for it, as a first prompt
// or sent after, never reaches that shell, whose commands they'd be. It says so.
func TestNothingTypedIntoTheShellAnAgentLeaves(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	dir := t.TempDir()
	first, sent := filepath.Join(dir, "first"), filepath.Join(dir, "sent")
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "crush"), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil { // takes its prompt typed
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	srv := startServer(t)
	c := connect(t, srv, nil)
	info, err := c.CreateSession(proto.SessionCreateParams{Agent: "crush", Dir: dir, Prompt: "touch " + first, Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := srv.sessions.Get(info.ID)
	if err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(promptTimeout + 10*time.Second); !strings.Contains(sess.Info().Note, "nothing was typed"); time.Sleep(100 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("no word of it: note %q, screen %q", sess.Info().Note, sess.Terminal().Text())
		}
	}
	err = c.Input(info.ID, "touch "+sent, false, true)
	if err == nil || !strings.Contains(err.Error(), "nothing was typed") {
		t.Fatalf("sending to it: %v", err)
	}
	time.Sleep(time.Second) // what got in would have run by now
	for _, path := range []string{first, sent} {
		if _, err := os.Stat(path); err == nil {
			t.Fatalf("the shell ran %s: %q", path, sess.Terminal().Text())
		}
	}
}

// A plain terminal and an agent still running take what's typed.
func TestTypingGoesToWhatStillRuns(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	srv := startServer(t)
	c := connect(t, srv, nil)
	dir := t.TempDir()
	done := filepath.Join(dir, "done")
	shell, err := c.CreateSession(proto.SessionCreateParams{Dir: dir, Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	waited := time.Now().Add(20 * time.Second)
	for {
		if err := c.Input(shell.ID, "touch "+done, false, true); err != nil {
			t.Fatalf("a terminal: %v", err)
		}
		if _, err := os.Stat(done); err == nil {
			break
		}
		if time.Now().After(waited) {
			t.Fatal("a terminal never ran what was typed")
		}
		time.Sleep(500 * time.Millisecond)
	}
	agentRun, err := c.CreateSession(proto.SessionCreateParams{Agent: "custom", Command: []string{"cat"}, Dir: dir, Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Input(agentRun.ID, "hello", false, true); err != nil {
		t.Fatalf("an agent running: %v", err)
	}
}

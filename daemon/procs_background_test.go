package daemon

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/enowx/enowx/core/procstat"
	"github.com/enowx/enowx/proto"
)

// A session's agent that starts agents, one of which leaves its tree, shows them; its own
// wrapper, a plain server and its MCP server don't count.
func TestSessionShowsItsBackgroundAgents(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("reads a process's arguments as Linux shows them")
	}
	bin := t.TempDir()
	dir := t.TempDir()
	// The agent, behind a wrapper of its own name, as npm's script starts a binary. Its children:
	// an agent under a shell, one whose parent ends, a server, its MCP server.
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"+body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write("server", "sleep 300; true\n")
	write("codex", `case "$1" in
mcp-server) sleep 300 ;;
inner) sh -c 'kilo; true' & sh -c 'gemini & sleep 2' & server & codex mcp-server & wait ;;
*) sh -c "exec codex inner"; true ;;
esac
`)
	write("kilo", "sleep 300; true\n")
	write("gemini", "sleep 300; true\n")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	srv := startServer(t)
	srv.procs.load(filepath.Join(dir, "procs.json")) // a server under test follows nothing by itself
	c := connect(t, srv, nil)
	created, err := c.CreateSession(proto.SessionCreateParams{Command: []string{"codex"}, Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := srv.sessions.Get(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for deadline := time.Now().Add(20 * time.Second); ; time.Sleep(200 * time.Millisecond) {
		srv.procsLook()
		names = names[:0]
		for _, b := range sess.Info().Background {
			names = append(names, b.Name)
		}
		slices.Sort(names)
		left := slices.ContainsFunc(sess.Info().Background, func(b proto.BackgroundAgent) bool { return b.Name == "gemini" && b.Left })
		if slices.Equal(names, []string{"gemini", "kilo"}) && left {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("background agents: %v", sess.Info().Background)
		}
	}
	for _, b := range sess.Info().Background {
		if b.PID == 0 || b.Start == "" || b.Since.IsZero() || b.Memory == 0 {
			t.Fatalf("what it says of %s: %+v", b.Name, b)
		}
	}
	// A helper isn't ended under them.
	info := sess.Info()
	info.CanvasFor, info.State = "c", proto.StateDone
	if helperEnds(info, true, time.Hour) {
		t.Fatal("ended with a background agent at work")
	}
	if err := c.KillSession(created.ID); err != nil {
		t.Fatal(err)
	}
}

// What left its session's tree before any look comes on the list by its session's tag; another
// daemon's session's doesn't, nor one with no tag.
func TestAdoptDetached(t *testing.T) {
	list := []procstat.Proc{
		{PID: 10, PPID: 1, Start: "a", Name: "codex"},
		{PID: 11, PPID: 1, Start: "b", Name: "codex"},
		{PID: 12, PPID: 1, Start: "c", Name: "cron"},
		{PID: 13, PPID: 99, Start: "d", Name: "under-another"},
	}
	env := func(pid int, _ string) string {
		return map[int]string{10: "mine", 11: "another-daemons", 13: "mine"}[pid]
	}
	known := map[int]startedProc{}
	if !adoptDetached(known, list, env, func(id string) bool { return id == "mine" }) || len(known) != 1 || !known[10].Left || known[10].Session != "mine" {
		t.Fatalf("%+v", known)
	}
}

// CPU is how much of a core between two looks, and one that has used next to none for long is stuck.
func TestBackgroundAgentsLiveCPU(t *testing.T) {
	list := func(cpu time.Duration) []procstat.Proc {
		return []procstat.Proc{{PID: 1, Start: "r"}, {PID: 2, PPID: 1, Start: "m"}, {PID: 3, PPID: 2, Start: "b", CPUTime: cpu, RSS: 7}}
	}
	known := map[int]startedProc{1: {PID: 1, Start: "r", Session: "s"}, 2: {PID: 2, Start: "m", Session: "s"}, 3: {PID: 3, Start: "b", Session: "s"}}
	args := func(pid int) []string {
		return map[int][]string{1: {"sh"}, 2: {"claude"}, 3: {"codex"}}[pid]
	}
	tr := &procTracker{known: known}
	at := time.Now()
	look := func(after time.Duration, cpu time.Duration) proto.BackgroundAgent {
		t.Helper()
		out := tr.backgroundLocked(list(cpu), known, args, at.Add(after))["s"]
		if len(out) != 1 || out[0].Name != "codex" {
			t.Fatalf("%+v", out)
		}
		return out[0]
	}
	look(0, time.Second)
	if b := look(10*time.Second, 6*time.Second); b.CPUPercent != 50 || b.Memory != 7 || b.Stuck || b.Since != at {
		t.Fatalf("%+v", b)
	}
	if b := look(10*time.Second+backgroundStuckAfter, 6*time.Second); !b.Stuck || b.CPUPercent != 0 {
		t.Fatalf("idle for long: %+v", b)
	}
	if b := look(20*time.Second+backgroundStuckAfter, 8*time.Second); b.Stuck {
		t.Fatalf("at work again: %+v", b)
	}
}

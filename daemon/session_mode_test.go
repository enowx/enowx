package daemon

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/proto"
)

// modeHarness has a fake Claude Code whose mode key goes round as the real one's does
// (run on 2.1.318): bypass only when allowed at its start. It logs its starts and every
// mode it is ever in, and shows none on its screen while the file quiet exists.
func modeHarness(t *testing.T, quiet bool) (*Server, *client.Client, string, proto.SessionInfo) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	if quiet {
		if err := os.WriteFile(filepath.Join(home, "quiet"), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	bin := t.TempDir()
	// Bash reads a key itself: a program it ran would count as what stopping it loses.
	script := `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$HOME/args"
mode=default; allow=
for a in "$@"; do
  [ "$prev" = "--permission-mode" ] && mode=$a
  case "$a" in
  --dangerously-skip-permissions) mode=bypassPermissions; allow=1 ;;
  --allow-dangerously-skip-permissions) allow=1 ;;
  esac
  prev=$a
done
show() {
  printf '%s\n' "$mode" >> "$HOME/modes"
  [ -f "$HOME/quiet" ] && return
  case "$mode" in
  auto) echo "auto mode on" ;;
  default) echo "manual mode on" ;;
  acceptEdits) echo "accept edits on" ;;
  plan) echo "plan mode on" ;;
  bypassPermissions) echo "bypass permissions on" ;;
  esac
}
show
while IFS= read -rsn3 k; do
  case "$k" in
  *'[Z'*)
    case "$mode" in
    auto) mode=default ;;
    default) mode=acceptEdits ;;
    acceptEdits) mode=plan ;;
    plan) if [ -n "$allow" ]; then mode=bypassPermissions; else mode=auto; fi ;;
    bypassPermissions) mode=auto ;;
    esac
    show ;;
  esac
done
`
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
	return srv, c, home, s
}

// modesVisited is every mode the fake has been in, in order.
func modesVisited(t *testing.T, home string) []string {
	t.Helper()
	data, _ := os.ReadFile(filepath.Join(home, "modes"))
	return strings.Fields(string(data))
}

// The owner's rule: switching lands on the mode named, and plan mode is never on the way.
func TestSwitchModeLandsWithoutPassingThroughPlan(t *testing.T) {
	srv, c, home, s := modeHarness(t, false)
	waitScreen(t, c, s.ID, "bypass permissions on")
	land := func(mode string) {
		t.Helper()
		// Its hooks would say so: at rest between turns.
		if err := c.Call(proto.MethodSessionState, proto.SessionStateParams{ID: s.ID, State: proto.StateDone}, nil); err != nil {
			t.Fatal(err)
		}
		if err := c.SwitchMode(s.ID, mode); err != nil {
			t.Fatalf("to %s: %v", mode, err)
		}
		visited := modesVisited(t, home)
		if got := visited[len(visited)-1]; got != mode {
			t.Fatalf("to %s: it is in %s, having been in %v", mode, got, visited)
		}
		if slices.Contains(visited, "plan") {
			t.Fatalf("to %s: went through plan mode: %v", mode, visited)
		}
		sess, err := srv.sessions.Get(s.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got := sess.Meta().Mode; got != mode {
			t.Fatalf("to %s: the session says %s", mode, got)
		}
	}
	land("auto")
	land("acceptEdits") // two steps, without a redraw counted twice
	// What the owner saw: on from a mode before plan, to bypass, a start of its own.
	land(agent.ModeBypass)
	land("default")
	land("auto") // round the other way would be through plan and bypass
	args, _ := os.ReadFile(filepath.Join(home, "args"))
	if !strings.Contains(string(args), "--permission-mode auto") {
		t.Fatal("it didn't start in auto")
	}
	if !strings.HasSuffix(strings.TrimSpace(string(args)), "--permission-mode auto") && !strings.Contains(string(args), "--allow-dangerously-skip-permissions") {
		t.Fatal("bypass isn't left to switch to")
	}
	// That start was once: asleep and back, it is in bypass.
	sess, _ := srv.sessions.Get(s.ID)
	if err := srv.hibernateSession(sess, true); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ContinueSession(s.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "back in bypass", func() bool {
		v := modesVisited(t, home)
		return v[len(v)-1] == agent.ModeBypass
	})
	if slices.Contains(modesVisited(t, home), "plan") {
		t.Fatalf("went through plan mode: %v", modesVisited(t, home))
	}
	// Named, plan mode is where it lands, and it leaves it in a step.
	if err := c.SwitchMode(s.ID, "plan"); err != nil {
		t.Fatal(err)
	}
	waitScreen(t, c, s.ID, "plan mode on")
	if err := c.SwitchMode(s.ID, agent.ModeBypass); err != nil {
		t.Fatal(err)
	}
	v := modesVisited(t, home)
	if v[len(v)-1] != agent.ModeBypass || v[len(v)-2] != "plan" {
		t.Fatalf("%v", v)
	}
}

// What would need a start of its own waits for the agent's turn to end.
func TestSwitchModeDoesNotRestartAnAgentAtWork(t *testing.T) {
	_, c, home, s := modeHarness(t, false)
	waitScreen(t, c, s.ID, "bypass permissions on")
	if err := c.SwitchMode(s.ID, "acceptEdits"); err != nil {
		t.Fatal(err)
	}
	if err := c.Call(proto.MethodSessionState, proto.SessionStateParams{ID: s.ID, State: proto.StateWorking}, nil); err != nil {
		t.Fatal(err)
	}
	before := len(modesVisited(t, home))
	err := c.SwitchMode(s.ID, agent.ModeBypass)
	if err == nil || !strings.Contains(err.Error(), "turn's end") {
		t.Fatalf("at work: %v", err)
	}
	if len(modesVisited(t, home)) != before {
		t.Fatalf("something moved: %v", modesVisited(t, home))
	}
}

// A screen that doesn't say its mode gets no key: that is how a session ended up in plan.
func TestSwitchModeWithNoModeShownPressesNothing(t *testing.T) {
	_, c, home, s := modeHarness(t, true)
	waitFor(t, "started", func() bool { return len(modesVisited(t, home)) == 1 })
	err := c.SwitchMode(s.ID, "auto")
	if err == nil || !strings.Contains(err.Error(), "no key was pressed") {
		t.Fatalf("no mode shown: %v", err)
	}
	if v := modesVisited(t, home); len(v) != 1 {
		t.Fatalf("keys went: %v", v)
	}
	if err := c.SwitchMode(s.ID, "nonsense"); err == nil || !strings.Contains(err.Error(), "unknown mode") {
		t.Fatalf("a mode that isn't: %v", err)
	}
}

// As root, bypass can't be had: refused by name, and no key goes round looking for it.
func TestSwitchToBypassAsRootIsRefused(t *testing.T) {
	old := agent.AsRoot
	agent.AsRoot = func() bool { return true }
	t.Cleanup(func() { agent.AsRoot = old })
	_, c, home, s := modeHarness(t, false)
	waitScreen(t, c, s.ID, "manual mode on")
	err := c.SwitchMode(s.ID, agent.ModeBypass)
	if err == nil || !strings.Contains(err.Error(), "refuses bypass as root") {
		t.Fatalf("as root: %v", err)
	}
	if v := modesVisited(t, home); len(v) != 1 {
		t.Fatalf("keys went: %v", v)
	}
}

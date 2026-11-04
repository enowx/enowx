package agent

import (
	"slices"
	"strings"
	"testing"
)

func runsAs(t *testing.T, root bool) {
	t.Helper()
	old := AsRoot
	AsRoot = func() bool { return root }
	t.Cleanup(func() { AsRoot = old })
}

// Whatever a launch asks or carries, Claude Code's command line says bypass and nothing else.
func TestClaudeCommandLineIsAlwaysBypass(t *testing.T) {
	runsAs(t, false)
	for _, id := range []string{"claude", "claude-agent-teams"} {
		p, ok := Find(id)
		if !ok {
			t.Fatal(id)
		}
		for name, l := range map[string]Launch{
			"nothing asked":   {},
			"allow only":      {AllowSkip: true},
			"continue":        {Continue: true},
			"resume":          {Resume: "abc"},
			"fork":            {Resume: "abc", Fork: true},
			"a saved mode":    {Hooks: []string{"--model", "opus", "--permission-mode", "auto"}},
			"a mode in one":   {Hooks: []string{"--permission-mode=plan", "--effort", "high"}},
			"asked to skip":   {SkipPermissions: true},
			"a first prompt":  {Prompt: "fix it"},
			"its hooks given": {Hooks: []string{"--settings", "{}"}},
		} {
			argv := p.Argv(l)
			line := strings.Join(argv, " ")
			if !slices.Contains(argv, "--dangerously-skip-permissions") || strings.Contains(line, "--permission-mode") || strings.Contains(line, "--allow-dangerously") {
				t.Errorf("%s, %s: %q", id, name, argv)
			}
		}
	}
	p, _ := Find("claude")
	if got := p.Argv(Launch{Hooks: []string{"--model", "opus", "--permission-mode", "auto", "--effort", "high"}}); !slices.Contains(got, "opus") || !slices.Contains(got, "high") {
		t.Errorf("took more than the mode: %q", got)
	}
}

func TestOneStartInAnotherMode(t *testing.T) {
	runsAs(t, false)
	p, _ := Find("claude")
	argv := p.Argv(Launch{Mode: "default", Hooks: []string{"--permission-mode", "auto"}})
	line := strings.Join(argv, " ")
	if !strings.Contains(line, "--permission-mode default") || strings.Contains(line, "auto") || slices.Contains(argv, "--dangerously-skip-permissions") || !slices.Contains(argv, "--allow-dangerously-skip-permissions") {
		t.Fatalf("%q", argv)
	}
}

// Other agents do as they are asked: the rule is Claude Code's.
func TestOtherAgentsAreLeftAlone(t *testing.T) {
	runsAs(t, false)
	for _, id := range []string{"codex", "opencode", "openclaude", "gemini"} {
		p, ok := Find(id)
		if !ok {
			t.Fatal(id)
		}
		if AlwaysSkips(id) || Skips(id, false) || !Skips(id, true) {
			t.Errorf("%s follows the rule", id)
		}
		for _, f := range p.SkipPermissions {
			if slices.Contains(p.Argv(Launch{}), f) {
				t.Errorf("%s skips unasked", id)
			}
		}
	}
	p, _ := Find("codex")
	if got := p.Argv(Launch{Hooks: []string{"--permission-mode", "x"}}); !slices.Contains(got, "--permission-mode") {
		t.Errorf("codex's arguments changed: %q", got)
	}
}

// As root Claude Code refuses bypass and would exit: it starts asking, as it can.
func TestClaudeAsRootDoesNotAskForBypass(t *testing.T) {
	runsAs(t, true)
	if Skips("claude", true) || !AlwaysSkips("claude") {
		t.Fatal("as root")
	}
	p, _ := Find("claude")
	argv := p.Argv(Launch{SkipPermissions: true, AllowSkip: true, Hooks: []string{"--permission-mode", "auto"}})
	if slices.Contains(argv, "--dangerously-skip-permissions") || slices.Contains(argv, "--allow-dangerously-skip-permissions") || !slices.Contains(argv, "--permission-mode") {
		t.Fatalf("%q", argv)
	}
	if got := TabAgentBypass([]string{"claude", "--continue"}); slices.Contains(got, "--dangerously-skip-permissions") {
		t.Fatalf("a tab's: %q", got)
	}
}

func TestTabAgentBypass(t *testing.T) {
	runsAs(t, false)
	for _, c := range []struct{ in, want []string }{
		{[]string{"claude", "--continue", "--permission-mode", "auto"}, []string{"claude", "--continue", "--dangerously-skip-permissions"}},
		{[]string{"claude", "--continue", "--permission-mode=plan", "--allow-dangerously-skip-permissions"}, []string{"claude", "--continue", "--dangerously-skip-permissions"}},
		{[]string{"claude", "--resume", "a", "--dangerously-skip-permissions"}, []string{"claude", "--resume", "a", "--dangerously-skip-permissions"}},
		{[]string{"codex", "resume", "--last", "--sandbox", "read-only"}, []string{"codex", "resume", "--last", "--sandbox", "read-only"}},
		{nil, nil},
	} {
		if got := TabAgentBypass(c.in); !slices.Equal(got, c.want) {
			t.Errorf("%q: %q, want %q", c.in, got, c.want)
		}
	}
}

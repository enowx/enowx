package agent

import (
	"slices"
	"strings"
	"testing"
)

func TestTabAgentCommand(t *testing.T) {
	for _, c := range []struct {
		name string
		args []string
		want []string
		ok   bool
	}{
		{"keeps its mode and model, never its prompt", []string{"/usr/local/bin/claude", "--model", "opus", "--permission-mode=plan", "--dangerously-skip-permissions", "fix the bug"},
			[]string{"claude", "--continue", "--model", "opus", "--dangerously-skip-permissions"}, true},
		{"its own conversation", []string{"claude", "--resume", "abc-123"}, []string{"claude", "--resume", "abc-123", "--dangerously-skip-permissions"}, true},
		{"through node", []string{"node", "/opt/bin/claude", "--effort", "high"}, []string{"claude", "--continue", "--effort", "high", "--dangerously-skip-permissions"}, true},
		{"a secret stays out", []string{"claude", "--api-key", "sk-ant-abcdefgh"}, []string{"claude", "--continue", "--dangerously-skip-permissions"}, true},
		{"one shot", []string{"claude", "-p", "write the plan"}, nil, false},
		{"one shot of Codex", []string{"codex", "exec", "write the plan"}, nil, false},
		{"not an agent", []string{"npm", "run", "dev"}, nil, false},
		{"nothing", nil, nil, false},
	} {
		got, ok := TabAgentCommand(c.args)
		if ok != c.ok || !slices.Equal(got, c.want) {
			t.Errorf("%s: %q, %v; want %q", c.name, got, ok, c.want)
		}
		if strings.Contains(strings.Join(got, " "), "sk-ant") || strings.Contains(strings.Join(got, " "), "fix the bug") {
			t.Errorf("%s: kept %q", c.name, got)
		}
	}
}

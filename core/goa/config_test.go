package goa

import (
	"slices"
	"testing"
)

func TestConfigKeepsOnlyWhatLeavesGOAHeld(t *testing.T) {
	args := []string{"--tools", "Bash", "--model", "opus", "--dangerously-skip-permissions", "--effort=high", "--sandbox", "danger-full-access", "-m", "o3", "-c", "mcp_servers.enowx.command=x"}
	if got, want := ConfigArgs(args), []string{"--model", "opus", "--effort=high", "-m", "o3"}; !slices.Equal(got, want) {
		t.Errorf("flags: %q, want %q", got, want)
	}
	env := []string{"ENOWX_GOA=", "ENOWX_BIN=/tmp/x", "OPENCODE_CONFIG_CONTENT={}", "OPENCODE_PERMISSION={}", "ENABLE_TOOL_SEARCH=true", "HTTPS_PROXY=http://p", "CLAUDE_CONFIG_DIR=/a"}
	if got, want := ConfigEnv(env), []string{"HTTPS_PROXY=http://p", "CLAUDE_CONFIG_DIR=/a"}; !slices.Equal(got, want) {
		t.Errorf("variables: %q, want %q", got, want)
	}
}

package record

import (
	"os"
	"path/filepath"
)

// dataDir is enowX's folder under home, as transport.Dir has it.
func dataDir(home string) string {
	if dir := os.Getenv("ENOWX_HOME"); dir != "" {
		return dir
	}
	return filepath.Join(home, ".enowx-app")
}

// SessionsDir is where a conversation goes when no session names a folder for it: the
// agent run by hand, outside enowX's sessions.
func SessionsDir(home string) string {
	return filepath.Join(dataDir(home), "claude-platform", "sessions")
}

// StoresDir holds a folder per enowX session (daemon's agentStore), each its agent's
// conversations, as --session-dir names it.
func StoresDir(home string) string { return filepath.Join(dataDir(home), "agent-sessions") }

// SystemPromptFile is the user's own system prompt for every session on this host, when
// they keep one; --system-prompt-file goes before it.
func SystemPromptFile(home string) string {
	return filepath.Join(dataDir(home), "claude-platform", "system-prompt.md")
}

package tokens

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Claude Code keeps each conversation as JSON lines under
// ~/.claude/projects/<folder>/ (its subagents' in <session>/subagents/):
// each assistant record carries its message's id, model and usage, the
// same on every record of one message (one per content block). Input
// there leaves out the cache. OpenClaude (~/.openclaude/projects), ZCode
// (~/.zcode/projects) and Qoder CLI (~/.qoder/projects) keep their
// conversations in the same shape.

// claudeLike lists the conversations under the projects folder of each
// of homes (relative to the user's home), or of the env variables
// naming one.
func claudeLike(envVars []string, homes ...string) func(env Env) []string {
	return func(env Env) []string {
		var roots []string
		for _, h := range homes {
			roots = append(roots, filepath.Join(env.Home, h, "projects"))
		}
		for _, v := range envVars {
			if d := os.Getenv(v); d != "" {
				roots = append(roots, filepath.Join(d, "projects"))
			}
		}
		var out []string
		for _, r := range roots {
			out = append(out, walk(r, ".jsonl")...)
		}
		return out
	}
}

type claudeLine struct {
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Cwd       string    `json:"cwd"`
	SessionID string    `json:"sessionId"`
	Message   *struct {
		ID    string `json:"id"`
		Model string `json:"model"`
		Usage *struct {
			Input       int64 `json:"input_tokens"`
			CacheRead   int64 `json:"cache_read_input_tokens"`
			CacheCreate int64 `json:"cache_creation_input_tokens"`
			Output      int64 `json:"output_tokens"`
			Details     struct {
				Thinking int64 `json:"thinking_tokens"`
			} `json:"output_tokens_details"`
		} `json:"usage"`
	} `json:"message"`
}

// readClaudeAs reads a Claude Code-shaped conversation, its calls' IDs
// prefixed by prefix.
func readClaudeAs(prefix string) func(path string, st *FileState) ([]Call, error) {
	return func(path string, st *FileState) ([]Call, error) {
		var out []Call
		err := readLines(path, st, func(line []byte) {
			var l claudeLine
			if json.Unmarshal(line, &l) != nil {
				return
			}
			if l.Cwd != "" {
				st.Dir = l.Cwd
			}
			if l.SessionID != "" {
				st.Conv = l.SessionID
			}
			if l.Type != "assistant" || l.Message == nil || l.Message.Usage == nil || l.Message.ID == "" {
				return
			}
			u := l.Message.Usage
			out = append(out, Call{ID: prefix + ":" + l.Message.ID, Time: l.Timestamp, Model: l.Message.Model,
				Dir: st.Dir, Conversation: st.Conv,
				Input: u.Input, CacheRead: u.CacheRead, CacheWrite: u.CacheCreate, Output: u.Output, Reasoning: u.Details.Thinking})
		})
		return out, err
	}
}

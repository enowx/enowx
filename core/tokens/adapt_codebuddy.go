package tokens

import (
	"cmp"
	"encoding/json"
	"path/filepath"
)

// CodeBuddy keeps each session as JSON lines in
// ~/.codebuddy/projects/<folder>/<session>.jsonl: each assistant message
// has its cwd, sessionId, timestamp (Unix ms), message.usage
// (input_tokens with the cached ones in, cache_read_input_tokens,
// cache_creation_input_tokens, output_tokens) and providerData (the
// model, the messageId, rawUsage with the reasoning tokens).

func codebuddyFiles(env Env) []string {
	return walk(filepath.Join(env.Home, ".codebuddy", "projects"), ".jsonl")
}

type codebuddyLine struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Role      string          `json:"role"`
	Timestamp json.RawMessage `json:"timestamp"`
	Cwd       string          `json:"cwd"`
	SessionID string          `json:"sessionId"`
	Message   *struct {
		Usage *struct {
			Input       int64 `json:"input_tokens"`
			CacheRead   int64 `json:"cache_read_input_tokens"`
			CacheCreate int64 `json:"cache_creation_input_tokens"`
			Output      int64 `json:"output_tokens"`
		} `json:"usage"`
	} `json:"message"`
	ProviderData *struct {
		MessageID string `json:"messageId"`
		Model     string `json:"model"`
		RawUsage  *struct {
			Completion struct {
				Reasoning int64 `json:"reasoning_tokens"`
			} `json:"completion_tokens_details"`
		} `json:"rawUsage"`
	} `json:"providerData"`
}

func readCodeBuddy(path string, st *FileState) ([]Call, error) {
	var out []Call
	err := readLines(path, st, func(line []byte) {
		var l codebuddyLine
		if json.Unmarshal(line, &l) != nil {
			return
		}
		st.Dir, st.Conv = cmp.Or(l.Cwd, st.Dir), cmp.Or(l.SessionID, st.Conv)
		if l.Role != "assistant" || l.Message == nil || l.Message.Usage == nil {
			return
		}
		u := l.Message.Usage
		c := Call{Time: anyTime(l.Timestamp), Dir: st.Dir, Conversation: st.Conv, Model: st.Model,
			Input: max(u.Input-u.CacheRead-u.CacheCreate, 0), CacheRead: u.CacheRead, CacheWrite: u.CacheCreate, Output: u.Output}
		id := l.ID
		if pd := l.ProviderData; pd != nil {
			id = cmp.Or(pd.MessageID, id)
			c.Model = cmp.Or(pd.Model, c.Model)
			st.Model = c.Model
			if pd.RawUsage != nil {
				c.Reasoning = pd.RawUsage.Completion.Reasoning
			}
		}
		if id == "" {
			return
		}
		c.ID = "codebuddy:" + id
		out = append(out, c)
	})
	return out, err
}

package tokens

import (
	"cmp"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Pi keeps each session as JSON lines in
// ~/.pi/agent/sessions/<cwd>/<time>_<id>.jsonl ($PI_CODING_AGENT_DIR
// over ~/.pi/agent): a session header (its id, cwd), then entries, each
// assistant message with its provider, model and usage (input without
// the cache, output with the reasoning in, cacheRead, cacheWrite, and
// the cost Pi worked out). OMP (~/.omp/agent/sessions), Prime Agent and
// OpenClaw keep the same format.

func piFiles(env Env) []string {
	root := os.Getenv("PI_CODING_AGENT_DIR")
	if root == "" {
		root = filepath.Join(env.Home, ".pi", "agent")
	}
	return walk(filepath.Join(root, "sessions"), ".jsonl")
}

func ompFiles(env Env) []string {
	return walk(filepath.Join(env.Home, ".omp", "agent", "sessions"), ".jsonl")
}

type piLine struct {
	Type      string          `json:"type"`
	ID        string          `json:"id"`
	Cwd       string          `json:"cwd"`
	Timestamp json.RawMessage `json:"timestamp"`
	Provider  string          `json:"provider"`
	ModelID   string          `json:"modelId"`
	Message   *struct {
		Role       string          `json:"role"`
		Provider   string          `json:"provider"`
		Model      string          `json:"model"`
		ResponseID string          `json:"responseId"`
		Timestamp  json.RawMessage `json:"timestamp"`
		Usage      *struct {
			Input      int64 `json:"input"`
			Output     int64 `json:"output"`
			CacheRead  int64 `json:"cacheRead"`
			CacheWrite int64 `json:"cacheWrite"`
			Reasoning  int64 `json:"reasoning"`
			Cost       struct {
				Total float64 `json:"total"`
			} `json:"cost"`
		} `json:"usage"`
	} `json:"message"`
}

func readPi(path string, st *FileState) ([]Call, error) {
	var out []Call
	err := readLines(path, st, func(line []byte) {
		if c, ok := piCall(line, st, filepath.Base(path)); ok {
			out = append(out, c)
		}
	})
	return out, err
}

// piCall reads one line of a Pi session: the header and model changes
// into st, an assistant message's usage as a call.
func piCall(line []byte, st *FileState, file string) (Call, bool) {
	var l piLine
	if json.Unmarshal(line, &l) != nil {
		return Call{}, false
	}
	switch l.Type {
	case "session":
		st.Conv, st.Dir = cmp.Or(l.ID, st.Conv), cmp.Or(l.Cwd, st.Dir)
		st.Provider, st.Model = cmp.Or(l.Provider, st.Provider), cmp.Or(l.ModelID, st.Model)
		return Call{}, false
	case "model_change":
		st.Provider, st.Model = cmp.Or(l.Provider, st.Provider), cmp.Or(l.ModelID, st.Model)
		return Call{}, false
	case "message":
	default:
		return Call{}, false
	}
	m := l.Message
	if m == nil || m.Role != "assistant" || m.Usage == nil || l.ID == "" {
		return Call{}, false
	}
	at := anyTime(l.Timestamp)
	if at.IsZero() {
		at = anyTime(m.Timestamp)
	}
	u := m.Usage
	return Call{ID: "pi:" + cmp.Or(st.Conv, file) + ":" + l.ID, Time: at,
		Provider: providerName(cmp.Or(m.Provider, st.Provider)), Model: cmp.Or(m.Model, st.Model),
		Dir: st.Dir, Conversation: st.Conv, Cost: u.Cost.Total,
		Input: u.Input, CacheRead: u.CacheRead, CacheWrite: u.CacheWrite, Output: u.Output, Reasoning: u.Reasoning}, true
}

// anyTime reads a time given as RFC 3339 or as Unix milliseconds.
func anyTime(raw json.RawMessage) time.Time {
	if len(raw) == 0 {
		return time.Time{}
	}
	var t time.Time
	if json.Unmarshal(raw, &t) == nil {
		return t
	}
	var ms float64
	if json.Unmarshal(raw, &ms) == nil && ms > 0 {
		if ms < 1e11 { // seconds
			return time.Unix(int64(ms), 0)
		}
		return time.UnixMilli(int64(ms))
	}
	return time.Time{}
}

// providerName is a provider as the price table names it, from what an
// agent calls it ("openai-codex", "google-gemini-cli", "github-copilot").
func providerName(p string) string {
	switch {
	case p == "":
		return ""
	case contains(p, "anthropic"):
		return "anthropic"
	case contains(p, "openai"), contains(p, "codex"):
		return "openai"
	case contains(p, "google"), contains(p, "gemini"), contains(p, "vertex"):
		return "google"
	case contains(p, "openrouter"):
		return "openrouter"
	case contains(p, "ollama"), contains(p, "lmstudio"), contains(p, "llama"):
		return "local"
	}
	return p
}

func contains(s, sub string) bool { return strings.Contains(strings.ToLower(s), sub) }

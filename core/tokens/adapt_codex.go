package tokens

import (
	"cmp"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// Codex keeps each session as JSON lines in
// $CODEX_HOME/sessions/YYYY/MM/DD/rollout-*.jsonl: session_meta (its id,
// the cwd), turn_context (the model), and after each model call an
// event_msg token_count whose info has last_token_usage (that call) and
// total_token_usage (the session's so far): input_tokens (the cached
// ones included), cached_input_tokens, cache_write_input_tokens,
// output_tokens (reasoning included), reasoning_output_tokens. Its
// rate_limits say the plan (free, plus, pro…) when signed in to one.

func codexFiles(env Env) []string {
	home := os.Getenv("CODEX_HOME")
	if home == "" {
		home = filepath.Join(env.Home, ".codex")
	}
	return walk(filepath.Join(home, "sessions"), ".jsonl")
}

type codexUsage struct {
	Input      int64 `json:"input_tokens"`
	Cached     int64 `json:"cached_input_tokens"`
	CacheWrite int64 `json:"cache_write_input_tokens"`
	Output     int64 `json:"output_tokens"`
	Reasoning  int64 `json:"reasoning_output_tokens"`
	Total      int64 `json:"total_tokens"`
}

type codexLine struct {
	Timestamp time.Time       `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

type codexPayload struct {
	Type  string `json:"type"`
	ID    string `json:"id"`
	Cwd   string `json:"cwd"`
	Model string `json:"model"`
	Info  *struct {
		Total codexUsage `json:"total_token_usage"`
		Last  codexUsage `json:"last_token_usage"`
	} `json:"info"`
	RateLimits *struct {
		Plan string `json:"plan_type"`
	} `json:"rate_limits"`
}

func readCodex(path string, st *FileState) ([]Call, error) {
	var out []Call
	err := readLines(path, st, func(line []byte) {
		var l codexLine
		if json.Unmarshal(line, &l) != nil {
			return
		}
		var p codexPayload
		if json.Unmarshal(l.Payload, &p) != nil {
			return
		}
		switch l.Type {
		case "session_meta":
			st.Conv, st.Dir = cmp.Or(p.ID, st.Conv), cmp.Or(p.Cwd, st.Dir)
		case "turn_context":
			st.Model, st.Dir = cmp.Or(p.Model, st.Model), cmp.Or(p.Cwd, st.Dir)
		case "event_msg":
			if p.Type != "token_count" || p.Info == nil || p.Info.Last.Total == 0 {
				return
			}
			u := p.Info.Last
			// The session's running total names the call: Codex says a
			// count again without a call in between.
			id := "codex:" + cmp.Or(st.Conv, filepath.Base(path)) + ":" + strconv.FormatInt(p.Info.Total.Total, 10)
			plan := p.RateLimits != nil && p.RateLimits.Plan != "" && p.RateLimits.Plan != "api"
			out = append(out, Call{ID: id, Time: l.Timestamp, Model: st.Model, Dir: st.Dir, Conversation: st.Conv, Plan: plan,
				Input: max(u.Input-u.Cached-u.CacheWrite, 0), CacheRead: u.Cached, CacheWrite: u.CacheWrite, Output: u.Output, Reasoning: u.Reasoning})
		}
	})
	return out, err
}

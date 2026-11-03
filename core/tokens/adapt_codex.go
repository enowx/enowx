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
// rate_limits say the plan (free, plus, pro…) when signed in to one. A record may hold
// rate limits alone, or (in older builds) only the running total. Resuming writes a new
// file with the same id, whose history_base points at the old one's end and copies
// nothing (all 4 of the 23 rollouts on this Mac, November 2026). None there was a fork
// to another id, so what a fork writes is taken from its fields, not from a real one.

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
	// ForkedFromID and HistoryBase name where a session's history came from.
	ForkedFromID string `json:"forked_from_id"`
	HistoryBase  *struct {
		ThreadID string `json:"thread_id"`
	} `json:"history_base"`
	Info *struct {
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
			if p.forkedFromAnother() {
				st.ForkAt = l.Timestamp
			}
		case "turn_context":
			st.Model, st.Dir = cmp.Or(p.Model, st.Model), cmp.Or(p.Cwd, st.Dir)
		case "event_msg":
			if p.Type != "token_count" || p.Info == nil {
				return // rate limits alone
			}
			if !st.ForkAt.IsZero() && l.Timestamp.Before(st.ForkAt) {
				st.Running = &p.Info.Total // a fork's copy of its parent's call
				return
			}
			u, ok := codexCallUsage(p.Info.Last, p.Info.Total, st)
			if !ok {
				return
			}
			// The session's running total names the call: Codex says a count again
			// without a call in between, also on resuming in a new file. Identical
			// counts in two sessions are two calls, so the conversation stays in it.
			name := strconv.FormatInt(p.Info.Total.Total, 10)
			if p.Info.Total.Total == 0 {
				name = "t" + strconv.FormatInt(l.Timestamp.UnixNano(), 10) // no running total to go by
			}
			id := "codex:" + cmp.Or(st.Conv, filepath.Base(path)) + ":" + name
			plan := p.RateLimits != nil && p.RateLimits.Plan != "" && p.RateLimits.Plan != "api"
			cached := min(u.Cached, u.Input) // what is cached is part of the input
			out = append(out, Call{ID: id, Time: l.Timestamp, Model: st.Model, Dir: st.Dir, Conversation: st.Conv, Plan: plan,
				Input: max(u.Input-cached-u.CacheWrite, 0), CacheRead: cached, CacheWrite: u.CacheWrite, Output: u.Output, Reasoning: u.Reasoning})
		}
	})
	return out, err
}

// forkedFromAnother says a session began as a fork of another. Resuming writes a new
// file too, its history_base naming its own conversation, and copies nothing.
func (p codexPayload) forkedFromAnother() bool {
	if p.ForkedFromID != "" && p.ForkedFromID != p.ID {
		return true
	}
	return p.HistoryBase != nil && p.HistoryBase.ThreadID != "" && p.HistoryBase.ThreadID != p.ID
}

// codexCallUsage is a token_count's call: what the record says of it, else the
// difference from the running total before it. The first running total of a file is
// where it starts from, not use. False when it holds no new use.
func codexCallUsage(last, total codexUsage, st *FileState) (codexUsage, bool) {
	prev := st.Running
	if total.Total > 0 || total.Input > 0 {
		st.Running = &total
	}
	if last.Total > 0 || last.Input > 0 || last.Output > 0 {
		return last, true
	}
	if prev == nil || total.Total <= prev.Total {
		return codexUsage{}, false
	}
	return codexUsage{
		Input: max(total.Input-prev.Input, 0), Cached: max(total.Cached-prev.Cached, 0), CacheWrite: max(total.CacheWrite-prev.CacheWrite, 0),
		Output: max(total.Output-prev.Output, 0), Reasoning: max(total.Reasoning-prev.Reasoning, 0), Total: total.Total - prev.Total,
	}, true
}

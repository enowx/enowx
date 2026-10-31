package tokens

import (
	"cmp"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Agents keeping a session as one JSON document, read whole whenever it
// changes (the store counting each call once):
//
//   - Cline: ~/.cline/data/sessions/<id>/<id>.messages.json, each
//     assistant message's metrics (inputTokens, outputTokens,
//     cacheReadTokens, cacheWriteTokens, cost) and modelInfo, its
//     manifest <id>.json the cwd; the older tasks/<id>/ui_messages.json
//     an api_req_started entry a call (tokensIn, tokensOut, cacheReads,
//     cacheWrites, cost) ($CLINE_DIR over ~/.cline).
//   - Continue's cn: ~/.continue/sessions/<id>.json, history[].message.
//     usage (promptTokens, completionTokens, promptTokensDetails) and
//     the workspaceDirectory ($CONTINUE_GLOBAL_DIR over ~/.continue).
//   - Amp (older builds; threads are now on Amp's servers):
//     ~/.local/share/amp/threads/T-*.json, usageLedger.events.
//   - Auggie: ~/.augment/sessions/<id>.json, chatHistory[].exchange's
//     response_nodes' token_usage, the last of a finished turn.
//   - Kiro CLI: ~/.kiro/sessions/cli/<id>.json, session_state.
//     conversation_metadata.user_turn_metadatas (often zeros: Kiro
//     meters credits).
//   - Jcode: ~/.jcode/sessions/session_*.json, messages' token_usage.

func readJSON(path string, v any) bool {
	data, err := os.ReadFile(path)
	return err == nil && json.Unmarshal(data, v) == nil
}

// splitInput is input without the cache when an agent's count may or
// may not include it (it depends on the provider): a count smaller than
// the cache read can't include it.
func splitInput(input, cacheRead, cacheWrite int64) int64 {
	if input >= cacheRead+cacheWrite && cacheRead+cacheWrite > 0 {
		return input - cacheRead - cacheWrite
	}
	return input
}

func clineFiles(env Env) []string {
	root := cmp.Or(os.Getenv("CLINE_DIR"), filepath.Join(env.Home, ".cline"))
	var out []string
	for _, f := range walk(filepath.Join(root, "data", "sessions"), ".json") {
		if strings.HasSuffix(f, ".messages.json") {
			out = append(out, f)
		}
	}
	for _, f := range walk(filepath.Join(root, "data", "tasks"), ".json") {
		if filepath.Base(f) == "ui_messages.json" {
			out = append(out, f)
		}
	}
	return out
}

func readCline(path string, _ *FileState) ([]Call, error) {
	if filepath.Base(path) == "ui_messages.json" {
		return readClineTask(path), nil
	}
	id := strings.TrimSuffix(filepath.Base(path), ".messages.json")
	var manifest struct {
		Cwd      string `json:"cwd"`
		Model    string `json:"model"`
		Provider string `json:"provider"`
	}
	readJSON(filepath.Join(filepath.Dir(path), id+".json"), &manifest)
	var msgs []struct {
		ID        string          `json:"id"`
		Role      string          `json:"role"`
		TS        json.RawMessage `json:"ts"`
		ModelInfo struct {
			ID       string `json:"id"`
			Provider string `json:"provider"`
		} `json:"modelInfo"`
		Metrics *struct {
			Input      int64   `json:"inputTokens"`
			Output     int64   `json:"outputTokens"`
			CacheRead  int64   `json:"cacheReadTokens"`
			CacheWrite int64   `json:"cacheWriteTokens"`
			Reasoning  int64   `json:"reasoningTokenCount"`
			Cost       float64 `json:"cost"`
		} `json:"metrics"`
	}
	if !readJSON(path, &msgs) {
		return nil, nil
	}
	var out []Call
	for i, m := range msgs {
		if m.Role != "assistant" || m.Metrics == nil {
			continue
		}
		u := m.Metrics
		out = append(out, Call{ID: "cline:" + id + ":" + cmp.Or(m.ID, strconv.Itoa(i)), Time: anyTime(m.TS),
			Model: cmp.Or(m.ModelInfo.ID, manifest.Model), Provider: providerName(cmp.Or(m.ModelInfo.Provider, manifest.Provider)),
			Dir: manifest.Cwd, Conversation: id, Cost: u.Cost,
			Input: splitInput(u.Input, u.CacheRead, u.CacheWrite), CacheRead: u.CacheRead, CacheWrite: u.CacheWrite, Output: u.Output, Reasoning: u.Reasoning})
	}
	return out, nil
}

func readClineTask(path string) []Call {
	task := filepath.Base(filepath.Dir(path))
	var msgs []struct {
		TS   int64  `json:"ts"`
		Say  string `json:"say"`
		Text string `json:"text"`
	}
	if !readJSON(path, &msgs) {
		return nil
	}
	var out []Call
	for _, m := range msgs {
		if m.Say != "api_req_started" {
			continue
		}
		var r struct {
			In         int64   `json:"tokensIn"`
			Out        int64   `json:"tokensOut"`
			CacheRead  int64   `json:"cacheReads"`
			CacheWrite int64   `json:"cacheWrites"`
			Cost       float64 `json:"cost"`
		}
		if json.Unmarshal([]byte(m.Text), &r) != nil || r.In+r.Out == 0 {
			continue
		}
		out = append(out, Call{ID: "cline:" + task + ":" + strconv.FormatInt(m.TS, 10), Time: time.UnixMilli(m.TS), Conversation: task, Cost: r.Cost,
			Input: r.In, CacheRead: r.CacheRead, CacheWrite: r.CacheWrite, Output: r.Out})
	}
	return out
}

func continueFiles(env Env) []string {
	root := cmp.Or(os.Getenv("CONTINUE_GLOBAL_DIR"), filepath.Join(env.Home, ".continue"))
	return walk(filepath.Join(root, "sessions"), ".json")
}

func readContinue(path string, _ *FileState) ([]Call, error) {
	var doc struct {
		SessionID string `json:"sessionId"`
		Dir       string `json:"workspaceDirectory"`
		Model     string `json:"chatModelTitle"`
		History   []struct {
			Message struct {
				Role  string `json:"role"`
				Usage *struct {
					Prompt     int64 `json:"promptTokens"`
					Completion int64 `json:"completionTokens"`
					Details    struct {
						Cached     int64 `json:"cachedTokens"`
						CacheWrite int64 `json:"cacheWriteTokens"`
					} `json:"promptTokensDetails"`
				} `json:"usage"`
			} `json:"message"`
		} `json:"history"`
	}
	if !readJSON(path, &doc) {
		return nil, nil
	}
	at := time.Time{}
	if info, err := os.Stat(path); err == nil {
		at = info.ModTime() // the history keeps no times: the session's last change
	}
	id := cmp.Or(doc.SessionID, strings.TrimSuffix(filepath.Base(path), ".json"))
	var out []Call
	for i, h := range doc.History {
		u := h.Message.Usage
		if u == nil {
			continue
		}
		out = append(out, Call{ID: "continue:" + id + ":" + strconv.Itoa(i), Time: at, Model: doc.Model, Dir: doc.Dir, Conversation: id,
			Input: splitInput(u.Prompt, u.Details.Cached, u.Details.CacheWrite), CacheRead: u.Details.Cached, CacheWrite: u.Details.CacheWrite, Output: u.Completion})
	}
	return out, nil
}

func ampFiles(env Env) []string {
	return walk(filepath.Join(env.Home, ".local", "share", "amp", "threads"), ".json")
}

func readAmp(path string, _ *FileState) ([]Call, error) {
	var doc struct {
		ID          string `json:"id"`
		UsageLedger struct {
			Events []struct {
				ID        string          `json:"id"`
				Timestamp json.RawMessage `json:"timestamp"`
				Model     string          `json:"model"`
				ToMessage string          `json:"toMessageId"`
				Tokens    struct {
					Input       int64 `json:"input"`
					Output      int64 `json:"output"`
					CacheRead   int64 `json:"cacheReadInputTokens"`
					CacheCreate int64 `json:"cacheCreationInputTokens"`
				} `json:"tokens"`
			} `json:"events"`
		} `json:"usageLedger"`
	}
	if !readJSON(path, &doc) {
		return nil, nil
	}
	thread := cmp.Or(doc.ID, strings.TrimSuffix(filepath.Base(path), ".json"))
	var out []Call
	for i, e := range doc.UsageLedger.Events {
		t := e.Tokens
		out = append(out, Call{ID: "amp:" + thread + ":" + cmp.Or(e.ID, e.ToMessage, strconv.Itoa(i)), Time: anyTime(e.Timestamp), Model: e.Model, Conversation: thread,
			Input: t.Input, CacheRead: t.CacheRead, CacheWrite: t.CacheCreate, Output: t.Output})
	}
	return out, nil
}

func auggieFiles(env Env) []string {
	return walk(filepath.Join(env.Home, ".augment", "sessions"), ".json")
}

func readAuggie(path string, _ *FileState) ([]Call, error) {
	type usage struct {
		Input       int64 `json:"input_tokens"`
		Output      int64 `json:"output_tokens"`
		CacheRead   int64 `json:"cache_read_input_tokens"`
		CacheCreate int64 `json:"cache_creation_input_tokens"`
	}
	var doc struct {
		SessionID   string `json:"sessionId"`
		ChatHistory []struct {
			Completed  bool            `json:"completed"`
			FinishedAt json.RawMessage `json:"finishedAt"`
			SequenceID json.RawMessage `json:"sequenceId"`
			Exchange   struct {
				Model     string `json:"model_id"`
				RequestID string `json:"request_id"`
				Nodes     []struct {
					Usage *usage `json:"token_usage"`
				} `json:"response_nodes"`
			} `json:"exchange"`
		} `json:"chatHistory"`
	}
	if !readJSON(path, &doc) {
		return nil, nil
	}
	session := cmp.Or(doc.SessionID, strings.TrimSuffix(filepath.Base(path), ".json"))
	var out []Call
	for i, turn := range doc.ChatHistory {
		if !turn.Completed {
			continue
		}
		var last *usage
		for _, n := range turn.Exchange.Nodes {
			if n.Usage != nil && n.Usage.Input+n.Usage.Output > 0 {
				last = n.Usage
			}
		}
		if last == nil {
			continue
		}
		out = append(out, Call{ID: "auggie:" + cmp.Or(turn.Exchange.RequestID, session+":"+strconv.Itoa(i)), Time: anyTime(turn.FinishedAt),
			Model: turn.Exchange.Model, Conversation: session,
			Input: last.Input, CacheRead: last.CacheRead, CacheWrite: last.CacheCreate, Output: last.Output})
	}
	return out, nil
}

func kiroFiles(env Env) []string {
	return walk(filepath.Join(env.Home, ".kiro", "sessions", "cli"), ".json")
}

func readKiro(path string, _ *FileState) ([]Call, error) {
	var doc struct {
		Cwd   string `json:"cwd"`
		State struct {
			ID       string `json:"conversation_id"`
			Metadata struct {
				Turns []struct {
					Input      int64           `json:"input_token_count"`
					Output     int64           `json:"output_token_count"`
					CacheRead  int64           `json:"cache_read_input_token_count"`
					CacheWrite int64           `json:"cache_write_input_token_count"`
					MessageIDs []string        `json:"message_ids"`
					End        json.RawMessage `json:"end_timestamp"`
				} `json:"user_turn_metadatas"`
			} `json:"conversation_metadata"`
			Model struct {
				Info *struct {
					ID string `json:"model_id"`
				} `json:"model_info"`
			} `json:"rts_model_state"`
		} `json:"session_state"`
	}
	if !readJSON(path, &doc) {
		return nil, nil
	}
	session := cmp.Or(doc.State.ID, strings.TrimSuffix(filepath.Base(path), ".json"))
	model := ""
	if doc.State.Model.Info != nil {
		model = doc.State.Model.Info.ID
	}
	var out []Call
	for i, t := range doc.State.Metadata.Turns {
		id := strconv.Itoa(i)
		if len(t.MessageIDs) > 0 {
			id = t.MessageIDs[len(t.MessageIDs)-1]
		}
		out = append(out, Call{ID: "kiro:" + session + ":" + id, Time: anyTime(t.End), Model: model, Dir: doc.Cwd, Conversation: session,
			Input: t.Input, CacheRead: t.CacheRead, CacheWrite: t.CacheWrite, Output: t.Output})
	}
	return out, nil
}

func jcodeFiles(env Env) []string {
	return walk(filepath.Join(env.Home, ".jcode", "sessions"), ".json")
}

func readJcode(path string, _ *FileState) ([]Call, error) {
	var doc struct {
		ID       string `json:"id"`
		Provider string `json:"provider_key"`
		Model    string `json:"model"`
		Dir      string `json:"working_dir"`
		Messages []struct {
			ID        string          `json:"id"`
			Role      string          `json:"role"`
			Timestamp json.RawMessage `json:"timestamp"`
			Usage     *struct {
				Input       int64 `json:"input_tokens"`
				Output      int64 `json:"output_tokens"`
				CacheRead   int64 `json:"cache_read_input_tokens"`
				CacheCreate int64 `json:"cache_creation_input_tokens"`
				Reasoning   int64 `json:"reasoning_output_tokens"`
			} `json:"token_usage"`
		} `json:"messages"`
	}
	if !readJSON(path, &doc) {
		return nil, nil
	}
	session := cmp.Or(doc.ID, strings.TrimSuffix(filepath.Base(path), ".json"))
	var out []Call
	for i, m := range doc.Messages {
		if m.Usage == nil {
			continue
		}
		u := m.Usage
		out = append(out, Call{ID: "jcode:" + session + ":" + cmp.Or(m.ID, strconv.Itoa(i)), Time: anyTime(m.Timestamp), Model: doc.Model,
			Provider: providerName(doc.Provider), Dir: doc.Dir, Conversation: session,
			Input: splitInput(u.Input, u.CacheRead, u.CacheCreate), CacheRead: u.CacheRead, CacheWrite: u.CacheCreate, Output: u.Output, Reasoning: u.Reasoning})
	}
	return out, nil
}

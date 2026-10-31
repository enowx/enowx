package tokens

import (
	"bufio"
	"bytes"
	"cmp"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"
)

// Agents whose logs carry a call's usage among other events, read by
// the event's name:
//
//   - Kimi CLI: ~/.kimi/sessions/<md5 of the folder>/<id>/wire.jsonl
//     ($KIMI_SHARE_DIR over ~/.kimi), StatusUpdate's token_usage
//     (input_other, without the cache; output; input_cache_read;
//     input_cache_creation) and message_id; ~/.kimi/kimi.json's
//     work_dirs name the folders.
//   - Mistral Vibe 3: ~/.vibe/logs/session/unified/<id>/journal/*.jsonl
//     ($VIBE_HOME over ~/.vibe), completion_succeeded's result.usage
//     (input_tokens, the cached ones in; output_tokens;
//     cached_input_tokens; cache_write_tokens; reasoning_tokens).
//   - Grok Build: ~/.grok/logs/unified.jsonl, shell.turn.inference_done's
//     ctx (prompt_tokens, cached in; cached_prompt_tokens;
//     completion_tokens; reasoning_tokens).
//   - Muse: ~/.local/share/muse/sessions/YYYY/MM/DD/<id>/session.jsonl,
//     model_completed events' model and usage.
//   - Command Code: ~/.commandcode/projects/<folder>/<session>.jsonl,
//     messages' usage (inputTokens, without the cache; outputTokens;
//     cacheReadTokens; cacheWriteTokens; costUsd) and model.
//   - DeepSeek Harness: ~/.dsh/sessions/<folder>/<id>/session.jsonl,
//     zstd-compressed (.zstd) by default ($DSH_HOME over ~/.dsh):
//     assistant messages' data.usage and model, the session row its cwd.

// events walks a JSON value's objects.
func events(v any, fn func(m map[string]any)) {
	switch t := v.(type) {
	case map[string]any:
		fn(t)
		for _, x := range t {
			events(x, fn)
		}
	case []any:
		for _, x := range t {
			events(x, fn)
		}
	}
}

// num is the first of keys m holds as a number.
func num(m map[string]any, keys ...string) int64 {
	for _, k := range keys {
		if f, ok := m[k].(float64); ok {
			return int64(f)
		}
	}
	return 0
}

func str(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s, ok := m[k].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

// named reports whether m is an event named name (by type, kind, event
// or msg).
func named(m map[string]any, name string) bool {
	for _, k := range []string{"type", "kind", "event", "msg"} {
		if s, _ := m[k].(string); s == name {
			return true
		}
	}
	return false
}

func kimiFiles(env Env) []string {
	root := cmp.Or(os.Getenv("KIMI_SHARE_DIR"), filepath.Join(env.Home, ".kimi"))
	var out []string
	for _, f := range walk(filepath.Join(root, "sessions"), ".jsonl") {
		if filepath.Base(f) == "wire.jsonl" {
			out = append(out, f)
		}
	}
	return out
}

// kimiDir is the folder a Kimi session ran in: the work dir of
// kimi.json whose md5 names its sessions' folder.
func kimiDir(path string) string {
	hash := filepath.Base(filepath.Dir(filepath.Dir(path)))
	var cfg struct {
		WorkDirs []struct {
			Path string `json:"path"`
		} `json:"work_dirs"`
	}
	readJSON(filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(path)))), "kimi.json"), &cfg)
	for _, w := range cfg.WorkDirs {
		sum := md5.Sum([]byte(w.Path))
		if hex.EncodeToString(sum[:]) == hash {
			return w.Path
		}
	}
	return ""
}

func readKimi(path string, st *FileState) ([]Call, error) {
	if st.Dir == "" {
		st.Dir = kimiDir(path)
	}
	session := filepath.Base(filepath.Dir(path))
	var out []Call
	err := readLines(path, st, func(line []byte) {
		var l struct {
			Timestamp float64 `json:"timestamp"`
			Message   struct {
				Type    string `json:"type"`
				Payload struct {
					MessageID string `json:"message_id"`
					Usage     *struct {
						Input       int64 `json:"input_other"`
						Output      int64 `json:"output"`
						CacheRead   int64 `json:"input_cache_read"`
						CacheCreate int64 `json:"input_cache_creation"`
					} `json:"token_usage"`
				} `json:"payload"`
			} `json:"message"`
		}
		if json.Unmarshal(line, &l) != nil || l.Message.Type != "StatusUpdate" || l.Message.Payload.Usage == nil {
			return
		}
		u := l.Message.Payload.Usage
		at := time.UnixMilli(int64(l.Timestamp * 1000))
		id := cmp.Or(l.Message.Payload.MessageID, strconv.FormatInt(st.Offset, 10))
		out = append(out, Call{ID: "kimi:" + session + ":" + id, Time: at, Provider: "moonshot", Dir: st.Dir, Conversation: session,
			Input: u.Input, CacheRead: u.CacheRead, CacheWrite: u.CacheCreate, Output: u.Output})
	})
	return out, err
}

func vibeFiles(env Env) []string {
	root := cmp.Or(os.Getenv("VIBE_HOME"), filepath.Join(env.Home, ".vibe"))
	return walk(filepath.Join(root, "logs", "session", "unified"), ".jsonl")
}

// readEventLines reads a log's lines for the events named name, each
// made a call by fn (its ID the file's and the line's place).
func readEventLines(prefix, name string, fn func(m map[string]any, c *Call) bool) func(path string, st *FileState) ([]Call, error) {
	return func(path string, st *FileState) ([]Call, error) {
		var out []Call
		err := readLines(path, st, func(line []byte) {
			if !bytes.Contains(line, []byte(name)) {
				return
			}
			var v any
			if json.Unmarshal(line, &v) != nil {
				return
			}
			events(v, func(m map[string]any) {
				if !named(m, name) {
					return
				}
				c := Call{ID: prefix + ":" + path + ":" + strconv.FormatInt(st.Offset, 10)}
				if fn(m, &c) {
					out = append(out, c)
				}
			})
		})
		return out, err
	}
}

func vibeCall(m map[string]any, c *Call) bool {
	res, _ := m["result"].(map[string]any)
	u, _ := res["usage"].(map[string]any)
	if u == nil {
		return false
	}
	cached, write := num(u, "cached_input_tokens"), num(u, "cache_write_tokens")
	c.Input = max(num(u, "input_tokens")-cached-write, 0)
	c.CacheRead, c.CacheWrite, c.Output, c.Reasoning = cached, write, num(u, "output_tokens"), num(u, "reasoning_tokens")
	c.Model = cmp.Or(str(res, "model"), str(m, "model"))
	c.Provider = "mistral"
	c.Time = anyTime(json.RawMessage(strconv.Quote(cmp.Or(str(m, "timestamp", "time", "ts"), ""))))
	return true
}

func grokLogCall(m map[string]any, c *Call) bool {
	ctx, _ := m["ctx"].(map[string]any)
	if ctx == nil {
		return false
	}
	cached := num(ctx, "cached_prompt_tokens")
	c.Input, c.CacheRead = max(num(ctx, "prompt_tokens")-cached, 0), cached
	c.Output, c.Reasoning = num(ctx, "completion_tokens")+num(ctx, "reasoning_tokens"), num(ctx, "reasoning_tokens")
	c.Provider, c.Conversation = "xai", str(m, "sid")
	if ts, ok := m["ts"]; ok {
		raw, _ := json.Marshal(ts)
		c.Time = anyTime(raw)
	}
	return true
}

func museFiles(env Env) []string {
	var out []string
	for _, f := range walk(filepath.Join(env.Home, ".local", "share", "muse", "sessions"), ".jsonl") {
		if filepath.Base(f) == "session.jsonl" {
			out = append(out, f)
		}
	}
	return out
}

func museCall(m map[string]any, c *Call) bool {
	u, _ := m["usage"].(map[string]any)
	if u == nil {
		return false
	}
	cr := num(u, "cache_read_tokens", "cached_tokens")
	cw := num(u, "cache_write_tokens")
	c.Input, c.CacheRead, c.CacheWrite = splitInput(num(u, "input_tokens"), cr, cw), cr, cw
	c.Output, c.Reasoning, c.Model = num(u, "output_tokens"), num(u, "reasoning_tokens"), str(m, "model")
	return true
}

func commandCodeFiles(env Env) []string {
	return walk(filepath.Join(env.Home, ".commandcode", "projects"), ".jsonl")
}

func readCommandCode(path string, st *FileState) ([]Call, error) {
	session := strings.TrimSuffix(filepath.Base(path), ".jsonl")
	var out []Call
	err := readLines(path, st, func(line []byte) {
		var l struct {
			Type      string          `json:"type"`
			ID        string          `json:"id"`
			Timestamp json.RawMessage `json:"timestamp"`
			Model     string          `json:"model"`
			Usage     *struct {
				Input      int64   `json:"inputTokens"`
				Output     int64   `json:"outputTokens"`
				CacheRead  int64   `json:"cacheReadTokens"`
				CacheWrite int64   `json:"cacheWriteTokens"`
				Cost       float64 `json:"costUsd"`
			} `json:"usage"`
		}
		if json.Unmarshal(line, &l) != nil || l.Type != "message" || l.Usage == nil || l.ID == "" {
			return
		}
		u := l.Usage
		provider, model, ok := strings.Cut(l.Model, "/")
		if !ok {
			provider, model = "", l.Model
		}
		out = append(out, Call{ID: "commandcode:" + l.ID, Time: anyTime(l.Timestamp), Model: model, Provider: providerName(provider),
			Conversation: session, Cost: u.Cost, Input: u.Input, CacheRead: u.CacheRead, CacheWrite: u.CacheWrite, Output: u.Output})
	})
	return out, err
}

func dshFiles(env Env) []string {
	root := cmp.Or(os.Getenv("DSH_HOME"), filepath.Join(env.Home, ".dsh"))
	return append(walk(filepath.Join(root, "sessions"), ".jsonl"), walk(filepath.Join(root, "sessions"), ".zstd")...)
}

// readDSH reads a DeepSeek Harness session whole: compressed, it can't be
// read from where it was left.
func readDSH(path string, _ *FileState) ([]Call, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var sc *bufio.Scanner
	if strings.HasSuffix(path, ".zstd") {
		dec, err := zstd.NewReader(f)
		if err != nil {
			return nil, err
		}
		defer dec.Close()
		sc = bufio.NewScanner(dec)
	} else {
		sc = bufio.NewScanner(f)
	}
	sc.Buffer(make([]byte, 64<<10), 16<<20)
	session, dir := filepath.Base(filepath.Dir(path)), ""
	var out []Call
	for n := 0; sc.Scan(); n++ {
		var m map[string]any
		if json.Unmarshal(sc.Bytes(), &m) != nil {
			continue
		}
		data, _ := m["data"].(map[string]any)
		if named(m, "session") {
			dir = cmp.Or(str(m, "cwd"), str(data, "cwd"), dir)
			continue
		}
		u, _ := data["usage"].(map[string]any)
		if u == nil {
			continue
		}
		model := ""
		events(data["message"], func(x map[string]any) { model = cmp.Or(model, str(x, "model")) })
		raw, _ := json.Marshal(cmp.Or(m["timestamp"], m["time"], data["timestamp"]))
		cr, cw := num(u, "cacheReadTokens"), num(u, "cacheWriteTokens")
		out = append(out, Call{ID: "dsh:" + session + ":" + cmp.Or(str(m, "id"), str(data, "id"), strconv.Itoa(n)), Time: anyTime(raw),
			Model: model, Provider: "deepseek", Dir: dir, Conversation: session,
			Input: splitInput(num(u, "inputTokens"), cr, cw), CacheRead: cr, CacheWrite: cw, Output: num(u, "outputTokens")})
	}
	return out, sc.Err()
}

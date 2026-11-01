package daemon

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/transport"
)

// Real Claude Code and Codex on model API configs, against a stand-in API, on a
// throwaway daemon the caller started: its ENOWX_HOME in ENOWX_CONFIGS_REAL_HOME,
// its CLAUDE_CONFIG_DIR and CODEX_HOME that folder's "claude" and "codex", so
// neither agent's own files are touched.
//
//	ENOWX_CONFIGS_REAL_HOME=/some/throwaway go test ./daemon -run RealConfig -timeout 30m

// mockAPI stands in for a model API, speaking Anthropic Messages and OpenAI
// Responses, each answer naming model.
type mockAPI struct {
	*httptest.Server
	model   string
	limited atomic.Bool // answers as an account at its usage limit
	calls   atomic.Int64
	mu      sync.Mutex
	paths   map[string]int
	key     string
	badKey  atomic.Int64
}

func newMockAPI(t *testing.T, model, key string) *mockAPI {
	m := &mockAPI{model: model, key: key, paths: map[string]int{}}
	m.Server = httptest.NewServer(http.HandlerFunc(m.serve))
	t.Cleanup(m.Close)
	return m
}

func (m *mockAPI) serve(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	m.paths[r.Method+" "+r.URL.Path]++
	m.mu.Unlock()
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if token == "" {
		token = r.Header.Get("X-Api-Key")
	}
	if token != m.key && r.Method == http.MethodPost { // Claude Code first checks the address, keyless
		m.badKey.Add(1)
	}
	switch {
	case strings.HasSuffix(r.URL.Path, "/messages/count_tokens"):
		_ = json.NewEncoder(w).Encode(map[string]any{"input_tokens": 10})
	case strings.HasSuffix(r.URL.Path, "/messages"):
		m.calls.Add(1)
		m.messages(w, r)
	case strings.HasSuffix(r.URL.Path, "/responses"):
		m.calls.Add(1)
		m.responses(w)
	case strings.HasSuffix(r.URL.Path, "/models"):
		_ = json.NewEncoder(w).Encode(map[string]any{"object": "list", "data": []any{}, "models": []any{}})
	default:
		_, _ = w.Write([]byte("{}"))
	}
}

func (m *mockAPI) messages(w http.ResponseWriter, r *http.Request) {
	if m.limited.Load() {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"rate_limit_error","message":"You've hit your usage limit. Try again in 2 hours."}}`))
		return
	}
	var body struct {
		Stream bool `json:"stream"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	text := "Hello from " + m.model
	usage := map[string]any{"input_tokens": 12, "output_tokens": 5}
	if !body.Stream {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "msg_mock", "type": "message", "role": "assistant", "model": m.model, "stop_reason": "end_turn",
			"content": []any{map[string]any{"type": "text", "text": text}}, "usage": usage})
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	send := sender(w)
	send("message_start", map[string]any{"type": "message_start", "message": map[string]any{"id": "msg_mock", "type": "message", "role": "assistant", "model": m.model, "content": []any{}, "stop_reason": nil, "usage": usage}})
	send("content_block_start", map[string]any{"type": "content_block_start", "index": 0, "content_block": map[string]any{"type": "text", "text": ""}})
	send("content_block_delta", map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]any{"type": "text_delta", "text": text}})
	send("content_block_stop", map[string]any{"type": "content_block_stop", "index": 0})
	send("message_delta", map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": "end_turn", "stop_sequence": nil}, "usage": map[string]any{"output_tokens": 5}})
	send("message_stop", map[string]any{"type": "message_stop"})
}

func (m *mockAPI) responses(w http.ResponseWriter) {
	if m.limited.Load() {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"type":"usage_limit_reached","code":"usage_limit_reached","message":"You've hit your usage limit. Try again in 2 hours."}}`))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	send := sender(w)
	text := "Hello from " + m.model
	item := func(content []any) map[string]any {
		return map[string]any{"type": "message", "id": "item_mock", "role": "assistant", "status": "completed", "content": content}
	}
	send("response.created", map[string]any{"type": "response.created", "response": map[string]any{"id": "resp_mock", "model": m.model}})
	send("response.output_item.added", map[string]any{"type": "response.output_item.added", "output_index": 0, "item": item([]any{})})
	send("response.output_text.delta", map[string]any{"type": "response.output_text.delta", "item_id": "item_mock", "output_index": 0, "content_index": 0, "delta": text})
	send("response.output_item.done", map[string]any{"type": "response.output_item.done", "output_index": 0, "item": item([]any{map[string]any{"type": "output_text", "text": text}})})
	send("response.completed", map[string]any{"type": "response.completed", "response": map[string]any{"id": "resp_mock", "model": m.model, "output": []any{},
		"usage": map[string]any{"input_tokens": 12, "input_tokens_details": nil, "output_tokens": 5, "output_tokens_details": nil, "total_tokens": 17}}})
}

func sender(w http.ResponseWriter) func(string, map[string]any) {
	return func(event string, data map[string]any) {
		b, _ := json.Marshal(data)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}
}

// realConfigs is the throwaway daemon, reached as an app holding the vault.
type realConfigs struct {
	t    *testing.T
	root string
	key  string
	c    *client.Client
}

func newRealConfigs(t *testing.T) *realConfigs {
	root := os.Getenv("ENOWX_CONFIGS_REAL_HOME")
	if root == "" {
		t.Skip("set ENOWX_CONFIGS_REAL_HOME to a throwaway daemon's ENOWX_HOME")
	}
	path, err := transport.SocketPathIn(root)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatalf("no daemon there: %v", err)
	}
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	r := &realConfigs{t: t, root: root, key: "mock-key-" + hex.EncodeToString(b)}
	var app *client.Client
	app, err = client.New(conn, func(ev proto.Event) {
		var req proto.VaultRequest
		if ev.Name != proto.EventVaultRequest || json.Unmarshal(ev.Data, &req) != nil {
			return
		}
		go func() {
			reply := proto.VaultReply{ID: req.ID}
			if req.Action == proto.VaultEnv {
				reply.Lend = proto.VaultLend{Env: map[string]string{"API_KEY": r.key}, Credentials: []string{req.Name}}
			} else {
				reply.Error = "no"
			}
			_ = app.VaultReply(reply)
		}()
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	if err := app.ServeVault(); err != nil {
		t.Fatal(err)
	}
	r.c = app
	return r
}

// project is a repository the agents take as trusted, as a user would have
// answered their first questions.
func (r *realConfigs) project(name string) string {
	// Of this run's: a session of an earlier run's isn't picked up for its task.
	dir := filepath.Join(r.root, "projects", name+"-"+r.key[len(r.key)-6:])
	if err := os.MkdirAll(dir, 0o700); err != nil {
		r.t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-q"}, {"-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-q", "--allow-empty", "-m", "start"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			r.t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	if _, err := r.c.AddProject(dir); err != nil {
		r.t.Fatal(err)
	}
	r.trust(dir)
	return dir
}

func (r *realConfigs) trust(dir string) {
	claude := filepath.Join(r.root, "claude")
	claudeFile := filepath.Join(claude, ".claude.json")
	state := map[string]any{"hasCompletedOnboarding": true, "lastOnboardingVersion": "9.9.9", "bypassPermissionsModeAccepted": true, "projects": map[string]any{}}
	if data, err := os.ReadFile(claudeFile); err == nil {
		_ = json.Unmarshal(data, &state)
	}
	state["projects"].(map[string]any)[dir] = map[string]any{"hasTrustDialogAccepted": true, "hasCompletedProjectOnboarding": true}
	data, _ := json.Marshal(state)
	if err := os.MkdirAll(claude, 0o700); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(claudeFile, data, 0o600); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(claude, "settings.json"), []byte(`{"skipDangerousModePermissionPrompt":true}`), 0o600); err != nil {
		r.t.Fatal(err)
	}
	codex := filepath.Join(r.root, "codex")
	if err := os.MkdirAll(codex, 0o700); err != nil {
		r.t.Fatal(err)
	}
	f, err := os.OpenFile(filepath.Join(codex, "config.toml"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		r.t.Fatal(err)
	}
	defer f.Close()
	fmt.Fprintf(f, "\n[projects.%q]\ntrust_level = \"trusted\"\n", dir)
}

func (r *realConfigs) config(agentID, name, baseURL, model string) proto.Account {
	name += " " + r.key[len(r.key)-6:]
	cfg, err := r.c.SaveConfig(proto.Account{Agent: agentID, Name: name, Kind: proto.ConfigKindAPI,
		API: &proto.ConfigAPI{BaseURL: baseURL, KeyCred: "mock-" + name, Models: []proto.AgentAPIModel{{ID: model}}}})
	if err != nil {
		r.t.Fatal(err)
	}
	return cfg
}

func (r *realConfigs) info(id string) proto.SessionInfo {
	list, err := r.c.Sessions()
	if err != nil {
		r.t.Fatal(err)
	}
	for _, s := range list {
		if s.ID == id {
			return s
		}
	}
	return proto.SessionInfo{}
}

// waitReal waits up to d, what the session's screen shows said on failure.
func (r *realConfigs) waitReal(what, id string, d time.Duration, ok func() bool) {
	r.t.Helper()
	for deadline := time.Now().Add(d); !ok(); time.Sleep(time.Second) {
		if time.Now().After(deadline) {
			screen, _ := r.c.Screen(id)
			r.t.Fatalf("%s: never. Session %+v\nscreen:\n%s", what, r.info(id), screen)
		}
	}
}

// showScreen logs the end of what session id shows.
func (r *realConfigs) showScreen(what, id string) {
	screen, _ := r.c.Screen(id)
	r.t.Logf("%s, screen of %s:\n%s", what, id, tailOf(screen, 18))
}

func tailOf(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, " \n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// filesHoldingKey are the files under the throwaway folder holding the key.
func (r *realConfigs) filesHoldingKey() []string {
	var hits []string
	_ = filepath.WalkDir(r.root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.Size() > 64<<20 {
			return nil
		}
		if data, err := os.ReadFile(p); err == nil && bytes.Contains(data, []byte(r.key)) {
			rel, _ := filepath.Rel(r.root, p)
			hits = append(hits, rel)
		}
		return nil
	})
	return hits
}

func (r *realConfigs) keyOnACommandLine() bool {
	out, err := exec.Command("ps", "-axww", "-o", "command=").Output()
	if err != nil {
		r.t.Fatal(err)
	}
	return bytes.Contains(out, []byte(r.key))
}

func (r *realConfigs) taskRan(id string) []string {
	var board proto.GOABoard
	if err := r.c.Call(proto.MethodGOABoard, nil, &board); err != nil {
		r.t.Fatal(err)
	}
	for _, t := range board.Tasks {
		if t.ID == id {
			return t.Ran
		}
	}
	return nil
}

// A real agent on a model API config: it asks the API with the vault's key,
// the session and its task show the API's model, and the key is in no file.
func TestRealConfigShowsTheAPIsModel(t *testing.T) {
	r := newRealConfigs(t)
	for _, tc := range []struct{ agent, asked, said, path string }{
		// Claude Code records the model the API answered with; Codex what it asked for.
		{"claude", "mock-asked-claude", "mock-said-claude", ""},
		{"codex", "mock-asked-codex", "mock-said-codex", "/v1"},
	} {
		t.Run(tc.agent, func(t *testing.T) {
			r.t = t
			api := newMockAPI(t, tc.said, r.key)
			cfg := r.config(tc.agent, "Mock "+tc.agent, api.URL+tc.path, tc.asked)
			project := r.project("model-" + tc.agent)
			var task proto.GOATask
			if err := r.c.Call(proto.MethodGOADelegate, proto.GOADelegateParams{Agent: tc.agent, Project: project, Config: cfg.Name, NoRoute: true, Prompt: "Say hello in one word."}, &task); err != nil {
				t.Fatal(err)
			}
			r.waitReal("the mock answering", task.Session, 3*time.Minute, func() bool { return api.calls.Load() > 0 })
			want := tc.said
			if tc.agent == "codex" {
				want = tc.asked
			}
			r.waitReal("the session's model", task.Session, 2*time.Minute, func() bool { return r.info(task.Session).Model == want })
			info := r.info(task.Session)
			r.showScreen("answered", task.Session)
			t.Logf("session: model %q effort %q config %+v", info.Model, info.Effort, info.Config)
			r.waitReal("the task's Ran", task.Session, time.Minute, func() bool { return len(r.taskRan(task.ID)) > 0 })
			t.Logf("task ran: %q", r.taskRan(task.ID))
			if info.Config == nil || info.Config.Name != cfg.Name {
				t.Fatalf("config %+v", info.Config)
			}
			if api.badKey.Load() > 0 {
				t.Errorf("%d calls without the vault's key", api.badKey.Load())
			}
			api.mu.Lock()
			t.Logf("calls: %v", api.paths)
			api.mu.Unlock()
			if hits := r.filesHoldingKey(); len(hits) > 0 {
				t.Errorf("the key is in files: %q", hits)
			}
			if r.keyOnACommandLine() {
				t.Error("the key is on a command line")
			}
		})
	}
}

// A real agent's first config answering "usage limit": what its screen says
// has the daemon go on with the second, and the session show its model.
func TestRealConfigFallsBackAtALimit(t *testing.T) {
	r := newRealConfigs(t)
	for _, tc := range []struct{ agent, path string }{{"claude", ""}, {"codex", "/v1"}} {
		t.Run(tc.agent, func(t *testing.T) {
			r.t = t
			first := newMockAPI(t, "mock-first-"+tc.agent, r.key)
			second := newMockAPI(t, "mock-second-"+tc.agent, r.key)
			a := r.config(tc.agent, "First", first.URL+tc.path, first.model)
			b := r.config(tc.agent, "Second", second.URL+tc.path, second.model)
			if _, err := r.c.SetAccountPrefs(tc.agent, proto.AccountPrefs{Order: []string{a.Name, b.Name}, Fallback: proto.FallbackAuto}); err != nil {
				t.Fatal(err)
			}
			project := r.project("fallback-" + tc.agent)
			first.limited.Store(true)
			info, err := r.c.CreateSession(proto.SessionCreateParams{Agent: tc.agent, Account: a.ID, Dir: project, Cols: 120, Rows: 30, Prompt: "Say hello in one word."})
			if err != nil {
				t.Fatal(err)
			}
			r.waitReal("the first API asked", info.ID, 3*time.Minute, func() bool { return first.calls.Load() > 0 })
			r.waitReal("the fallback's API asked", info.ID, 6*time.Minute, func() bool { return second.calls.Load() > 0 })
			r.showScreen("fell back", info.ID)
			got := r.info(info.ID)
			if got.Config == nil || got.Config.Name != b.Name || got.Config.Model != second.model {
				t.Fatalf("config %+v", got.Config)
			}
			r.waitReal("the session's model", info.ID, 2*time.Minute, func() bool { return r.info(info.ID).Model == second.model })
			t.Logf("first called %d, second %d, model now %q, config %+v", first.calls.Load(), second.calls.Load(), r.info(info.ID).Model, r.info(info.ID).Config)
			log, err := r.c.AccountLog()
			if err != nil {
				t.Fatal(err)
			}
			for _, e := range log {
				if e.Session == info.ID {
					t.Logf("log: %s %s %q", e.Event, e.Account, e.Reason)
				}
			}
			if hits := r.filesHoldingKey(); len(hits) > 0 {
				t.Errorf("the key is in files: %q", hits)
			}
		})
	}
}

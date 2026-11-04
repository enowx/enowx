package platformagent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/enowx/enowx/core/platformagent/record"
	"github.com/enowx/enowx/core/platformagent/tools"
	"github.com/enowx/enowx/internal/standinapi"
)

// fakeKey is no real key: tests only talk to the stand-in.
const fakeKey = "sk-ant-standin-not-a-real-key-0123456789"

// fakeFront allows everything unless told otherwise, and keeps what it is shown.
type fakeFront struct {
	mu         sync.Mutex
	events     []record.Record
	onEvent    func(record.Record)
	permission func(PermissionRequest) (Decision, error)
	asked      []PermissionRequest
	overBudget func(spent, budget float64) bool
	budgetAsks int
}

func (f *fakeFront) Event(r record.Record) {
	f.mu.Lock()
	f.events = append(f.events, r)
	on := f.onEvent
	f.mu.Unlock()
	if on != nil {
		on(r)
	}
}

func (f *fakeFront) Permission(ctx context.Context, req PermissionRequest) (Decision, error) {
	f.mu.Lock()
	f.asked = append(f.asked, req)
	decide := f.permission
	f.mu.Unlock()
	if decide != nil {
		return decide(req)
	}
	return AllowOnce, nil
}

func (f *fakeFront) Questions(ctx context.Context, qs []record.Question) ([]string, error) {
	return make([]string, len(qs)), nil
}

func (f *fakeFront) OverBudget(ctx context.Context, spent, budget float64) (bool, error) {
	f.mu.Lock()
	f.budgetAsks++
	decide := f.overBudget
	f.mu.Unlock()
	return decide != nil && decide(spent, budget), nil
}

func (f *fakeFront) of(kind string) []record.Record {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []record.Record
	for _, r := range f.events {
		if r.Type == kind {
			out = append(out, r)
		}
	}
	return out
}

func (f *fakeFront) notices() string {
	var b strings.Builder
	for _, r := range f.of(record.Notice) {
		b.WriteString(r.Text + "\n")
	}
	return b.String()
}

func (f *fakeFront) turnStop(t *testing.T) string {
	t.Helper()
	ends := f.of(record.TurnEnd)
	if len(ends) == 0 {
		t.Fatal("no TurnEnd")
	}
	return ends[len(ends)-1].Stop
}

// fakeHooks answers ToolBefore with before, and keeps the states it is told.
type fakeHooks struct {
	mu      sync.Mutex
	before  func(tool string) HookAnswer
	prompt  HookAnswer
	follow  []string
	states  []string
	prompts []string
	after   []string
	done    int
}

func (h *fakeHooks) State(s string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.states = append(h.states, s)
}

func (h *fakeHooks) ToolBefore(ctx context.Context, tool string, input []byte) HookAnswer {
	if h.before != nil {
		return h.before(tool)
	}
	return HookAnswer{}
}

func (h *fakeHooks) ToolAfter(ctx context.Context, tool string, input []byte, failed bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.after = append(h.after, tool)
}

func (h *fakeHooks) Prompt(ctx context.Context, text string) HookAnswer {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.prompts = append(h.prompts, text)
	return h.prompt
}

func (h *fakeHooks) TurnDone(ctx context.Context) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.done++
	if len(h.follow) == 0 {
		return ""
	}
	more := h.follow[0]
	h.follow = h.follow[1:]
	return more
}

// fakeTool does what run says; a nil run says "ran <name>".
type fakeTool struct {
	name     string
	readOnly bool
	schema   string
	run      func(ctx context.Context, input json.RawMessage) tools.Result
}

func (t *fakeTool) Name() string        { return t.name }
func (t *fakeTool) Description() string { return "A test tool: " + t.name }
func (t *fakeTool) Schema() json.RawMessage {
	if t.schema == "" {
		return json.RawMessage(`{"type":"object","properties":{"x":{"type":"string"}}}`)
	}
	return json.RawMessage(t.schema)
}
func (t *fakeTool) ReadOnly() bool                      { return t.readOnly }
func (t *fakeTool) Edits() bool                         { return false }
func (t *fakeTool) Detail(input json.RawMessage) string { return t.name + " detail" }
func (t *fakeTool) Run(ctx context.Context, input json.RawMessage, env tools.Env) tools.Result {
	if t.run != nil {
		return t.run(ctx, input)
	}
	return tools.Result{Text: "ran " + t.name}
}

// rig is an Agent wired to a stand-in, in folders of the test's.
type rig struct {
	t     *testing.T
	srv   *standinapi.Server
	front *fakeFront
	opts  Options
	agent *Agent
}

// newRig starts the agent; change sets options first. It waits nothing but the stand-in.
func newRig(t *testing.T, change func(*Options), script ...standinapi.Reply) *rig {
	t.Helper()
	srv := standinapi.New(script...)
	t.Cleanup(srv.Close)
	t.Setenv("ANTHROPIC_API_KEY", fakeKey)
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "")
	t.Setenv("ANTHROPIC_BASE_URL", srv.URL)

	front := &fakeFront{}
	opts := Options{Dir: t.TempDir(), SessionDir: filepath.Join(t.TempDir(), "sessions"), Front: front}
	if change != nil {
		change(&opts)
	}
	r := &rig{t: t, srv: srv, front: front, opts: opts}
	r.start()
	return r
}

// start makes an agent of the rig's options.
func (r *rig) start() {
	r.t.Helper()
	r.t.Setenv("ANTHROPIC_API_KEY", fakeKey) // New takes it out of the environment
	a, err := New(r.opts)
	if err != nil {
		r.t.Fatal(err)
	}
	a.backoff = time.Millisecond
	r.agent = a
	r.t.Cleanup(func() { _ = a.Close() })
}

func (r *rig) turn(text string) error {
	r.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return r.agent.Turn(ctx, text, nil)
}

func (r *rig) mustTurn(text string) {
	r.t.Helper()
	if err := r.turn(text); err != nil {
		r.t.Fatal(err)
	}
}

func (r *rig) path() string {
	return conversationPath(r.opts.SessionDir, r.agent.Conversation())
}

// records reads the conversation's file back.
func (r *rig) records() []record.Record {
	r.t.Helper()
	data, err := os.ReadFile(r.path())
	if err != nil {
		r.t.Fatal(err)
	}
	var out []record.Record
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var rec record.Record
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			r.t.Fatalf("line %q: %v", line, err)
		}
		out = append(out, rec)
	}
	return out
}

// parsedMessage is a message of a request.
type parsedMessage struct {
	Role    string `json:"role"`
	Content []struct {
		Type      string          `json:"type"`
		Text      string          `json:"text"`
		Thinking  string          `json:"thinking"`
		Signature string          `json:"signature"`
		ID        string          `json:"id"`
		Input     json.RawMessage `json:"input"`
		ToolUseID string          `json:"tool_use_id"`
		IsError   bool            `json:"is_error"`
		Content   []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"content"`
}

func parseMessages(t *testing.T, req standinapi.Request) []parsedMessage {
	t.Helper()
	var out []parsedMessage
	for _, raw := range req.Messages() {
		var m parsedMessage
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("message %s: %v", raw, err)
		}
		out = append(out, m)
	}
	return out
}

// assertValidHistory checks what the API demands of a request's messages: a user first,
// roles alternating, every call answered in the next message and only there, no empty
// content.
func assertValidHistory(t *testing.T, req standinapi.Request) {
	t.Helper()
	messages := parseMessages(t, req)
	if len(messages) == 0 || messages[0].Role != "user" {
		t.Fatalf("first message: %s", req.Raw("messages"))
	}
	for i, m := range messages {
		if len(m.Content) == 0 {
			t.Fatalf("message %d is empty: %s", i, req.Raw("messages"))
		}
		if i > 0 && messages[i-1].Role == m.Role {
			t.Fatalf("messages %d and %d are both %s: %s", i-1, i, m.Role, req.Raw("messages"))
		}
		answered := map[string]bool{}
		if i > 0 {
			for _, b := range m.Content {
				if b.Type == "tool_result" {
					answered[b.ToolUseID] = true
				}
			}
		}
		if i > 0 && messages[i-1].Role == "assistant" {
			for _, b := range messages[i-1].Content {
				if b.Type == "tool_use" && !answered[b.ID] {
					t.Fatalf("call %s has no result: %s", b.ID, req.Raw("messages"))
				}
			}
		}
	}
}

// resultOf finds a call's tool_result in a request.
func resultOf(t *testing.T, req standinapi.Request, id string) (text string, isError bool) {
	t.Helper()
	for _, m := range parseMessages(t, req) {
		for _, b := range m.Content {
			if b.Type == "tool_result" && b.ToolUseID == id {
				var texts []string
				for _, c := range b.Content {
					texts = append(texts, c.Text)
				}
				return strings.Join(texts, "\n"), b.IsError
			}
		}
	}
	t.Fatalf("no result for %s in %s", id, req.Raw("messages"))
	return "", false
}

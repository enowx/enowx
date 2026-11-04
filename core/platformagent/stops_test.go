package platformagent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/enowx/enowx/core/platformagent/record"
	"github.com/enowx/enowx/core/platformagent/tools"
	"github.com/enowx/enowx/internal/standinapi"
)

func TestRefusalIsSaidAndItsPartialDiscarded(t *testing.T) {
	r := newRig(t, nil,
		standinapi.Refusal(standinapi.TextBlock("half of something")),
		standinapi.Text("fine"))
	r.mustTurn("something")
	if r.front.turnStop(t) != "refusal" {
		t.Fatalf("stop: %s", r.front.turnStop(t))
	}
	if !strings.Contains(r.front.notices(), "declined") {
		t.Errorf("notices: %s", r.front.notices())
	}
	r.mustTurn("something else")
	req := r.srv.Requests()[1]
	assertValidHistory(t, req)
	if strings.Contains(string(req.Raw("messages")), "half of something") {
		t.Errorf("partial kept: %s", req.Raw("messages"))
	}
	// The refused answer's cost still counts: it has a record without a message.
	answers := r.front.of(record.Assistant)
	if len(answers) != 2 || answers[0].Message != nil || answers[0].Stop != "refusal" {
		t.Errorf("assistant records: %+v", answers)
	}
}

func TestBrokenOrIncompleteInputDoesNotRun(t *testing.T) {
	ran := 0
	schema := `{"type":"object","properties":{"path":{"type":"string"},"n":{"type":"integer"}},"required":["path"]}`
	r := newRig(t, func(o *Options) {
		o.ExtraTools = []tools.Tool{&fakeTool{name: "Tool", schema: schema, run: func(context.Context, json.RawMessage) tools.Result {
			ran++
			return tools.Result{Text: "ran"}
		}}}
	},
		standinapi.Calls(
			standinapi.ToolUse("bad", "Tool", `{"path":"a"oops`),
			standinapi.ToolUse("missing", "Tool", `{"n":1}`),
			standinapi.ToolUse("wrong", "Tool", `{"path":3}`),
			standinapi.ToolUse("unknown", "Nothing", `{}`),
			standinapi.ToolUse("good", "Tool", `{"path":"a","n":2}`),
		),
		standinapi.Text("ok"))
	r.mustTurn("go")

	if ran != 1 {
		t.Errorf("ran %d times", ran)
	}
	req := r.srv.Requests()[1]
	assertValidHistory(t, req)
	for id, want := range map[string]string{
		"bad":     "INVALID_JSON",
		"missing": "missing required path",
		"wrong":   "path should be of type string",
		"unknown": "no tool named Nothing",
	} {
		if text, isError := resultOf(t, req, id); !isError || !strings.Contains(text, want) {
			t.Errorf("%s: %q %v", id, text, isError)
		}
	}
	if text, isError := resultOf(t, req, "good"); isError || !strings.Contains(text, "ran") {
		t.Errorf("good: %q %v", text, isError)
	}
	// What goes back in the history is valid JSON whatever the model sent.
	for _, m := range parseMessages(t, req) {
		for _, b := range m.Content {
			if b.Type == "tool_use" && !json.Valid(b.Input) {
				t.Errorf("input of %s: %s", b.ID, b.Input)
			}
		}
	}
}

func TestOutputLimitCallsDoNotRunAndTextContinues(t *testing.T) {
	ran := false
	r := newRig(t, func(o *Options) {
		o.ExtraTools = []tools.Tool{&fakeTool{name: "Write", run: func(context.Context, json.RawMessage) tools.Result {
			ran = true
			return tools.Result{}
		}}}
	},
		standinapi.Reply{Blocks: []standinapi.Block{standinapi.ToolUse("c1", "Write", `{"x":"cut`)}, Stop: "max_tokens"},
		standinapi.Reply{Blocks: []standinapi.Block{standinapi.TextBlock("half an ans")}, Stop: "max_tokens"},
		standinapi.Text("wer."))
	r.mustTurn("go")
	if ran {
		t.Error("a cut-off call ran")
	}
	if r.front.turnStop(t) != "done" {
		t.Errorf("stop: %s", r.front.turnStop(t))
	}
	reqs := r.srv.Requests()
	if len(reqs) != 3 {
		t.Fatalf("requests: %d", len(reqs))
	}
	assertValidHistory(t, reqs[1])
	assertValidHistory(t, reqs[2])
	if text, isError := resultOf(t, reqs[1], "c1"); !isError || !strings.Contains(text, "output limit") {
		t.Errorf("result: %q", text)
	}
	if !strings.Contains(string(reqs[2].Raw("messages")), "cut off") {
		t.Errorf("no nudge: %s", reqs[2].Raw("messages"))
	}
	if !strings.Contains(r.front.notices(), "output limit") {
		t.Errorf("notices: %s", r.front.notices())
	}
}

func TestPauseTurnGoesBackAsItIs(t *testing.T) {
	r := newRig(t, nil,
		standinapi.Reply{Blocks: []standinapi.Block{standinapi.TextBlock("working on it")}, Stop: "pause_turn"},
		standinapi.Text("done"))
	r.mustTurn("go")
	reqs := r.srv.Requests()
	if len(reqs) != 2 || r.front.turnStop(t) != "done" {
		t.Fatalf("requests %d, stop %s", len(reqs), r.front.turnStop(t))
	}
	messages := parseMessages(t, reqs[1])
	if len(messages) != 2 || messages[1].Role != "assistant" {
		t.Errorf("no extra user message expected: %s", reqs[1].Raw("messages"))
	}
}

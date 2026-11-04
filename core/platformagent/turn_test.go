package platformagent

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/enowx/enowx/core/platformagent/record"
	"github.com/enowx/enowx/core/platformagent/tools"
	"github.com/enowx/enowx/internal/standinapi"
	"github.com/enowx/enowx/proto"
)

func TestTextStreamsAndIsKept(t *testing.T) {
	long := strings.Repeat("All of this is the answer. ", 20)
	hooks := &fakeHooks{}
	r := newRig(t, func(o *Options) { o.Hooks = hooks }, standinapi.Reply{
		Blocks: []standinapi.Block{standinapi.Thinking("let me see", "sig-1"), standinapi.TextBlock(long)},
		Usage:  standinapi.Usage{Input: 1000, CacheRead: 500, CacheWrite: 100, Output: 200},
	})
	r.mustTurn("hello")

	var streamed strings.Builder
	for _, d := range r.front.of(record.Delta) {
		if d.Kind == "text" {
			streamed.WriteString(d.Text)
		}
	}
	if streamed.String() != long {
		t.Fatalf("deltas: %q", streamed.String())
	}
	if deltas := len(r.front.of(record.Delta)); deltas >= len(long)/16 {
		t.Errorf("%d delta records: not coalesced", deltas)
	}
	if got := len(r.front.of(record.BlockStart)); got != 2 {
		t.Errorf("block starts: %d", got)
	}
	answers := r.front.of(record.Assistant)
	if len(answers) != 1 || answers[0].Stop != "end_turn" || answers[0].Usage.CacheRead != 500 {
		t.Fatalf("assistant: %+v", answers)
	}
	wantCost := (1000*4 + 500*0.2 + 100*5 + 200*20) * 1e-6
	if diff := answers[0].Cost - wantCost; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("cost %v, want %v", answers[0].Cost, wantCost)
	}
	if r.front.turnStop(t) != "done" {
		t.Errorf("stop: %s", r.front.turnStop(t))
	}
	if strings.Join(hooks.states, " ") != proto.StateWorking+" "+proto.StateDone || hooks.done != 1 || hooks.prompts[0] != "hello" {
		t.Errorf("hooks: %+v", hooks)
	}
	if tokens, window := r.agent.Context(); tokens != 1800 || window != 1_000_000 {
		t.Errorf("context: %d of %d", tokens, window)
	}

	info, err := os.Stat(r.path())
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("file mode %o", mode)
	}
	if info, _ := os.Stat(r.opts.SessionDir); info.Mode().Perm() != 0o700 {
		t.Errorf("folder mode %o", info.Mode().Perm())
	}
	if r.records()[0].Type != record.Meta || r.records()[0].Conversation != r.agent.Conversation() {
		t.Errorf("first record: %+v", r.records()[0])
	}
}

func TestReadOnlyCallsRunSideBySideResultsInOneMessage(t *testing.T) {
	var running atomic.Int32
	both := make(chan struct{})
	reader := func(ctx context.Context, input json.RawMessage) tools.Result {
		if running.Add(1) == 2 {
			close(both)
		}
		select {
		case <-both:
			return tools.Result{Text: "read " + string(input)}
		case <-time.After(5 * time.Second):
			return tools.Result{Text: "ran alone", Failed: true}
		}
	}
	r := newRig(t, func(o *Options) {
		o.ExtraTools = []tools.Tool{
			&fakeTool{name: "ReadA", readOnly: true, run: reader},
			&fakeTool{name: "ReadB", readOnly: true, run: reader},
			&fakeTool{name: "Change"},
		}
	},
		standinapi.Calls(
			standinapi.ToolUse("c1", "ReadA", `{"x":"1"}`),
			standinapi.ToolUse("c2", "ReadB", `{"x":"2"}`),
			standinapi.ToolUse("c3", "Change", `{"x":"3"}`),
		),
		standinapi.Text("finished"))
	r.mustTurn("go")

	reqs := r.srv.Requests()
	if len(reqs) != 2 {
		t.Fatalf("requests: %d", len(reqs))
	}
	assertValidHistory(t, reqs[1])
	messages := parseMessages(t, reqs[1])
	last := messages[len(messages)-1]
	if last.Role != "user" || len(last.Content) != 3 {
		t.Fatalf("results: %s", reqs[1].Raw("messages"))
	}
	for i, id := range []string{"c1", "c2", "c3"} {
		if last.Content[i].ToolUseID != id || last.Content[i].IsError {
			t.Errorf("result %d: %+v", i, last.Content[i])
		}
	}
	ends := r.front.of(record.ToolEnd)
	if len(ends) != 3 || ends[0].Tool.Detail == "" || ends[0].Tool.Result == "" {
		t.Errorf("tool ends: %+v", ends)
	}
}

func TestPermissionDenied(t *testing.T) {
	ran := false
	hooks := &fakeHooks{}
	r := newRig(t, func(o *Options) {
		o.Hooks = hooks
		o.ExtraTools = []tools.Tool{&fakeTool{name: "Change", run: func(context.Context, json.RawMessage) tools.Result {
			ran = true
			return tools.Result{}
		}}}
	},
		standinapi.Calls(standinapi.ToolUse("c1", "Change", `{}`)),
		standinapi.Text("understood"))
	r.front.permission = func(PermissionRequest) (Decision, error) { return Deny, nil }
	r.mustTurn("change it")

	if ran {
		t.Error("the tool ran")
	}
	text, isError := resultOf(t, r.srv.Requests()[1], "c1")
	if !isError || !strings.Contains(text, "denied") {
		t.Errorf("result: %q %v", text, isError)
	}
	waits := r.front.of(record.ToolWait)
	if len(waits) != 1 || waits[0].Asks != "permission" {
		t.Errorf("waits: %+v", waits)
	}
	if !strings.Contains(strings.Join(hooks.states, " "), proto.StateWaiting+" "+proto.StateWorking) {
		t.Errorf("states: %v", hooks.states)
	}
}

func TestAlwaysAllowIsKept(t *testing.T) {
	r := newRig(t, func(o *Options) { o.ExtraTools = []tools.Tool{&fakeTool{name: "Change"}} },
		standinapi.Calls(standinapi.ToolUse("c1", "Change", `{}`)),
		standinapi.Text("ok"))
	r.front.permission = func(PermissionRequest) (Decision, error) { return AllowAlways, nil }
	r.mustTurn("go")
	allows := r.front.of(record.Allow)
	if len(allows) != 1 || allows[0].Rule != "Change" {
		t.Fatalf("allow records: %+v", allows)
	}
	saved, err := replayFile(r.path())
	if err != nil || len(saved.rules) != 1 {
		t.Fatalf("resume reads rules %v, %v", saved.rules, err)
	}
}

func TestHookDenyBeatsBypassMode(t *testing.T) {
	ran := false
	hooks := &fakeHooks{before: func(tool string) HookAnswer {
		return HookAnswer{Deny: true, Why: "GOA: delegate this", Context: "from the hook"}
	}}
	r := newRig(t, func(o *Options) {
		o.Hooks = hooks
		o.Mode = record.ModeBypass
		o.ExtraTools = []tools.Tool{&fakeTool{name: "Change", run: func(context.Context, json.RawMessage) tools.Result {
			ran = true
			return tools.Result{}
		}}}
	},
		standinapi.Calls(standinapi.ToolUse("c1", "Change", `{}`)),
		standinapi.Text("ok"))
	r.mustTurn("go")

	if ran || len(r.front.asked) != 0 {
		t.Errorf("ran %v, asked %v", ran, r.front.asked)
	}
	text, isError := resultOf(t, r.srv.Requests()[1], "c1")
	if !isError || !strings.Contains(text, "GOA: delegate this") || !strings.Contains(text, "from the hook") {
		t.Errorf("result: %q %v", text, isError)
	}
}

func TestHookAllowSparesTheAskingAndAddsContext(t *testing.T) {
	hooks := &fakeHooks{before: func(string) HookAnswer { return HookAnswer{Allow: true, Context: "note for the model"} }}
	r := newRig(t, func(o *Options) {
		o.Hooks = hooks
		o.ExtraTools = []tools.Tool{&fakeTool{name: "Change"}}
	},
		standinapi.Calls(standinapi.ToolUse("c1", "Change", `{}`)),
		standinapi.Text("ok"))
	r.mustTurn("go")
	if len(r.front.asked) != 0 {
		t.Errorf("asked: %v", r.front.asked)
	}
	text, isError := resultOf(t, r.srv.Requests()[1], "c1")
	if isError || !strings.Contains(text, "ran Change") || !strings.Contains(text, "note for the model") {
		t.Errorf("result: %q %v", text, isError)
	}
	if len(hooks.after) != 1 {
		t.Errorf("ToolAfter: %v", hooks.after)
	}
}

func TestCancelMidToolKeepsHistoryValid(t *testing.T) {
	started := make(chan struct{})
	r := newRig(t, func(o *Options) {
		o.ExtraTools = []tools.Tool{&fakeTool{name: "Slow", run: func(ctx context.Context, _ json.RawMessage) tools.Result {
			close(started)
			<-ctx.Done()
			return tools.Result{Text: "killed"}
		}}}
	},
		standinapi.Calls(standinapi.ToolUse("c1", "Slow", `{}`), standinapi.ToolUse("c2", "Slow", `{}`)),
		standinapi.Text("next turn's answer"))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		<-started
		cancel()
	}()
	if err := r.agent.Turn(ctx, "run it", nil); err != nil {
		t.Fatal(err)
	}
	if r.front.turnStop(t) != "interrupted" {
		t.Fatalf("stop: %s", r.front.turnStop(t))
	}
	r.mustTurn("what happened?")
	req := r.srv.Requests()[1]
	assertValidHistory(t, req)
	for _, id := range []string{"c1", "c2"} {
		if text, isError := resultOf(t, req, id); !isError || text != "interrupted" {
			t.Errorf("%s: %q %v", id, text, isError)
		}
	}
	// The results and the new prompt are one user message.
	messages := parseMessages(t, req)
	if last := messages[len(messages)-1]; last.Content[0].Type != "tool_result" || last.Content[len(last.Content)-1].Text != "what happened?" {
		t.Errorf("last message: %+v", last)
	}
}

func TestCancelMidStreamDropsThePartial(t *testing.T) {
	hold := make(chan struct{})
	defer close(hold)
	r := newRig(t, nil,
		standinapi.Reply{Blocks: []standinapi.Block{standinapi.TextBlock(strings.Repeat("partial ", 40))}, Hold: hold},
		standinapi.Text("second answer"))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.front.onEvent = func(rec record.Record) {
		if rec.Type == record.BlockStart {
			cancel()
		}
	}
	if err := r.agent.Turn(ctx, "first", nil); err != nil {
		t.Fatal(err)
	}
	if r.front.turnStop(t) != "interrupted" {
		t.Fatalf("stop: %s", r.front.turnStop(t))
	}
	if len(r.front.of(record.Assistant)) != 0 {
		t.Error("the partial answer was kept")
	}
	r.front.onEvent = nil
	r.mustTurn("second")

	req := r.srv.Requests()[1]
	assertValidHistory(t, req)
	if strings.Contains(string(req.Raw("messages")), "partial") {
		t.Errorf("the partial is in the history: %s", req.Raw("messages"))
	}
	messages := parseMessages(t, req)
	if len(messages) != 1 || messages[0].Role != "user" {
		t.Errorf("messages: %s", req.Raw("messages"))
	}
}

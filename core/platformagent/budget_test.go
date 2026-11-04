package platformagent

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/enowx/enowx/core/platformagent/record"
	"github.com/enowx/enowx/core/platformagent/tools"
	"github.com/enowx/enowx/internal/standinapi"
)

// fourDollars is a million input tokens on the default model.
var fourDollars = standinapi.Usage{Input: 1_000_000}

func TestOverBudgetNoEndsTheTurn(t *testing.T) {
	r := newRig(t, func(o *Options) {
		o.Budget = 1
		o.ExtraTools = []tools.Tool{&fakeTool{name: "Alpha", readOnly: true}}
	}, standinapi.Reply{Blocks: []standinapi.Block{standinapi.ToolUse("c1", "Alpha", `{}`)}, Usage: fourDollars})
	r.mustTurn("go")
	if r.front.turnStop(t) != "budget" {
		t.Fatalf("stop: %s", r.front.turnStop(t))
	}
	if r.front.budgetAsks != 1 || len(r.srv.Requests()) != 1 {
		t.Errorf("asks %d, requests %d", r.front.budgetAsks, len(r.srv.Requests()))
	}
	if r.agent.Cost() < 3.9 {
		t.Errorf("cost: %v", r.agent.Cost())
	}
	// Results are in, so the history can go on if the budget is raised.
	r.agent.SetBudget(100)
	r.mustTurn("again")
	assertValidHistory(t, r.srv.Requests()[1])
}

func TestOverBudgetYesRaisesByABudget(t *testing.T) {
	var asked []float64
	r := newRig(t, func(o *Options) {
		o.Budget = 3
		o.ExtraTools = []tools.Tool{&fakeTool{name: "Alpha", readOnly: true}}
	},
		standinapi.Reply{Blocks: []standinapi.Block{standinapi.ToolUse("c1", "Alpha", `{}`)}, Usage: fourDollars},
		standinapi.Reply{Blocks: []standinapi.Block{standinapi.ToolUse("c2", "Alpha", `{}`)}},
		standinapi.Text("done"))
	r.front.overBudget = func(spent, budget float64) bool {
		asked = append(asked, budget)
		return true
	}
	r.mustTurn("go")
	if r.front.turnStop(t) != "done" {
		t.Fatalf("stop: %s", r.front.turnStop(t))
	}
	// Spent $4 past $3: asked once, the limit becomes $6, and the rest fits.
	if len(asked) != 1 || asked[0] != 3 {
		t.Errorf("asked: %v", asked)
	}
	if r.agent.limit != 6 {
		t.Errorf("limit: %v", r.agent.limit)
	}
}

func TestTaskRunsANestedLoopMarkedAsSidechain(t *testing.T) {
	var offered string
	r := newRig(t, func(o *Options) {
		o.ExtraTools = []tools.Tool{
			&fakeTool{name: "Read", readOnly: true},
			&fakeTool{name: "Bash"},
			&fakeTool{name: "Write"},
		}
	},
		standinapi.Calls(standinapi.ToolUse("task1", "Task", `{"description":"look around","prompt":"find the thing"}`)),
		standinapi.Reply{
			Blocks: []standinapi.Block{standinapi.Thinking("sub thought", "sig-sub"), standinapi.ToolUse("sub1", "Read", `{}`), standinapi.ToolUse("sub2", "Bash", `{}`)},
			Usage:  standinapi.Usage{Input: 1000},
		},
		standinapi.Reply{Blocks: []standinapi.Block{standinapi.TextBlock("found it at x.go:3")}, Usage: standinapi.Usage{Input: 1000}},
		standinapi.Text("the subagent found it"),
		standinapi.Text("next"))
	r.mustTurn("delegate")

	reqs := r.srv.Requests()
	if len(reqs) != 4 {
		t.Fatalf("requests: %d", len(reqs))
	}
	offered = string(reqs[1].Raw("tools"))
	if strings.Contains(offered, `"Task"`) || strings.Contains(offered, `"Write"`) || !strings.Contains(offered, `"Read"`) || !strings.Contains(offered, `"Bash"`) {
		t.Errorf("subagent's tools: %s", offered)
	}
	if !strings.Contains(string(reqs[0].Raw("tools")), `"Task"`) {
		t.Errorf("main's tools: %s", reqs[0].Raw("tools"))
	}
	if !strings.Contains(string(reqs[1].Raw("system")), "subagent") {
		t.Errorf("subagent's system: %s", reqs[1].Raw("system"))
	}
	if strings.Contains(string(reqs[1].Raw("messages")), "delegate") {
		t.Errorf("subagent saw the conversation: %s", reqs[1].Raw("messages"))
	}
	assertValidHistory(t, reqs[2])

	if text, isError := resultOf(t, reqs[3], "task1"); isError || text != "found it at x.go:3" {
		t.Errorf("task result: %q %v", text, isError)
	}
	if strings.Contains(string(reqs[3].Raw("messages")), "sig-sub") {
		t.Errorf("the subagent's history leaked: %s", reqs[3].Raw("messages"))
	}

	marked := 0
	for _, rec := range r.records() {
		if rec.Sidechain == "task1" {
			marked++
		}
		if rec.Type == record.TurnEnd && rec.Sidechain != "" {
			t.Errorf("a subagent ended the turn: %+v", rec)
		}
	}
	if marked < 6 {
		t.Errorf("sidechain records: %d", marked)
	}

	// Its cost is the session's, and resuming skips its messages.
	spent := r.agent.Cost()
	if spent < 2*1000*4e-6 {
		t.Errorf("cost: %v", spent)
	}
	id := r.agent.Conversation()
	_ = r.agent.Close()
	r.opts.Resume = id
	r.start()
	if r.agent.Cost() != spent {
		t.Errorf("resumed cost %v, want %v", r.agent.Cost(), spent)
	}
	r.mustTurn("and now?")
	all := r.srv.Requests()
	last := all[len(all)-1]
	assertValidHistory(t, last)
	if strings.Contains(string(last.Raw("messages")), "sig-sub") {
		t.Errorf("resumed history holds the subagent's: %s", last.Raw("messages"))
	}
	if string(last.Raw("system")) != string(reqs[0].Raw("system")) {
		t.Errorf("resumed system differs")
	}
}

func TestSubagentCallsPassHooks(t *testing.T) {
	hooks := &fakeHooks{before: func(tool string) HookAnswer {
		return HookAnswer{Deny: tool == "Bash", Why: "no shell for subagents"}
	}}
	ran := false
	r := newRig(t, func(o *Options) {
		o.Hooks = hooks
		o.ExtraTools = []tools.Tool{&fakeTool{name: "Bash", run: func(context.Context, json.RawMessage) tools.Result {
			ran = true
			return tools.Result{}
		}}}
	},
		standinapi.Calls(standinapi.ToolUse("task1", "Task", `{"description":"d","prompt":"p"}`)),
		standinapi.Calls(standinapi.ToolUse("sub1", "Bash", `{}`)),
		standinapi.Text("couldn't"),
		standinapi.Text("ok"))
	r.mustTurn("go")
	if ran {
		t.Error("the hook's refusal didn't hold in a subagent")
	}
	if text, isError := resultOf(t, r.srv.Requests()[2], "sub1"); !isError || !strings.Contains(text, "no shell") {
		t.Errorf("sub result: %q %v", text, isError)
	}
}

// A context this full would compact; over budget and refused, it must spend nothing more.
func TestARefusedBudgetAtATurnsStartCompactsNothing(t *testing.T) {
	r := newRig(t, func(o *Options) { o.Budget = 1 },
		standinapi.Reply{Blocks: []standinapi.Block{standinapi.TextBlock("big")}, Usage: standinapi.Usage{Input: 900_000}})
	r.mustTurn("first")
	before := r.agent.Cost()
	r.mustTurn("second")
	if len(r.srv.Requests()) != 1 {
		t.Fatalf("requests: %d", len(r.srv.Requests()))
	}
	if r.agent.Cost() != before || r.front.turnStop(t) != "budget" || r.front.budgetAsks != 1 {
		t.Errorf("cost %v of %v, stop %s, asks %d", r.agent.Cost(), before, r.front.turnStop(t), r.front.budgetAsks)
	}
	// The message wasn't lost: it goes with the next once the budget allows.
	r.agent.SetBudget(1000)
	r.srv.Enqueue(standinapi.Text("<summary>s</summary>"), standinapi.Text("answer"))
	r.mustTurn("third")
	all := r.srv.Requests()
	assertValidHistory(t, all[len(all)-1])
	if !strings.Contains(string(all[len(all)-1].Raw("messages")), "second") {
		t.Errorf("the refused message is gone: %s", all[len(all)-1].Raw("messages"))
	}
}

func TestABudgetGoesToAContinueWithNothingToContinue(t *testing.T) {
	r := newRig(t, func(o *Options) { o.Continue, o.NewBudget = true, 10 })
	if _, _, _, got := r.agent.Settings(); got != 10 {
		t.Fatalf("budget: %v", got)
	}
}

func TestAnExplicitNoBudgetHoldsOverAKeptOne(t *testing.T) {
	r := newRig(t, func(o *Options) { o.Budget = 3 })
	id := r.agent.Conversation()
	_ = r.agent.Close()
	r.opts.Budget, r.opts.NoBudget, r.opts.NewBudget, r.opts.Resume = 0, true, 10, id
	r.start()
	if _, _, _, got := r.agent.Settings(); got != 0 {
		t.Fatalf("budget: %v", got)
	}
	_ = r.agent.Close()
	r.opts.NoBudget = false
	r.start()
	if _, _, _, got := r.agent.Settings(); got != 0 { // /budget 0 is a kept choice too
		t.Fatalf("budget: %v", got)
	}
}

func TestSetBudgetIgnoresWhatJSONCantHold(t *testing.T) {
	r := newRig(t, func(o *Options) { o.Budget = 3 })
	for _, bad := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), -1} {
		r.agent.SetBudget(bad) // it used to panic marshaling the Meta
	}
	if _, _, _, got := r.agent.Settings(); got != 3 {
		t.Fatalf("budget: %v", got)
	}
}

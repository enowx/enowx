package platformagent

import (
	"context"
	"strings"
	"testing"

	"github.com/enowx/enowx/core/platformagent/record"
	"github.com/enowx/enowx/core/platformagent/tools"
	"github.com/enowx/enowx/internal/standinapi"
)

func TestCompactStartsOverFromOneMessageWithNoThinking(t *testing.T) {
	r := newRig(t, func(o *Options) { o.ExtraTools = []tools.Tool{&fakeTool{name: "Alpha", readOnly: true}} },
		standinapi.Reply{Blocks: []standinapi.Block{standinapi.Thinking("deep", "sig-x"), standinapi.ToolUse("c1", "Alpha", `{}`)}},
		standinapi.Reply{Blocks: []standinapi.Block{standinapi.Thinking("more", "sig-y"), standinapi.TextBlock("answer")}},
		standinapi.Text("<summary>All that happened.</summary>"),
		standinapi.Text("after compaction"))
	r.mustTurn("do the task")
	if err := r.agent.Compact(context.Background()); err != nil {
		t.Fatal(err)
	}

	compacts := r.front.of(record.Compact)
	if len(compacts) != 1 || len(compacts[0].Messages) != 1 {
		t.Fatalf("compact records: %+v", compacts)
	}
	kept := string(compacts[0].Messages[0])
	if !strings.Contains(kept, "All that happened.") || !strings.Contains(kept, "do the task") {
		t.Errorf("kept: %s", kept)
	}
	if strings.Contains(kept, "thinking") || strings.Contains(kept, "tool_use") {
		t.Errorf("kept: %s", kept)
	}
	if !strings.Contains(r.front.notices(), "compacted") {
		t.Errorf("notices: %s", r.front.notices())
	}
	if tokens, _ := r.agent.Context(); tokens != 0 {
		t.Errorf("context after: %d", tokens)
	}

	reqs := r.srv.Requests()
	ask := reqs[2]
	assertValidHistory(t, ask)
	if !strings.Contains(string(ask.Raw("messages")), "sig-y") {
		t.Error("the summary's request didn't replay the history")
	}
	if !strings.Contains(string(ask.Raw("messages")), "<summary>") {
		t.Error("it didn't ask for a summary")
	}
	if string(ask.Raw("tools")) != string(reqs[0].Raw("tools")) || string(ask.Raw("system")) != string(reqs[0].Raw("system")) {
		t.Error("the summary's request changed the prefix")
	}
	if len(r.front.of(record.Assistant)) != 2 {
		t.Error("the summary went in as an answer")
	}

	r.mustTurn("next")
	next := r.srv.Requests()[3]
	assertValidHistory(t, next)
	messages := parseMessages(t, next)
	if len(messages) != 1 || strings.Contains(string(next.Raw("messages")), "sig-x") {
		t.Errorf("messages after: %s", next.Raw("messages"))
	}
	if string(next.Raw("tools")) != string(reqs[0].Raw("tools")) {
		t.Error("compaction changed the tools")
	}

	// A resumed session starts from the Compact too.
	id := r.agent.Conversation()
	_ = r.agent.Close()
	r.opts.Resume = id
	r.start()
	r.mustTurn("after resume")
	all := r.srv.Requests()
	last := all[len(all)-1]
	assertValidHistory(t, last)
	if strings.Contains(string(last.Raw("messages")), "sig-x") || !strings.Contains(string(last.Raw("messages")), "All that happened.") {
		t.Errorf("resumed messages: %s", last.Raw("messages"))
	}
}

func TestCompactsByItselfPastTheThreshold(t *testing.T) {
	full := standinapi.Usage{Input: 100, CacheRead: 760_000, Output: 100}
	r := newRig(t, nil,
		standinapi.Reply{Blocks: []standinapi.Block{standinapi.TextBlock("big")}, Usage: full},
		standinapi.Text("<summary>short</summary>"),
		standinapi.Text("small"))
	r.mustTurn("one")
	r.mustTurn("the second prompt")
	reqs := r.srv.Requests()
	if len(reqs) != 3 || len(r.front.of(record.Compact)) != 1 {
		t.Fatalf("requests %d, compacts %d", len(reqs), len(r.front.of(record.Compact)))
	}
	if !strings.Contains(string(reqs[1].Raw("messages")), "<summary>") {
		t.Errorf("second request: %s", reqs[1].Raw("messages"))
	}
	if strings.Contains(string(reqs[1].Raw("messages")), "the second prompt") {
		t.Errorf("the new prompt went in the summary: %s", reqs[1].Raw("messages"))
	}
	final := string(reqs[2].Raw("messages"))
	if !strings.Contains(final, "short") || !strings.Contains(final, "the second prompt") {
		t.Errorf("after: %s", final)
	}
	assertValidHistory(t, reqs[2])
	if r.front.of(record.Compact)[0].Before != 760_200 {
		t.Errorf("before: %d", r.front.of(record.Compact)[0].Before)
	}
}

func TestCompactsBetweenToolRoundsNeverInOne(t *testing.T) {
	full := standinapi.Usage{CacheRead: 800_000}
	r := newRig(t, func(o *Options) { o.ExtraTools = []tools.Tool{&fakeTool{name: "Alpha", readOnly: true}} },
		standinapi.Reply{Blocks: []standinapi.Block{standinapi.ToolUse("c1", "Alpha", `{}`)}, Usage: full},
		standinapi.Text("<summary>mid task</summary>"),
		standinapi.Text("finished"))
	r.mustTurn("work")
	reqs := r.srv.Requests()
	if len(reqs) != 3 {
		t.Fatalf("requests: %d", len(reqs))
	}
	// The summary's request has the call's result, and comes before going on.
	assertValidHistory(t, reqs[1])
	if text, _ := resultOf(t, reqs[1], "c1"); text != "ran Alpha" {
		t.Errorf("result: %q", text)
	}
	assertValidHistory(t, reqs[2])
	if strings.Contains(string(reqs[2].Raw("messages")), "tool_use") {
		t.Errorf("a call survived: %s", reqs[2].Raw("messages"))
	}
	if r.front.turnStop(t) != "done" {
		t.Errorf("stop: %s", r.front.turnStop(t))
	}
}

func TestCompactForbidsToolsWhenTheModelCallsOne(t *testing.T) {
	r := newRig(t, func(o *Options) { o.ExtraTools = []tools.Tool{&fakeTool{name: "Alpha", readOnly: true}} },
		standinapi.Text("first"),
		standinapi.Calls(standinapi.ToolUse("c9", "Alpha", `{}`)),
		standinapi.Text("<summary>at last</summary>"))
	r.mustTurn("one")
	if err := r.agent.Compact(context.Background()); err != nil {
		t.Fatal(err)
	}
	reqs := r.srv.Requests()
	if len(reqs) != 3 || reqs[1].Raw("tool_choice") != nil || string(reqs[2].Raw("tool_choice")) != `{"type":"none"}` {
		t.Fatalf("requests %d, tool_choice %s then %s", len(reqs), reqs[1].Raw("tool_choice"), reqs[2].Raw("tool_choice"))
	}
	if strings.Contains(string(r.front.of(record.Compact)[0].Messages[0]), "c9") {
		t.Error("the stray call was kept")
	}
}

func TestCompactOnEmptyConversationSaysSo(t *testing.T) {
	r := newRig(t, nil)
	if err := r.agent.Compact(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(r.srv.Requests()) != 0 || !strings.Contains(r.front.notices(), "nothing to compact") {
		t.Errorf("requests %d, notices %s", len(r.srv.Requests()), r.front.notices())
	}
}

// A compaction with no summary is tried once a turn, however many tool rounds follow,
// and not again until the context has grown.
func TestAFailedCompactionBacksOff(t *testing.T) {
	full := standinapi.Usage{CacheRead: 800_000}
	round := func(id string) standinapi.Reply {
		return standinapi.Reply{Blocks: []standinapi.Block{standinapi.ToolUse(id, "Alpha", `{}`)}, Usage: full}
	}
	empty := standinapi.Reply{Blocks: []standinapi.Block{standinapi.ToolUse("x", "Alpha", `{}`)}, Usage: standinapi.Usage{Input: 100}}
	emptyText := standinapi.Reply{Blocks: []standinapi.Block{}, Usage: standinapi.Usage{Input: 100}}
	r := newRig(t, func(o *Options) { o.ExtraTools = []tools.Tool{&fakeTool{name: "Alpha", readOnly: true}} },
		round("c1"),
		empty, emptyText, // the one try: a stray call, then nothing
		round("c2"), round("c3"), standinapi.Reply{Blocks: []standinapi.Block{standinapi.TextBlock("done")}, Usage: full},
		standinapi.Reply{Blocks: []standinapi.Block{standinapi.TextBlock("second turn")}, Usage: full})
	r.mustTurn("work")
	if got := len(r.srv.Requests()); got != 6 {
		t.Fatalf("requests in the turn: %d", got)
	}
	if n := strings.Count(r.front.notices(), "Compaction failed"); n != 1 {
		t.Errorf("failures told: %d: %s", n, r.front.notices())
	}
	// A new turn with the context no bigger doesn't pay for another.
	r.mustTurn("again")
	if got := len(r.srv.Requests()); got != 7 {
		t.Errorf("requests after a turn: %d", got)
	}
	// Its unsummarized answers are on record for their cost.
	spent := 0
	for _, rec := range r.records() {
		if rec.Type == record.Assistant && rec.Message == nil && rec.Usage != nil && rec.Usage.Input == 100 {
			spent++
		}
	}
	if spent != 2 {
		t.Errorf("records of the failed compaction's answers: %d", spent)
	}
	// Once it has grown, it tries again.
	r.agent.main.compactFailed = false
	r.agent.mu.Lock()
	r.agent.used = 900_000
	r.agent.mu.Unlock()
	r.srv.Enqueue(standinapi.Text("<summary>at last</summary>"), standinapi.Text("after"))
	r.mustTurn("grown")
	if len(r.front.of(record.Compact)) != 1 {
		t.Error("no compaction once the context grew")
	}
}

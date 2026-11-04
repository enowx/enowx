package platformagent

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/enowx/enowx/core/platformagent/record"
	"github.com/enowx/enowx/core/platformagent/tools"
	"github.com/enowx/enowx/internal/standinapi"
)

// cutFileAfter keeps the conversation's file up to the first record of a kind, as a
// crash would, and half a line after it.
func cutFileAfter(t *testing.T, path, kind string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.SplitAfter(string(data), "\n")
	var kept strings.Builder
	for _, line := range lines {
		kept.WriteString(line)
		var r record.Record
		if json.Unmarshal([]byte(line), &r) == nil && r.Type == kind {
			kept.WriteString(`{"type":"tool_start","ti`)
			break
		}
	}
	if err := os.WriteFile(path, []byte(kept.String()), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestResumeAfterCrashAnswersDanglingCalls(t *testing.T) {
	r := newRig(t, func(o *Options) { o.ExtraTools = []tools.Tool{&fakeTool{name: "Change"}} },
		standinapi.Reply{
			Blocks: []standinapi.Block{standinapi.Thinking("plan it", "sig-a"), standinapi.ToolUse("c1", "Change", `{"x":"1"}`)},
			Usage:  standinapi.Usage{Input: 100, Output: 10},
		},
		standinapi.Text("first turn over"),
		standinapi.Text("after the crash"))
	r.mustTurn("go")
	spent := r.agent.Cost()
	id := r.agent.Conversation()
	path := r.path()
	_ = r.agent.Close()
	cutFileAfter(t, path, record.Assistant)

	r.opts.Resume = id
	r.start()
	if r.agent.Conversation() != id {
		t.Fatalf("conversation %s", r.agent.Conversation())
	}
	if r.agent.Cost() == 0 || r.agent.Cost() > spent {
		t.Errorf("cost %v of %v", r.agent.Cost(), spent)
	}
	r.mustTurn("are you there?")

	reqs := r.srv.Requests()
	last := reqs[len(reqs)-1]
	assertValidHistory(t, last)
	if text, isError := resultOf(t, last, "c1"); !isError || text != "interrupted" {
		t.Errorf("result: %q %v", text, isError)
	}
	if !strings.Contains(string(last.Raw("messages")), "sig-a") {
		t.Errorf("the thinking block was lost: %s", last.Raw("messages"))
	}
	// The repair is on file, so a second resume has it without repairing.
	saved, err := replayFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.dangling) != 0 {
		t.Errorf("dangling after repair: %+v", saved.dangling)
	}
	// And the half line didn't swallow what came after it.
	if len(saved.history) != 5 {
		t.Errorf("history after the half line: %d messages", len(saved.history))
	}
}

func TestContinueTakesTheNewest(t *testing.T) {
	r := newRig(t, nil)
	r.mustTurn("one")
	id := r.agent.Conversation()
	_ = r.agent.Close()
	r.opts.Continue = true
	r.start()
	if r.agent.Conversation() != id {
		t.Errorf("continued %s, want %s", r.agent.Conversation(), id)
	}
	r.mustTurn("two")
	reqs := r.srv.Requests()
	if got := len(reqs[len(reqs)-1].Messages()); got != 3 {
		t.Errorf("messages: %d", got)
	}
}

func TestResumeRefusesAStrangeIDAndAMissingOne(t *testing.T) {
	r := newRig(t, nil)
	for _, id := range []string{"../../etc/passwd", "nothing-here"} {
		o := r.opts
		o.Resume = id
		if _, err := New(o); err == nil {
			t.Errorf("resumed %q", id)
		}
	}
}

func TestThinkingIsReplayedByteForByte(t *testing.T) {
	thinking := standinapi.Thinking("a <b> & \"quoted\" thought é", "c2lnbmF0dXJl+/==")
	redacted := standinapi.Block{Type: "redacted_thinking", Text: "ZW5jcnlwdGVk"}
	r := newRig(t, nil,
		standinapi.Reply{Blocks: []standinapi.Block{thinking, redacted, standinapi.TextBlock("one <&>")}},
		standinapi.Text("two"),
		standinapi.Text("three"))
	r.mustTurn("first")

	kept := r.front.of(record.Assistant)[0].Message
	if !strings.Contains(string(kept), `"signature":"c2lnbmF0dXJl+/=="`) || !strings.Contains(string(kept), `a <b> & \"quoted\"`) {
		t.Fatalf("kept: %s", kept)
	}
	if !strings.Contains(string(kept), `"redacted_thinking"`) {
		t.Fatalf("redacted block lost: %s", kept)
	}

	r.mustTurn("second")
	second := r.srv.Requests()[1].Messages()
	if string(second[1]) != string(kept) {
		t.Errorf("replayed differently:\n%s\n%s", second[1], kept)
	}

	// Through the file, too.
	id := r.agent.Conversation()
	_ = r.agent.Close()
	r.opts.Resume = id
	r.start()
	r.mustTurn("third")
	third := r.srv.Requests()[2].Messages()
	if string(third[1]) != string(kept) {
		t.Errorf("after resume:\n%s\n%s", third[1], kept)
	}
	if string(third[0]) != string(second[0]) {
		t.Errorf("the first message changed:\n%s\n%s", third[0], second[0])
	}
}

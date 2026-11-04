package platformagent

import (
	"context"
	"strings"
	"testing"

	"github.com/enowx/enowx/core/platformagent/record"
	"github.com/enowx/enowx/core/platformagent/tools"
)

func TestBuiltInScriptDrivesTheLoop(t *testing.T) {
	schema := `{"type":"object","properties":{"command":{"type":"string"}},"required":["command"]}`
	r := newRig(t, func(o *Options) {
		o.ExtraTools = []tools.Tool{
			&fakeTool{name: "Bash", schema: schema},
			&fakeTool{name: "Read", readOnly: true},
		}
	})
	r.mustTurn("please use a tool")
	if err := r.agent.Compact(context.Background()); err != nil {
		t.Fatal(err)
	}
	r.mustTurn("and a plain question")

	if got := len(r.front.of(record.ToolEnd)); got != 2 {
		t.Errorf("tool calls: %d", got)
	}
	if len(r.front.of(record.Compact)) != 1 {
		t.Error("no compaction")
	}
	reqs := r.srv.Requests()
	for _, req := range reqs {
		assertValidHistory(t, req)
	}
	if !strings.Contains(string(reqs[len(reqs)-1].Raw("messages")), "stand-in's summary") {
		t.Errorf("last request: %s", reqs[len(reqs)-1].Raw("messages"))
	}
}

func TestQuestionsWaitOnRecord(t *testing.T) {
	r := newRig(t, nil)
	ctx := context.WithValue(context.Background(), callKey{}, toolCall{ID: "c1", Name: askToolName})
	answers, err := asker{r.agent}.Questions(ctx, []record.Question{{Question: "Which?"}})
	if err != nil || len(answers) != 1 {
		t.Fatalf("answers %v, %v", answers, err)
	}
	waits := r.front.of(record.ToolWait)
	if len(waits) != 1 || waits[0].Asks != "questions" || waits[0].Tool.ID != "c1" || waits[0].Tool.Questions[0].Question != "Which?" {
		t.Errorf("waits: %+v", waits)
	}
}

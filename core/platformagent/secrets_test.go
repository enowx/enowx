package platformagent

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/enowx/enowx/core/platformagent/record"
	"github.com/enowx/enowx/core/platformagent/tools"
	"github.com/enowx/enowx/internal/standinapi"
)

func TestTheKeyIsForgottenFromTheEnvironmentAndStillSent(t *testing.T) {
	r := newRig(t, nil, standinapi.Text("hi"))
	if got := os.Getenv("ANTHROPIC_API_KEY"); got != "" {
		t.Fatal("the key is still in the environment")
	}
	for _, kv := range r.agent.environ {
		if strings.Contains(kv, fakeKey) {
			t.Fatal("the key would reach a program a tool starts")
		}
	}
	r.mustTurn("hello")
	if got := r.srv.Requests()[0].Header.Get("X-Api-Key"); got != fakeKey {
		t.Fatalf("the request's key: %q", got)
	}
}

// A result holding the key reaches neither the model, nor the file, nor the screen.
func TestToolResultsAreScrubbedOfTheKey(t *testing.T) {
	leak := func(context.Context, json.RawMessage) tools.Result {
		return tools.Result{Text: "token=" + fakeKey, Old: fakeKey, New: "x" + fakeKey}
	}
	r := newRig(t, func(o *Options) {
		o.ExtraTools = []tools.Tool{&fakeTool{name: "mcp__srv__leak", run: leak}, &fakeTool{name: "Leak", readOnly: true, run: leak}}
	},
		standinapi.Calls(standinapi.ToolUse("c1", "mcp__srv__leak", `{}`), standinapi.ToolUse("c2", "Leak", `{}`)),
		standinapi.Text("done"))
	r.mustTurn("go")

	if body := string(r.srv.Requests()[1].Raw("messages")); strings.Contains(body, fakeKey) || !strings.Contains(body, "token=[key]") {
		t.Errorf("sent to the model: %s", body)
	}
	data, err := os.ReadFile(r.path())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), fakeKey) {
		t.Errorf("in the file: %s", data)
	}
	r.front.mu.Lock()
	defer r.front.mu.Unlock()
	for _, ev := range r.front.events {
		if line, _ := json.Marshal(ev); strings.Contains(string(line), fakeKey) {
			t.Errorf("on the screen: %s", line)
		}
	}
}

// What the model itself wrote with the key goes through the same scrubbing on its way out.
func TestEventsShownAreScrubbedOfTheKey(t *testing.T) {
	r := newRig(t, nil)
	r.agent.emit(record.Record{Type: record.Notice, Level: "info", Text: "echo " + fakeKey})
	got := r.front.of(record.Notice)
	if len(got) != 1 || got[0].Text != "echo [key]" {
		t.Fatalf("shown: %+v", got)
	}
}

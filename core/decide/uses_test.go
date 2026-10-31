package decide

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/proto"
)

// A prompt's difficulty maps to models the agent has, cheapest for the
// easy ones; an agent set to another API gets that API's models; what to
// type in a running session follows the agent's own commands.
func TestRouteFor(t *testing.T) {
	for id, byLevel := range routes {
		for level, r := range byLevel {
			s := agent.SettingsOf(id)
			if r.Model != "" && !slices.ContainsFunc(s.Models, func(c agent.Choice) bool { return c.ID == r.Model }) {
				t.Errorf("%s %s: no model %q in its list", id, level, r.Model)
			}
			if !slices.ContainsFunc(s.Efforts, func(c agent.Choice) bool { return c.ID == r.Effort }) {
				t.Errorf("%s %s: no effort %q", id, level, r.Effort)
			}
		}
	}
	if r := RouteFor("claude", proto.VerdictEasy, nil); r != (Route{"claude-haiku-5-5", "low"}) {
		t.Fatalf("claude easy: %+v", r)
	}
	if got := RouteFor("claude", proto.VerdictEasy, nil).Switch("claude"); strings.Join(got, "|") != "/model claude-haiku-5-5|/effort low" {
		t.Fatalf("switch %q", got)
	}
	if r := RouteFor("codex", proto.VerdictHard, nil); r != (Route{"", "high"}) {
		t.Fatalf("codex hard: %+v", r)
	}
	if r := RouteFor("claude", proto.VerdictEasy, &RouteAPI{Model: "glm-5", SmallModel: "glm-5-air"}); r.Model != "glm-5-air" {
		t.Fatalf("claude on another API: %+v", r)
	}
	if r := RouteFor("aider", proto.VerdictHard, nil); r != (Route{}) {
		t.Fatalf("an agent without models or efforts: %+v", r)
	}
}

// The later uses through a local judge: each answers in its own terms, an
// answer that doesn't fit its verdict fails over (a part not given, a
// decision with no text), and a use off asks nobody.
func TestLaterUses(t *testing.T) {
	local := localServer(t, []string{"a", "b"}, func(q string) string {
		switch {
		case strings.Contains(q, "<prompt>") && strings.Contains(q, "How hard"):
			return `{"verdict":"easy","confidence":0.9,"why":"a typo"}`
		case strings.Contains(q, `<part n="3">`) && strings.Contains(q, "WRONG"):
			return `{"verdict":"relevant","confidence":0.9,"why":"x","keep":[7]}`
		case strings.Contains(q, `<part n="`):
			return `{"verdict":"relevant","confidence":0.9,"why":"the billing parts","keep":[1,3]}`
		case strings.Contains(q, "<answer>") && strings.Contains(q, "ALLDONE"):
			return `{"verdict":"decision","confidence":0.9,"why":"x","text":""}`
		case strings.Contains(q, "<answer>"):
			return `{"verdict":"decision","confidence":0.9,"why":"a choice of library","text":"Money is kept in cents (int64) because floats rounded totals."}`
		case strings.Contains(q, "<message>"):
			return `{"verdict":"urgent","confidence":0.9,"why":"production is down"}`
		case strings.Contains(q, "did it fail by chance"):
			return `{"verdict":"flaky","confidence":0.85,"why":"dial tcp: i/o timeout"}`
		}
		return `{"verdict":"nonsense"}`
	})
	e := engine(t, Found{Local: []string{local}})
	ask := func(a proto.DecideAsk) proto.Decision {
		t.Helper()
		d, err := e.Decide(context.Background(), Request{DecideAsk: a, Agent: "claude", AgentName: "Claude Code"})
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	if _, err := e.Decide(context.Background(), Request{DecideAsk: proto.DecideAsk{Use: proto.DecideRoute, Prompt: "fix the typo"}}); err != ErrOff {
		t.Fatalf("route off: %v", err)
	}
	st := e.settings.Get()
	for _, u := range []string{proto.DecideRoute, proto.DecideContext, proto.DecideMemory, proto.DecideChat, proto.DecideFlaky} {
		st.Uses[u] = proto.DecideUse{On: true}
	}
	if err := e.settings.Set(st); err != nil {
		t.Fatal(err)
	}
	if d := ask(proto.DecideAsk{Use: proto.DecideRoute, Prompt: "fix the typo in README"}); d.Verdict != proto.VerdictEasy {
		t.Fatalf("route %+v", d)
	}
	parts := []string{"billing/total.go", "ui/theme.go", "billing/tax.go"}
	d := ask(proto.DecideAsk{Use: proto.DecideContext, Prompt: "totals round wrong", Parts: parts})
	if got := Keep(parts, d); strings.Join(got, " ") != "billing/total.go billing/tax.go" {
		t.Fatalf("context kept %q (%+v)", got, d)
	}
	if d := ask(proto.DecideAsk{Use: proto.DecideContext, Prompt: "WRONG", Parts: parts}); d.Judge != "" || len(d.Tried) != 2 {
		t.Fatalf("a part not given was taken: %+v", d)
	}
	e.failed = map[string]time.Time{} // the judges that failed get another go
	if d := ask(proto.DecideAsk{Use: proto.DecideMemory, Text: "I switched totals to integer cents; floats rounded wrong."}); d.Verdict != proto.VerdictDecision || !strings.Contains(d.Text, "cents") {
		t.Fatalf("memory %+v", d)
	}
	if d := ask(proto.DecideAsk{Use: proto.DecideMemory, Text: "ALLDONE"}); d.Judge != "" {
		t.Fatalf("a decision with no text was taken: %+v", d)
	}
	e.failed = map[string]time.Time{}
	if d := ask(proto.DecideAsk{Use: proto.DecideChat, From: "Ana", Place: "Shop", Text: "prod checkout is down, can you look?"}); d.Verdict != proto.VerdictUrgent {
		t.Fatalf("chat %+v", d)
	}
	if d := ask(proto.DecideAsk{Use: proto.DecideFlaky, Command: "go test ./...", Output: "dial tcp 127.0.0.1:5432: i/o timeout\nFAIL", Exit: 1}); d.Verdict != proto.VerdictFlaky {
		t.Fatalf("flaky %+v", d)
	}
}

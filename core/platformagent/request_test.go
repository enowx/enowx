package platformagent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/enowx/enowx/core/platformagent/tools"
	"github.com/enowx/enowx/internal/standinapi"
)

func TestPrefixIsByteStableAcrossRequests(t *testing.T) {
	r := newRig(t, func(o *Options) {
		o.Append = "enowX's guide"
		// Deliberately out of order.
		o.ExtraTools = []tools.Tool{&fakeTool{name: "Zed", readOnly: true}, &fakeTool{name: "Alpha", readOnly: true}}
	},
		standinapi.Calls(standinapi.ToolUse("c1", "Alpha", `{}`)),
		standinapi.Text("ok"))
	r.mustTurn("go")
	reqs := r.srv.Requests()
	if len(reqs) != 2 {
		t.Fatalf("requests: %d", len(reqs))
	}
	if len(reqs[0].Raw("tools")) == 0 || string(reqs[0].Raw("tools")) != string(reqs[1].Raw("tools")) {
		t.Errorf("tools differ:\n%s\n%s", reqs[0].Raw("tools"), reqs[1].Raw("tools"))
	}
	if len(reqs[0].Raw("system")) == 0 || string(reqs[0].Raw("system")) != string(reqs[1].Raw("system")) {
		t.Errorf("system differs:\n%s\n%s", reqs[0].Raw("system"), reqs[1].Raw("system"))
	}
	if string(reqs[0].Messages()[0]) != string(reqs[1].Messages()[0]) {
		t.Errorf("first message differs")
	}

	var list []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(reqs[0].Raw("tools"), &list); err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range list {
		names = append(names, tool.Name)
	}
	if got := strings.Join(names, " "); !strings.HasPrefix(got, "Alpha") || !strings.HasSuffix(got, "Zed") {
		t.Errorf("tools aren't sorted: %s", got)
	}

	if string(reqs[0].Raw("cache_control")) != `{"type":"ephemeral"}` {
		t.Errorf("no automatic caching: %s", reqs[0].Raw("cache_control"))
	}
	var system []struct {
		Text         string          `json:"text"`
		CacheControl json.RawMessage `json:"cache_control"`
	}
	if err := json.Unmarshal(reqs[0].Raw("system"), &system); err != nil {
		t.Fatal(err)
	}
	if len(system) != 2 || system[0].CacheControl != nil || system[1].CacheControl == nil || system[1].Text != "enowX's guide" {
		t.Errorf("system blocks: %+v", system)
	}
	if strings.Contains(string(reqs[0].Raw("system")), "Today's date") {
		t.Error("the date is in the cached prefix")
	}
	if strings.Count(string(reqs[1].Body), "cache_control") > 4 {
		t.Error("more than 4 breakpoints")
	}
}

func TestPromptAndRepositoryInstructions(t *testing.T) {
	r := newRig(t, func(o *Options) {
		o.SystemPrompt = "Only this."
		if err := os.WriteFile(filepath.Join(o.Dir, "AGENTS.md"), []byte("Run the linter."), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(o.Dir, "CLAUDE.md"), []byte("@AGENTS.md\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	})
	r.mustTurn("one")
	r.mustTurn("two")
	reqs := r.srv.Requests()
	if system := string(reqs[0].Raw("system")); !strings.Contains(system, "Only this.") || strings.Contains(system, "Claude Platform") {
		t.Errorf("system: %s", system)
	}
	first := string(reqs[0].Messages()[0])
	if !strings.Contains(first, `<repository-instructions file=\"AGENTS.md\">`) || !strings.Contains(first, "Run the linter.") {
		t.Errorf("first message: %s", first)
	}
	if !strings.Contains(first, "not from the user or the system prompt") || !strings.Contains(first, "Today's date") {
		t.Errorf("first message: %s", first)
	}
	if strings.Contains(first, "CLAUDE.md") {
		t.Errorf("a file that only points elsewhere was included: %s", first)
	}
	if strings.Contains(string(reqs[1].Messages()[2]), "repository-instructions") {
		t.Error("the instructions came twice")
	}
}

func TestBuiltInPromptAndEffortPerModel(t *testing.T) {
	for _, tc := range []struct {
		model, effort            string
		wantThinking, wantOutput string
		maxTokens                string
	}{
		{"", "", thinkingSummarized, `{"effort":"high"}`, "64000"},
		{"claude-fable-5-1", "max", thinkingSummarized, `{"effort":"max"}`, "64000"},
		{"claude-sonnet-4-6", "xhigh", thinkingAdaptive, `{"effort":"high"}`, "64000"},
		{"claude-haiku-4-5", "high", thinkingBudget, "", "32000"},
	} {
		r := newRig(t, func(o *Options) { o.Model, o.Effort = tc.model, tc.effort })
		r.mustTurn("hi")
		req := r.srv.Requests()[0]
		if string(req.Raw("thinking")) != tc.wantThinking || string(req.Raw("output_config")) != tc.wantOutput || string(req.Raw("max_tokens")) != tc.maxTokens {
			t.Errorf("%q: thinking %s, output %s, max %s", tc.model, req.Raw("thinking"), req.Raw("output_config"), req.Raw("max_tokens"))
		}
		if !strings.Contains(string(req.Raw("system")), "Claude Platform") {
			t.Errorf("%q: system %s", tc.model, req.Raw("system"))
		}
		if req.Raw("temperature") != nil || req.Raw("tool_choice") != nil {
			t.Errorf("%q: sampling or forced tools: %s", tc.model, req.Body)
		}
	}
	if w := specOf("claude-haiku-4-5").window; w != 200_000 {
		t.Errorf("haiku 4.5 window: %d", w)
	}
	if w := specOf("claude-opus-5-5").window; w != 1_000_000 {
		t.Errorf("opus 5.5 window: %d", w)
	}
}

func TestFallbacksAndEagerInputOnlyOnTheFirstPartyAPI(t *testing.T) {
	tool := &fakeTool{name: "A"}
	proxied := newRig(t, func(o *Options) { o.ExtraTools = []tools.Tool{tool} })
	proxied.mustTurn("hi")
	req := proxied.srv.Requests()[0]
	if req.Raw("fallbacks") != nil || strings.Contains(req.Header.Get("Anthropic-Beta"), "fallback") {
		t.Errorf("a proxy got fallbacks: %s", req.Raw("fallbacks"))
	}
	if strings.Contains(string(req.Raw("tools")), "eager_input_streaming") {
		t.Error("a proxy got eager input streaming")
	}

	direct := newRig(t, func(o *Options) {
		o.FirstParty = true
		o.ExtraTools = []tools.Tool{tool}
	})
	direct.mustTurn("hi")
	req = direct.srv.Requests()[0]
	if string(req.Raw("fallbacks")) != `"default"` || !strings.Contains(req.Header.Get("Anthropic-Beta"), "server-side-fallback-2026-07-01") {
		t.Errorf("fallbacks %s, header %q", req.Raw("fallbacks"), req.Header.Get("Anthropic-Beta"))
	}
	if !strings.Contains(string(req.Raw("tools")), `"eager_input_streaming":true`) {
		t.Errorf("tools: %s", req.Raw("tools"))
	}

	noFallback := newRig(t, func(o *Options) {
		o.FirstParty = true
		o.Model = "claude-haiku-5-5"
	})
	noFallback.mustTurn("hi")
	if noFallback.srv.Requests()[0].Raw("fallbacks") != nil {
		t.Error("haiku has no server-side fallback")
	}
	if !isAnthropicAddress("https://api.anthropic.com/") || isAnthropicAddress("https://proxy.example.com") {
		t.Error("isAnthropicAddress")
	}
}

func TestFallbackAnswerKeepsOnlyWhatFollowsTheSwitch(t *testing.T) {
	ran := []string{}
	r := newRig(t, func(o *Options) {
		o.ExtraTools = []tools.Tool{&fakeTool{name: "A", readOnly: true, run: func(_ context.Context, in json.RawMessage) tools.Result {
			ran = append(ran, string(in))
			return tools.Result{Text: "ok"}
		}}}
	},
		standinapi.Reply{
			Model: "claude-opus-5-5",
			Blocks: []standinapi.Block{
				standinapi.Thinking("declined model's", "sig-declined"),
				standinapi.TextBlock("said before the switch"),
				standinapi.ToolUse("before", "A", `{"x":"before"}`),
				standinapi.Fallback("claude-opus-5-5", "claude-opus-5"),
				standinapi.Thinking("fallback's", "sig-served"),
				standinapi.ToolUse("after", "A", `{"x":"after"}`),
			},
		},
		standinapi.Text("done"))
	r.mustTurn("go")

	if len(ran) != 1 || ran[0] != `{"x":"after"}` {
		t.Errorf("ran: %v", ran)
	}
	req := r.srv.Requests()[1]
	assertValidHistory(t, req)
	history := string(req.Raw("messages"))
	for _, gone := range []string{"sig-declined", `"before"`, `"fallback"`} {
		if strings.Contains(history, gone) {
			t.Errorf("%s went back: %s", gone, history)
		}
	}
	for _, kept := range []string{"said before the switch", "sig-served"} {
		if !strings.Contains(history, kept) {
			t.Errorf("%s was lost: %s", kept, history)
		}
	}
}

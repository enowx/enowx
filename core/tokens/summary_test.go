package tokens

import (
	"math"
	"testing"
	"time"

	"github.com/enowx/enowx/proto"
)

func TestSummaryKeepsEnowXsOwnUseApart(t *testing.T) {
	store := Open(t.TempDir())
	store.Now = func() time.Time { return fixedNow }
	at := fixedNow.Add(-2 * time.Hour)
	store.Add([]Call{
		{ID: "a", Time: at, Agent: "claude", Session: "s1", Project: "/p", Model: "claude-opus-5-5", Input: 1_000_000, Output: 100_000, Layer: proto.TokenLayerRecords},
		{ID: "b", Time: at, Agent: "codex", Session: "s2", Project: "/p", Model: "gpt-5.1", Input: 2000, Plan: true, Layer: proto.TokenLayerRecords},
		{ID: "judge:1", Time: at, Own: proto.TokensOwnJudge, Session: "s1", Model: "claude-haiku-5-5", Input: 900},
		{ID: "rag:1", Time: at, Own: proto.TokensOwnRAG, Project: "/p", Model: "text-embedding-3-small", Input: 50_000},
	})
	s := store.Summarize(proto.TokensQuery{By: proto.TokensByAgent, Until: fixedNow}, NewPrices(nil), fixedNow)
	if s.Total.Calls != 2 || s.Total.Input != 1_002_000 {
		t.Fatalf("agents' total: %+v", s.Total)
	}
	if s.Own.Calls != 2 || s.Own.Input != 50_900 || len(s.OwnGroups) != 2 || s.OwnGroups[0].Key != proto.TokensOwnRAG {
		t.Fatalf("own: %+v %+v", s.Own, s.OwnGroups)
	}
	// Opus 5.5: $4 in, $20 out a million; gpt-5.1's on a plan is
	// API-equivalent.
	wantCost := 4 + 2 + 2000*1.25e-6
	if math.Abs(s.Total.Cost-wantCost) > 1e-9 || math.Abs(s.Total.Equivalent-2000*1.25e-6) > 1e-12 {
		t.Fatalf("cost %v (equivalent %v), want %v", s.Total.Cost, s.Total.Equivalent, wantCost)
	}
	if len(s.Groups) != 2 || s.Groups[0].Key != "claude" {
		t.Fatalf("groups: %+v", s.Groups)
	}
	if totals := store.SessionTotals(fixedNow.AddDate(0, 0, -1), nil); len(totals) != 2 || totals[0].Input != 1_000_000 {
		t.Fatalf("a session's total took the judge's in: %+v", totals)
	}
}

func TestBudgetAtLimitDoesWhatTheUserPicked(t *testing.T) {
	store := Open(t.TempDir())
	now := time.Now()
	store.Add([]Call{{ID: "x", Time: now.Add(-time.Minute), Agent: "claude", Project: "/p", Model: "claude-opus-5-5", Input: 900, Output: 100}})
	budgets := []proto.TokenBudget{
		{ID: "all", Period: proto.BudgetDay, Scope: proto.BudgetAll, Tokens: 1250, AtLimit: proto.BudgetNotify},
		{ID: "claude", Period: proto.BudgetDay, Scope: proto.BudgetAgent, Target: "claude", Tokens: 1000, AtLimit: proto.BudgetAsk},
		{ID: "codex", Period: proto.BudgetDay, Scope: proto.BudgetAgent, Target: "codex", Tokens: 10, AtLimit: proto.BudgetStop},
	}
	states := store.Budgets(budgets, NewPrices(nil), now)
	if states[0].Level != 80 || states[1].Level != 100 || states[2].Level != 0 {
		t.Fatalf("levels: %d %d %d", states[0].Level, states[1].Level, states[2].Level)
	}
	if action, b := AtLimit(states, "claude", "/p", ""); action != proto.BudgetAsk || b.ID != "claude" {
		t.Fatalf("claude: %q %s", action, b.ID)
	}
	if action, _ := AtLimit(states, "codex", "/p", ""); action != "" {
		t.Fatalf("codex has used nothing, yet %q", action)
	}
	budgets[1].AtLimit = proto.BudgetStop
	if action, _ := AtLimit(store.Budgets(budgets, NewPrices(nil), now), "claude", "/q", ""); action != proto.BudgetStop {
		t.Fatalf("stop: %q", action)
	}
	if !Near(states) {
		t.Fatal("a budget at 80% isn't near")
	}
	var alerts Alerts
	if crossed := alerts.Crossed(states); len(crossed) != 2 {
		t.Fatalf("crossed: %d", len(crossed))
	}
	if crossed := alerts.Crossed(states); len(crossed) != 0 {
		t.Fatalf("told twice: %d", len(crossed))
	}
}

func TestPricesNameModelsAsProvidersDo(t *testing.T) {
	p := NewPrices([]proto.TokenPrice{{Model: "my-local-*", Input: 0, Output: 0}, {Model: "claude-opus-5-5", Config: "work-api", Input: 1, Output: 2}})
	for _, tc := range []struct{ provider, model, config, want string }{
		{"anthropic", "claude-sonnet-4-5-20250929", "", "claude-sonnet-4*"},
		{"openrouter", "anthropic/claude-sonnet-4.5", "", "claude-sonnet-4*"},
		{"anthropic", "claude-opus-5-5[1m]", "", "claude-opus-5-5*"},
		{"anthropic", "claude-opus-5-5", "work-api", "claude-opus-5-5"},
		{"bedrock", "us.anthropic.claude-haiku-4-5-20251001-v1:0", "", "claude-haiku-4-5*"},
		{"ollama", "my-local-coder", "", "my-local-*"},
	} {
		got, ok := p.Find(tc.provider, tc.model, tc.config)
		if !ok || got.Model != tc.want {
			t.Errorf("%s %s: %q %v, want %q", tc.provider, tc.model, got.Model, ok, tc.want)
		}
	}
	if _, ok := p.Find("", "gpt-6-luna", ""); ok {
		t.Error("an unknown model got a price")
	}
	if cost, ok := p.Cost(Call{Provider: "local", Model: "qwen3", Input: 1e6}); !ok || cost != 0 {
		t.Errorf("a local model costs %v", cost)
	}
}

package daemon

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"

	"github.com/enowx/enowx/core/session"
	"github.com/enowx/enowx/proto"
)

// The user's rules for GOA are kept, never holding a secret, in the guide
// of a GOA starting, and told to one running with its next prompt once
// changed.
func TestGOARules(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	srv, _ := hookServer(t)
	c := connect(t, srv, nil)
	save := func(rules string) error { return c.Call(proto.MethodGOARulesSave, proto.GOARules{Rules: rules}, nil) }
	if err := save("- Give frontend tasks to Codex"); err != nil {
		t.Fatal(err)
	}
	var got proto.GOARules
	if err := c.Call(proto.MethodGOARules, nil, &got); err != nil || got.Rules != "- Give frontend tasks to Codex" {
		t.Fatalf("kept: %+v %v", got, err)
	}
	if err := save("- key ghp_abcdefghijklmnopqrstuvwxyz0123456789"); err == nil || !strings.Contains(err.Error(), "secret") {
		t.Fatalf("a secret kept: %v", err)
	}
	if chat := srv.goaChat(chatFor{}, session.Meta{GOA: true}); !strings.Contains(chat.guide, "Give frontend tasks to Codex") {
		t.Fatal("not in a GOA's guide")
	}
	if chat := srv.goaChat(chatFor{}, session.Meta{}); strings.Contains(chat.guide, "Give frontend tasks") {
		t.Fatal("in another session's guide")
	}

	meta := session.Meta{Agent: "claude", GOA: true}
	meta.Rules = srv.rulesAtStart(meta, srv.goaDir(), false)
	g, err := srv.sessions.Create(session.Spec{Command: []string{"sh", "-c", "cat"}, Dir: srv.goaDir(), Cols: 80, Rows: 24, Meta: meta})
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(session.EnvSession, g.ID())
	prompt := func() string {
		_, out, _ := hookOf(`{"prompt":"next task"}`, "--event", proto.HookPromptStart)
		return out
	}
	if out := prompt(); strings.Contains(out, "changed their rules") {
		t.Fatalf("its first prompt told what its guide holds: %s", out)
	}
	if err := save("- Ask me before merging"); err != nil {
		t.Fatal(err)
	}
	if out := prompt(); !strings.Contains(out, "Ask me before merging") {
		t.Fatalf("the change not told: %s", out)
	}
	if out := prompt(); strings.Contains(out, "Ask me before merging") {
		t.Fatalf("told twice: %s", out)
	}
}

// An AI writes rules from what the user asked, with the current ones;
// nothing is saved, and no decision model is asked.
func TestDraftRules(t *testing.T) {
	var asked string
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{{"id": "writer"}}})
			return
		}
		var in struct {
			Messages []struct{ Content string } `json:"messages"`
		}
		json.NewDecoder(r.Body).Decode(&in)
		asked = in.Messages[len(in.Messages)-1].Content
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{
			"content": "```markdown\n- Ask before merging to main\n```"}}}})
	}))
	defer model.Close()
	srv := startServer(t)
	c := connect(t, srv, nil)
	if err := srv.judge.Settings().Set(proto.DecideSettings{Policy: proto.DecideLocalOnly, Own: []proto.Judge{
		{Kind: proto.JudgeDecision, Provider: proto.ProviderLocal, URL: "http://127.0.0.1:1", Model: "picks-only", Label: "Picker"},
		{Kind: proto.JudgeLocal, URL: model.URL + "/v1", Model: "writer", Label: "Writer"}}}); err != nil {
		t.Fatal(err)
	}
	var d proto.RulesDraft
	err := c.Call(proto.MethodRulesDraft, proto.RulesDraftParams{For: proto.RulesForGOA, Ask: "jangan merge tanpa tanya", Current: "- Use Codex"}, &d)
	if err != nil || d.Rules != "- Ask before merging to main" || d.By != "Writer" {
		t.Fatalf("draft %+v %v", d, err)
	}
	first := asked
	// The AI sees what's saved too, even if the editor holds nothing.
	if err := c.Call(proto.MethodGOARulesSave, proto.GOARules{Rules: "- Saved earlier"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := c.Call(proto.MethodRulesDraft, proto.RulesDraftParams{For: proto.RulesForGOA, Ask: "one more"}, &d); err != nil || !strings.Contains(asked, "- Saved earlier") {
		t.Fatalf("asked %q: %v", asked, err)
	}
	if err := c.Call(proto.MethodGOARulesSave, proto.GOARules{Clear: true}, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(first, "jangan merge tanpa tanya") || !strings.Contains(first, "- Use Codex") {
		t.Fatalf("asked %q", first)
	}
	if r := srv.goaRules(); r.Rules != "" {
		t.Fatalf("saved without the user: %q", r.Rules)
	}
	// What was saved over can be brought back, the newest first.
	var versions []proto.RulesVersion
	if err := c.Call(proto.MethodRulesHistory, proto.RulesDraftParams{For: proto.RulesForGOA}, &versions); err != nil || len(versions) != 1 || versions[0].Rules != "- Saved earlier" {
		t.Fatalf("history %+v %v", versions, err)
	}
	if err := c.Call(proto.MethodRulesDraft, proto.RulesDraftParams{For: proto.RulesForGOA}, &d); err == nil {
		t.Fatal("nothing asked, and an answer")
	}
	asked = ""
	if err := c.Call(proto.MethodRulesDraft, proto.RulesDraftParams{For: proto.RulesForGOA, Ask: "use ghp_abcdefghijklmnopqrstuvwxyz0123456789"}, &d); err == nil || asked != "" {
		t.Fatalf("a secret sent to the AI: %v %q", err, asked)
	}
}

package decide

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/enowx/enowx/proto"
)

// soServer is a System One server: it answers each question by answer(its
// id, the question, the state), keeps what it was asked, and wraps its
// answer as Workers AI does when wrap.
func soServer(t *testing.T, wrap bool, answer func(id string, q map[string]any, state map[string]any) map[string]any) (*httptest.Server, *[]map[string]any) {
	t.Helper()
	var asked []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			http.Error(w, "no key", http.StatusUnauthorized)
			return
		}
		var in map[string]any
		json.NewDecoder(r.Body).Decode(&in)
		in["path"] = r.URL.Path
		asked = append(asked, in)
		state, _ := in["state"].(map[string]any)
		answers := map[string]any{}
		for id, q := range in["questions"].(map[string]any) {
			answers[id] = answer(id, q.(map[string]any), state)
		}
		out := map[string]any{"model": in["model"], "answers": answers, "usage": map[string]int{"input_tokens": 1000, "output_tokens": 1}}
		if wrap {
			out = map[string]any{"result": out, "success": true}
		}
		json.NewEncoder(w).Encode(out)
	}))
	t.Cleanup(srv.Close)
	return srv, &asked
}

func choiceOf(c string, p float64) map[string]any {
	return map[string]any{"type": "choice", "choice": c, "confidence": p, "probabilities": map[string]float64{c: p}}
}

func decisionEngine(t *testing.T, judges ...proto.Judge) *Engine {
	t.Helper()
	e := engine(t, Found{})
	e.KeyOf = func(string) (string, error) { return "tok", nil }
	e.EnvOf = func(cred string) (map[string]string, error) {
		return map[string]string{"CLOUDFLARE_API_TOKEN": "tok", "CLOUDFLARE_ACCOUNT_ID": "acct1", "TYPESAFE_API_KEY": "tok", "OPENAI_API_KEY": "tok"}, nil
	}
	st := e.settings.Get()
	st.Own = judges
	for _, u := range proto.DecideUses {
		st.Uses[u] = proto.DecideUse{On: true}
	}
	if err := e.settings.Set(st); err != nil {
		t.Fatal(err)
	}
	return e
}

// A choice use asked of a decision model: one typed choice over the
// situation as state, the verdict its pick, the reason its meaning.
func TestDecisionModelChoice(t *testing.T) {
	srv, asked := soServer(t, false, func(id string, q map[string]any, state map[string]any) map[string]any {
		if strings.Contains(state["command"].(string), "rm -rf ~") {
			return choiceOf(proto.VerdictRefuse, 0.97)
		}
		return choiceOf(proto.VerdictAllow, 0.93)
	})
	e := decisionEngine(t, proto.Judge{Kind: proto.JudgeDecision, Provider: proto.ProviderTypeSafe, URL: srv.URL + "/v1/systemone", Cred: "ts", Model: "jev-latest"})
	d, err := e.Decide(context.Background(), Request{DecideAsk: proto.DecideAsk{Use: proto.DecideApprove, Command: "rm -rf ~", Project: "shop"}, Agent: "claude"})
	if err != nil || d.Verdict != proto.VerdictRefuse || d.Confidence != 0.97 || !strings.Contains(d.Why, "home folder") || !strings.HasPrefix(d.Judge, "decision:") {
		t.Fatalf("decision %+v %v", d, err)
	}
	q := (*asked)[0]["questions"].(map[string]any)["verdict"].(map[string]any)
	if (*asked)[0]["model"] != "jev-latest" || q["type"] != "choice" || len(q["criteria"].(map[string]any)) != 3 {
		t.Fatalf("asked %+v", (*asked)[0])
	}
}

// Clef on Workers AI: its address made from the credential's account ID,
// its answer unwrapped; a trim keeps the lines the model picks, copied.
func TestCloudflareTrim(t *testing.T) {
	srv, asked := soServer(t, true, func(id string, q map[string]any, state map[string]any) map[string]any {
		var n int
		json.Unmarshal([]byte(strings.TrimPrefix(id, "g")), &n)
		text := state["groups"].([]any)[n].(string)
		if strings.Contains(text, "FAIL") || strings.Contains(text, "want 11") {
			return map[string]any{"type": "noul", "noul": 0.95}
		}
		return map[string]any{"type": "noul", "noul": 0.03}
	})
	e := decisionEngine(t, proto.Judge{Kind: proto.JudgeDecision, Provider: proto.ProviderCloudflare, URL: srv.URL + "/accounts/{account}/ai/run/@cf/cloudflare/{model}", Cred: "cf", Model: "clef-flash"})
	var out strings.Builder
	for i := range 400 {
		out.WriteString("--- PASS: TestCase" + strings.Repeat("x", i%7) + "\n")
		if i == 200 {
			out.WriteString("    invoice_test.go:42: Total = 10.99, want 11.00\n--- FAIL: TestTotal\n")
		}
	}
	out.WriteString("FAIL\tshop/billing\t0.4s\n")
	d, err := e.Decide(context.Background(), Request{DecideAsk: proto.DecideAsk{Use: proto.DecideTrim, Command: "go test ./...", Output: out.String(), Exit: 1}})
	if err != nil || d.Verdict != proto.VerdictTrimmed {
		t.Fatalf("trim %+v %v", d, err)
	}
	for _, want := range []string{"want 11.00", "--- FAIL: TestTotal", "FAIL\tshop/billing"} {
		if !strings.Contains(d.Text, want) {
			t.Fatalf("lost %q: %q", want, d.Text)
		}
	}
	if strings.Contains(d.Text, "PASS") || (*asked)[0]["path"] != "/accounts/acct1/ai/run/@cf/cloudflare/clef-flash" || d.Cost == 0 {
		t.Fatalf("kept %q, asked %v, cost %v", d.Text, (*asked)[0]["path"], d.Cost)
	}
}

// A memory is a sentence the model picks from what the agent said, copied
// as it is; RAG context keeps the parts it says bear on the prompt.
func TestDecisionModelPicksNotWrites(t *testing.T) {
	srv, _ := soServer(t, false, func(id string, q map[string]any, state map[string]any) map[string]any {
		switch {
		case id == "has":
			return map[string]any{"type": "noul", "noul": 0.9}
		case id == "which":
			for k, v := range q["criteria"].(map[string]any) {
				if strings.Contains(v.(string), "because") {
					return choiceOf(k, 0.8)
				}
			}
		case strings.HasPrefix(id, "part"):
			p := 0.1
			if id == "part1" {
				p = 0.9
			}
			return map[string]any{"type": "noul", "noul": p}
		}
		return choiceOf("s0", 0.5)
	})
	e := decisionEngine(t, proto.Judge{Kind: proto.JudgeDecision, URL: srv.URL + "/v1/systemone", Cred: "x", Model: "kev-latest"})
	said := "I updated the invoice code. We round half up because the tax office requires it. Tests pass now."
	d, err := e.Decide(context.Background(), Request{DecideAsk: proto.DecideAsk{Use: proto.DecideMemory, Text: said}})
	if err != nil || d.Verdict != proto.VerdictDecision || d.Text != "We round half up because the tax office requires it." {
		t.Fatalf("memory %+v %v", d, err)
	}
	c, err := e.Decide(context.Background(), Request{DecideAsk: proto.DecideAsk{Use: proto.DecideContext, Prompt: "fix rounding", Parts: []string{"auth code", "invoice rounding", "readme"}}})
	if err != nil || c.Verdict != proto.VerdictRelevant || len(c.Keep) != 1 || c.Keep[0] != 2 {
		t.Fatalf("context %+v %v", c, err)
	}
}

// OpenAI's Decisions API, in its own shape: a list of questions with
// choices as objects, answers read back.
func TestOpenAIDecisions(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&got)
		json.NewEncoder(w).Encode(map[string]any{"model": "gpt-6-luna", "answers": []any{map[string]any{"type": "choice", "name": "verdict", "choice": "waiting", "confidence": 0.9,
			"probabilities": []any{map[string]any{"value": "waiting", "probability": 0.92}}}}, "usage": map[string]int{"input_tokens": 50}})
	}))
	defer srv.Close()
	e := decisionEngine(t, proto.Judge{Kind: proto.JudgeDecision, Provider: proto.ProviderOpenAI, URL: srv.URL + "/v1/decisions", Cred: "oa", Model: "gpt-6-luna", Protocol: DecisionOpenAI})
	d, err := e.Decide(context.Background(), Request{DecideAsk: proto.DecideAsk{Use: proto.DecideState, Screen: "Proceed? (y/n)"}})
	if err != nil || d.Verdict != proto.VerdictWaiting {
		t.Fatalf("decision %+v %v", d, err)
	}
	qs := got["questions"].([]any)
	q := qs[0].(map[string]any)
	if _, ok := got["input"].(string); !ok || q["name"] != "verdict" || len(q["choices"].([]any)) != 4 || q["choices"].([]any)[0].(map[string]any)["value"] == nil {
		t.Fatalf("asked %+v", got)
	}
}

// Decision models are found and asked first; one that fails hands over to
// a small chat model; local only keeps the local decision model alone.
func TestDecisionFirstThenFallback(t *testing.T) {
	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			json.NewEncoder(w).Encode(map[string]any{"models": []map[string]string{{"name": "nimble:latest"}, {"name": "llama3:8b"}}})
		default:
			http.Error(w, "down", http.StatusInternalServerError)
		}
	}))
	defer ollama.Close()
	chat := localServer(t, []string{"small"}, func(string) string { return `{"verdict":"allow","confidence":0.95,"why":"read-only"}` })
	found := Found{Creds: []proto.CredentialInfo{{Name: "ts", Platform: "typesafe"}, {Name: "cf", Platform: "cloudflare"}, {Name: "gh", Platform: "github"}},
		Local: []string{chat}, LocalDecision: []string{ollama.URL}}
	var ids []string
	for _, j := range Ordered(proto.DecideSettings{}, Detect(context.Background(), found)) {
		ids = append(ids, j.ID)
	}
	want := []string{"decision:ts:jev-latest", "decision:cf:clef-flash", "decision:cf:clef", "decision:" + ollama.URL + "/v1/systemone:nimble:latest", "local:" + chat + ":small"}
	if strings.Join(ids, " ") != strings.Join(want, " ") {
		t.Fatalf("judges %q", ids)
	}
	e := engine(t, Found{Local: []string{chat}, LocalDecision: []string{ollama.URL}})
	st := e.settings.Get()
	st.Policy = proto.DecideLocalOnly
	st.Uses[proto.DecideApprove] = proto.DecideUse{On: true}
	e.settings.Set(st)
	d, _ := e.Decide(context.Background(), Request{DecideAsk: proto.DecideAsk{Use: proto.DecideApprove, Command: "git status"}, Agent: "claude"})
	if d.Verdict != proto.VerdictAllow || !strings.HasPrefix(d.Judge, "local:") || len(d.Tried) != 1 || !strings.Contains(d.Tried[0], "nimble") {
		t.Fatalf("fallback %+v", d)
	}
}

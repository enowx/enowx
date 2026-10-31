package daemon

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/enowx/enowx/core/session"
	"github.com/enowx/enowx/proto"
)

// laterJudge is a local model server judging the later uses as a person
// would: a typo is easy and a migration hard, the search results about
// invoices bear on an invoice prompt and none on one about colors, an
// answer that settles on cents holds a decision, an outage is urgent and
// lunch quiet, a timeout is flaky and a wrong total real.
func laterJudge(t *testing.T) string {
	t.Helper()
	part := regexp.MustCompile(`(?s)<part n="(\d+)">(.*?)</part>`)
	prompt := regexp.MustCompile(`(?s)<prompt>(.*?)</prompt>`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{{"id": "judge"}}})
			return
		}
		var in struct {
			Messages []struct{ Content string } `json:"messages"`
		}
		json.NewDecoder(r.Body).Decode(&in)
		q := in.Messages[len(in.Messages)-1].Content
		asked := ""
		if m := prompt.FindStringSubmatch(q); m != nil {
			asked = m[1]
		}
		answer := `{"verdict":"medium","confidence":0.9,"why":"ordinary work"}`
		switch {
		case strings.Contains(q, "How hard") && strings.Contains(asked, "typo"):
			answer = `{"verdict":"easy","confidence":0.9,"why":"a one-word fix"}`
		case strings.Contains(q, "How hard") && strings.Contains(asked, "migrate"):
			answer = `{"verdict":"hard","confidence":0.9,"why":"a schema migration across services"}`
		case strings.Contains(q, "<part n="):
			var keep []string
			if !strings.Contains(asked, "colors") {
				for _, m := range part.FindAllStringSubmatch(q, -1) {
					if strings.Contains(m[2], "invoice") {
						keep = append(keep, m[1])
					}
				}
			}
			if len(keep) == 0 {
				answer = `{"verdict":"none","confidence":0.9,"why":"nothing about colors","keep":[]}`
			} else {
				answer = `{"verdict":"relevant","confidence":0.9,"why":"the invoice code","keep":[` + strings.Join(keep, ",") + `]}`
			}
		case strings.Contains(q, "<answer>") && strings.Contains(q, "cents"):
			answer = `{"verdict":"decision","confidence":0.9,"why":"a choice of representation","text":"Totals are kept in integer cents: floats rounded them wrong."}`
		case strings.Contains(q, "<answer>"):
			answer = `{"verdict":"nothing","confidence":0.9,"why":"a status report"}`
		case strings.Contains(q, "<message>") && strings.Contains(q, "down"):
			answer = `{"verdict":"urgent","confidence":0.95,"why":"checkout is down"}`
		case strings.Contains(q, "<message>"):
			answer = `{"verdict":"quiet","confidence":0.9,"why":"small talk"}`
		case strings.Contains(q, "did it fail by chance") && strings.Contains(q, "i/o timeout"):
			answer = `{"verdict":"flaky","confidence":0.9,"why":"a network timeout"}`
		case strings.Contains(q, "did it fail by chance"):
			answer = `{"verdict":"real","confidence":0.9,"why":"the total is wrong"}`
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": answer}}}})
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/v1"
}

// judgeLater turns the later uses on, judged by a local laterJudge only.
func judgeLater(t *testing.T, srv *Server) {
	t.Helper()
	on := proto.DecideUse{On: true}
	if err := srv.judge.Settings().Set(proto.DecideSettings{Policy: proto.DecideLocalOnly,
		Own: []proto.Judge{{Kind: proto.JudgeLocal, URL: laterJudge(t), Model: "judge"}},
		Uses: map[string]proto.DecideUse{proto.DecideRoute: on, proto.DecideContext: on, proto.DecideMemory: on,
			proto.DecideChat: on, proto.DecideFlaky: on}}); err != nil {
		t.Fatal(err)
	}
}

// A model per prompt, through the daemon: a typo goes to Claude Code's
// cheapest model on low effort, a migration keeps Codex's own model on
// high; off, it says so. A chat message: an outage urgent, lunch quiet,
// but never quiet when it names the user. Checks that failed: a timeout
// flaky, a wrong total real, shown on each.
func TestRouteChatAndFlakyThroughTheDaemon(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	srv, sess := hookServer(t)
	judgeLater(t, srv)
	c := connect(t, srv, nil)

	r, err := c.DecideRoute(proto.DecideRouteParams{Agent: "claude", Prompt: "fix the typo in the README"})
	if err != nil || r.Model != "claude-haiku-5-5" || r.Effort != "low" || r.Difficulty != proto.VerdictEasy || r.Why == "" ||
		strings.Join(r.Switch, "|") != "/model claude-haiku-5-5|/effort low" {
		t.Fatalf("easy: %+v %v", r, err)
	}
	r, err = c.DecideRoute(proto.DecideRouteParams{Agent: "codex", Prompt: "migrate the orders table to the new schema"})
	if err != nil || r.Model != "" || r.Effort != "high" || r.Difficulty != proto.VerdictHard {
		t.Fatalf("hard: %+v %v", r, err)
	}
	if log := srv.judge.Log(proto.DecideLogParams{Use: proto.DecideRoute}); len(log) != 2 || log[0].Acted != proto.ActedRouted {
		t.Fatalf("route log %+v", log)
	}

	chat := func(text string, mention bool) proto.Decision {
		t.Helper()
		d, err := c.AskDecision(proto.DecideAsk{Use: proto.DecideChat, From: "Ana", Place: "Shop", Text: text, Mention: mention})
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	if d := chat("checkout is down for everyone, can you look?", false); d.Acted != proto.ActedUrgent || !d.Sure {
		t.Fatalf("urgent: %+v", d)
	}
	if d := chat("lunch at noon?", false); d.Acted != proto.ActedQuiet {
		t.Fatalf("quiet: %+v", d)
	}
	if d := chat("lunch at noon, you?", true); d.Verdict != proto.VerdictQuiet || d.Acted != proto.ActedNothing {
		t.Fatalf("a mention kept quiet: %+v", d)
	}

	dir := t.TempDir()
	srv.runner.Run(sess.ID(), dir, []proto.Check{
		{Name: "net", Command: `echo "dial tcp 10.0.0.1:443: i/o timeout"; exit 1`},
		{Name: "unit", Command: `echo "--- FAIL: TestTotal: got 9, want 10"; exit 1`},
		{Name: "lint", Command: "true"},
	})
	judged := func() map[string]proto.CheckRun {
		out := map[string]proto.CheckRun{}
		for _, run := range srv.runner.Results(sess.ID()) {
			out[run.Name] = run
		}
		return out
	}
	waitFor(t, "the failed checks judged", func() bool {
		j := judged()
		return j["net"].Judged != "" && j["unit"].Judged != "" && j["lint"].State == proto.CheckPassed
	})
	if j := judged(); j["net"].Judged != proto.VerdictFlaky || j["net"].JudgedWhy != "a network timeout" || j["net"].Judging ||
		j["unit"].Judged != proto.VerdictReal || j["lint"].Judged != "" {
		t.Fatalf("checks judged: %+v", j)
	}

	// Off: a route says so; a check that fails isn't judged.
	st := srv.judge.Settings().Get()
	st.Uses[proto.DecideRoute], st.Uses[proto.DecideFlaky] = proto.DecideUse{}, proto.DecideUse{}
	srv.judge.Settings().Set(st)
	if _, err := c.DecideRoute(proto.DecideRouteParams{Agent: "claude", Prompt: "fix the typo"}); err == nil || !strings.Contains(err.Error(), "off") {
		t.Fatalf("route while off: %v", err)
	}
	srv.runner.Run(sess.ID(), dir, []proto.Check{{Name: "unit", Command: "exit 1"}})
	waitFor(t, "the check run again", func() bool { return judged()["unit"].State == proto.CheckFailed })
	time.Sleep(200 * time.Millisecond)
	if j := judged()["unit"]; j.Judged != "" || j.Judging {
		t.Fatalf("judged while off: %+v", j)
	}
}

// RAG choices, through the daemon: a prompt gets only the parts the judge
// says bear on it, none when none do; a session's last answer is offered
// as a memory only when it holds a decision, distilled.
func TestRAGChoicesThroughTheDaemon(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	srv, _ := hookServer(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	judgeLater(t, srv)
	var mu sync.Mutex
	var offers []proto.RAGRememberParams
	connect(t, srv, func(ev proto.Event) {
		if ev.Name == proto.EventRAGOffer {
			var p proto.RAGRememberParams
			json.Unmarshal(ev.Data, &p)
			mu.Lock()
			offers = append(offers, p)
			mu.Unlock()
		}
	})

	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "billing"), 0o755)
	os.MkdirAll(filepath.Join(dir, "ui"), 0o755)
	os.WriteFile(filepath.Join(dir, "billing", "invoice.go"), []byte("package billing\n\n// Total sums an invoice's lines with tax.\nfunc Total(lines []float64, tax float64) float64 {\n\treturn 0\n}\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "ui", "theme.go"), []byte("package ui\n\n// Palette is the app's colors: tax and total screens use it too.\nvar Palette = []string{\"#fff\"}\n"), 0o644)
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Skipf("git: %s", out)
	}
	if err := srv.ragSettings.Set(proto.RAGSettings{Embedder: proto.RAGOpenAI, Model: "fake", BaseURL: ragEmbeddings(t)}); err != nil {
		t.Fatal(err)
	}
	st, _ := srv.setRAGProject(proto.RAGProjectParams{Project: dir, Settings: proto.RAGProject{On: true, Context: true, Offer: true}})
	srv.setRAGProject(proto.RAGProjectParams{Project: dir, Settings: proto.RAGProject{On: true, Context: true, Offer: true, Consent: st.Embedder}})
	if _, err := srv.ragIndex(dir); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the index", func() bool { s := srv.ragStatus(dir); return !s.Indexing && s.Chunks >= 2 })

	inProject, err := srv.sessions.Create(session.Spec{Command: []string{"sh", "-c", "cat"}, Dir: dir, Cols: 80, Rows: 24, Meta: session.Meta{Project: dir}})
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(session.EnvSession, inProject.ID())
	_, ctx, _ := hookOf(`{"prompt":"fix the invoice total tax"}`, "--event", proto.HookPromptStart)
	if !strings.Contains(ctx, "billing/invoice.go") || strings.Contains(ctx, "ui/theme.go") {
		t.Fatalf("the parts kept: %q", ctx)
	}
	_, ctx, _ = hookOf(`{"prompt":"make the colors of the total and tax screens warmer"}`, "--event", proto.HookPromptStart)
	if strings.Contains(ctx, "enowX RAG") {
		t.Fatalf("parts kept though none bear on it: %q", ctx)
	}
	acted := map[string]int{}
	for _, d := range srv.judge.Log(proto.DecideLogParams{Use: proto.DecideContext}) {
		acted[d.Acted]++
	}
	if acted[proto.ActedChosen] != 1 || acted[proto.ActedDropped] != 1 {
		t.Fatalf("context log %v", acted)
	}

	// Two Claude Code sessions end: one settled on cents, one only reports.
	ended := func(said string) proto.SessionInfo {
		t.Helper()
		work := t.TempDir()
		s, err := srv.sessions.Create(session.Spec{Command: []string{"sleep", "30"}, Dir: work, Meta: session.Meta{Agent: "claude", Project: dir}})
		if err != nil {
			t.Fatal(err)
		}
		folder := filepath.Join(home, ".claude", "projects", claudeFolderOf(work))
		os.MkdirAll(folder, 0o755)
		text, _ := json.Marshal(said)
		lines := `{"type":"user","message":{"content":"fix the totals"}}` + "\n" +
			`{"type":"assistant","message":{"content":[{"type":"text","text":` + string(text) + `}]}}` + "\n"
		os.WriteFile(filepath.Join(folder, "s.jsonl"), []byte(lines), 0o600)
		return s.Info()
	}
	srv.ragOffer(ended("Done. I moved totals to integer cents, since floats rounded 0.1+0.2 wrong; all tests pass."))
	srv.ragOffer(ended("All done: 3 files changed, tests pass."))
	waitFor(t, "the offer", func() bool { mu.Lock(); defer mu.Unlock(); return len(offers) >= 1 })
	time.Sleep(300 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if len(offers) != 1 || offers[0].Text != "Totals are kept in integer cents: floats rounded them wrong." {
		t.Fatalf("offers %+v", offers)
	}
}

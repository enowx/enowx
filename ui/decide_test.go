package ui

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/decide"
	"github.com/enowx/enowx/daemon"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
)

// Settings → Decisions lists the judges and tests one, turns a use on,
// and logs a decision to mark wrong; a loop the judge stopped asks
// whether to let the agent go on.
func TestDecisionsSection(t *testing.T) {
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{{"id": "judge-8b"}}})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{
			"content": `{"verdict":"allow","confidence":0.96,"why":"a read-only git command"}`}}}})
	}))
	defer model.Close()
	srv, err := daemon.NewServer(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	a1, b1 := net.Pipe()
	go srv.ServeConn(a1)
	app := New(Options{})
	cl, err := client.New(b1, func(ev proto.Event) { app.decideEvent(ev) })
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	if _, err := cl.SetDecideSettings(proto.DecideSettings{Policy: proto.DecideLocalOnly, Own: []proto.Judge{{Kind: proto.JudgeLocal, URL: model.URL + "/v1", Model: "judge-8b", Label: "Judge 8B"}}}); err != nil {
		t.Fatal(err)
	}
	posted := make(chan func(), 64)
	app.onMain = func(fn func()) { posted <- fn }
	app.client, app.agents = cl, agent.List()
	app.host.Name = "this Mac"
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(theme.Window())
		ui.Column(c).Width(900).Height(900).Background(theme.Background).TextColor(theme.Text).Children(func() {
			app.decideSection(c)
			app.confirmDialog(c)
			app.decisionDialog(c)
		})
	}, 900, 900)
	frames := func(until func() bool, what string) {
		deadline := time.Now().Add(15 * time.Second)
		for !until() {
			select {
			case fn := <-posted:
				fn()
			case <-time.After(30 * time.Millisecond):
			}
			tt.Frame()
			if time.Now().After(deadline) {
				t.Fatalf("never: %s: %q", what, tt.Texts())
			}
		}
	}
	tt.Frame()
	frames(func() bool { return hasText(tt, "Judge 8B") }, "the judge listed")
	st := &app.decideSet
	app.testJudges(st, "local:"+model.URL+"/v1:judge-8b")
	frames(func() bool { return hasText(tt, "s") && len(st.testing) == 0 && st.judges[len(st.judges)-1].OK }, "the test")
	if os.Getenv("SHOTS") != "" {
		shot(t, tt, "decisions-judges.png")
	}

	if err := tt.Click("Uses"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("Use: Approving commands"); err != nil {
		t.Fatal(err)
	}
	frames(func() bool {
		s, _ := cl.DecideSettings()
		return s.Uses[proto.DecideApprove].On
	}, "the use turned on")
	frames(func() bool { return hasText(tt, "Add the hook…") }, "the hook offered")
	if os.Getenv("SHOTS") != "" {
		shot(t, tt, "decisions-uses.png")
	}

	var d proto.Decision
	if err := cl.Call(proto.MethodDecideAsk, proto.DecideAsk{Use: proto.DecideApprove, Command: "git status"}, &d); err != nil || d.Verdict != proto.VerdictAllow {
		t.Fatalf("ask: %+v %v", d, err)
	}
	if err := tt.Click("Log"); err != nil {
		t.Fatal(err)
	}
	frames(func() bool { return hasText(tt, "a read-only git command") }, "the log")
	if err := tt.Click("This was wrong"); err != nil {
		t.Fatal(err)
	}
	frames(func() bool {
		log, _ := cl.DecideLog(proto.DecideLogParams{})
		return len(log) > 0 && log[0].Wrong
	}, "marked wrong")
	if os.Getenv("SHOTS") != "" {
		shot(t, tt, "decisions-log.png")
	}

	// A loop the judge stopped: the user lets the agent go on.
	app.decision = &proto.Decision{ID: d.ID, Use: proto.DecideLoop, Session: "s1", Verdict: proto.VerdictLooping, Why: "The same failing test five times.", Acted: proto.ActedPaused}
	tt.Frame()
	if !hasText(tt, "it seemed to loop") {
		t.Fatalf("no dialog: %q", tt.Texts())
	}
	if err := tt.Click("Leave it"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if app.decision != nil || strings.Contains(strings.Join(tt.Texts(), " "), "it seemed to loop") {
		t.Fatal("the dialog stayed")
	}
}

// Adding a decision model: a provider fills in its address and model; a
// custom address may speak OpenAI's Decisions shape; an address is needed.
func TestJudgeFormDecisionModel(t *testing.T) {
	f := judgeForm{kind: proto.JudgeDecision}
	for _, p := range decide.DecisionPresets {
		if p.Provider == proto.ProviderCloudflare {
			f.pick(p)
		}
	}
	f.cred = "cf"
	j, err := f.judge()
	if err != "" || j.Model != "clef-flash" || !strings.Contains(j.URL, "{account}") || j.Provider != proto.ProviderCloudflare {
		t.Fatalf("cloudflare %+v %q", j, err)
	}
	f.pick(decide.DecisionPresets[len(decide.DecisionPresets)-1]) // another address
	if _, err := f.judge(); err == "" {
		t.Fatal("no address taken")
	}
	f.url, f.model, f.openAI = "https://d.example.com/v1/decisions", "m", true
	if j, err := f.judge(); err != "" || j.Protocol != decide.DecisionOpenAI || j.Provider != "" {
		t.Fatalf("custom %+v %q", j, err)
	}
}

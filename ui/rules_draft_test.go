//go:build !windows

package ui

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/daemon"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
)

// What an AI writes for rules is only ever added: whatever it answers, new
// rules, rules rewritten or nothing, what was there stays byte for byte.
func TestDraftedRulesOnlyAdd(t *testing.T) {
	const existing = "### Mine\n\n- Keep   this spacing.\n- Reply in Indonesian."
	var answer atomic.Value
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{{"id": "m"}}})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": answer.Load().(string)}}}})
	}))
	defer model.Close()
	srv, err := daemon.NewServer(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	a1, b1 := net.Pipe()
	go srv.ServeConn(a1)
	cl, err := client.New(b1, func(proto.Event) {})
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	if _, err := cl.SetDecideSettings(proto.DecideSettings{Policy: proto.DecideLocalOnly, Own: []proto.Judge{{Kind: proto.JudgeLocal, URL: model.URL + "/v1", Model: "m", Label: "Local"}}}); err != nil {
		t.Fatal(err)
	}
	app := New(Options{})
	posted := make(chan func(), 16)
	app.onMain = func(fn func()) { posted <- fn }
	app.client = cl
	draft := func(text *string, d *rulesDraft, says string) {
		t.Helper()
		answer.Store(says)
		d.ask = "add a rule"
		app.draftRules(d, proto.RulesForGOA, "", text)
		select {
		case fn := <-posted:
			fn()
		case <-time.After(10 * time.Second):
			t.Fatal("no answer")
		}
	}
	cases := []struct {
		name, says, adds string
		conflicts        int
		err              bool
	}{
		{name: "new rules", says: "- Never use emoji.", adds: "- Never use emoji."},
		{name: "rules there repeated", says: "- Reply in Indonesian.\n- Never use emoji.", adds: "- Never use emoji."},
		{name: "all rewritten, as a model may", says: "- Reply in English.", adds: "- Reply in English."},
		{name: "a conflict", says: "- Reply in English.\nCONFLICT: goes against Reply in Indonesian", adds: "- Reply in English.", conflicts: 1},
		{name: "nothing", says: "   ", err: true},
	}
	for _, tc := range cases {
		text, d := existing, &rulesDraft{}
		draft(&text, d, tc.says)
		if !strings.HasPrefix(text, existing) {
			t.Errorf("%s: what was there changed: %q", tc.name, text)
		}
		if tc.err {
			if d.err == "" || text != existing {
				t.Errorf("%s: err %q, text %q", tc.name, d.err, text)
			}
			continue
		}
		if text != existing+"\n"+tc.adds || len(d.conflicts) != tc.conflicts {
			t.Errorf("%s: text %q, conflicts %q", tc.name, text, d.conflicts)
		}
		if d.before != existing {
			t.Errorf("%s: no way back: %q", tc.name, d.before)
		}
	}
	// Near the limit: refused, nothing lost or cut.
	big := strings.Repeat("- a rule\n", proto.RulesMaxBytes/9)
	text, d := big, &rulesDraft{}
	draft(&text, d, "- "+strings.Repeat("one more ", 40))
	if text != big || !strings.Contains(d.err, "nothing was") {
		t.Fatalf("near the limit: err %q, len %d", d.err, len(text))
	}
	// Saved rules the editor doesn't hold still reach the AI, and aren't lost.
	if err := cl.Call(proto.MethodGOARulesSave, proto.GOARules{Rules: existing}, nil); err != nil {
		t.Fatal(err)
	}
	var got proto.GOARules
	if err := cl.Call(proto.MethodGOARulesSave, proto.GOARules{}, &got); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("saved empty over rules: %+v %v", got, err)
	}
}

// Rules that couldn't be read offer nothing to write over them: no draft, no
// saving, and the reason. Those being read neither.
func TestRulesNotReadCantBeOverwritten(t *testing.T) {
	srv, err := daemon.NewServer(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	a1, b1 := net.Pipe()
	go srv.ServeConn(a1)
	cl, err := client.New(b1, func(proto.Event) {})
	if err != nil {
		t.Fatal(err)
	}
	app := New(Options{})
	posted := make(chan func(), 16)
	app.onMain = func(fn func()) { posted <- fn }
	app.client = cl
	project := proto.ProjectInfo{Path: t.TempDir(), Name: "app"}
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(theme.Window())
		ui.Column(c).Width(900).Height(800).Background(theme.Background).TextColor(theme.Text).Children(func() {
			app.goaRulesDialog(c)
			app.rulesDialog(c)
		})
	}, 900, 800)
	cl.Close() // the host can't answer
	app.openGOARules()
	app.openRules(project)
	for range 2 {
		select {
		case fn := <-posted:
			fn()
		case <-time.After(10 * time.Second):
			t.Fatal("no answer")
		}
	}
	for range 3 {
		tt.Frame()
	}
	if !tt.HasText("The rules couldn't be read") || tt.HasText("Write with AI") {
		t.Fatalf("after a failed read: %q", tt.Texts())
	}
	if app.goa.rules.loaded || app.rulesEditing.loaded {
		t.Fatal("taken as read")
	}
	if err := tt.Click("Save"); err == nil {
		tt.Frame()
		if app.goa.rules == nil || !app.goa.rules.open {
			t.Fatal("saved rules it couldn't read")
		}
	}
}

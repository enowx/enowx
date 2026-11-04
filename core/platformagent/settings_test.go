package platformagent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/enowx/enowx/core/platformagent/record"
	"github.com/enowx/enowx/core/platformagent/tools"
	"github.com/enowx/enowx/internal/standinapi"
	"github.com/enowx/enowx/proto"
)

func TestNoKeySaysWhereToAddOne(t *testing.T) {
	srv := standinapi.New()
	defer srv.Close()
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "")
	t.Setenv("ANTHROPIC_BASE_URL", srv.URL)
	front := &fakeFront{}
	a, err := New(Options{Dir: t.TempDir(), SessionDir: t.TempDir(), Front: front})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if err := a.Turn(context.Background(), "hi", nil); !errors.Is(err, errNoKey) {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(front.notices(), "Settings, vault, platform anthropic") {
		t.Errorf("notices: %s", front.notices())
	}
	if front.turnStop(t) != "error" || len(srv.Requests()) != 0 {
		t.Errorf("stop %s, requests %d", front.turnStop(t), len(srv.Requests()))
	}
}

func TestAuthTokenIsUsedWithoutAKey(t *testing.T) {
	srv := standinapi.New(standinapi.Text("hi"))
	defer srv.Close()
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("ANTHROPIC_AUTH_TOKEN", fakeKey)
	t.Setenv("ANTHROPIC_BASE_URL", srv.URL)
	a, err := New(Options{Dir: t.TempDir(), SessionDir: t.TempDir(), Front: &fakeFront{}})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if err := a.Turn(context.Background(), "hi", nil); err != nil {
		t.Fatal(err)
	}
	if got := srv.Requests()[0].Header.Get("Authorization"); got != "Bearer "+fakeKey {
		t.Errorf("Authorization header isn't the token")
	}
}

func TestAttachmentsAndPathsInThePrompt(t *testing.T) {
	r := newRig(t, nil, standinapi.Text("seen"))
	shot := filepath.Join(r.opts.Dir, "shot.png")
	report := filepath.Join(r.opts.Dir, "my report.pdf")
	for _, path := range []string{shot, report} {
		if err := os.WriteFile(path, []byte("bytes"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	prompt := "look at this\n" + shot + "\n" + report + "\nand not /nowhere/missing.png"
	if err := r.agent.Turn(context.Background(), prompt, []string{"shot.png"}); err != nil {
		t.Fatal(err)
	}
	var m struct {
		Content []struct {
			Type   string `json:"type"`
			Source struct {
				MediaType string `json:"media_type"`
				Data      string `json:"data"`
			} `json:"source"`
		} `json:"content"`
	}
	if err := json.Unmarshal(r.srv.Requests()[0].Messages()[0], &m); err != nil {
		t.Fatal(err)
	}
	kinds := map[string]string{}
	for _, b := range m.Content {
		kinds[b.Type] = b.Source.MediaType
	}
	if kinds["image"] != "image/png" || kinds["document"] != "application/pdf" {
		t.Errorf("blocks: %+v", m.Content)
	}
	users := r.front.of(record.User)
	if users[0].Text != prompt || len(users[0].Attachments) != 3 {
		t.Errorf("user record: %q %v", users[0].Text, users[0].Attachments)
	}
}

func TestSettingsChangeOnlyTheNextRequest(t *testing.T) {
	r := newRig(t, func(o *Options) { o.ExtraTools = []tools.Tool{&fakeTool{name: "A"}} })
	r.mustTurn("one")
	before := r.srv.Requests()[0]
	firstAnswer := r.records()

	if err := r.agent.SetModel("claude-sonnet-5-5"); err != nil {
		t.Fatal(err)
	}
	if err := r.agent.SetEffort("low"); err != nil {
		t.Fatal(err)
	}
	if err := r.agent.SetEffort("huge"); err == nil {
		t.Error("an unknown effort was taken")
	}
	if err := r.agent.SetMode(record.ModePlan); err != nil {
		t.Fatal(err)
	}
	r.agent.SetBudget(3)
	r.mustTurn("two")

	after := r.srv.Requests()[1]
	if !strings.Contains(string(after.Raw("model")), "claude-sonnet-5-5") || string(after.Raw("output_config")) != `{"effort":"low"}` {
		t.Errorf("request: %s %s", after.Raw("model"), after.Raw("output_config"))
	}
	if string(before.Raw("system")) != string(after.Raw("system")) || string(before.Raw("tools")) != string(after.Raw("tools")) {
		t.Error("a setting changed the prefix")
	}
	if string(before.Messages()[0]) != string(after.Messages()[0]) {
		t.Error("a setting changed the history")
	}
	metas := 0
	for _, rec := range r.records()[len(firstAnswer):] {
		if rec.Type == record.Meta {
			metas++
		}
	}
	if metas != 4 {
		t.Errorf("meta records: %d", metas)
	}

	// A resumed session has them.
	id := r.agent.Conversation()
	_ = r.agent.Close()
	r.opts.Resume = id
	r.start()
	model, effort, mode, budget := r.agent.Settings()
	if model != "claude-sonnet-5-5" || effort != "low" || mode != record.ModePlan || budget != 3 {
		t.Errorf("resumed: %s %s %s %v", model, effort, mode, budget)
	}
}

func TestClearStartsAnotherFile(t *testing.T) {
	r := newRig(t, nil)
	r.mustTurn("one")
	old := r.agent.Conversation()
	spent := r.agent.Cost()
	if err := r.agent.Clear(); err != nil {
		t.Fatal(err)
	}
	if r.agent.Conversation() == old {
		t.Fatal("same conversation")
	}
	r.mustTurn("two")
	req := r.srv.Requests()[1]
	if len(req.Messages()) != 1 || strings.Contains(string(req.Raw("messages")), "one") {
		t.Errorf("history carried over: %s", req.Raw("messages"))
	}
	if r.agent.Cost() <= spent {
		t.Errorf("cost %v after %v", r.agent.Cost(), spent)
	}
	if r.records()[0].Type != record.Meta {
		t.Errorf("first record of the new file: %+v", r.records()[0])
	}
}

// How a conversation's mode goes on: what was kept stands unless --permission-mode says.
func TestResumedMode(t *testing.T) {
	cases := []struct {
		name  string
		saved string
		o     Options
		want  string
	}{
		{"new: default", "", Options{}, record.ModeDefault},
		{"new: mode", "", Options{Mode: record.ModePlan}, record.ModePlan},
		{"new: if new", "", Options{ModeIfNew: record.ModeAcceptEdits}, record.ModeAcceptEdits},
		{"new: skip", "", Options{SkipPermissions: true, AllowBypass: true}, record.ModeBypass},
		{"plain resume keeps a mode", record.ModeAcceptEdits, Options{}, record.ModeAcceptEdits},
		{"plain resume doesn't restore bypass", record.ModeBypass, Options{}, record.ModeDefault},
		{"if new doesn't restore bypass", record.ModeBypass, Options{ModeIfNew: record.ModePlan}, record.ModeDefault},
		{"the allow flag restores bypass", record.ModeBypass, Options{AllowBypass: true}, record.ModeBypass},
		{"the skip flag restores bypass", record.ModeBypass, Options{SkipPermissions: true, AllowBypass: true}, record.ModeBypass},
		{"the skip flag doesn't override safer", record.ModePlan, Options{SkipPermissions: true, AllowBypass: true}, record.ModePlan},
		{"a stale mode from enowX doesn't change it", record.ModeAcceptEdits, Options{ModeIfNew: record.ModeDefault}, record.ModeAcceptEdits},
		{"an explicit mode does", record.ModeAcceptEdits, Options{Mode: record.ModePlan}, record.ModePlan},
		{"an explicit bypass does", record.ModePlan, Options{Mode: record.ModeBypass}, record.ModeBypass},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newRig(t, func(o *Options) { o.Mode = c.saved })
			id := r.agent.Conversation()
			_ = r.agent.Close()
			change := c.o
			change.Resume, change.Dir, change.SessionDir, change.Front = id, r.opts.Dir, r.opts.SessionDir, r.front
			if c.saved == "" {
				change.Resume = "" // nothing kept
			}
			r.opts = change
			r.start()
			if _, _, got, _ := r.agent.Settings(); got != c.want {
				t.Fatalf("mode %s, want %s", got, c.want)
			}
		})
	}
}

// GOD of Agents' rules stay out of the record: a later, ordinary run of the conversation
// must ask.
func TestGOARulesNeverGoOnRecord(t *testing.T) {
	t.Setenv(proto.EnvGOA, "1")
	r := newRig(t, func(o *Options) {
		o.ExtraTools = []tools.Tool{&fakeTool{name: "mcp__enowx__goa_board"}}
	}, standinapi.Calls(standinapi.ToolUse("c1", "mcp__enowx__goa_board", `{}`)), standinapi.Text("done"))
	r.mustTurn("go")
	if len(r.front.asked) != 0 {
		t.Fatalf("GOA's tool asked: %+v", r.front.asked)
	}
	r.agent.engine.Add("Bash(git status *)")
	if err := r.agent.Clear(); err != nil {
		t.Fatal(err)
	}
	var kept []string
	for _, rec := range r.records() {
		if rec.Type == record.Allow {
			kept = append(kept, rec.Rule)
		}
	}
	if len(kept) != 1 || kept[0] != "Bash(git status *)" {
		t.Fatalf("on record: %v", kept)
	}
}

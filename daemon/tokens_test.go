package daemon

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/tokens"
	"github.com/enowx/enowx/core/transcript"
	"github.com/enowx/enowx/proto"
)

// fakeAgents puts stand-ins for agents on the PATH: each waits for input.
func fakeAgents(t *testing.T, names ...string) {
	t.Helper()
	bin := t.TempDir()
	for _, n := range names {
		os.WriteFile(filepath.Join(bin, n), []byte("#!/bin/sh\necho \"$HTTPS_PROXY\"\nwhile read -r line; do echo \"heard: $line\"; done\n"), 0o755)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func tokensHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, v := range []string{"CODEX_HOME", "CLAUDE_CONFIG_DIR", "XDG_DATA_HOME", "PI_CODING_AGENT_DIR"} {
		t.Setenv(v, "")
	}
	return home
}

func tokenSummary(t *testing.T, c *client.Client, q proto.TokensQuery) proto.TokensSummary {
	t.Helper()
	var s proto.TokensSummary
	if err := c.Call(proto.MethodTokensSummary, q, &s); err != nil {
		t.Fatal(err)
	}
	return s
}

// Turning tracking on reads back what an agent recorded, ties it to the
// session that ran it and its project, and counts it once however often
// it is read; the judge's tokens are counted apart.
func TestTokensRecordedPerSession(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	home := tokensHome(t)
	fakeAgents(t, "claude")
	srv := startServer(t)
	changed := make(chan proto.TokensChanged, 8)
	c := connect(t, srv, func(e proto.Event) {
		var ch proto.TokensChanged
		if e.Name == proto.EventTokensChanged && json.Unmarshal(e.Data, &ch) == nil {
			changed <- ch
		}
	})
	project := t.TempDir()
	if _, err := c.AddProject(project); err != nil {
		t.Fatal(err)
	}
	sess, err := c.CreateSession(proto.SessionCreateParams{Project: project, Agent: "claude", BranchMode: proto.BranchNone, Cols: 80, Rows: 20})
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC().Format(time.RFC3339Nano)
	line := func(id string, in, out int) string {
		return fmt.Sprintf(`{"type":"assistant","timestamp":%q,"cwd":%q,"sessionId":"conv-1","message":{"id":%q,"model":"claude-opus-5-5","usage":{"input_tokens":%d,"cache_read_input_tokens":1000,"output_tokens":%d}}}`+"\n", at, sess.Dir, id, in, out)
	}
	file := filepath.Join(home, ".claude", "projects", transcript.ClaudeFolder(sess.Dir), "conv-1.jsonl")
	os.MkdirAll(filepath.Dir(file), 0o700)
	os.WriteFile(file, []byte(line("msg_1", 100, 50)+line("msg_1", 100, 50)+line("msg_2", 10, 5)), 0o600)

	var st proto.TokensSettings
	if err := c.Call(proto.MethodTokensSetSettings, proto.TokensSettings{On: true}, &st); err != nil || !st.On {
		t.Fatalf("on: %+v %v", st, err)
	}
	select {
	case ch := <-changed:
		if len(ch.Sessions) != 1 || ch.Sessions[0].Session != sess.ID || ch.Sessions[0].Input != 110 {
			t.Fatalf("told: %+v", ch)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("the backfill told nothing")
	}
	s := tokenSummary(t, c, proto.TokensQuery{By: proto.TokensBySession})
	if s.Total.Calls != 2 || s.Total.Input != 110 || s.Total.CacheRead != 2000 || s.Total.Output != 55 || len(s.Groups) != 1 || s.Groups[0].Key != sess.ID {
		t.Fatalf("summary: %+v", s)
	}
	if s.Total.Cost <= 0 || s.Total.Equivalent != s.Total.Cost {
		t.Fatalf("Claude Code's own sign-in is a plan, its cost API-equivalent: %+v", s.Total)
	}
	if p := tokenSummary(t, c, proto.TokensQuery{By: proto.TokensByProject}); len(p.Groups) != 1 || p.Groups[0].Key != project {
		t.Fatalf("by project: %+v", p.Groups)
	}

	// Read again from the start: counted once.
	var res proto.TokensBackfillResult
	if err := c.Call(proto.MethodTokensBackfill, proto.TokensBackfill{}, &res); err != nil || res.Calls != 0 || res.Files == 0 {
		t.Fatalf("backfill again: %+v %v", res, err)
	}

	// The judge's tokens are enowX's own, apart.
	srv.judgeTokens(proto.Decision{ID: "d1", Time: time.Now(), Session: sess.ID, Judge: "local:http://127.0.0.1:1:judge", Tokens: 700})
	s = tokenSummary(t, c, proto.TokensQuery{})
	if s.Total.Calls != 2 || s.Own.Calls != 1 || s.Own.Input != 700 || s.OwnGroups[0].Key != proto.TokensOwnJudge {
		t.Fatalf("own apart: total %+v own %+v", s.Total, s.Own)
	}
	var one proto.TokensSummary
	if err := c.Call(proto.MethodTokensSession, proto.SessionRef{ID: sess.ID}, &one); err != nil || one.Total.Calls != 2 || one.Groups[0].Key != "claude-opus-5-5" {
		t.Fatalf("session: %+v %v", one, err)
	}
	var cov []proto.TokenCoverage
	if err := c.Call(proto.MethodTokensCoverage, nil, &cov); err != nil || len(cov) < 40 {
		t.Fatalf("coverage: %d %v", len(cov), err)
	}
}

// A budget at its limit does what the user picked: new agent sessions
// stopped, or asked first and started once the user says yes.
func TestTokenBudgetAtItsLimit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	tokensHome(t)
	fakeAgents(t, "claude")
	srv := startServer(t)
	alerts := make(chan proto.TokenBudgetState, 8)
	c := connect(t, srv, func(e proto.Event) {
		var b proto.TokenBudgetState
		if e.Name == proto.EventTokensBudget && json.Unmarshal(e.Data, &b) == nil {
			alerts <- b
		}
	})
	budget := proto.TokenBudget{Period: proto.BudgetDay, Scope: proto.BudgetAgent, Target: "claude", Tokens: 1000, AtLimit: proto.BudgetStop}
	if err := c.Call(proto.MethodTokensSetSettings, proto.TokensSettings{On: true, Budgets: []proto.TokenBudget{budget}}, nil); err != nil {
		t.Fatal(err)
	}
	srv.keepCalls(oneCall("claude", 1500))
	srv.afterScan()
	select {
	case b := <-alerts:
		if b.Level != 100 {
			t.Fatalf("alert: %+v", b)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no alert at 100%")
	}
	create := func(ok bool) error {
		_, err := c.CreateSession(proto.SessionCreateParams{Agent: "claude", Dir: t.TempDir(), Cols: 80, Rows: 20, TokenBudgetOK: ok})
		return err
	}
	if err := create(false); err == nil || !strings.Contains(err.Error(), proto.TokenBudgetStops) {
		t.Fatalf("stop: %v", err)
	}
	if _, err := c.CreateSession(proto.SessionCreateParams{Agent: "codex", Dir: t.TempDir(), Cols: 80, Rows: 20}); err != nil && strings.Contains(err.Error(), "token budget") {
		t.Fatalf("another agent held by claude's budget: %v", err)
	}
	budget.AtLimit = proto.BudgetAsk
	if err := c.Call(proto.MethodTokensSetSettings, proto.TokensSettings{On: true, Budgets: []proto.TokenBudget{budget}}, nil); err != nil {
		t.Fatal(err)
	}
	if err := create(false); err == nil || !strings.Contains(err.Error(), proto.TokenBudgetAsks) {
		t.Fatalf("ask: %v", err)
	}
	if err := create(true); err != nil {
		t.Fatalf("the user said yes: %v", err)
	}
	budget.AtLimit = proto.BudgetNotify
	c.Call(proto.MethodTokensSetSettings, proto.TokensSettings{On: true, Budgets: []proto.TokenBudget{budget}}, nil)
	if err := create(false); err != nil {
		t.Fatalf("notify only: %v", err)
	}
}

// oneCall is a call of agentID's with n input tokens, made now.
func oneCall(agentID string, n int64) []tokens.Call {
	return []tokens.Call{{ID: "test:" + agentID + fmt.Sprint(time.Now().UnixNano()), Time: time.Now(), Agent: agentID, Model: "claude-opus-5-5", Input: n, Layer: proto.TokenLayerRecords}}
}

// An agent that keeps no records is metered through the interceptor:
// its model API's streamed answers (Anthropic's, OpenAI's) counted.
func TestTokensMeteredFromTraffic(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	tokensHome(t)
	fakeAgents(t, "crush")
	up := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.ReadAll(r.Body)
		switch r.URL.Path {
		case "/v1/messages":
			io.WriteString(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_t1\",\"model\":\"claude-sonnet-5-5\",\"usage\":{\"input_tokens\":30,\"cache_read_input_tokens\":4000,\"output_tokens\":1}}}\n\n"+
				"event: message_delta\ndata: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":77}}\n\n")
		case "/v1/chat/completions":
			io.WriteString(w, "data: {\"id\":\"chatcmpl-t2\",\"model\":\"gpt-4.1\",\"choices\":[{\"delta\":{\"content\":\"hi\"}}],\"usage\":null}\n\n"+
				"data: {\"id\":\"chatcmpl-t2\",\"model\":\"gpt-4.1\",\"choices\":[],\"usage\":{\"prompt_tokens\":500,\"completion_tokens\":20,\"prompt_tokens_details\":{\"cached_tokens\":300}}}\n\ndata: [DONE]\n\n")
		}
	}))
	defer up.Close()
	srv := startServer(t)
	c := connect(t, srv, nil)
	if err := c.Call(proto.MethodTokensSetSettings, proto.TokensSettings{On: true, Traffic: true}, nil); err != nil {
		t.Fatal(err)
	}
	sess, err := c.CreateSession(proto.SessionCreateParams{Agent: "crush", Dir: t.TempDir(), Cols: 80, Rows: 20})
	if err != nil {
		t.Fatal(err)
	}
	live, _ := srv.sessions.Get(sess.ID)
	waitFor(t, "the agent's proxy", func() bool { return strings.Contains(live.Terminal().Text(), "http://127.0.0.1:") })
	env, err := c.TrafficEnv(proto.TrafficEnvParams{Session: sess.ID})
	if err != nil {
		t.Fatal(err)
	}
	hc := clientFromEnv(t, env.Env)
	for _, path := range []string{"/v1/messages", "/v1/chat/completions"} {
		resp, err := hc.Post(up.URL+path, "application/json", strings.NewReader(`{"stream":true}`))
		if err != nil {
			t.Fatal(err)
		}
		io.ReadAll(resp.Body)
		resp.Body.Close()
	}
	var s proto.TokensSummary
	waitFor(t, "two calls metered", func() bool {
		s = tokenSummary(t, c, proto.TokensQuery{Session: sess.ID})
		return s.Total.Calls == 2
	})
	if s.Total.Input != 30+200 || s.Total.CacheRead != 4300 || s.Total.Output != 97 {
		t.Fatalf("metered: %+v", s.Total)
	}
}

// A call is recorded with its session's config: its name, and a model
// API's provider, billed rather than a plan's.
func TestTokensRecordTheSessionsConfig(t *testing.T) {
	srv := startServer(t)
	at := time.Now()
	sessions := []proto.SessionInfo{{ID: "s1", Agent: "claude", Dir: "/work/x", Created: at.Add(-time.Hour),
		Config: &proto.SessionConfig{ID: "c1", Name: "Work OpenRouter", Provider: "openrouter", Model: "anthropic/claude-sonnet-4.5"}}}
	c := tokens.Call{ID: "x", Time: at, Agent: "claude", Dir: "/work/x", Input: 10}
	srv.attribute(&c, sessions, nil)
	if c.Session != "s1" || c.Config != "Work OpenRouter" || c.Provider != "openrouter" || c.Plan {
		t.Fatalf("config not recorded: %+v", c)
	}
}

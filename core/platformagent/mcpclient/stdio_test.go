package mcpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/enowx/enowx/core/platformagent/tools"
)

const testSecret = "hush-this-value-1234"

func fakeSpec(t *testing.T, servers map[string]string, log string) string {
	t.Helper()
	entries := map[string]any{}
	for name, mode := range servers {
		entries[name] = map[string]any{"command": os.Args[0], "env": map[string]string{"FAKE_MCP": mode, "FAKE_LOG": log, "FAKE_SECRET": testSecret}}
	}
	b, err := json.Marshal(map[string]any{"mcpServers": entries})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func connectFake(t *testing.T, log string) *Servers {
	t.Helper()
	s, err := Connect(context.Background(), fakeSpec(t, map[string]string{"fake": "serve"}, log), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if len(s.Problems()) != 0 {
		t.Fatalf("problems: %v", s.Problems())
	}
	return s
}

func find(t *testing.T, s *Servers, name string) tools.Tool {
	t.Helper()
	for _, tool := range s.Tools() {
		if tool.Name() == name {
			return tool
		}
	}
	t.Fatalf("no tool %s", name)
	return nil
}

func run(tool tools.Tool, input string) tools.Result {
	return tool.Run(context.Background(), json.RawMessage(input), tools.Env{})
}

func logHas(t *testing.T, path, want string) bool {
	t.Helper()
	b, _ := os.ReadFile(path)
	return strings.Contains(string(b), want)
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("waited for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestListsToolsAcrossPages(t *testing.T) {
	s := connectFake(t, "")
	var names []string
	for _, tool := range s.Tools() {
		names = append(names, tool.Name())
	}
	want := "mcp__fake__block mcp__fake__crash mcp__fake__echo mcp__fake__fail mcp__fake__image mcp__fake__open mcp__fake__serverask"
	if got := strings.Join(names, " "); got != want {
		t.Fatalf("tools: %s", got)
	}
	echo := find(t, s, "mcp__fake__echo")
	if !echo.ReadOnly() || echo.Edits() || echo.Description() != "Echoes." {
		t.Errorf("echo: readOnly %v edits %v description %q", echo.ReadOnly(), echo.Edits(), echo.Description())
	}
	if find(t, s, "mcp__fake__fail").ReadOnly() {
		t.Error("a tool without the hint is read-only")
	}
}

func TestCallResults(t *testing.T) {
	s := connectFake(t, "")
	if r := run(find(t, s, "mcp__fake__echo"), `{"x":1}`); r.Failed || r.Text != `{"x":1}` {
		t.Errorf("echo: %+v", r)
	}
	if r := run(find(t, s, "mcp__fake__echo"), ``); r.Failed || r.Text != `{}` {
		t.Errorf("no input: %+v", r)
	}
	if r := run(find(t, s, "mcp__fake__fail"), `{}`); !r.Failed || r.Text != "it went wrong" {
		t.Errorf("error result: %+v", r)
	}
	r := run(find(t, s, "mcp__fake__image"), `{}`)
	if r.Failed || len(r.Images) != 1 || r.Images[0] != (tools.Image{MediaType: "image/png", Base64: "aGk="}) {
		t.Errorf("image: %+v", r)
	}
	// What the model's API would refuse, for good, stays out of Images.
	if !strings.HasPrefix(r.Text, "a picture\n") || !strings.Contains(r.Text, "image/svg+xml") {
		t.Errorf("image text: %q", r.Text)
	}
	if r := run(find(t, s, "mcp__fake__echo"), `{not json`); !r.Failed {
		t.Errorf("bad input: %+v", r)
	}
}

// The first call's answer only comes after the second's: both on one connection.
func TestCallsSideBySideAnswerOutOfOrder(t *testing.T) {
	s := connectFake(t, "")
	var blocked tools.Result
	var wg sync.WaitGroup
	wg.Go(func() { blocked = run(find(t, s, "mcp__fake__block"), `{}`) })
	time.Sleep(50 * time.Millisecond)
	if r := run(find(t, s, "mcp__fake__open"), `{}`); r.Text != "opened" {
		t.Errorf("open: %+v", r)
	}
	wg.Wait()
	if blocked.Text != "released" {
		t.Errorf("block: %+v", blocked)
	}
}

func TestAnswersWhatTheServerAsks(t *testing.T) {
	log := filepath.Join(t.TempDir(), "log")
	s := connectFake(t, log)
	if r := run(find(t, s, "mcp__fake__serverask"), `{}`); r.Text != "asked" {
		t.Fatalf("call: %+v", r)
	}
	waitFor(t, "the ping's answer", func() bool { return logHas(t, log, "client answered 99") })
}

func TestCancelTellsTheServerAndReturns(t *testing.T) {
	log := filepath.Join(t.TempDir(), "log")
	s := connectFake(t, log)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan tools.Result, 1)
	go func() { done <- find(t, s, "mcp__fake__block").Run(ctx, json.RawMessage(`{}`), tools.Env{}) }()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case r := <-done:
		if !r.Failed || !strings.Contains(r.Text, "Interrupted") {
			t.Errorf("result: %+v", r)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the call didn't return on cancel")
	}
	waitFor(t, "notifications/cancelled", func() bool { return logHas(t, log, "cancelled ") })
	// The connection is still good.
	if r := run(find(t, s, "mcp__fake__echo"), `{}`); r.Failed {
		t.Errorf("after cancel: %+v", r)
	}
}

func TestCrashFailsTheCallThenRestarts(t *testing.T) {
	log := filepath.Join(t.TempDir(), "log")
	s := connectFake(t, log)
	r := run(find(t, s, "mcp__fake__crash"), `{}`)
	if !r.Failed || !strings.Contains(r.Text, "stopped during the call") || !strings.Contains(r.Text, "starts it again") {
		t.Fatalf("crash: %+v", r)
	}
	if r := run(find(t, s, "mcp__fake__echo"), `{"again":true}`); r.Failed || r.Text != `{"again":true}` {
		t.Fatalf("after restart: %+v", r)
	}
	b, _ := os.ReadFile(log)
	if n := strings.Count(string(b), "start "); n != 2 {
		t.Errorf("starts: %d", n)
	}
	// A second crash this soon isn't one more restart: no loop.
	run(find(t, s, "mcp__fake__crash"), `{}`)
	if r := run(find(t, s, "mcp__fake__echo"), `{}`); !r.Failed || !strings.Contains(r.Text, "won't be started again") {
		t.Fatalf("second crash: %+v", r)
	}
	b, _ = os.ReadFile(log)
	if n := strings.Count(string(b), "start "); n != 2 {
		t.Errorf("starts after the second crash: %d", n)
	}
}

func TestOneBadServerOfTwo(t *testing.T) {
	s, err := Connect(context.Background(), fakeSpec(t, map[string]string{"good": "serve", "bad": "die"}, ""), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if r := run(find(t, s, "mcp__good__echo"), `{}`); r.Failed {
		t.Errorf("good: %+v", r)
	}
	problems := strings.Join(s.Problems(), "\n")
	if len(s.Problems()) != 1 || !strings.Contains(problems, "MCP server bad:") || !strings.Contains(problems, "exit status 3") {
		t.Errorf("problems: %s", problems)
	}
	if strings.Contains(problems, testSecret) {
		t.Errorf("a secret in problems: %s", problems)
	}
	// Its last line of stderr says why, its env's value masked.
	if !strings.Contains(problems, "fatal: [hidden]") {
		t.Errorf("no reason in problems: %s", problems)
	}
}

// A program can print what it inherited as well as what its entry set.
func TestStderrHidesInheritedValues(t *testing.T) {
	spec := `{"mcpServers":{"bad":{"command":"` + os.Args[0] + `","env":{"FAKE_MCP":"die"}}}}`
	s, err := Connect(context.Background(), spec, []string{"FAKE_SECRET=" + testSecret})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	problems := strings.Join(s.Problems(), "\n")
	if !strings.Contains(problems, "fatal: [hidden]") || strings.Contains(problems, testSecret) {
		t.Errorf("problems: %s", problems)
	}
}

func TestSlowServerDoesntHoldUpTheOthers(t *testing.T) {
	start := time.Now()
	s, err := connectWithin(context.Background(), fakeSpec(t, map[string]string{"good": "serve", "slow": "hang"}, ""), nil, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if time.Since(start) > 10*time.Second {
		t.Errorf("took %s", time.Since(start))
	}
	if len(s.Tools()) == 0 {
		t.Error("the good server's tools are missing")
	}
	problems := strings.Join(s.Problems(), "\n")
	if !strings.Contains(problems, "MCP server slow: no answer within 2s") {
		t.Errorf("problems: %s", problems)
	}
}

func TestMissingProgramIsAProblem(t *testing.T) {
	spec := `{"mcpServers":{"gone":{"command":"/no/such/program","env":{"KEY":"` + testSecret + `"}}}}`
	s, err := Connect(context.Background(), spec, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	problems := strings.Join(s.Problems(), "\n")
	if !strings.Contains(problems, "MCP server gone: can't run /no/such/program") || strings.Contains(problems, testSecret) {
		t.Errorf("problems: %s", problems)
	}
}

func TestSchemaBytesAreStable(t *testing.T) {
	var first []byte
	for range 2 {
		s := connectFake(t, "")
		schema := find(t, s, "mcp__fake__echo").Schema()
		if first != nil && !bytes.Equal(first, schema) {
			t.Fatalf("schemas differ:\n%s\n%s", first, schema)
		}
		first = schema
		want := `{"properties":{"a":{"maximum":12345678901234567890,"type":"integer"},"z":{"type":"string"}},"type":"object"}`
		if string(schema) != want {
			t.Fatalf("schema: %s", schema)
		}
		if got := string(find(t, s, "mcp__fake__fail").Schema()); got != `{"properties":{},"type":"object"}` {
			t.Fatalf("no schema: %s", got)
		}
	}
}

func TestEnvAndVariablesReachTheServer(t *testing.T) {
	// The fake runs in FAKE_MCP's mode only when ${MODE} expands in its env, and a
	// variable nothing sets is reported, by name.
	spec := `{"mcpServers":{"fake":{"command":"` + os.Args[0] + `","env":{"FAKE_MCP":"${MODE}","X":"${NOT_SET}","Y":"${ALSO_UNSET:-fine}"}}}}`
	s, err := Connect(context.Background(), spec, []string{"MODE=serve"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if len(s.Tools()) == 0 {
		t.Errorf("no tools: %v", s.Problems())
	}
	problems := strings.Join(s.Problems(), "\n")
	if !strings.Contains(problems, "${NOT_SET} isn't set") || strings.Contains(problems, "ALSO_UNSET") {
		t.Errorf("problems: %s", problems)
	}
}

func TestSpecFromAFileAndBadSpecs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcp.json")
	if err := os.WriteFile(path, []byte(fakeSpec(t, map[string]string{"fake": "serve"}, "")), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Connect(context.Background(), path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Tools()) == 0 {
		t.Errorf("no tools: %v", s.Problems())
	}
	_ = s.Close()
	_ = s.Close()
	if s, err := Connect(context.Background(), "", nil); err != nil || len(s.Tools()) != 0 {
		t.Errorf("none: %v", err)
	}
	if _, err := Connect(context.Background(), "{nope", nil); err == nil {
		t.Error("bad JSON was taken")
	}
	s, err = Connect(context.Background(), `{"mcpServers":{"odd":{"type":"ws","url":"ws://x"},"empty":{},"num":{"command":5}}}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Problems()) != 3 {
		t.Errorf("problems: %v", s.Problems())
	}
}

func TestCallAfterCloseFailsCleanly(t *testing.T) {
	s := connectFake(t, "")
	echo := find(t, s, "mcp__fake__echo")
	_ = s.Close()
	if r := run(echo, `{}`); !r.Failed || !strings.Contains(r.Text, "closed") {
		t.Errorf("after close: %+v", r)
	}
}

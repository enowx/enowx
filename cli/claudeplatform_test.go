package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/platformagent"
	"github.com/enowx/enowx/core/platformagent/hookbridge"
	"github.com/enowx/enowx/core/platformagent/record"
	"github.com/enowx/enowx/proto"
)

func TestClaudePlatformFlags(t *testing.T) {
	dir := t.TempDir()
	appendFile := filepath.Join(dir, "append.md")
	if err := os.WriteFile(appendFile, []byte("from the file"), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := parseClaudePlatformFlags([]string{
		"--model", "m", "--effort=max", "--permission-mode", "plan", "--allow-dangerously-skip-permissions",
		"--resume", "abc", "--session-dir", "/s", "--append-system-prompt-file", appendFile,
		"--append-system-prompt", "text", "--mcp-config", "a.json", "--mcp-config", "b.json", "--max-budget", "2.5",
		"fix", "--", "--this",
	})
	if err != nil {
		t.Fatal(err)
	}
	if f.model != "m" || f.effort != "max" || f.mode != "plan" || !f.allowBypass || f.resume != "abc" || f.sessionDir != "/s" ||
		f.budget != 2.5 || !f.budgetSet || f.prompt != "fix --this" {
		t.Fatalf("%+v", f)
	}
	if !reflect.DeepEqual(f.appendPrompts, []string{"from the file", "text"}) || !reflect.DeepEqual(f.mcpConfigs, []string{"a.json", "b.json"}) {
		t.Fatalf("%+v", f)
	}

	f, err = parseClaudePlatformFlags([]string{"--dangerously-skip-permissions", "--continue"})
	if err != nil || f.mode != record.ModeBypass || !f.continueLast {
		t.Fatalf("%+v %v", f, err)
	}

	for args, want := range map[string][]string{
		"unknown flag --nonesuch":     {"--nonesuch"},
		"--model needs a value":       {"--model"},
		"unknown --permission-mode":   {"--permission-mode", "wild"},
		"isn't a number":              {"--max-budget", "lots"},
		"different conversations":     {"--continue", "--resume", "x"},
		"--append-system-prompt-file": {"--append-system-prompt-file", filepath.Join(dir, "gone")},
	} {
		if _, err := parseClaudePlatformFlags(want); err == nil || !strings.Contains(err.Error(), args) {
			t.Errorf("%v: %v", want, err)
		}
	}
}

func TestClaudePlatformOptions(t *testing.T) {
	home := t.TempDir()
	f, err := parseClaudePlatformFlags([]string{"--model", "m", "--append-system-prompt", "a", "--append-system-prompt", "b"})
	if err != nil {
		t.Fatal(err)
	}
	o, err := f.options("/work/my project", home)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, ".enowx-app", "claude-platform", "sessions", "-work-my-project"); o.SessionDir != want {
		t.Fatalf("session dir %q", o.SessionDir)
	}
	if o.SystemPrompt != "" || o.Append != "a\n\nb" || o.Model != "m" || o.Budget != record.DefaultBudget || o.Dir != "/work/my project" {
		t.Fatalf("%+v", o)
	}

	// The app's editor keeps the prompt here.
	path := filepath.Join(home, ".enowx-app", "claude-platform", "system-prompt.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("edited"), 0o600); err != nil {
		t.Fatal(err)
	}
	if o, _ := f.options("/w", home); o.SystemPrompt != "edited" {
		t.Fatalf("%q", o.SystemPrompt)
	}

	// A file named replaces it; one missing says so.
	named := filepath.Join(home, "named.md")
	if err := os.WriteFile(named, []byte("named"), 0o600); err != nil {
		t.Fatal(err)
	}
	f.systemPromptFile = named
	if o, _ := f.options("/w", home); o.SystemPrompt != "named" {
		t.Fatalf("%q", o.SystemPrompt)
	}
	f.systemPromptFile = named + ".gone"
	if _, err := f.options("/w", home); err == nil || !strings.Contains(err.Error(), "--system-prompt-file") {
		t.Fatalf("%v", err)
	}

	// A conversation resumed keeps its budget unless told otherwise.
	r := claudePlatformFlags{continueLast: true}
	if o, _ := r.options("/w", home); o.Budget != 0 {
		t.Fatalf("budget %v", o.Budget)
	}
	n := claudePlatformFlags{budgetSet: true}
	if o, _ := n.options("/w", home); o.Budget != 0 {
		t.Fatalf("budget %v", o.Budget)
	}
}

func TestClaudePlatformMCPConfigsMerge(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "mcp.json")
	if err := os.WriteFile(file, []byte(`{"mcpServers":{"a":{"command":"from-file"},"b":{"command":"b"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	merged, err := mergeMCPConfigs([]string{file, `{"mcpServers":{"a":{"command":"from-text"},"c":{"command":"c"}}}`})
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Servers map[string]struct {
			Command string `json:"command"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal([]byte(merged), &got); err != nil {
		t.Fatal(err)
	}
	if got.Servers["a"].Command != "from-text" || got.Servers["b"].Command != "b" || got.Servers["c"].Command != "c" {
		t.Fatalf("%s", merged)
	}

	if one, err := mergeMCPConfigs([]string{file}); err != nil || one != file {
		t.Fatalf("%q %v", one, err)
	}
	if none, err := mergeMCPConfigs(nil); err != nil || none != "" {
		t.Fatalf("%q %v", none, err)
	}
	for _, bad := range [][]string{{file, filepath.Join(dir, "gone.json")}, {file, "{not json"}} {
		if _, err := mergeMCPConfigs(bad); err == nil || !strings.Contains(err.Error(), "--mcp-config") {
			t.Fatalf("%v: %v", bad, err)
		}
	}
}

func TestClaudePlatformIsACommand(t *testing.T) {
	if !Is([]string{"claude-platform"}) {
		t.Fatal("not a command")
	}
	var out bytes.Buffer
	if err := Run([]string{"claude-platform", "--version"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out.String(), record.AgentID) {
		t.Fatalf("%q", out.String())
	}
	if err := Run([]string{"claude-platform", "--nope"}, &out); err == nil || !strings.Contains(err.Error(), "unknown flag --nope") {
		t.Fatalf("%v", err)
	}
	// Nothing about the agent in the list of commands for people.
	out.Reset()
	if err := Run([]string{"help"}, &out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "claude-platform") {
		t.Fatal("in the help")
	}
}

// fakeClaudePlatform is a Runner recording its first message.
type fakeClaudePlatform struct {
	turns  []string
	closed bool
}

func (f *fakeClaudePlatform) Turn(_ context.Context, text string, _ []string) error {
	f.turns = append(f.turns, text)
	return nil
}
func (f *fakeClaudePlatform) Settings() (string, string, string, float64) {
	return "claude-test", "high", record.ModeDefault, 1
}
func (f *fakeClaudePlatform) SetModel(string) error         { return nil }
func (f *fakeClaudePlatform) SetEffort(string) error        { return nil }
func (f *fakeClaudePlatform) SetMode(string) error          { return nil }
func (f *fakeClaudePlatform) SetBudget(float64)             {}
func (f *fakeClaudePlatform) Compact(context.Context) error { return nil }
func (f *fakeClaudePlatform) Clear() error                  { return nil }
func (f *fakeClaudePlatform) Cost() float64                 { return 0 }
func (f *fakeClaudePlatform) Context() (int64, int64)       { return 0, 0 }
func (f *fakeClaudePlatform) Conversation() string          { return "c" }
func (f *fakeClaudePlatform) Close() error                  { f.closed = true; return nil }

func TestClaudePlatformRunsLineByLineFromAPipe(t *testing.T) {
	t.Setenv(EnvSession, "") // no daemon to tell
	t.Setenv("HOME", t.TempDir())
	f, err := parseClaudePlatformFlags([]string{"--model", "m", "first prompt"})
	if err != nil {
		t.Fatal(err)
	}
	fake := &fakeClaudePlatform{}
	var opts platformagent.Options
	var out bytes.Buffer
	err = runClaudePlatform(f, strings.NewReader("second\n"), &out, func(o platformagent.Options) (platformagent.Runner, error) {
		opts = o
		return fake, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fake.turns, []string{"first prompt", "second"}) || !fake.closed {
		t.Fatalf("%+v; output:\n%s", fake, out.String())
	}
	if opts.Front == nil || opts.Hooks == nil || opts.Model != "m" {
		t.Fatalf("%+v", opts)
	}
	if strings.Contains(out.String(), "\x1b") {
		t.Fatalf("escape sequences in a pipe: %q", out.String())
	}
}

func TestClaudePlatformSaysWhyItCouldntStart(t *testing.T) {
	t.Setenv(EnvSession, "")
	t.Setenv("HOME", t.TempDir())
	var out bytes.Buffer
	err := runClaudePlatform(claudePlatformFlags{}, strings.NewReader(""), &out, func(platformagent.Options) (platformagent.Runner, error) {
		return nil, errors.New("no API key in the environment")
	})
	if err == nil || !strings.Contains(err.Error(), "no API key") {
		t.Fatalf("%v", err)
	}
}

// The bridge against a real daemon: a session's hook refuses a tool, and states reach it.
func TestClaudePlatformBridgeReachesTheDaemon(t *testing.T) {
	srv := newServer(t)
	c := dialServer(t, srv, nil)
	info, err := c.CreateSession(proto.SessionCreateParams{Command: []string{"sh", "-c", "sleep 30"}})
	if err != nil {
		t.Fatal(err)
	}
	block := proto.Hook{On: true, Event: proto.HookToolBefore, Tool: "Bash", Action: proto.HookActionBlock, Reason: "not here"}
	if err := c.SaveSessionHooks(info.ID, []proto.Hook{block}); err != nil {
		t.Fatal(err)
	}
	b := hookbridge.NewOver(info.ID, false, "", func() (hookbridge.Conn, error) {
		a, z := net.Pipe()
		go srv.ServeConn(a)
		return client.New(z, nil)
	})
	ctx := context.Background()
	if got := b.ToolBefore(ctx, "Bash", []byte(`{"command":"make deploy"}`)); !got.Deny || got.Why != "not here" {
		t.Fatalf("answer %+v", got)
	}
	if got := b.ToolBefore(ctx, "Read", []byte(`{"file_path":"/a"}`)); got.Deny {
		t.Fatalf("answer %+v", got)
	}
	b.Prompt(ctx, "hello")
	state := func() string {
		list, err := c.Sessions()
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range list {
			if s.ID == info.ID {
				return s.State
			}
		}
		return ""
	}
	if got := state(); got != proto.StateWorking {
		t.Fatalf("state after a prompt: %q", got)
	}
	b.TurnDone(ctx)
	if got := state(); got != proto.StateDone {
		t.Fatalf("state after a turn: %q", got)
	}
}

// keyConn is a daemon holding a key for the agent.
type keyConn struct{ env map[string]string }

func (k keyConn) Close() error { return nil }
func (k keyConn) Call(method string, params, out any) error {
	if method != proto.MethodVaultAgentKey {
		return errors.New("unexpected " + method)
	}
	out.(*proto.VaultAgentKey).Env = k.env
	return nil
}

// The key the daemon holds reaches the agent's environment, under a config's API key if any.
func TestClaudePlatformTakesItsKeyFromTheDaemon(t *testing.T) {
	bridge := hookbridge.NewOver("s1", false, "", func() (hookbridge.Conn, error) {
		return keyConn{map[string]string{"ANTHROPIC_API_KEY": "from-the-daemon"}}, nil
	})
	t.Setenv("ANTHROPIC_API_KEY", "")
	takeKey(context.Background(), bridge)
	if got := os.Getenv("ANTHROPIC_API_KEY"); got != "from-the-daemon" {
		t.Fatalf("key: %q", got)
	}
	t.Setenv("ANTHROPIC_API_KEY", "a-configs")
	takeKey(context.Background(), bridge)
	if got := os.Getenv("ANTHROPIC_API_KEY"); got != "a-configs" {
		t.Fatalf("a config's key was replaced: %q", got)
	}
}

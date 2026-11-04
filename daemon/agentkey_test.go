package daemon

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/vault"
	"github.com/enowx/enowx/proto"
)

// A key of the shape Anthropic's have; made here, it opens nothing.
const platformTestKey = "sk-ant-api03-not-a-real-key-0123456789abcdefghijklmn"

// fakePlatform stands in for enowX's program as the agent: it notes in a file each start's
// arguments, what its environment says of the key and what its wrapper shell's does, as a
// command could read it, and waits.
func fakePlatform(t *testing.T) (out string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	dir := t.TempDir()
	out = filepath.Join(dir, "starts")
	script := "#!/bin/sh\nprintf 'args:%s\\nkey:%s\\n' \"$*\" \"$ANTHROPIC_API_KEY\" >> '" + out + "'\ncat /proc/$PPID/environ >> '" + out + "' 2>/dev/null\nwhile :; do sleep 1; done\n"
	bin := filepath.Join(dir, "enowx-fake")
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(agent.EnvBin, bin)
	t.Setenv("HOME", t.TempDir())
	return out
}

func startsOf(path string) int { return strings.Count(readT(path), "args:") }

// takesKey is the agent's asking the daemon as it starts: the key, and only the once.
func takesKey(t *testing.T, c interface {
	Call(method string, params, out any) error
}, id string) {
	t.Helper()
	take := func() string {
		var got proto.VaultAgentKey
		if err := c.Call(proto.MethodVaultAgentKey, proto.VaultAgentKeyParams{Session: id}, &got); err != nil {
			t.Fatal(err)
		}
		return got.Env["ANTHROPIC_API_KEY"]
	}
	if got := take(); got != platformTestKey {
		t.Fatalf("the agent's key: %q", got)
	}
	if got := take(); got != "" {
		t.Fatal("the key is handed over a second time")
	}
}

func keyLender(names *[]string) func(proto.VaultRequest) (proto.VaultLend, error) {
	return func(r proto.VaultRequest) (proto.VaultLend, error) {
		if r.Action == proto.VaultSession && slices.Contains(r.Names, "anthropic") {
			if names != nil {
				*names = r.Names
			}
			return proto.VaultLend{Credentials: []string{"anthropic"}, Env: map[string]string{"ANTHROPIC_API_KEY": platformTestKey}}, nil
		}
		return proto.VaultLend{}, errors.New("no")
	}
}

// filesHolding are the files under dir with the key in them.
func filesHolding(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && d.Type().IsRegular() {
			if data, err := os.ReadFile(path); err == nil && strings.Contains(string(data), platformTestKey) {
				out = append(out, path)
			}
		}
		return nil
	})
	return out
}

// The agent runs on the vault's credential: lent with the app's say and taken from the
// daemon by the agent alone, in no session's environment; a wake and a daemon's restart
// have it lent again.
func TestPlatformKeyReachesTheAgentAlone(t *testing.T) {
	starts := fakePlatform(t)
	dir, work := t.TempDir(), t.TempDir()
	first, err := NewServer(dir)
	if err != nil {
		t.Fatal(err)
	}
	c := connect(t, first, nil)
	app := serveVault(t, first, keyLender(nil))
	putVault(t, app.Client, vault.Credential{Name: "anthropic", Platform: "anthropic", Fields: map[string]string{"api_key": platformTestKey}})
	info, err := c.CreateSession(proto.SessionCreateParams{Agent: agent.PlatformID, Dir: work, Prompt: "hello there", Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "its program started", func() bool { return strings.Contains(readT(starts), "key:") })
	takesKey(t, c, info.ID)
	said := readT(starts)
	if strings.Contains(said, platformTestKey) {
		t.Fatal("the key is in its environment or its wrapper's")
	}
	for _, want := range []string{"claude-platform", "--session-dir", "--append-system-prompt-file", "--mcp-config", "mcp.json", "hello there"} {
		if !strings.Contains(said, want) {
			t.Errorf("its command lacks %s: %s", want, said)
		}
	}
	if !slices.Equal(info.Creds, []string{"anthropic"}) {
		t.Errorf("creds = %v", info.Creds)
	}
	app.mu.Lock()
	asked := app.seen[len(app.seen)-1]
	app.mu.Unlock()
	if asked.Agent != agent.PlatformID {
		t.Errorf("the app isn't told who asks: %+v", asked)
	}

	// Hibernated and woken, with no app there: the lease still holds it.
	if err := c.Call(proto.MethodSessionState, proto.SessionStateParams{ID: info.ID, State: proto.StateDone}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := c.HibernateSessions(proto.SessionHibernateParams{IDs: []string{info.ID}, WhenDone: true}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "hibernated", func() bool {
		first.hibernate()
		list, _ := c.Sessions()
		return len(list) == 1 && list[0].Hibernated
	})
	app.Close()
	waitFor(t, "the app gone", func() bool { return !first.vault.served() })
	if woke, err := c.WakeSession(info.ID); err != nil || woke.Exited {
		t.Fatalf("woken: %+v, %v", woke, err)
	}
	waitFor(t, "started again", func() bool { return startsOf(starts) == 2 })
	takesKey(t, c, info.ID)

	first.Close()
	if held := filesHolding(t, dir); len(held) > 0 {
		t.Fatalf("the key is in files: %v", held)
	}

	// A new daemon has no app yet: it waits, stopped, rather than start with no key.
	second, err := NewServer(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(second.Close)
	c2 := connect(t, second, nil)
	list, err := c2.Sessions()
	if err != nil || len(list) != 1 || !list[0].Exited {
		t.Fatalf("after the restart: %+v, %v", list, err)
	}
	if n := strings.Count(readT(starts), "args:"); n != 2 {
		t.Fatalf("it started with no one to lend its key: %d", n)
	}
	if _, err := c2.WakeSession(list[0].ID); err == nil {
		t.Fatal("woken with no app to lend its key")
	}
	serveVault(t, second, keyLender(nil))
	waitFor(t, "continued once an app serves", func() bool { return startsOf(starts) == 3 })
	takesKey(t, c2, list[0].ID)
	if strings.Contains(readT(starts), platformTestKey) {
		t.Fatal("the key is in its environment or its wrapper's")
	}
	if held := filesHolding(t, dir); len(held) > 0 {
		t.Fatalf("the key is in files: %v", held)
	}
}

// A tab holds what the session was lent, but for the key its agent alone runs on; a
// visitor can't take it, nor a tab by its own ID.
func TestPlatformKeyStaysOutOfTabs(t *testing.T) {
	fakePlatform(t)
	srv := startServer(t)
	c := connect(t, srv, nil)
	app := serveVault(t, srv, func(r proto.VaultRequest) (proto.VaultLend, error) {
		return proto.VaultLend{Credentials: []string{"anthropic", "other"}, Env: map[string]string{
			"ANTHROPIC_API_KEY": platformTestKey, "OTHER_TOKEN": "other-value",
		}}, nil
	})
	putVault(t, app.Client, vault.Credential{Name: "anthropic", Platform: "anthropic", Fields: map[string]string{"api_key": platformTestKey}},
		vault.Credential{Name: "other", Platform: "custom", Auto: true, Fields: map[string]string{"value": "other-value"}})
	info, err := c.CreateSession(proto.SessionCreateParams{Agent: agent.PlatformID, Dir: t.TempDir(), Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "tab")
	tab, err := c.CreateSession(proto.SessionCreateParams{Parent: info.ID, Name: "Tab",
		Command: []string{"sh", "-c", `printf 'key:%s other:%s\n' "$ANTHROPIC_API_KEY" "$OTHER_TOKEN" > '` + out + `'; sleep 30`}, Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the tab ran", func() bool { return strings.Contains(readT(out), "other:") })
	if got := strings.TrimSpace(readT(out)); got != "key: other:other-value" {
		t.Errorf("the tab's environment: %q", got)
	}
	if slices.Contains(tab.Creds, "anthropic") {
		t.Errorf("the tab keeps the key's name, which a restart would lend again: %v", tab.Creds)
	}
	var got proto.VaultAgentKey
	if err := c.Call(proto.MethodVaultAgentKey, proto.VaultAgentKeyParams{Session: tab.ID}, &got); err != nil || len(got.Env) > 0 {
		t.Errorf("a tab took the key: %v, %v", got.Env, err)
	}
	takesKey(t, c, info.ID)

	// A tab that is itself this agent runs on it too, taking its own.
	again, err := c.CreateSession(proto.SessionCreateParams{Parent: info.ID, Agent: agent.PlatformID, Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	takesKey(t, c, again.ID)
}

// With no credential in the vault the session still starts: the agent says what's missing.
func TestPlatformStartsWithNoKey(t *testing.T) {
	starts := fakePlatform(t)
	srv := startServer(t)
	c := connect(t, srv, nil)
	if _, err := c.CreateSession(proto.SessionCreateParams{Agent: agent.PlatformID, Dir: t.TempDir(), Cols: 80, Rows: 24}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "started", func() bool { return strings.Contains(readT(starts), "key:\n") })
}

// GOD of Agents on it: enowX's MCP as GOA, its duty with no exception, and every call
// held by the answer to tool.before, as it has no flags limiting its tools.
func TestGOAHoldsPlatformByHooks(t *testing.T) {
	starts := fakePlatform(t)
	srv := startServer(t)
	c := connect(t, srv, nil)
	info, err := c.CreateSession(proto.SessionCreateParams{Agent: agent.PlatformID, GOA: true, Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "started", func() bool { return strings.Contains(readT(starts), "args:") })
	said := readT(starts)
	if strings.Contains(said, "--tools") || strings.Contains(said, "--allowedTools") {
		t.Errorf("flags it doesn't take: %s", said)
	}
	files := srv.agentFilesDir(info.ID)
	if !strings.Contains(readT(filepath.Join(files, "mcp.json")), proto.EnvGOA) {
		t.Errorf("mcp.json: %s", readT(filepath.Join(files, "mcp.json")))
	}
	prompt := readT(filepath.Join(files, "system-prompt.md"))
	if !strings.Contains(prompt, "no exception") || !strings.Contains(prompt, "You are GOD of Agents") {
		t.Errorf("its prompt lacks the duty: %.300s", prompt)
	}
	call := func(tool, command string) proto.HookDecision {
		t.Helper()
		var d proto.HookDecision
		ev := proto.HookEvent{Event: proto.HookToolBefore, Tool: tool, Command: command}
		if err := c.Call(proto.MethodSessionHook, proto.SessionHookParams{ID: info.ID, Event: ev}, &d); err != nil {
			t.Fatal(err)
		}
		return d
	}
	for _, refused := range [][2]string{{"Bash", "go test ./..."}, {"Task", "go build it"}} {
		if d := call(refused[0], refused[1]); !d.Block {
			t.Errorf("%s not refused: %+v", refused[0], d)
		}
	}
	for _, allowed := range [][2]string{{"Bash", `"$ENOWX_BIN" sessions`}, {"Read", ""}, {"Glob", ""}, {"Grep", ""}, {"AskUserQuestion", ""}, {"mcp__enowx__goa_board", ""}} {
		if d := call(allowed[0], allowed[1]); d.Block {
			t.Errorf("%s refused: %s", allowed[0], d.Reason)
		}
	}
	list := goaAgents()
	i := slices.IndexFunc(list, func(a proto.GOAAgent) bool { return a.Agent == agent.PlatformID })
	if i < 0 || list[i].NotYet != "" || !list[i].Installed || list[i].Level != proto.GOAEnforced {
		t.Fatalf("goa agents: %+v", list)
	}
}

// A hook that changes a command goes to it as it is: the agent runs what it says.
func TestPlatformHookRewriteReachesIt(t *testing.T) {
	srv := startServer(t)
	info := proto.SessionInfo{Agent: agent.PlatformID}
	d := srv.rewritten(info, proto.HookEvent{}, proto.HookDecision{Command: "go vet"})
	if d.Block || d.Command != "go vet" {
		t.Fatalf("%+v", d)
	}
}

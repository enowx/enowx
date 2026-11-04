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

// fakePlatform stands in for enowX's program as the agent: it notes each start's
// arguments and key in a file, and waits.
func fakePlatform(t *testing.T) (out string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	dir := t.TempDir()
	out = filepath.Join(dir, "starts")
	script := "#!/bin/sh\nprintf 'args:%s\\nkey:%s\\n' \"$*\" \"$ANTHROPIC_API_KEY\" >> '" + out + "'\nwhile :; do sleep 1; done\n"
	bin := filepath.Join(dir, "enowx-fake")
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(agent.EnvBin, bin)
	t.Setenv("HOME", t.TempDir())
	return out
}

func startsWithKey(path string) int { return strings.Count(readT(path), "key:"+platformTestKey) }

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

// The agent runs on the vault's credential: lent into its environment with the app's
// say, and nowhere else; a wake and a daemon's restart have it lent again.
func TestPlatformKeyReachesItsEnvironmentAlone(t *testing.T) {
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
	waitFor(t, "its program started with the key", func() bool { return startsWithKey(starts) == 1 })
	said := readT(starts)
	for _, want := range []string{"claude-platform", "--session-dir", "--append-system-prompt-file", "--mcp-config", "mcp.json", "hello there"} {
		if !strings.Contains(said, want) {
			t.Errorf("its command lacks %s: %s", want, said)
		}
	}
	if strings.Contains(strings.Split(said, "\nkey:")[0], platformTestKey) {
		t.Fatal("the key is on its command line")
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
	waitFor(t, "started again with the key", func() bool { return startsWithKey(starts) == 2 })

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
	waitFor(t, "continued once an app serves", func() bool { return startsWithKey(starts) == 3 })
	if held := filesHolding(t, dir); len(held) > 0 {
		t.Fatalf("the key is in files: %v", held)
	}
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

// A hook that changes a command can't reach it: the call is refused, naming the command.
func TestPlatformHookRewriteIsARefusal(t *testing.T) {
	srv := startServer(t)
	info := proto.SessionInfo{Agent: agent.PlatformID}
	d := srv.rewritten(info, proto.HookEvent{}, proto.HookDecision{Command: "go vet"})
	if !d.Block || !strings.Contains(d.Reason, "go vet") {
		t.Fatalf("%+v", d)
	}
}

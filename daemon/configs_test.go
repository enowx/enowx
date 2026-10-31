package daemon

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/agentapi"
	"github.com/enowx/enowx/proto"
)

// recordingAgent puts a program named name on the PATH that writes what it
// started with (its arguments, its environment) to out/<session>.args and
// .env, then waits.
func recordingAgent(t *testing.T, name, out string) {
	t.Helper()
	bin := t.TempDir()
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"" + out + "/$ENOWX_SESSION.args.tmp\"\nenv > \"" + out + "/$ENOWX_SESSION.env.tmp\"\n" +
		"mv \"" + out + "/$ENOWX_SESSION.args.tmp\" \"" + out + "/$ENOWX_SESSION.args\"\nmv \"" + out + "/$ENOWX_SESSION.env.tmp\" \"" + out + "/$ENOWX_SESSION.env\"\nsleep 60\n"
	if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// started is what session id's fake agent started with.
func started(t *testing.T, out, id string) (args []string, env map[string]string) {
	t.Helper()
	a := waitFile(t, filepath.Join(out, id+".args"))
	e := waitFile(t, filepath.Join(out, id+".env"))
	env = map[string]string{}
	for _, l := range strings.Split(e, "\n") {
		if k, v, ok := strings.Cut(l, "="); ok {
			env[k] = v
		}
	}
	return strings.Split(strings.TrimSpace(a), "\n"), env
}

// keyVault serves the vault, lending the credential named name as key.
func keyVault(t *testing.T, srv *Server, name, key string) *vaultApp {
	return serveVault(t, srv, func(r proto.VaultRequest) (proto.VaultLend, error) {
		if r.Action == proto.VaultEnv && r.Name == name {
			return proto.VaultLend{Env: map[string]string{"API_KEY": key}, Credentials: []string{name}}, nil
		}
		return proto.VaultLend{}, errors.New("no")
	})
}

// writeOfficial gives the agent's own home a settings file, and returns
// what it holds.
func writeOfficial(t *testing.T, home, rel, text string) []byte {
	t.Helper()
	path := filepath.Join(home, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	return []byte(text)
}

func sameFile(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("%s changed:\n%s\nwas:\n%s (%v)", path, got, want, err)
	}
}

// Two Claude Code sessions at once, one on the official config and one
// on a model API config: each gets its own (the second's address, model
// and key in its environment and --settings), and the official settings
// stay byte for byte as they were.
func TestClaudeSessionsRunWithTheirOwnConfigs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	home, out := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	official := writeOfficial(t, home, ".claude/settings.json", "{\n  // the user's own\n  \"model\": \"opus\",\n  \"env\": {\"FOO\": \"1\"}\n}\n")
	recordingAgent(t, "claude", out)
	srv := startServer(t)
	c := connect(t, srv, nil)
	const key = "fake-or-key-1"
	keyVault(t, srv, "openrouter", key)
	cfg, err := c.SaveConfig(proto.Account{Agent: "claude", Name: "GLM", Kind: proto.ConfigKindAPI, Color: "#22aa66",
		API: &proto.ConfigAPI{BaseURL: "https://openrouter.ai/api/", KeyCred: "openrouter",
			Models: []proto.AgentAPIModel{{ID: "z-ai/glm-5", Effort: "high"}, {ID: "moonshot/kimi-k3"}}, SmallModel: "z-ai/glm-5-air"}})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.API.BaseURL != "https://openrouter.ai/api" || cfg.API.Model != "z-ai/glm-5" || cfg.API.Protocol != agentapi.Anthropic {
		t.Fatalf("kept as %+v", cfg.API)
	}
	plain, err := c.CreateSession(proto.SessionCreateParams{Agent: "claude", Dir: t.TempDir(), Cols: 100, Rows: 20})
	if err != nil {
		t.Fatal(err)
	}
	custom, err := c.CreateSession(proto.SessionCreateParams{Agent: "claude", Account: cfg.ID, Dir: t.TempDir(), Cols: 100, Rows: 20})
	if err != nil {
		t.Fatal(err)
	}
	if plain.Config != nil || custom.Config == nil || custom.Config.Name != "GLM" || custom.Config.Provider != proto.ProviderOpenRouter || custom.Config.Model != "z-ai/glm-5" {
		t.Fatalf("configs shown: %+v / %+v", plain.Config, custom.Config)
	}

	args, env := started(t, out, custom.ID)
	if env["ANTHROPIC_BASE_URL"] != "https://openrouter.ai/api" || env["ANTHROPIC_AUTH_TOKEN"] != key || env["ANTHROPIC_MODEL"] != "z-ai/glm-5" || env["ANTHROPIC_DEFAULT_HAIKU_MODEL"] != "z-ai/glm-5-air" {
		t.Fatalf("custom's environment: base %q model %q small %q key set %v", env["ANTHROPIC_BASE_URL"], env["ANTHROPIC_MODEL"], env["ANTHROPIC_DEFAULT_HAIKU_MODEL"], env["ANTHROPIC_AUTH_TOKEN"] != "")
	}
	settings := settingsArg(t, args)
	if e, _ := settings["env"].(map[string]any); e["ANTHROPIC_BASE_URL"] != "https://openrouter.ai/api" || e["ANTHROPIC_AUTH_TOKEN"] != nil {
		t.Fatalf("custom's --settings env: %v", settings["env"])
	}
	if !strings.Contains(strings.Join(args, " "), "moonshot/kimi-k3") || strings.Contains(strings.Join(args, " "), key) {
		t.Fatalf("custom's command line: %q", args)
	}

	args, env = started(t, out, plain.ID)
	if env["ANTHROPIC_BASE_URL"] != "" || env["ANTHROPIC_AUTH_TOKEN"] != "" || env["ANTHROPIC_MODEL"] != "" {
		t.Fatal("the official session got the config's API")
	}
	if e, _ := settingsArg(t, args)["env"].(map[string]any); e["ANTHROPIC_BASE_URL"] != nil {
		t.Fatalf("official's --settings env: %v", e)
	}
	sameFile(t, filepath.Join(home, ".claude/settings.json"), official)
	if _, err := os.Stat(filepath.Join(home, ".enowx-app", "agentapi")); err == nil {
		t.Fatal("enowX wrote the agent's API files")
	}

	// The judge's policy reads the session's provider.
	sess, _ := srv.sessions.Get(custom.ID)
	if p := sess.Info().Config.Provider; p != proto.ProviderOpenRouter {
		t.Fatalf("provider %q", p)
	}
	// Its key is masked like what the vault lends.
	if !slices.Contains(srv.lentSecrets(), key) {
		t.Fatal("the config's key is not masked")
	}
	// Switched to the official config, it goes on without the API.
	if _, err := c.SwitchAccount(proto.AccountSwitchParams{Session: custom.ID, Account: ""}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the switch", func() bool {
		s, _ := srv.sessions.Get(custom.ID)
		return s.Info().Config == nil
	})
	if slices.Contains(srv.lentSecrets(), key) {
		t.Fatal("the key stayed after switching")
	}
}

// settingsArg is Claude Code's --settings, as JSON.
func settingsArg(t *testing.T, args []string) map[string]any {
	t.Helper()
	i := slices.Index(args, "--settings")
	if i < 0 || i+1 >= len(args) {
		t.Fatalf("no --settings in %q", args)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(args[i+1]), &out); err != nil {
		t.Fatalf("--settings %q: %v", args[i+1], err)
	}
	return out
}

// Codex: a model API config's provider on its command line (-c), its key
// in the variable the provider names; its config.toml untouched.
func TestCodexSessionRunsWithItsConfig(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	home, out := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	writeOfficial(t, home, ".codex/config.toml", "model = \"gpt-5.5\"\n\n[projects.\"/x\"]\ntrust_level = \"trusted\"\n")
	recordingAgent(t, "codex", out)
	srv := startServer(t)
	c := connect(t, srv, nil)
	const key = "fake-gw-key-2"
	keyVault(t, srv, "gateway", key)
	cfg, err := c.SaveConfig(proto.Account{Agent: "codex", Name: "Gateway", Kind: proto.ConfigKindAPI,
		API: &proto.ConfigAPI{BaseURL: "https://gw.example.com/v1", KeyCred: "gateway", Models: []proto.AgentAPIModel{{ID: "gpt-oss-200b", Effort: "high", Context: 200000}}}})
	if err != nil {
		t.Fatal(err)
	}
	plain, _ := c.CreateSession(proto.SessionCreateParams{Agent: "codex", Dir: t.TempDir(), Cols: 100, Rows: 20})
	started(t, out, plain.ID)
	// What every Codex session of enowX's does to it (trusting enowX's
	// hooks) done, the config's session changes nothing more.
	official, err := os.ReadFile(filepath.Join(home, ".codex/config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	custom, err := c.CreateSession(proto.SessionCreateParams{Agent: "codex", Account: cfg.ID, Dir: t.TempDir(), Cols: 100, Rows: 20})
	if err != nil {
		t.Fatal(err)
	}
	args, env := started(t, out, custom.ID)
	line := strings.Join(args, " ")
	for _, want := range []string{`model_provider="enowx"`, `model_providers.enowx.base_url="https://gw.example.com/v1"`, `model_providers.enowx.env_key="ENOWX_CODEX_API_KEY"`,
		`model="gpt-oss-200b"`, `model_reasoning_effort="high"`, `model_context_window=200000`} {
		if !strings.Contains(line, want) {
			t.Fatalf("custom's command line lacks %s: %q", want, args)
		}
	}
	if env["ENOWX_CODEX_API_KEY"] != key || strings.Contains(line, key) {
		t.Fatal("the key isn't in the environment alone")
	}
	args, env = started(t, out, plain.ID)
	if strings.Contains(strings.Join(args, " "), "model_provider") || env["ENOWX_CODEX_API_KEY"] != "" {
		t.Fatalf("the official session got the config: %q", args)
	}
	sameFile(t, filepath.Join(home, ".codex/config.toml"), official)
}

// A model API set in Claude Code's settings before configs becomes a
// config: its key kept in the vault (the app asked), the config the
// default for new sessions, the settings as the user had them.
func TestModelAPIMovesToAConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	official := writeOfficial(t, home, ".claude/settings.json", "{\n  \"model\": \"opus\",\n  \"env\": {\"FOO\": \"1\"}\n}\n")
	const key = "fake-legacy-key-3"
	if err := agentapi.Set(home, "claude", agentapi.API{BaseURL: "https://gw.example.com", APIKey: key, Model: "glm-5", SmallModel: "glm-5-air"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(home, ".claude/settings.json")); !strings.Contains(string(got), "gw.example.com") {
		t.Fatal("not set")
	}
	srv := startServer(t)
	srv.migrateAPIsOn = true
	c := connect(t, srv, nil)

	// The vault refusing (an app connecting tries), nothing moves.
	refuse := serveVault(t, srv, func(r proto.VaultRequest) (proto.VaultLend, error) {
		return proto.VaultLend{}, errors.New("vault: the user refused")
	})
	waitFor(t, "asked to keep the key", func() bool {
		refuse.mu.Lock()
		defer refuse.mu.Unlock()
		return len(refuse.seen) > 0 && refuse.seen[0].Action == proto.VaultAdd
	})
	if len(configsOf(t, c, "claude")) != 0 {
		t.Fatal("a config without its key")
	}
	if got, _ := os.ReadFile(filepath.Join(home, ".claude/settings.json")); !strings.Contains(string(got), "gw.example.com") {
		t.Fatal("taken out of its settings without its key kept")
	}
	refuse.Close()

	var mu sync.Mutex
	var kept map[string]string
	serveVault(t, srv, func(r proto.VaultRequest) (proto.VaultLend, error) {
		mu.Lock()
		defer mu.Unlock()
		if r.Action == proto.VaultAdd {
			kept = r.Fields
		}
		return proto.VaultLend{}, nil
	})
	waitFor(t, "the config", func() bool { srv.migrateAPIs(); return len(configsOf(t, c, "claude")) == 1 })
	waitFor(t, "the files restored", func() bool { a, _ := agentapi.Of(home, "claude"); return a.Current == nil })
	mu.Lock()
	defer mu.Unlock()
	cfgs := configsOf(t, c, "claude")
	if len(cfgs) != 1 || cfgs[0].Kind != proto.ConfigKindAPI || cfgs[0].API.BaseURL != "https://gw.example.com" || cfgs[0].API.Model != "glm-5" || cfgs[0].API.KeyCred != "claude-model-api" {
		t.Fatalf("configs: %+v", cfgs)
	}
	if kept["value"] != key {
		t.Fatal("the key wasn't kept in the vault")
	}
	if srv.accountPrefs.Of("claude").Default != cfgs[0].Name {
		t.Fatal("new sessions don't go on with the API")
	}
	sameFile(t, filepath.Join(home, ".claude/settings.json"), official)
	if a, _ := agentapi.Of(home, "claude"); a.Current != nil {
		t.Fatal("still set in its files")
	}
}

func configsOf(t *testing.T, c *client.Client, agentID string) []proto.Account {
	t.Helper()
	list, err := c.Accounts()
	if err != nil {
		t.Fatal(err)
	}
	return slices.DeleteFunc(list, func(a proto.Account) bool { return a.Agent != agentID })
}

// A session on the official config at its usage limit goes on as the
// next config in the order, a model API's, its conversation resumed.
func TestSessionFallsBackToAnAPIConfig(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	t.Setenv("HOME", t.TempDir())
	srv := startServer(t)
	falls := make(chan proto.AccountFallback, 8)
	c := connect(t, srv, func(e proto.Event) {
		var f proto.AccountFallback
		if e.Name == proto.EventAccountFallback && json.Unmarshal(e.Data, &f) == nil {
			falls <- f
		}
	})
	keyVault(t, srv, "gateway", "fake-gw-key-4")
	cfg, err := c.SaveConfig(proto.Account{Agent: "claude", Name: "Gateway", Kind: proto.ConfigKindAPI,
		API: &proto.ConfigAPI{BaseURL: "https://gw.example.com", KeyCred: "gateway", Models: []proto.AgentAPIModel{{ID: "glm-5"}}}})
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	script := "#!/bin/sh\necho \"base=$ANTHROPIC_BASE_URL\"\n[ -n \"$ANTHROPIC_BASE_URL\" ] || echo 'Claude usage limit reached. Your limit will reset at 3pm'\nwhile read -r line; do echo \"heard: $line\"; done\n"
	os.WriteFile(filepath.Join(bin, "claude"), []byte(script), 0o755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if _, err := srv.setAccountPrefs(proto.AccountPrefsParams{Agent: "claude", Prefs: proto.AccountPrefs{Order: []string{"", "Gateway"}, Fallback: proto.FallbackAuto}}); err != nil {
		t.Fatal(err)
	}
	info, err := c.CreateSession(proto.SessionCreateParams{Agent: "claude", Dir: t.TempDir(), Cols: 120, Rows: 20})
	if err != nil {
		t.Fatal(err)
	}
	var f proto.AccountFallback
	select {
	case f = <-falls:
	case <-time.After(15 * time.Second):
		t.Fatal("no fallback")
	}
	if !f.Switched || f.From != "" || f.To != "Gateway" || f.ToID != cfg.ID {
		t.Fatalf("fallback: %+v", f)
	}
	sess, _ := srv.sessions.Get(info.ID)
	waitFor(t, "going on as the API config, told to", func() bool {
		text := sess.Terminal().Text()
		return strings.Contains(text, "base=https://gw.example.com") && strings.Contains(text, "heard: ")
	})
	if got, _ := srv.sessions.Get(info.ID); got.Info().Config == nil || got.Info().Config.Name != "Gateway" {
		t.Fatalf("config %+v", got.Info().Config)
	}
}

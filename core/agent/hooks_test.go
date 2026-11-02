package agent

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

// fakeBin is an ENOWX_BIN that records its arguments in log.
func fakeBin(t *testing.T, log string) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "enowx")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho \"$@\" >> "+log+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin
}

func TestCodexNotifyReportsDoneAndRunsTheUsersOwn(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("hooks are POSIX shell")
	}
	dir := t.TempDir()
	log := filepath.Join(dir, "log")
	user := []string{"sh", "-c", `echo "user $1 $2" >> ` + log, "x", "turn-ended"}
	args := codexHookArgs(user)
	if len(args) != 2+2*len(codexEvents) || args[0] != "-c" {
		t.Fatalf("args = %q", args)
	}
	// Each hook is TOML too, a group of one handler.
	for i := 2; i < len(args); i += 2 {
		var hooks map[string]any
		if _, err := toml.Decode(args[i+1], &hooks); err != nil {
			t.Fatalf("%s: %v", args[i+1], err)
		}
	}
	// Codex parses the value as TOML.
	var cfg struct {
		Notify []string `toml:"notify"`
	}
	if _, err := toml.Decode(args[1], &cfg); err != nil {
		t.Fatalf("%s: %v", args[1], err)
	}
	// Codex runs notify with the turn as its last argument.
	cmd := exec.Command(cfg.Notify[0], append(cfg.Notify[1:], `{"type":"agent-turn-complete"}`)...)
	cmd.Env = append(os.Environ(), EnvBin+"="+fakeBin(t, log))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	got, _ := os.ReadFile(log)
	want := "hook done\nuser turn-ended {\"type\":\"agent-turn-complete\"}\n"
	if string(got) != want {
		t.Fatalf("ran:\n%s\nwant:\n%s", got, want)
	}

	// Without ENOWX_BIN (outside enowX) and without a notify of the user's,
	// it does nothing and succeeds.
	cfg.Notify = nil
	if _, err := toml.Decode(codexHookArgs(nil)[1], &cfg); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(cfg.Notify[0], append(cfg.Notify[1:], "{}")...).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
}

func TestClaudeHooksReportEachState(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("hooks are POSIX shell")
	}
	args := claudeHookArgs()
	if len(args) != 2 || args[0] != "--settings" {
		t.Fatalf("args = %q", args)
	}
	var settings struct {
		Hooks map[string][]struct {
			Matcher string
			Hooks   []struct{ Type, Command string }
		}
	}
	if err := json.Unmarshal([]byte(args[1]), &settings); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(t.TempDir(), "log")
	bin := fakeBin(t, log)
	for _, event := range []string{"UserPromptSubmit", "Notification", "Stop"} {
		for _, m := range settings.Hooks[event] {
			for _, h := range m.Hooks {
				cmd := exec.Command("sh", "-c", h.Command)
				cmd.Env = append(os.Environ(), EnvBin+"="+bin)
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("%s: %v: %s", event, err, out)
				}
			}
		}
	}
	got, _ := os.ReadFile(log)
	if want := "hook --event prompt.start --state working\nhook --event waiting --state waiting\nhook --event turn.done --state done\n"; string(got) != want {
		t.Fatalf("ran %q, want %q", got, want)
	}
	if !strings.Contains(settings.Hooks["Notification"][0].Matcher, "permission_prompt") {
		t.Fatalf("Notification matcher = %q", settings.Hooks["Notification"][0].Matcher)
	}
}

// Codex reads -c values as TOML: the guide must come back unchanged.
func TestCodexGuideIsTOML(t *testing.T) {
	t.Setenv("CODEX_HOME", t.TempDir())
	args := GuideArgs("codex")
	if len(args) != 2 || args[0] != "-c" {
		t.Fatalf("args = %q", args)
	}
	var got struct {
		Instructions string `toml:"developer_instructions"`
	}
	if _, err := toml.Decode(args[1], &got); err != nil {
		t.Fatal(err)
	}
	if got.Instructions != Guide {
		t.Fatalf("decoded %q", got.Instructions)
	}
}

// Codex trusts a hook by the hash of its definition: the hash of a hook
// Codex itself trusted (command "true", 10 s, on prompts) is computed the
// same, and enowX's own are recorded once, the rest of the file kept.
func TestCodexHookTrust(t *testing.T) {
	h := agentHook{Type: "command", Command: "true", Timeout: 10}
	if got, want := codexHookHash("user_prompt_submit", h), "sha256:8d92cb18f47b2766730ec51ce987dd790a3e6ee9b3237b1aba8b3e00c9885990"; got != want {
		t.Fatalf("hash %s, want %s", got, want)
	}
	if runtime.GOOS == "windows" {
		return
	}
	home := t.TempDir()
	user := "model = \"gpt\"\n\n[hooks.state.\"/x/hooks.json:stop:0:0\"]\nenabled = false\n"
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(user), 0o600); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := TrustCodexHooks(home); err != nil {
			t.Fatal(err)
		}
	}
	data, _ := os.ReadFile(filepath.Join(home, "config.toml"))
	var cfg struct {
		Model string
		Hooks struct {
			State map[string]struct {
				Enabled     *bool  `toml:"enabled"`
				TrustedHash string `toml:"trusted_hash"`
			}
		}
	}
	if _, err := toml.Decode(string(data), &cfg); err != nil {
		t.Fatalf("%v:\n%s", err, data)
	}
	if cfg.Model != "gpt" || cfg.Hooks.State["/x/hooks.json:stop:0:0"].Enabled == nil || len(cfg.Hooks.State) != 1+len(codexEvents) {
		t.Fatalf("config:\n%s", data)
	}
	g := codexOn(codexEvents[2].event, codexEvents[2].state, codexEvents[2].timeout)
	if got := cfg.Hooks.State[codexFlagsSource+":pre_tool_use:0:0"].TrustedHash; got != codexHookHash("pre_tool_use", g.Hooks[0]) {
		t.Fatalf("pre_tool_use trusted as %q", got)
	}
}

func TestCodexHookCommandsIdentifyAgent(t *testing.T) {
	args := codexHookArgs(nil)
	for i := 2; i < len(args); i += 2 {
		var cfg struct {
			Hooks map[string][]agentMatcher `toml:"hooks"`
		}
		if _, err := toml.Decode(args[i+1], &cfg); err != nil {
			t.Fatal(err)
		}
		for _, groups := range cfg.Hooks {
			if len(groups) != 1 || len(groups[0].Hooks) != 1 || !strings.Contains(groups[0].Hooks[0].Command, "--agent=codex") {
				t.Fatalf("hook missing agent: %s", args[i+1])
			}
		}
	}
	if strings.Contains(claudeHookArgs()[1], "--agent=codex") {
		t.Fatal("Claude hook identifies as Codex")
	}
}

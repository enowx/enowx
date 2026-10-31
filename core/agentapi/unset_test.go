package agentapi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Taking enowX's API back out: the files as the user had them when
// nothing else changed them, byte for byte; else enowX's entries alone
// removed, the user's later change kept.
func TestUnsetRestoresOrClears(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".claude", "settings.json")
	os.MkdirAll(filepath.Dir(path), 0o700)
	original := "{\n  // mine\n  \"model\": \"opus\"\n}\n"
	os.WriteFile(path, []byte(original), 0o600)
	api := API{BaseURL: "https://gw.example.com", APIKey: "fake-key-1", Models: []Model{{ID: "m1", Effort: "high"}, {ID: "m2"}}}
	if err := Set(home, "claude", api); err != nil {
		t.Fatal(err)
	}
	restored, err := Unset(home, "claude")
	if err != nil || !restored {
		t.Fatalf("restored %v, %v", restored, err)
	}
	if got, _ := os.ReadFile(path); string(got) != original {
		t.Fatalf("not as it was:\n%s", got)
	}

	if err := Set(home, "claude", api); err != nil {
		t.Fatal(err)
	}
	if err := editJSON(home, ".claude/settings.json", jsonEdit{path: []string{"theme"}, value: "dark"}); err != nil {
		t.Fatal(err)
	}
	restored, err = Unset(home, "claude")
	if err != nil || restored {
		t.Fatalf("restored %v, %v", restored, err)
	}
	got, _ := os.ReadFile(path)
	if !strings.Contains(string(got), "dark") || strings.Contains(string(got), "gw.example.com") || strings.Contains(string(got), "fake-key-1") {
		t.Fatalf("cleared:\n%s", got)
	}
}

// A session's API reaches it by its environment and command line, its key
// in the environment alone; Claude Code's settings merge into the
// --settings its hooks have.
func TestSessionLaunch(t *testing.T) {
	l, err := SessionLaunch("claude-agent-teams", API{BaseURL: "https://gw.example.com/", APIKey: "fake-key-2", Models: []Model{{ID: "m1"}}}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !contains(l.Env, "ANTHROPIC_AUTH_TOKEN=fake-key-2") || !contains(l.Env, "ANTHROPIC_MODEL=m1") {
		t.Fatalf("env %q", l.Env)
	}
	args := WithSettings([]string{"--settings", `{"hooks":{"Stop":[]}}`, "--x"}, l.Settings)
	if len(args) != 3 || !strings.Contains(args[1], `"hooks"`) || !strings.Contains(args[1], `"ANTHROPIC_BASE_URL":"https://gw.example.com"`) || strings.Contains(args[1], "fake-key-2") {
		t.Fatalf("args %q", args)
	}
	dir := t.TempDir()
	l, err = SessionLaunch("opencode", API{BaseURL: "https://gw.example.com/v1", APIKey: "fake-key-3", Models: []Model{{ID: "m1"}}}, dir)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, ".config/opencode/opencode.json"))
	if !contains(l.Env, openCodeKeyVar+"=fake-key-3") || strings.Contains(string(data), "fake-key-3") || !strings.Contains(string(data), "{env:"+openCodeKeyVar+"}") {
		t.Fatalf("env %q, config %s", l.Env, data)
	}
	if _, err := SessionLaunch("droid", API{BaseURL: "https://x"}, dir); err == nil {
		t.Fatal("droid per session")
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

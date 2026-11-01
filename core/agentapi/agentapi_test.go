package agentapi

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// The user's own settings, which must come through every change.
var userFiles = map[string]string{
	".claude/settings.json":          "{\n  // mine\n  \"theme\": \"dark\",\n  \"env\": {\"MY_VAR\": \"1\"}\n}\n",
	".codex/config.toml":             "approval_policy = \"never\"\n\n[mcp_servers.docs]\ncommand = \"docs-mcp\"\n",
	".config/opencode/opencode.json": "{\"theme\": \"opencode\", \"provider\": {\"mine\": {\"options\": {}}}}\n",
	".gemini/settings.json":          "{\"ui\": {\"theme\": \"GitHub\"}}\n",
	".gemini/.env":                   "OTHER=1\n",
	".qwen/settings.json":            "{\"modelProviders\": {\"openai\": [{\"id\": \"theirs\", \"envKey\": \"X\"}]}}\n",
	".factory/settings.json":         "{\"customModels\": [{\"model\": \"theirs\", \"displayName\": \"Theirs\"}]}\n",
	".config/goose/config.yaml":      "GOOSE_MODE: auto\nextensions:\n  dev:\n    enabled: true\n",
	".aider.conf.yml":                "dark-mode: true\n",
	".kimi-code/config.toml":         "theme = \"dark\"\n\n[providers.kimi]\ntype = \"kimi\"\n",
	".vibe/config.toml":              "theme = \"x\"\n\n[[providers]]\nname = \"mistral\"\napi_base = \"https://api.mistral.ai/v1\"\n",
	".pi/agent/settings.json":        "{\"theme\": \"light\"}\n",
}

func TestEveryAgentTakesAnAPIAndGivesItBack(t *testing.T) {
	home := t.TempDir()
	for rel, text := range userFiles {
		os.MkdirAll(filepath.Join(home, filepath.Dir(rel)), 0o755)
		os.WriteFile(filepath.Join(home, rel), []byte(text), 0o644)
	}
	for _, id := range Supported() {
		a := adapters[id]
		for _, proto := range a.protocols {
			api := API{BaseURL: "https://gw.example.com/v1", APIKey: "sk-test-" + id, Protocol: proto, Model: "gw-model",
				Models: []Model{{ID: "gw-model", Vision: true, Context: 200000, Output: 32000}, {ID: "gw-alt", Context: 64000, Output: 8000}}}
			if len(a.caps.Efforts) > 0 {
				api.Effort = a.caps.Efforts[len(a.caps.Efforts)-1]
				api.Models[0].Effort, api.Models[1].Effort = a.caps.Efforts[0], a.caps.Efforts[len(a.caps.Efforts)-1]
			}
			if a.small {
				api.SmallModel = "gw-alt"
			}
			if err := Set(home, id, api); err != nil {
				t.Fatalf("%s %s: set: %v", id, proto, err)
			}
			want := api
			want.Models = slices.Clone(api.Models)
			normalize(&want, a)
			if !a.caps.Multi && want.SmallModel != "" && id != "aider" {
				t.Fatalf("%s: a quick model without several", id)
			}
			got, err := Of(home, id)
			if err != nil || got.Current == nil {
				t.Fatalf("%s %s: read back: %+v %v", id, proto, got, err)
			}
			if c := *got.Current; !reflect.DeepEqual(c, want) {
				t.Errorf("%s %s: read back\n%+v\nwant\n%+v", id, proto, c, want)
			}
		}
		// The user's own settings stayed.
		for rel, text := range userFiles {
			if !slicesContains(a.files, rel) {
				continue
			}
			data, _ := os.ReadFile(filepath.Join(home, rel))
			for _, keep := range []string{"dark", "docs-mcp", "opencode", "GitHub", "OTHER", "theirs", "GOOSE_MODE", "dark-mode", "mistral", "light", "MY_VAR"} {
				if strings.Contains(text, keep) && !strings.Contains(string(data), keep) {
					t.Errorf("%s: %s lost %q:\n%s", id, rel, keep, data)
				}
			}
		}
		if err := Clear(home, id); err != nil {
			t.Fatalf("%s: clear: %v", id, err)
		}
		if got, _ := Of(home, id); got.Current != nil {
			t.Errorf("%s: still set after clear: %+v", id, got.Current)
		}
		// The original backup puts the files back byte for byte.
		bs, _ := Backups(home, id)
		orig := ""
		for _, b := range bs {
			if b.Original {
				orig = b.ID
			}
		}
		if orig == "" {
			t.Fatalf("%s: no original backup in %+v", id, bs)
		}
		Set(home, id, API{BaseURL: "https://other.example.com", APIKey: "k", Model: "m", Protocol: a.protocols[0]})
		if err := Restore(home, id, orig); err != nil {
			t.Fatalf("%s: restore: %v", id, err)
		}
		for _, rel := range a.files {
			data, err := os.ReadFile(filepath.Join(home, rel))
			want, had := userFiles[rel]
			switch {
			case !had && err == nil:
				t.Errorf("%s: %s was made and not removed on restore", id, rel)
			case had && string(data) != want:
				t.Errorf("%s: %s restored as\n%s\nwant\n%s", id, rel, data, want)
			}
		}
	}
}

func slicesContains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// Kimi's settings are in ~/.kimi-code now; what enowX wrote in ~/.kimi before
// is still found, and taken out, leaving the user's own.
func TestKimiFindsWhatWasWrittenInItsOldFolder(t *testing.T) {
	home := t.TempDir()
	old := filepath.Join(home, ".kimi", "config.toml")
	os.MkdirAll(filepath.Dir(old), 0o700)
	os.WriteFile(old, []byte("theme = \"dark\"\ndefault_model = \"enowx-m\"\n\n[providers.enowx]\ntype = \"openai_legacy\"\nbase_url = \"https://gw.example.com/v1\"\napi_key = \"fake-kimi\"\n\n[models.enowx-m]\nprovider = \"enowx\"\nmodel = \"m\"\nmax_context_size = 131072\n"), 0o600)
	got, err := Of(home, "kimi")
	if err != nil || got.Current == nil || got.Current.Protocol != OpenAI || got.Current.Model != "m" || got.Current.APIKey != "fake-kimi" {
		t.Fatalf("read %+v, %v", got.Current, err)
	}
	if err := Clear(home, "kimi"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(old)
	if strings.Contains(string(data), "fake-kimi") || !strings.Contains(string(data), "dark") {
		t.Fatalf("cleared:\n%s", data)
	}
	if _, err := os.Stat(filepath.Join(home, kimiFile)); err == nil {
		t.Error("clearing made a config in its new folder")
	}
	if err := Set(home, "kimi", API{BaseURL: "https://gw.example.com/v1", APIKey: "fake-kimi", Model: "m"}); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(filepath.Join(home, kimiFile))
	if !strings.Contains(string(data), `type = "openai"`) {
		t.Fatalf("written:\n%s", data)
	}
}

func TestSessionsGetTheKeysOfAgentsThatReadThemFromTheEnvironment(t *testing.T) {
	home := t.TempDir()
	if err := Set(home, "codex", API{BaseURL: "https://gw.example.com/v1", APIKey: "sk-codex", Model: "m"}); err != nil {
		t.Fatal(err)
	}
	if env := LaunchEnv(home, "codex"); len(env) != 1 || env[0] != codexKeyVar+"=sk-codex" {
		t.Fatalf("codex env %q", env)
	}
	if err := Set(home, "copilot", API{BaseURL: "https://gw.example.com/v1", APIKey: "sk-cp", Model: "m", Protocol: Anthropic}); err != nil {
		t.Fatal(err)
	}
	env := strings.Join(LaunchEnv(home, "copilot"), " ")
	if !strings.Contains(env, "COPILOT_PROVIDER_TYPE=anthropic") || !strings.Contains(env, "COPILOT_PROVIDER_API_KEY=sk-cp") || strings.Contains(env, protocolVar) {
		t.Fatalf("copilot env %s", env)
	}
	if err := Set(home, "continue", API{BaseURL: "https://gw.example.com/v1", APIKey: "k", Model: "m"}); err != nil {
		t.Fatal(err)
	}
	if args := LaunchArgs(home, "continue"); len(args) != 2 || args[0] != "--config" {
		t.Fatalf("continue args %q", args)
	}
	if fi, _ := os.Stat(filepath.Join(home, envFile("codex"))); fi.Mode().Perm() != 0o600 {
		t.Errorf("the keys' file is %v", fi.Mode().Perm())
	}
	if err := Set(home, "codex", API{BaseURL: "ftp://x"}); err == nil {
		t.Error("an address that isn't http was taken")
	}
	if err := Set(home, "codex", API{BaseURL: "https://x", Protocol: Anthropic}); err == nil {
		t.Error("a protocol codex doesn't speak was taken")
	}
}

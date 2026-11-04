package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func configOf(t *testing.T, env []string) map[string]any {
	t.Helper()
	for _, e := range env {
		if v, ok := strings.CutPrefix(e, "OPENCODE_CONFIG_CONTENT="); ok {
			var cfg map[string]any
			if err := json.Unmarshal([]byte(v), &cfg); err != nil {
				t.Fatal(err)
			}
			return cfg
		}
	}
	t.Fatal("no config")
	return nil
}

// Version 2 gets policies, version 1 an agent's permissions and the plugin, kept out of
// the folder GOA may edit.
func TestGOAOpenCodeEnvFollowsItsVersion(t *testing.T) {
	data := t.TempDir()
	env, err := GOAOpenCodeEnv(2, "/bin/enowx", data)
	if err != nil {
		t.Fatal(err)
	}
	cfg := configOf(t, env)
	if cfg["policies"] == nil || cfg["agents"] == nil || cfg["agent"] != nil {
		t.Errorf("v2: %v", cfg)
	}
	if strings.Contains(strings.Join(env, " "), "OPENCODE_CONFIG_DIR") {
		t.Error("v2 got the plugin")
	}

	env, err = GOAOpenCodeEnv(1, "/bin/enowx", data)
	if err != nil {
		t.Fatal(err)
	}
	cfg = configOf(t, env)
	if cfg["agent"] == nil || cfg["policies"] != nil {
		t.Errorf("v1: %v", cfg)
	}
	plugin := filepath.Join(data, "goa-opencode", "plugins", "goa.js")
	if _, err := os.Stat(plugin); err != nil {
		t.Error(err)
	}
	if strings.HasPrefix(plugin, filepath.Join(data, "goa")+string(filepath.Separator)) {
		t.Error("the plugin is in GOA's own folder")
	}
}

// The guide doesn't promise a shell OpenCode's GOA hasn't got, and its first prompt
// doesn't send it to a file its config already gave.
func TestGOAOpenCodeGuide(t *testing.T) {
	if !strings.Contains(GOAAgentNote("opencode"), "no shell") || GOAAgentNote("claude") != "" {
		t.Error("note")
	}
	if GOANote("opencode", "guide") != "" {
		t.Error("first prompt names a file")
	}
}

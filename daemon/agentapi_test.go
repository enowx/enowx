package daemon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/enowx/enowx/proto"
)

// An agent that takes a model API only in its own files, pointed at one
// through the daemon: the key comes back masked, a change keeping the key
// keeps it, and the original settings come back. An agent that can take one per session
// (Codex) isn't written to: its API is a config.
func TestAgentAPIThroughTheDaemon(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	os.MkdirAll(filepath.Join(home, ".vibe"), 0o755)
	original := "active_model = \"x\"\n"
	os.WriteFile(filepath.Join(home, ".vibe", "config.toml"), []byte(original), 0o644)
	srv := startServer(t)
	c := connect(t, srv, nil)

	if _, err := c.SetAgentAPI("codex", proto.AgentAPI{BaseURL: "https://gw.example.com/v1", APIKey: "sk-secret-1234", Model: "gw-model"}); err == nil {
		t.Fatal("Codex's own settings written")
	}
	info, err := c.SetAgentAPI("mistral-vibe", proto.AgentAPI{BaseURL: "https://gw.example.com/v1", APIKey: "sk-secret-1234", Model: "gw-model"})
	if err != nil {
		t.Fatal(err)
	}
	if info.Current == nil || info.Current.APIKey != "••••1234" || info.Current.Model != "gw-model" || len(info.Backups) != 1 || !info.Backups[0].Original {
		t.Fatalf("set: %+v", info)
	}
	info, err = c.SetAgentAPI("mistral-vibe", proto.AgentAPI{BaseURL: "https://gw2.example.com/v1", APIKey: proto.AgentAPIKeepKey, Model: "other"})
	if err != nil || info.Current.BaseURL != "https://gw2.example.com/v1" || info.Current.APIKey != "••••1234" {
		t.Fatalf("kept key: %+v %v", info.Current, err)
	}
	list, err := c.AgentAPIs()
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := json.Marshal(list); strings.Contains(string(b), "sk-secret") {
		t.Fatalf("a key left the host: %s", b)
	}
	if _, err := c.RestoreAgentAPI("mistral-vibe", ""); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(home, ".vibe", "config.toml")); string(data) != original {
		t.Fatalf("restored:\n%s", data)
	}
}

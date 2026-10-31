package accounts

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/enowx/enowx/proto"
)

// Accounts kept before configs (accounts.json) become configs of the
// account kind in the synced settings, their homes where they were; a
// team member's stays the host's own.
func TestAccountsBecomeConfigs(t *testing.T) {
	dir := t.TempDir()
	old := []proto.Account{{ID: "aaa", Agent: "claude", Name: "Work"}, {ID: "bbb", Agent: "codex", Name: "Key", Cred: "openai-work"}, {ID: "ccc", Agent: "claude", Name: "Ada", Member: "m1"}}
	data, _ := json.Marshal(old)
	os.WriteFile(filepath.Join(dir, "accounts.json"), data, 0o600)
	os.MkdirAll(filepath.Join(dir, "accounts", "aaa"), 0o700)
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	var synced []proto.Account
	data, _ = os.ReadFile(filepath.Join(dir, "settings", "configs.json"))
	if json.Unmarshal(data, &synced) != nil || len(synced) != 2 || synced[0].ID != "aaa" || synced[1].Cred != "openai-work" {
		t.Fatalf("synced configs: %s", data)
	}
	var members []proto.Account
	data, _ = os.ReadFile(filepath.Join(dir, "accounts.json"))
	if json.Unmarshal(data, &members) != nil || len(members) != 1 || members[0].Member != "m1" {
		t.Fatalf("host's own: %s", data)
	}
	if !s.HasHome("aaa") || len(s.List()) != 3 {
		t.Fatalf("list %+v", s.List())
	}
	// Opened again, nothing more moves.
	if s, err = Open(dir); err != nil || len(s.List()) != 3 {
		t.Fatalf("again: %v %d", err, len(s.List()))
	}
}

// A config is checked as it is kept: its kind fits its agent, its name is
// its agent's alone, its variables are NAME=VALUE, its API is one its
// agent takes per session.
func TestConfigsAreChecked(t *testing.T) {
	s, _ := Open(t.TempDir())
	api := &proto.ConfigAPI{BaseURL: "https://gw.example.com/", KeyCred: "gw", Models: []proto.AgentAPIModel{{ID: "m1"}}}
	for name, c := range map[string]proto.Account{
		"no name":           {Agent: "claude", Kind: proto.ConfigKindAPI, API: api},
		"official's name":   {Agent: "claude", Name: "Official", Kind: proto.ConfigKindAPI, API: api},
		"bad variable":      {Agent: "claude", Name: "X", Env: []string{"1A=b"}, Kind: proto.ConfigKindAPI, API: api},
		"no account kind":   {Agent: "opencode", Name: "X"},
		"no per-session":    {Agent: "droid", Name: "X", Kind: proto.ConfigKindAPI, API: api},
		"not a URL":         {Agent: "claude", Name: "X", Kind: proto.ConfigKindAPI, API: &proto.ConfigAPI{BaseURL: "gw.example.com"}},
		"protocol it lacks": {Agent: "codex", Name: "X", Kind: proto.ConfigKindAPI, API: &proto.ConfigAPI{BaseURL: "https://x", Protocol: "anthropic"}},
	} {
		if _, err := s.Save(c); err == nil {
			t.Errorf("%s: kept", name)
		}
	}
	c, err := s.Save(proto.Account{Agent: "claude-agent-teams", Name: "GLM", Kind: proto.ConfigKindAPI, API: api, Env: []string{" FOO=1 ", ""}})
	if err != nil {
		t.Fatal(err)
	}
	if c.Agent != "claude" || c.API.BaseURL != "https://gw.example.com" || c.API.Model != "m1" || len(c.Env) != 1 || s.HasHome(c.ID) {
		t.Fatalf("kept as %+v", c)
	}
	if _, err := s.Save(proto.Account{Agent: "claude", Name: "glm", Kind: proto.ConfigKindAPI, API: api}); err == nil {
		t.Fatal("a second GLM")
	}
	c.Name = "GLM 5"
	if _, err := s.Save(c); err != nil {
		t.Fatal(err)
	}
	c.Kind, c.API = proto.ConfigKindAccount, nil
	if _, err := s.Save(c); err == nil {
		t.Fatal("changed kind")
	}
	if got, ok := s.ByName("claude", "glm 5"); !ok || got.ID != c.ID {
		t.Fatal("renamed config not found")
	}
}

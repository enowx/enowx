package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// What an agent's start gives it: enowX's server for one session, by the way its
// documentation has, and nothing else's name for it.
func TestToolArgsPerAgent(t *testing.T) {
	const exe = "/bin/enowx"
	for _, id := range []string{"claude", "claude-agent-teams", "openclaude", "kimi"} {
		args := ToolArgs(id, exe)
		if len(args) != 2 || args[0] != "--mcp-config" || !strings.Contains(args[1], `"command":"/bin/enowx"`) {
			t.Errorf("%s: %q", id, args)
		}
	}
	if a := ToolArgs("copilot", exe); len(a) != 1 || !strings.HasPrefix(a[0], "--additional-mcp-config=") || !strings.Contains(a[0], `"tools":["*"]`) {
		t.Errorf("copilot: %q", a)
	}
	if a := strings.Join(ToolArgs("codex", exe), " "); !strings.Contains(a, "mcp_servers.enowx.command") || !strings.Contains(a, `mcp_servers.enowx.args=["mcp"]`) {
		t.Errorf("codex: %q", a)
	}
	if a := ToolArgs("gemini", exe); a != nil {
		t.Errorf("gemini has no flag for it: %q", a)
	}
}

// OpenCode and Kilo Code take it all from their environment, naming the guide's
// file: nothing of the user's is written, and their own config still applies.
func TestToolEnvCarriesTheGuide(t *testing.T) {
	for id, key := range map[string]string{"opencode": "OPENCODE_CONFIG_CONTENT", "kilo": "KILO_CONFIG_CONTENT"} {
		env := ToolEnv(id, "/bin/enowx", "/home/u")
		if len(env) != 1 || !strings.HasPrefix(env[0], key+"=") {
			t.Fatalf("%s: %q", id, env)
		}
		var cfg struct {
			Instructions []string
			MCP          map[string]struct {
				Type    string
				Command []string
			} `json:"mcp"`
		}
		if err := json.Unmarshal([]byte(strings.TrimPrefix(env[0], key+"=")), &cfg); err != nil {
			t.Fatal(err)
		}
		if !slices.Contains(cfg.Instructions, filepath.Join("/home/u", ".enowx-app", "agent-guide.md")) ||
			cfg.MCP["enowx"].Type != "local" || !slices.Equal(cfg.MCP["enowx"].Command, []string{"/bin/enowx", "mcp"}) {
			t.Fatalf("%s: %+v", id, cfg)
		}
		home := t.TempDir()
		if err := PlaceGuide(id, home); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(home, ".enowx-app", "agent-guide.md")); err != nil {
			t.Errorf("%s: the file its config names: %v", id, err)
		}
		if _, err := os.Stat(filepath.Join(home, ".config")); err == nil {
			t.Errorf("%s: wrote under the user's .config", id)
		}
	}
	if env := ToolEnv("claude", "/bin/enowx", "/home/u"); env != nil {
		t.Errorf("claude: %q", env)
	}
}

// The docs' table lists every agent, none of them claimed as checked.
func TestDocsListWhatEachAgentGets(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "docs", "agents.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	_, table, ok := strings.Cut(text, "### What each agent gets")
	if !ok {
		t.Fatal("no table")
	}
	table, _, _ = strings.Cut(table, "## The model a session runs on")
	for _, p := range Presets {
		if p.ID == Custom || p.ID == Shell {
			continue
		}
		if !strings.Contains(table, "| "+p.Name+" |") {
			t.Errorf("%s isn't in the table", p.Name)
		}
	}
}

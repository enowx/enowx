package agent

import (
	"encoding/json"
	"path/filepath"
)

// enowX's tools are "$ENOWX_BIN" commands, which every agent that runs a shell
// has by the guide. Those whose documentation gives a way to add an MCP server
// to one session get them as MCP tools too, never by writing the user's own
// settings. docs/agents.md says where each comes from.

// toolServer is enowX's MCP server, as the agents' mcpServers JSON has it.
func toolServer(exe string) map[string]any {
	return map[string]any{"command": exe, "args": []string{"mcp"}}
}

// ToolArgs are the arguments that give agent id enowX's MCP tools for this
// session, nil when its command line has no way. They go before the guide's: a
// list of Claude Code's takes several values, and a flag after it ends it.
func ToolArgs(id, exe string) []string {
	cfg := func(server map[string]any) string {
		b, _ := json.Marshal(map[string]any{"mcpServers": map[string]any{"enowx": server}})
		return string(b)
	}
	switch id {
	case "claude", "claude-agent-teams", "openclaude":
		return []string{"--mcp-config", cfg(toolServer(exe))}
	case "kimi":
		return []string{"--mcp-config", cfg(toolServer(exe))}
	case "copilot":
		server := toolServer(exe)
		server["tools"] = []string{"*"}
		return []string{"--additional-mcp-config=" + cfg(server)}
	case "codex":
		return []string{
			"-c", "mcp_servers.enowx.command=" + marshal(exe),
			"-c", `mcp_servers.enowx.args=["mcp"]`,
			"-c", `mcp_servers.enowx.env_vars=["ENOWX_SESSION","ENOWX_HOME","ENOWX_BIN"]`,
		}
	}
	return nil
}

// ToolEnv is what agent id's environment adds for the same, for an agent that
// takes this session's configuration there. It also carries the guide of OpenCode
// and Kilo Code, so that no file of the user's has to.
func ToolEnv(id, exe, home string) []string {
	key := map[string]string{"opencode": "OPENCODE_CONFIG_CONTENT", "kilo": "KILO_CONFIG_CONTENT"}[id]
	if key == "" {
		return nil
	}
	b, _ := json.Marshal(map[string]any{
		"instructions": []string{filepath.Join(home, ".enowx-app", "agent-guide.md")},
		"mcp": map[string]any{"enowx": map[string]any{
			"type": "local", "command": []string{exe, "mcp"}, "enabled": true,
		}},
	})
	return []string{key + "=" + string(b)}
}

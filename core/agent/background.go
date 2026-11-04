package agent

import (
	"path/filepath"
	"slices"
	"strings"
)

// interpreters run an agent whose program is their first argument that isn't a flag.
var interpreters = []string{"node", "nodejs", "bun", "deno", "python", "python3", "uv", "pipx", "sh", "bash", "dash"}

// ProgramOf is the agent a process runs as, from its arguments, "" for what no preset names: a
// dev server, a command. An agent's own MCP server isn't another agent at work, though it runs
// the same program.
func ProgramOf(args []string) string {
	if len(args) == 0 {
		return ""
	}
	name := programName(args[0])
	rest := args[1:]
	if slices.Contains(interpreters, name) {
		i := slices.IndexFunc(rest, func(a string) bool { return !strings.HasPrefix(a, "-") })
		if i < 0 {
			return ""
		}
		name, rest = programName(rest[i]), rest[i+1:]
	}
	// Its first word only: Codex's own session carries -c mcp_servers.enowx.* among its flags.
	if !isAgentProgram(name) || len(rest) > 0 && strings.HasPrefix(rest[0], "mcp") {
		return ""
	}
	return name
}

// programName is a program's name without its folder and its extension.
func programName(path string) string {
	name := filepath.Base(strings.ReplaceAll(path, `\`, "/"))
	for _, ext := range []string{".exe", ".cmd", ".js", ".mjs", ".cjs", ".ts", ".py"} {
		name = strings.TrimSuffix(name, ext)
	}
	return name
}

func isAgentProgram(name string) bool {
	if name == "" || name == Shell {
		return false
	}
	for _, p := range Presets {
		if command, _, _ := strings.Cut(p.Command, " "); command == name || slices.Contains(p.Aliases, name) {
			return true
		}
	}
	return false
}

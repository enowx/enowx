package goa

import (
	"path/filepath"
	"strings"
)

// metaTools are an agent's own tools that do no work: loading enowX's
// tools, keeping a to-do list.
var metaTools = map[string]bool{"ToolSearch": true, "TodoWrite": true, "ListMcpResourcesTool": true, "ReadMcpResourceTool": true}

// Allowed says whether GOA may make a tool call: one of enowX's MCP tools,
// or a shell command that runs enowX's program (bin, or "$ENOWX_BIN") and
// nothing else. why says what was refused.
func Allowed(tool, command, bin string) (ok bool, why string) {
	switch {
	case tool == "mcp__enowx" || strings.HasPrefix(tool, "mcp__enowx__"):
		return true, ""
	case metaTools[tool]:
		return true, ""
	case command != "" && isShellTool(tool):
		if EnowxCommand(command, bin) {
			return true, ""
		}
		return false, "GOD of Agents runs enowX's commands only (\"$ENOWX_BIN\" …, one command, nothing chained): hand this to a session with goa delegate"
	}
	return false, "GOD of Agents never does the work itself: " + tool + " is refused. Hand it to a session (goa delegate), or use enowX's tools"
}

// isShellTool says whether tool runs a shell command, by the names agents
// give theirs.
func isShellTool(tool string) bool {
	switch strings.ToLower(tool) {
	case "bash", "shell", "exec_command", "local_shell", "container.exec", "run_shell_command", "terminal":
		return true
	}
	return false
}

// EnowxCommand says whether command runs enowX's program and nothing else:
// its first word "$ENOWX_BIN", ${ENOWX_BIN}, $ENOWX_BIN or bin itself,
// and no other command chained, piped, substituted or redirected outside
// quotes. Inside double quotes, $( and backquotes still run commands, so
// they are refused there too.
func EnowxCommand(command, bin string) bool {
	command = strings.TrimSpace(command)
	first, rest := firstWord(command)
	switch first {
	case `"$ENOWX_BIN"`, `"${ENOWX_BIN}"`, `$ENOWX_BIN`, `${ENOWX_BIN}`:
	default:
		unq := strings.Trim(first, `"'`)
		if bin == "" || filepath.Clean(unq) != filepath.Clean(bin) {
			return false
		}
	}
	return plainArgs(rest)
}

// firstWord splits command at its first unquoted space.
func firstWord(command string) (string, string) {
	quote := byte(0)
	for i := 0; i < len(command); i++ {
		c := command[i]
		switch {
		case quote != 0 && c == quote:
			quote = 0
		case quote == 0 && (c == '"' || c == '\''):
			quote = c
		case quote == 0 && (c == ' ' || c == '\t'):
			return command[:i], command[i:]
		}
	}
	return command, ""
}

// plainArgs says whether args are only words: no ; & | < > newline,
// backquote or $( outside single quotes, no backquote or $( inside
// double ones.
func plainArgs(args string) bool {
	quote := byte(0)
	for i := 0; i < len(args); i++ {
		c := args[i]
		switch quote {
		case '\'':
			if c == '\'' {
				quote = 0
			}
			continue
		case '"':
			switch {
			case c == '\\' && i+1 < len(args):
				i++
			case c == '"':
				quote = 0
			case c == '`' || c == '$' && i+1 < len(args) && args[i+1] == '(':
				return false
			}
			continue
		}
		switch {
		case c == '\\' && i+1 < len(args) && args[i+1] != '\n':
			i++
		case c == '\'' || c == '"':
			quote = c
		case strings.IndexByte(";&|<>\n`", c) >= 0:
			return false
		case c == '$' && i+1 < len(args) && args[i+1] == '(':
			return false
		}
	}
	return quote == 0
}

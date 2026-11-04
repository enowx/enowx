package goa

import (
	"path/filepath"
	"regexp"
	"strings"
)

// metaTools are an agent's own tools that do no work: loading enowX's
// tools, keeping a to-do list.
var metaTools = map[string]bool{"ToolSearch": true, "TodoWrite": true, "ListMcpResourcesTool": true, "ReadMcpResourceTool": true}

// readTools and editTools are agents' tools for a file: GOA reads a
// project's context and writes what is small itself, and hands the rest
// over. An edit is the user's to allow, as their agent asks.
var (
	readTools = map[string]bool{"Read": true, "Grep": true, "Glob": true, "LS": true, "NotebookRead": true}
	editTools = map[string]bool{"Edit": true, "Write": true, "MultiEdit": true, "NotebookEdit": true, "apply_patch": true}
)

// Allowed says whether GOA may make a tool call: one of enowX's MCP tools,
// a tool reading or editing a file, or a shell command that runs enowX's
// program (bin, or "$ENOWX_BIN") or only reads, and nothing else. why says
// what was refused.
func Allowed(tool, command, bin string) (ok bool, why string) {
	switch {
	case tool == "mcp__enowx" || strings.HasPrefix(tool, "mcp__enowx__"):
		return true, ""
	case metaTools[tool], readTools[tool], editTools[tool]:
		return true, ""
	case command != "" && isShellTool(tool):
		if EnowxCommand(command, bin) || readCommand(command) {
			return true, ""
		}
		return false, "GOD of Agents runs enowX's commands and ones that only read (\"$ENOWX_BIN\" …, cat, rg …, one command, nothing chained, no secrets): hand this to a session with goa delegate"
	}
	return false, "GOD of Agents reads and edits files and uses enowX's tools; " + tool + " is refused. Hand this to a session (goa delegate)"
}

// agentSettings are what an agent loads from a folder when it starts there: hooks and
// plugins run as programs, and they could give it back what GOA lacks.
var agentSettings = map[string]bool{".claude": true, ".codex": true, ".opencode": true, ".mcp.json": true, "opencode.json": true, "opencode.jsonc": true}

// EditInside says whether tool, if it edits, stays in GOA's folder dir: its files only,
// through whatever links lead there, and not its agent's settings. Paths are relative to
// dir or absolute. Codex's apply_patch names none: its sandbox holds it.
func EditInside(tool string, paths []string, dir string) (ok bool, why string) {
	if !editTools[tool] || tool == "apply_patch" && len(paths) == 0 {
		return true, ""
	}
	refuse := "GOD of Agents edits only in its own folder, and not its agent's settings there: hand this to a session (goa delegate)"
	if len(paths) == 0 {
		return false, refuse
	}
	root := realPath(dir)
	for _, p := range paths {
		if !filepath.IsAbs(p) {
			p = filepath.Join(dir, p)
		}
		rel, err := filepath.Rel(root, realPath(p))
		if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return false, refuse
		}
		if first, _, _ := strings.Cut(filepath.ToSlash(rel), "/"); agentSettings[first] {
			return false, refuse
		}
	}
	return true, ""
}

// realPath is p with its links followed, as far as it exists.
func realPath(p string) string {
	p = filepath.Clean(p)
	if real, err := filepath.EvalSymlinks(p); err == nil {
		return real
	}
	parent := filepath.Dir(p)
	if parent == p {
		return p
	}
	return filepath.Join(realPath(parent), filepath.Base(p))
}

// ReadOnly says whether tool call only reads a file, so nothing is lost
// by running it unasked.
func ReadOnly(tool, command string) bool {
	return readTools[tool] || command != "" && isShellTool(tool) && readCommand(command)
}

// sedPrint is sed printing lines and nothing else: its other commands can
// write files and run programs.
var sedPrint = regexp.MustCompile(`^\s+-n\s+'?\d+(,\d+)?p'?\s`)

// readCommand says whether command is one program that only reads, for
// agents that read through a shell (Codex): nothing chained, no secret
// named, which a tool's permission prompt would catch but a sandboxed
// shell wouldn't.
func readCommand(command string) bool {
	command = strings.TrimSpace(command)
	first, rest := firstWord(command)
	switch first {
	case "cat", "head", "tail", "ls", "wc", "grep", "pwd", "stat":
	case "rg":
		if strings.Contains(rest, "--pre") {
			return false // runs a program on each file
		}
	case "sed":
		if !sedPrint.MatchString(rest) {
			return false
		}
	default:
		return false
	}
	return plainArgs(rest) && Urgent("", command, nil) == ""
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

package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// An agent the Canvas page talks to designs the canvas beside it: it runs
// in an empty folder, gets CanvasChatGuide and the canvas skill, enowX's
// canvas tools, and reads (only) the canvas's own folder and the projects
// the canvas is attached to.

// CanvasChatGuide is what such an agent is told, before the canvas skill.
const CanvasChatGuide = `You are the design partner in enowX's Canvas page: the user talks to you in a chat beside a canvas they watch, and every change you make to it shows there at once, an element at a time. You design with the canvas tools only (MCP tools canvas_*, or the command line "$ENOWX_BIN" canvas …); you don't write files or run programs. You may read files: the canvas's own folder (its images, and pictures the user attached to a message) and the projects the canvas belongs to, to match their design system and components; read them, never change them.

Each message starts with where the user is, in brackets: the canvas, the design shown (its number and name), its frame, and the element selected, if any. "This", "here" and "it" mean that. @design-3 (or @desain-3) names design 3, @#id an element of the design shown. A message may forward a comment the user left on an element: do what it asks, then resolve it (canvas_resolve).

Work in small, visible steps the user can follow: a skeleton, then each piece. When asked for several designs or variations, make each a design of its own (canvas_new_design), titled and described in a line. Look at what you made (canvas_screenshot) before saying it is done. Keep chat replies short: what you changed, and what you would try next. Answer in the user's language. The user may undo a message's changes; when a change is refused as paused, stop and say so.

`

// CanvasArgs are the arguments that start agent id as canvas cv's in the
// Canvas page: the guide and canvas skill in place of the Guide and, for
// Claude Code, only the canvas tools of enowX's MCP server (program exe,
// "mcp"), reading dirs (the canvas's folder and its projects), its tools
// for writing files and running commands turned off. Other agents get the
// guide alone.
func CanvasArgs(id, exe, cv string, dirs []string) []string {
	args := guideArgsWith(id, CanvasChatGuide+CanvasGuide+canvasOpen(cv), canvasGuideFile)
	switch id {
	case "claude", "claude-agent-teams", "openclaude":
		cfg, _ := json.Marshal(map[string]any{"mcpServers": map[string]any{
			"enowx": map[string]any{"command": exe, "args": []string{"mcp"}, "env": map[string]string{"ENOWX_CANVAS_CHAT": "1"}},
		}})
		if len(dirs) > 0 {
			args = append(append(args, "--add-dir"), dirs...)
		}
		// Their lists take several values: the flag of none last, so the
		// prompt after it isn't read as one.
		args = append(args, "--mcp-config", string(cfg),
			"--allowedTools", strings.Join([]string{"mcp__enowx", "Read", "Glob", "Grep", "WebSearch", "WebFetch"}, ","),
			"--disallowedTools", strings.Join([]string{"Bash", "Edit", "Write", "NotebookEdit", "Task"}, ","),
			"--strict-mcp-config")
	case PlatformID:
		args = append(args, platformMCP(exe, map[string]string{"ENOWX_CANVAS_CHAT": "1"})...)
	}
	return args
}

// canvasOpen says which canvas the conversation is about.
func canvasOpen(cv string) string {
	if cv == "" {
		return ""
	}
	return "\n\nThe canvas beside you is " + cv + ": canvas_show and canvas_designs tell what it holds."
}

// canvasGuideFile is the guide and skill as a file, for agents that read
// one.
func canvasGuideFile() string {
	home, _ := os.UserHomeDir()
	path := filepath.Join(home, ".enowx-app", "canvas-guide.md")
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	_ = os.WriteFile(path, []byte(CanvasChatGuide+CanvasGuide+"\n"), 0o644)
	return path
}

// CanvasNote is the line a Canvas page agent's first prompt starts with
// when its command line takes no guide: where its guide is.
func CanvasNote(id, cv string) string {
	if len(guideArgsWith(id, CanvasChatGuide, func() string { return "" })) > 0 {
		return ""
	}
	return "(You design the canvas beside the user in enowX's Canvas page. Read your guide first: " + canvasGuideFile() + "." + canvasOpen(cv) + ")"
}

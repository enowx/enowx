package agent

import (
	"encoding/json"

	"github.com/enowx/enowx/proto"
)

// Claude Platform is enowX's own agent (core/platformagent, docs/claude-platform.md): the
// enowX program itself, run as "<program> claude-platform", so it needs nothing
// installed on any host, Windows and servers included. Its flags are named as Claude
// Code's. It tells the daemon of itself over session.hook rather than through hooks in
// its settings, so HookArgs is nil for it.

// PlatformID is its preset's ID (record.AgentID).
const PlatformID = "claude-platform"

// IsBuiltin says agent id is a part of enowX's program rather than one installed.
func IsBuiltin(id string) bool {
	p, ok := Find(id)
	return ok && p.Builtin
}

// ReportsItself says agent id tells enowX its state and turns as they happen, by hooks
// in its command line or, built in, directly. One that doesn't is only watched from
// outside, and what it asks may wait unseen.
func ReportsItself(id string) bool { return len(HookArgs(id)) > 0 || IsBuiltin(id) }

// platformSettings are what its terminal front takes: the commands below, Shift+Tab
// through its modes, each named on its status line.
var platformSettings = Settings{
	Models: []Choice{
		{ID: "claude-opus-5-5", Label: "Opus 5.5", Note: "the default: complex work and everyday tasks", Window: 1_000_000},
		{ID: "claude-fable-5-1", Label: "Fable 5.1", Note: "your toughest challenges", Window: 1_000_000},
		{ID: "claude-sonnet-5-5", Label: "Sonnet 5.5", Note: "efficient for simpler tasks", Window: 1_000_000},
		{ID: "claude-haiku-5-5", Label: "Haiku 5.5", Note: "fastest for quick answers", Window: 1_000_000},
		{ID: "claude-opus-5", Label: "Opus 5", Note: "everyday, complex tasks", Window: 1_000_000},
		{ID: "claude-sonnet-5", Label: "Sonnet 5", Note: "efficient for routine tasks", Window: 1_000_000},
		{ID: "claude-haiku-4-5", Label: "Haiku 4.5", Note: "fast for quick answers", Window: 200_000},
	},
	ModelCommand: "/model %s",
	Modes: []Choice{
		{ID: "default", Label: "Ask first", Note: "asks before edits and commands", Shown: "ask first mode on"},
		{ID: "acceptEdits", Label: "Accept edits", Note: "edits files without asking", Shown: "accept edits on"},
		{ID: "plan", Label: "Plan", Note: "reads and plans, changes nothing", Shown: "plan mode on"},
		{ID: "bypassPermissions", Label: "Skip permissions", Note: "runs everything without asking", Shown: "bypass permissions on"},
	},
	ModeKey: "\x1b[Z", // Shift+Tab
	Efforts: []Choice{
		{ID: "low", Label: "Low"}, {ID: "medium", Label: "Medium"}, {ID: "high", Label: "High"},
		{ID: "xhigh", Label: "Extra high"}, {ID: "max", Label: "Max"},
	},
	EffortCommand: "/effort %s",
	ModelFlag:     []string{"--model", "%s"},
	EffortFlag:    []string{"--effort", "%s"},
	ModeFlag:      []string{"--permission-mode", "%s"},
}

// platformFileForms take its guide and enowX's tools in files of the session's folder.
var platformFileForms = []fileForm{
	{flag: "--append-system-prompt", to: "--append-system-prompt-file", name: "system-prompt.md"},
	{flag: "--mcp-config", to: "--mcp-config", name: "mcp.json", json: true},
}

// platformMCP gives it enowX's MCP server (program exe, "mcp") with env added to the
// server's environment.
func platformMCP(exe string, env map[string]string) []string {
	server := toolServer(exe)
	if len(env) > 0 {
		server["env"] = env
	}
	cfg, _ := json.Marshal(map[string]any{"mcpServers": map[string]any{"enowx": server}})
	return []string{"--mcp-config", string(cfg)}
}

// platformCommands are the commands of its terminal front.
func platformCommands() []proto.AgentCommand {
	list := [][2]string{
		{"model", "choose the model"},
		{"effort", "set the effort: low, medium, high, xhigh, max"},
		{"mode", "choose the permission mode"},
		{"budget", "set what this session may spend before asking"},
		{"cost", "show what this session has cost"},
		{"context", "show how much of the model's context is used"},
		{"compact", "summarize the conversation to free context"},
		{"clear", "start a new conversation"},
		{"system-prompt", "use a file as this session's system prompt"},
		{"help", "list the commands and keys"},
		{"exit", "leave Claude Platform"},
	}
	out := make([]proto.AgentCommand, len(list))
	for i, c := range list {
		out[i] = proto.AgentCommand{Name: c[0], Description: c[1], Kind: "built-in"}
	}
	return out
}

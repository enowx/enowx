package agent

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/enowx/enowx/proto"
)

// GOD of Agents (docs/goa.md): a session that controls every session on
// every host and only delegates. It runs in a folder of its own, gets
// GOAGuide before the guide of every feature, and enowX's MCP server; the
// agents that can are started with enowX's tools only.

// GOAGuide is what GOA is told, before the features' guide.
const GOAGuide = `You are GOD of Agents (GOA) in enowX: the one session the user drives, controlling every enowX session on every host (this one, servers, teammates' shared hosts). You don't do the coding yourself: changing code, running builds or tests and long digging go to a session that does it, and you follow, check and report. You may read files to learn a project's context, and write or edit a file yourself when what is asked is small and no session is needed (a plan, a note, a doc, a line of config); an edit goes by your session's permission mode. Your tools are enowX's (its MCP tools, mcp__enowx__*, or "$ENOWX_BIN" commands) and your agent's for reading and editing files, a shell only for commands that read (cat, head, tail, ls, wc, grep, rg, sed -n 'Np'), one at a time, never a secret; any other tool is refused. Starting, stopping and switching sessions, opening projects, reviewing and merging their work, changing settings: that is control, and yours to do.

How you work:
1. Get what the user wants. "$ENOWX_BIN" goa recall "…" finds what you kept: past tasks, outcomes, decisions, the user's preferences; --project P (--host H) searches a project, read-only.
2. Split it into tasks, and hand each to a session: an existing one ("$ENOWX_BIN" sessions lists every host's) or a new one:
    "$ENOWX_BIN" goa delegate --agent claude --project ~/code/shop --branch fix/totals --title "Fix totals" "Fix the rounding of invoice totals; add a test"
    "$ENOWX_BIN" goa delegate --session ID --host H "Now update the changelog"
    "$ENOWX_BIN" goa delegate --agent claude --project ~/code/shop "Add a test for the totals"
   Talk to the agent that knows the project: with --project and no --session or --branch, the project's own session takes it, even busy, asking, hibernated or started with other settings. A busy one has it queued (the board says queued), delivered when its turn ends; a stopped one is brought back first. A new session starts only when the project has none, or when the user asks: for a fork, or --branch, for parallel work they agreed to, or a --config they named. delegate says when one started, and why. If a session can't take it, delegate fails: don't work around it with a new one.
   Never your own session: it is refused. With no other session, start a new one.
   The judge picks each prompt's model and effort (cheaper near a budget): delegate prints them and why; tell the user in a line. When the user names a model, pass --model M --effort E (--config C for another provider); --no-route keeps the agent's own.
3. Don't poll. When a worker finishes, asks something or fails, enowX types a line starting with "[enowX GOA]" into this conversation. Then read what it did ("$ENOWX_BIN" chat ID --host H, changes ID --host H), check it against the task, and go on: the next task, a fix sent back, a review and merge (the user confirms), or a word to the user.
4. Workers' permission prompts: the judge answers what it is sure of. The rest you answer after reading what is asked ("$ENOWX_BIN" answer ID --host H, then answer ID allow|deny --host H). Urgent ones (what can't be undone, money, publishing, secrets, the judge unsure) are the user's: they get a sound and a banner; don't answer those, say they wait on the user.
5. A turn's end isn't a task's: "turn-ended" on the board means the worker stopped, maybe mid-work. Only you settle it (goa task ID --status done|failed --result "its report"), and that report is what's remembered. Keep the board true: "$ENOWX_BIN" goa board shows it to you as the user sees it; goa task ID --status done|failed|stopped --result "…" settles one.
6. Keep what is worth it: "$ENOWX_BIN" goa remember --kind decision|preference|outcome "…".
There is no limit on workers; near a budget ("$ENOWX_BIN" usage budgets), fewer and cheaper ones. Answer the user in their language, briefly: what you started, where it stands, what needs them.

`

// GOAClaudeTools are Claude Code's own tools GOA starts with.
const GOAClaudeTools = "Read,Grep,Glob,Edit,Write,NotebookEdit"

// GOALimitsTools says whether agent id starts as GOA with enowX's tools
// only (GOAArgs).
func GOALimitsTools(id string) bool {
	switch id {
	case "claude", "claude-agent-teams", "openclaude":
		return true
	}
	return false
}

// GOAArgs are the arguments that start agent id as GOA: guide in place of
// the Guide (GOAGuide and the features'), enowX's MCP server (program
// exe, "mcp") and, for Claude Code, only its tools for files; Codex runs its
// commands read-only. Other agents get the guide alone.
func GOAArgs(id, exe, guide string) []string {
	args := guideArgsWith(id, guide, goaGuideFile(guide))
	server := map[string]any{"command": exe, "args": []string{"mcp"}, "env": map[string]string{proto.EnvGOA: "1"}}
	switch id {
	case "claude", "claude-agent-teams", "openclaude":
		cfg, _ := json.Marshal(map[string]any{"mcpServers": map[string]any{"enowx": server}})
		// Its tools for files and enowX's MCP tools, the "enowx" one running
		// any command, are all it has. The flag of none last, so the prompt
		// after the lists isn't read as one of their values.
		args = append(args, "--tools", GOAClaudeTools, "--mcp-config", string(cfg),
			"--allowedTools", "mcp__enowx", "--strict-mcp-config")
	case "codex":
		args = append(args, "--sandbox", "workspace-write",
			"-c", "mcp_servers.enowx.command="+marshal(exe),
			"-c", `mcp_servers.enowx.args=["mcp"]`,
			"-c", `mcp_servers.enowx.env={ENOWX_GOA="1"}`,
			"-c", `mcp_servers.enowx.env_vars=["ENOWX_SESSION","ENOWX_HOME","ENOWX_BIN"]`,
			"-c", `mcp_servers.enowx.default_tools_approval_mode="approve"`)
	}
	return args
}

// GOAEnv is what GOA's session of agent id gets in its environment: Claude
// Code loads every MCP tool up front, as it has no tool to search them.
func GOAEnv(id string) []string {
	if GOALimitsTools(id) {
		return []string{"ENABLE_TOOL_SEARCH=false", proto.EnvGOA + "=1"}
	}
	return []string{proto.EnvGOA + "=1"}
}

// goaGuideFile writes guide for the agents that read a file, and is where.
func goaGuideFile(guide string) func() string {
	return func() string {
		home, _ := os.UserHomeDir()
		path := filepath.Join(home, ".enowx-app", "goa-guide.md")
		_ = os.MkdirAll(filepath.Dir(path), 0o755)
		_ = os.WriteFile(path, []byte(guide+"\n"), 0o644)
		return path
	}
}

// GOANote is the line GOA's first prompt starts with when agent id's
// command line takes no guide: where its guide is.
func GOANote(id, guide string) string {
	if len(guideArgsWith(id, guide, func() string { return "" })) > 0 {
		return ""
	}
	return "(You are GOD of Agents in enowX: you only delegate, never do the work. Read your guide first: " + goaGuideFile(guide)() + ".)"
}

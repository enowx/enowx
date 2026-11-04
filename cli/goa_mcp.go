package cli

import "slices"

// goaTools are GOD of Agents' commands as MCP tools (feature "goa").
var goaTools = []mcpTool{
	{name: "goa_skill", description: "A built-in guide of GOD of Agents: setup (doing every setting of enowX with the user, stopping for what needs them), features (every feature).",
		props:    map[string]mcpProp{"name": {"string", "setup or features"}},
		required: []string{"name"},
		args:     func(a map[string]any) []string { return []string{"skill", str(a, "name")} }},
	{name: "goa_doc", description: "enowX's docs, shipped in the program: with no name, each doc and a line on what it covers; with a name, that doc; with a section too, that part of it; with search, which docs and sections hold those words. Read this to answer a question about a feature, not the code or memory.",
		props: map[string]mcpProp{"name": {"string", "A doc's name, as the list gives it"}, "section": {"string", "A heading of the doc, or a word of one; a wrong one lists the headings"},
			"search": {"string", "Words to find in every doc, instead of a name"}},
		args: func(a map[string]any) []string {
			args := []string{"doc"}
			if name := str(a, "name"); name != "" {
				args = append(args, name)
			}
			return withFlag(withFlag(args, a, "section", "section"), a, "search", "search")
		}},
	{name: "goa_board", description: "GOD of Agents' task board as JSON: each task, the session (and host) it went to, its model and effort and why, its status (working, waiting, stalled, queued, turn-ended, done, failed, stopped), its result, what it asks and whether that is urgent (the user's to answer).",
		args: func(map[string]any) []string { return []string{"board", "--json"} }},
	{name: "goa_delegate", description: "Hand a task to a session and put it on the board: an existing session (session, host) or a new one (agent, project, branch, host). The judge picks the prompt's model and effort (cheaper near a token budget) unless model/effort/config are given or no_route is true. Returns the task as JSON with model, effort and why: tell the user. You are told when it finishes or asks something; don't poll.",
		props: map[string]mcpProp{"prompt": {"string", "The task, as the worker should read it"}, "title": {"string", "Its title on the board"},
			"session": {"string", "An existing session to send it to"}, "agent": {"string", "The agent of the session (claude, codex, …)"},
			"project": {"string", "The project (a path on its host): its idle or stopped session takes the task, else a new one"}, "branch": {"string", "A new branch in a worktree of its own"},
			"host":  {"string", "The host, as the app names it (sessions_everywhere lists them); this one when left out"},
			"model": {"string", "A model chosen by hand"}, "effort": {"string", "An effort chosen by hand"}, "config": {"string", "A config of the agent's (another provider)"},
			"no_route": {"boolean", "Keep the agent's own model and effort"},
			"note":     {"string", "The note the task belongs to"}, "items": {"string", "Its to-dos the task finishes, one per line: the worker is told to tick them, and settling it done says which aren't"}},
		required: []string{"prompt"},
		args: func(a map[string]any) []string {
			args := []string{"delegate", "--json"}
			for _, k := range []string{"note", "items", "title", "session", "agent", "project", "branch", "host", "model", "effort", "config"} {
				args = withFlag(args, a, k, k)
			}
			return append(withFlag(args, a, "no_route", "no-route"), "--", str(a, "prompt"))
		}},
	{name: "goa_task", description: "Settle a task on the board, or say where it stands: its status (working, waiting, done, failed, stopped) and result.",
		props:    map[string]mcpProp{"id": {"string", "The task's ID"}, "status": {"string", "working, waiting, done, failed or stopped"}, "result": {"string", "What came of it, in a line or two"}},
		required: []string{"id"},
		args: func(a map[string]any) []string {
			return withFlag(withFlag([]string{"task", str(a, "id")}, a, "status", "status"), a, "result", "result")
		}},
	{name: "goa_remember", description: "Keep something in GOD of Agents' own RAG collection (never a project's): a task and who did it, a delegation, an outcome, a decision, the user's preference.",
		props:    map[string]mcpProp{"text": {"string", "What to keep, in a sentence or two"}, "kind": {"string", "task, delegation, outcome, decision (default) or preference"}},
		required: []string{"text"},
		args: func(a map[string]any) []string {
			return append(withFlag([]string{"remember"}, a, "kind", "kind"), "--", str(a, "text"))
		}},
	{name: "goa_recall", description: "Search GOD of Agents' own collection (past tasks, outcomes, decisions, the user's preferences) and, read-only, a project's (project, and its host).",
		props:    map[string]mcpProp{"query": {"string", "What you look for, in words"}, "project": {"string", "A project to search too, read-only"}, "host": {"string", "The project's host (this one when left out)"}},
		required: []string{"query"},
		args: func(a map[string]any) []string {
			return append(withFlag(withFlag([]string{"recall", "--json"}, a, "project", "project"), a, "host", "host"), "--", str(a, "query"))
		}},
	{name: "goa_stall", description: "How long a task on GOD of Agents' board may not move, on this host, before it is told and the task shows as stalled; a limit given changes it. A time such as 90s, 10m or 1h, or 0 for its default.",
		props: map[string]mcpProp{"blank": {"string", "A screen still empty after the task (default 1m)"}, "idle": {"string", "Its session hibernated or gone, or its agent not at work (default 2m)"},
			"stuck": {"string", "Its agent says it works, but nothing shows or uses the CPU (default 10m)"}},
		args: func(a map[string]any) []string {
			args := []string{"stall"}
			for _, k := range []string{"blank", "idle", "stuck"} {
				args = withFlag(args, a, k, k)
			}
			return args
		}},
	{name: "goa_agents", description: "The agents GOD of Agents can run on here, and how far each holds it to delegating: enforced, limited or watched.",
		args: func(map[string]any) []string { return []string{"agents"} }},
}

// goaOfferedTools are the tools GOD of Agents' server offers: every tool offered goes
// with each request, 222 of them at about 40,000 tokens each time against 3,000 for 6.
// Its work is delegating; its "enowx" tool runs every other command.
var goaOfferedTools = []string{"goa_agents", "goa_delegate", "goa_board", "goa_task", "goa_recall", "goa_remember",
	"sessions_everywhere", "session_read", "session_chat", "session_answer", "enowx"}

// goaOffered is groups with the tools offered GOD of Agents only.
func goaOffered(groups []mcpGroup) []mcpGroup {
	out := make([]mcpGroup, 0, len(groups))
	for _, g := range groups {
		g.tools = slices.DeleteFunc(slices.Clone(g.tools), func(t mcpTool) bool { return !slices.Contains(goaOfferedTools, t.name) })
		if len(g.tools) > 0 {
			out = append(out, g)
		}
	}
	return out
}

package cli

// goaTools are GOD of Agents' commands as MCP tools (feature "goa").
var goaTools = []mcpTool{
	{name: "goa_board", description: "GOD of Agents' task board as JSON: each task, the session (and host) it went to, its model and effort and why, its status (working, waiting, done, failed, stopped), its result, what it asks and whether that is urgent (the user's to answer).",
		args: func(map[string]any) []string { return []string{"board", "--json"} }},
	{name: "goa_delegate", description: "Hand a task to a session and put it on the board: an existing session (session, host) or a new one (agent, project, branch, host). The judge picks the prompt's model and effort (cheaper near a token budget) unless model/effort/config are given or no_route is true. Returns the task as JSON with model, effort and why: tell the user. You are told when it finishes or asks something; don't poll.",
		props: map[string]mcpProp{"prompt": {"string", "The task, as the worker should read it"}, "title": {"string", "Its title on the board"},
			"session": {"string", "An existing session to send it to"}, "agent": {"string", "The agent of a new session (claude, codex, …)"},
			"project": {"string", "The project of a new session (a path on its host)"}, "branch": {"string", "A new branch in a worktree of its own"},
			"host":  {"string", "The host, as the app names it (sessions_everywhere lists them); this one when left out"},
			"model": {"string", "A model chosen by hand"}, "effort": {"string", "An effort chosen by hand"}, "config": {"string", "A config of the agent's (another provider)"},
			"no_route": {"boolean", "Keep the agent's own model and effort"},
			"doc":      {"string", "A Project Docs document, NOTE or NOTE#SECTION: refused while its project's gate is closed, its words prepended to the prompt"}},
		required: []string{"prompt"},
		args: func(a map[string]any) []string {
			args := []string{"delegate", "--json"}
			for _, k := range []string{"title", "session", "agent", "project", "branch", "host", "model", "effort", "config", "doc"} {
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
	{name: "goa_agents", description: "The agents GOD of Agents can run on here, and how far each holds it to delegating: enforced, limited or watched.",
		args: func(map[string]any) []string { return []string{"agents"} }},
}

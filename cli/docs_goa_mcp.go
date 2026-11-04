package cli

// docsGOATools are a project's plan, progress and checks as MCP tools.
var docsGOATools = []mcpTool{
	{name: "docs_plan", description: "A Project Docs project's plan as JSON: the Work Breakdown's tasks, their dependencies, which board tasks carry them, which are ready and the exact goa delegate command for each. It delegates nothing.",
		props:    map[string]mcpProp{"project": {"string", "The project's note, or any document of it: its ID or title"}},
		required: []string{"project"},
		args:     func(a map[string]any) []string { return []string{"plan", "--json", "--", str(a, "project")} }},
	{name: "docs_progress", description: "A Project Docs project's progress as JSON: documents by status, what its gate still waits on, tasks planned and done, its tasks on the board, acceptance criteria done.",
		props:    map[string]mcpProp{"project": {"string", "The project's note, or any document of it: its ID or title"}},
		required: []string{"project"},
		args:     func(a map[string]any) []string { return []string{"progress", "--json", "--", str(a, "project")} }},
	{name: "docs_verify", description: "A board task's result beside the acceptance criteria that apply to it, as JSON, and whether it is of this project at all. It ticks nothing: tick a criterion yourself with notes_check, only with evidence you have seen, and name what remains.",
		props:    map[string]mcpProp{"project": {"string", "The project's note, or any document of it: its ID or title"}, "task": {"string", "The board task's ID"}},
		required: []string{"project", "task"},
		args: func(a map[string]any) []string {
			return []string{"verify", "--json", "--task", str(a, "task"), "--", str(a, "project")}
		}},
}

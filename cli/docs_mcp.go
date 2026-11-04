package cli

// docsTools are Project Docs' commands as MCP tools. Approving, and changing
// whether a project needs it, aren't among them: those are the owner's.
var docsTools = []mcpTool{
	{name: "docs_new", description: "Make a project documented as notes: a project note with its documents under it, from templates. Answers its ID. size core is the documents every project needs, full is every one; a session's project is attached by itself.",
		props: map[string]mcpProp{"title": {"string", "The project's title"}, "size": {"string", "core (default) or full"}, "approval": {"string", "required (default) or off: whether PRD and Technical Spec must be approved before building"},
			"with": {"string", "More documents, catalog keys separated by commas"}, "template": {"string", "A template of the user's to add"}, "group": {"string", "The group it goes in"},
			"team": {"string", "A team's space"}, "project": {"string", "A project to attach every note to"}, "host": {"string", "That project's host, when names are the same on several"}},
		required: []string{"title"},
		args: func(a map[string]any) []string {
			args := []string{"new", str(a, "title")}
			for _, k := range []string{"size", "approval", "with", "template", "group", "team", "project", "host"} {
				args = withFlag(args, a, k, k)
			}
			return args
		}},
	{name: "docs_list", description: "The projects documented as notes: ID, title, size, how many documents, whether approval is required.",
		args: func(map[string]any) []string { return []string{"list"} }},
	{name: "docs_summary", description: "A project at a glance, as JSON: each document's status, version, to-dos, stale links and unfilled sections, its open questions, and what stops it being built.",
		props: map[string]mcpProp{"project": {"string", "The project, or one of its documents: ID or title"}}, required: []string{"project"},
		args: func(a map[string]any) []string { return []string{"summary", str(a, "project"), "--json"} }},
	{name: "docs_catalog", description: "Every document there is a template for, as JSON: key, name, area, whether it's in the core pack, and what kind of project needs it.",
		args: func(map[string]any) []string { return []string{"catalog", "--json"} }},
	{name: "docs_add", description: "Add a document to a project, from the catalog (docs_catalog's key) or a template of the user's. Answers its ID.",
		props: map[string]mcpProp{"project": {"string", "The project: ID or title"}, "key": {"string", "The catalog's key, or the template's name"}}, required: []string{"project", "key"},
		args: func(a map[string]any) []string { return []string{"add", str(a, "project"), str(a, "key")} }},
	{name: "docs_skip", description: "Record that a project doesn't need a document of the catalog, and why.",
		props: map[string]mcpProp{"project": {"string", "The project: ID or title"}, "key": {"string", "The catalog's key"}, "why": {"string", "Why it doesn't apply"}}, required: []string{"project", "key", "why"},
		args: func(a map[string]any) []string {
			return []string{"skip", str(a, "project"), str(a, "key"), "--why", str(a, "why")}
		}},
	{name: "docs_status", description: "A document's status and version, or with status, set it to Draft, In review or Outdated. Only the owner approves; leaving Approved is proposed to them.",
		props: map[string]mcpProp{"doc": {"string", "The document: ID or title, or its kind with in_project"}, "status": {"string", "Draft, In review or Outdated"}, "in_project": {"string", "The project it's in, when doc is its kind"}}, required: []string{"doc"},
		args: func(a map[string]any) []string {
			args := []string{"status", str(a, "doc")}
			if s := str(a, "status"); s != "" {
				args = append(args, s)
			}
			return withFlag(args, a, "in_project", "in-project")
		}},
	{name: "docs_link", description: "Say a document is based on another (or one section of it) as it is now. Called again after reviewing a change, it re-confirms the link, so it stops being stale.",
		props: map[string]mcpProp{"doc": {"string", "The document: ID or title, or its kind with in_project"}, "to": {"string", "The source: a document of the project, with #Section for one section"},
			"in": {"string", "The section of doc that is based on it (else the whole document)"}, "in_project": {"string", "The project it's in, when doc is its kind"}}, required: []string{"doc", "to"},
		args: func(a map[string]any) []string {
			return withFlag(withFlag([]string{"link", str(a, "doc"), "--to", str(a, "to")}, a, "in", "in"), a, "in_project", "in-project")
		}},
	{name: "docs_gaps", description: "What's left to write in a project, as JSON: the topics still unanswered, in the order to ask them, each with its question, document and section; then other sections to fill.",
		props: map[string]mcpProp{"project": {"string", "The project: ID or title"}}, required: []string{"project"},
		args: func(a map[string]any) []string { return []string{"gaps", str(a, "project"), "--json"} }},
	{name: "docs_topics", description: "Everything to settle about a project, in the order to ask: area, topic, document#section, question.",
		args: func(map[string]any) []string { return []string{"topics"} }},
	{name: "docs_gate", description: "Whether a project may be built: what still needs the owner's approval. An error while something does; don't start building then.",
		props: map[string]mcpProp{"project": {"string", "The project: ID or title"}}, required: []string{"project"},
		args: func(a map[string]any) []string { return []string{"gate", str(a, "project")} }},
	{name: "docs_attach", description: "Attach a project and every document to projects of any host.",
		props: map[string]mcpProp{"project": {"string", "The Project Docs project: ID or title"}, "to": {"array", "The projects: name, folder or . for this session's"}, "host": {"string", "Their host, when names are the same on several"}}, required: []string{"project", "to"},
		args: func(a map[string]any) []string {
			args := []string{"attach", str(a, "project")}
			if list, ok := a["to"].([]any); ok {
				for _, v := range list {
					if s, ok := v.(string); ok {
						args = append(args, "--project", s)
					}
				}
			}
			return withFlag(args, a, "host", "host")
		}},
	{name: "docs_diagram", description: "Embed a canvas that exists in a document, in place of the section's first To fill line (default section Diagram, or Canvases), with where it was drawn from.",
		props: map[string]mcpProp{"doc": {"string", "The document: ID or title, or its kind with in_project"}, "canvas": {"string", "The canvas's ID"}, "section": {"string", "The section"},
			"from": {"string", "What it was drawn from, as text"}, "in_project": {"string", "The project it's in, when doc is its kind"}}, required: []string{"doc", "canvas"},
		args: func(a map[string]any) []string {
			args := []string{"diagram", str(a, "doc"), "--canvas", str(a, "canvas")}
			for _, k := range []string{"section", "from"} {
				args = withFlag(args, a, k, k)
			}
			return withFlag(args, a, "in_project", "in-project")
		}},
	{name: "docs_export", description: "Write a project out: with to, into a folder of a repository as Markdown (the project note as README.md), refusing to write over anyone's files or to export what looks like a secret; or with format, as one file. Notes stay the source.",
		props: map[string]mcpProp{"project": {"string", "The project: ID or title"}, "to": {"string", "A folder, an absolute path"}, "force": {"boolean", "Write over files that aren't an export's, only when the user asked"},
			"dry_run": {"boolean", "Only say what it would write"}, "format": {"string", "md, zip, html, pdf or json, instead of to"}, "out": {"string", "The file or folder to write it to, with format"}}, required: []string{"project"},
		args: func(a map[string]any) []string {
			args := []string{"export", str(a, "project")}
			for _, k := range []string{"to", "force", "format", "out"} {
				args = withFlag(args, a, k, k)
			}
			return withFlag(args, a, "dry_run", "dry-run")
		}},
}

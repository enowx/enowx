package cli

import (
	"slices"

	"github.com/enowx/enowx/core/agent"
)

// docsSkillTools serve the skills. Installing them isn't a tool: it writes to
// the user's folders, on the user's explicit request.
var docsSkillTools = []mcpTool{
	{name: "docs_skill", description: "How to work on Project Docs: the list of skills with when to use each, or one skill's steps (project-brainstorm, write-prd, write-spec, plan-work, build-from-spec, review-against-prd, draw-diagram).",
		props: map[string]mcpProp{"name": {"string", "The skill; the list when left out"}},
		args: func(a map[string]any) []string {
			if name := str(a, "name"); name != "" {
				return []string{"skill", name}
			}
			return []string{"skill"}
		}},
}

// notesDocsGroups are the tools of groups the Notes page's agent gets of
// Project Docs: those that only read. By name, since which list holds a tool
// doesn't say whether it writes.
func notesDocsGroups(groups []mcpGroup) []mcpGroup {
	var out []mcpGroup
	for _, g := range groups {
		kept := slices.DeleteFunc(slices.Clone(g.tools), func(t mcpTool) bool {
			return !slices.Contains(agent.NotesDocsTools, t.name)
		})
		if len(kept) > 0 {
			out = append(out, mcpGroup{kept, g.run})
		}
	}
	return out
}

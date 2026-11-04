package ui

import (
	"path/filepath"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
)

// Facts a row of a session or a task shows: the model its agent says it runs
// on, and its project's name and path, cut in the middle, whole on hover.

// pathChars is how much of a path a narrow row shows.
const pathChars = 34

// sessionFacts are session s's lines: its model, its project.
func (a *App) sessionFacts(c *ui.Context, s proto.SessionInfo) {
	if model, full := modelLine(s); model != "" {
		ui.Text(c, model).FontSize(theme.TextLabel).Font(theme.MonoFont).TextColor(theme.Muted).SingleLine().Shrink(1).Tooltip(full)
	}
	branch := ""
	if s.Worktree {
		branch = s.Branch
	}
	dir := s.Dir
	if !s.Worktree {
		dir = s.Project
	}
	projectLine(c, s.Project, dir, branch, "")
}

// modelLine is what a session's row says of its model, short and whole: ""
// for what runs no agent, "unknown" for one that says nothing.
func modelLine(s proto.SessionInfo) (short, full string) {
	if _, ok := agent.Find(s.Agent); !ok || s.Agent == agent.Shell {
		return "", ""
	}
	if s.Model == "" {
		return "unknown model", "The agent doesn't say which model it runs on, or hasn't yet"
	}
	effort := nonEmpty(s.Effort, "unknown")
	short, full = textfmt.ShortModel(s.Model)+" · "+effort, s.Model+" · effort "+effort
	if s.ModelLast {
		short += " (last)"
		full += ", the last it said: it isn't running"
	}
	if s.ModelStarted != "" {
		short += " (started on " + textfmt.ShortModel(s.ModelStarted) + ")"
		full += "\nStarted on " + s.ModelStarted + ": its agent moved to this one, by a fallback or a change in the agent"
	}
	return short, full
}

// projectLine says a project by name and path, with host when it's another's;
// dir is where it works, shown for a worktree, with its branch. "" for project
// says no project.
func projectLine(c *ui.Context, project, dir, branch, host string) {
	var text, full string
	switch project {
	case "":
		text, full = "no project", dir
	case "unknown":
		text = "project unknown"
	default:
		text, full = filepath.Base(project)+" · "+textfmt.ShortPath(nonEmpty(dir, project), pathChars), nonEmpty(dir, project)
		if dir != "" && dir != project {
			full = project + "\nworktree " + dir
		}
	}
	if branch != "" {
		text += " · " + branch
	}
	if host != "" {
		text += " on " + host
	}
	line := ui.Text(c, text).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine().Shrink(1)
	if full != "" {
		line.Tooltip(full).Selectable()
	}
}

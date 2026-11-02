package ui

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/diffview"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
)

// The conflict radar: files two running sessions of the shown project
// both change, found before their branches meet. The session list marks
// them, and the Git tab says which files and with whom.

// conflictsEvery is how often the shown project is looked at.
const conflictsEvery = 10 * time.Second

// pollConflicts looks at the shown project while connection gen lasts.
func (a *App) pollConflicts(cl *client.Client, gen int) {
	go func() {
		for {
			ask := make(chan string, 1)
			a.onMain(func() {
				if gen != a.gen {
					close(ask)
					return
				}
				project := ""
				if len(a.projectSessions(a.project)) > 1 {
					project = a.project
				}
				ask <- project
			})
			project, live := <-ask
			if !live {
				return
			}
			if project != "" {
				list, err := cl.Conflicts(project)
				a.onMain(func() {
					if gen != a.gen || err != nil {
						return
					}
					if a.conflicts == nil {
						a.conflicts = map[string][]proto.Conflict{}
					}
					a.conflicts[project] = list
				})
			}
			time.Sleep(conflictsEvery)
		}
	}()
}

// conflictsOf are the conflicts session s is in, of its project's last
// look.
func (a *App) conflictsOf(s proto.SessionInfo) []proto.Conflict {
	var out []proto.Conflict
	for _, c := range a.conflicts[s.Project] {
		if c.Sessions[0] == s.ID || c.Sessions[1] == s.ID {
			out = append(out, c)
		}
	}
	return out
}

// conflictNotes are session s's conflicts as the Git tab tells them.
func (a *App) conflictNotes(s proto.SessionInfo) []diffview.ConflictNote {
	var out []diffview.ConflictNote
	for _, c := range a.conflictsOf(s) {
		other := c.Sessions[0]
		if other == s.ID {
			other = c.Sessions[1]
		}
		name := other
		for _, o := range a.sessions {
			if o.ID == other {
				name = a.sessionName(o)
			}
		}
		out = append(out, diffview.ConflictNote{File: c.File, Session: other, With: name, Lines: c.Lines})
	}
	return out
}

// conflictMark marks a session row whose changes meet another session's.
func (a *App) conflictMark(c *ui.Context, s proto.SessionInfo) {
	list := a.conflictsOf(s)
	if len(list) == 0 {
		return
	}
	col, lines := theme.TextSoft, 0
	for _, x := range list {
		if x.Lines {
			col = theme.Warning
			lines++
		}
	}
	tip := fmt.Sprintf("Changes %d %s another session changes too", len(list), textfmt.Plural(len(list), "file", "files"))
	if lines > 0 {
		tip += fmt.Sprintf(", %d at the same lines", lines)
	}
	if s.ID == a.selected && theme.OnPicked != theme.TextStrong {
		col = theme.OnPicked // on a bright field, its words' color reads
	}
	ui.Icon(c, icon.Alert).FontSize(12).TextColor(col).Tooltip(tip).Label(tip)
}

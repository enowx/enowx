package ui

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// A team's projects (docs/cloud.md): Git links its admins add, listed in
// the team's dialog with where each is on the connected host, cloned from
// there when it isn't; the session list marks a project that is one, and
// a team's notes attach to them.

// hostTeamProjects is the connected host's teams' projects, as last
// listed. It belongs to the main thread.
type hostTeamProjects struct {
	host   string
	list   []proto.TeamProject
	adding string // a Git link typed, to add to the open team
	err    string
}

func (a *App) teamProjectsOf() *hostTeamProjects {
	if a.teamProjectList == nil || a.teamProjectList.host != a.host.ID {
		a.teamProjectList = &hostTeamProjects{host: a.host.ID}
		a.loadTeamProjects()
	}
	return a.teamProjectList
}

func (a *App) loadTeamProjects() {
	tp, cl := a.teamProjectList, a.client
	if tp == nil || cl == nil {
		return
	}
	go func() {
		list, err := cl.TeamProjects()
		if err != nil {
			return
		}
		a.onMain(func() { tp.list = list })
	}()
}

// teamProjectsEvent takes the teams' projects as they changed.
func (a *App) teamProjectsEvent(data json.RawMessage) func() {
	var list []proto.TeamProject
	if json.Unmarshal(data, &list) != nil {
		return nil
	}
	return func() {
		if tp := a.teamProjectList; tp != nil && tp.host == a.host.ID {
			tp.list = list
		}
	}
}

// teamOfProject is the team whose project the host's project at path
// holds, "" for none.
func (a *App) teamOfProject(path string) (proto.TeamProject, bool) {
	if a.client == nil {
		return proto.TeamProject{}, false
	}
	for _, p := range a.teamProjectsOf().list {
		if slices.ContainsFunc(p.Here, func(c proto.TeamProjectCopy) bool { return c.Path == path }) {
			return p, true
		}
	}
	return proto.TeamProject{}, false
}

// teamProjectMark marks a project of the session list that is a team's.
func (a *App) teamProjectMark(c *ui.Context, path string) {
	if p, ok := a.teamOfProject(path); ok {
		name := a.spaceName(p.Team)
		widget.TeamMark(c, p.Team, name, 13).Tooltip(name + "'s project · its notes are the team's")
	}
}

// cloudTeamProjects is the team dialog's Projects tab.
func (a *App) cloudTeamProjects(c *ui.Context, cs *cloudSettings) {
	tp := a.teamProjectsOf()
	team := cs.teams.team
	admin := canAdmin(team.Role)
	if tp.err != "" {
		widget.Notice(c, icon.Alert, theme.Error, tp.err, "", nil)
	}
	var list []proto.TeamProject
	for _, p := range tp.list {
		if p.Team == team.ID {
			list = append(list, p)
		}
	}
	if len(list) == 0 {
		msg := "No projects yet: an admin adds one by its Git link."
		if admin {
			msg = "No projects yet: add one by its Git link. Members who have it already are found by its remote; the others clone it from here."
		}
		ui.Text(c, msg).FontSize(theme.TextLabel).TextColor(theme.Muted)
	}
	ui.Column(c.Key("rows")).FillWidth().Shrink(0).Gap(theme.S2).Children(func() {
		for _, p := range list {
			a.teamProjectRow(c, p, admin)
		}
	})
	if !admin {
		return
	}
	widget.Field(c, "Add a project", func() {
		ui.Row(c).FillWidth().Gap(8).Children(func() {
			ui.Column(c).Grow(1).Children(func() {
				widget.Input(c, &tp.adding, "https://github.com/me/app or git@github.com:me/app.git").FillWidth().Font(theme.MonoFont)
			})
			b := widget.Button(c, widget.Secondary, icon.Plus, "Add", "")
			link := strings.TrimSpace(tp.adding)
			if link == "" {
				b.Disabled(true)
			} else if b.Clicked() {
				a.addTeamProject(team.ID, link)
			}
		})
		ui.Text(c, "The link only, no token in it: each member clones with their own login.").FontSize(theme.TextLabel).TextColor(theme.Muted)
	})
}

func (a *App) teamProjectRow(c *ui.Context, p proto.TeamProject, admin bool) {
	ui.Row(c.Key("tp:"+p.ID)).FillWidth().Shrink(0).Gap(theme.S2).Padding(theme.S2, theme.S3).Radius(theme.RadiusCard).Background(theme.Raised).Children(func() {
		ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
			widget.Running(c, p.Name, widget.RunStyle{Weight: 500})
			where := strings.TrimPrefix(p.Ref, "git:")
			if len(p.Here) > 0 {
				where = "On " + a.host.Name + ": " + textfmt.TildePath(p.Here[0].Path, a.daemonInfo.Home)
			}
			widget.Running(c.Key("where"), where, widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
		})
		if notes := a.teamProjectNotes(p); notes > 0 {
			widget.Chip(c, textfmt.Plural(notes, "1 note", fmt.Sprintf("%d notes", notes)), ui.Color{})
		}
		if len(p.Here) == 0 {
			if widget.Button(c, widget.Secondary, icon.Download, "Clone", "").Tooltip("Clone " + p.Repo + " on " + a.host.Name).Clicked() {
				a.cloneTeamProject(p)
			}
		} else {
			widget.Chip(c, "here", theme.Done)
		}
		if admin && widget.IconButton(c, icon.Close, "Take "+p.Name+" out of the team", false).Clicked() {
			a.confirmTakeProjectOut(p)
		}
	})
}

// confirmTakeProjectOut asks before p leaves its team's projects.
func (a *App) confirmTakeProjectOut(p proto.TeamProject) {
	cl, team, id, name := a.client, p.Team, p.ID, p.Name
	a.confirm("Take "+name+" out of the team?", "Members stop seeing it as the team's. Their copies stay theirs, and the notes attached to it stay attached.", "Take it out", func() {
		go func() {
			if err := cl.RemoveTeamProject(team, id); err != nil {
				a.onMain(func() { a.teamProjectsOf().err = client.Explain(err) })
			}
		}()
	})
}

// addTeamProject adds the Git link typed to team.
func (a *App) addTeamProject(team, link string) {
	tp, cl := a.teamProjectsOf(), a.client
	go func() {
		_, err := cl.AddTeamProject(proto.TeamProjectAddParams{Team: team, Repo: link})
		a.onMain(func() {
			tp.err = ""
			if err != nil {
				tp.err = client.Explain(err)
				return
			}
			tp.adding = ""
			a.loadTeamProjects()
		})
	}()
}

// teamProjectNotes counts the team's notes attached to p.
func (a *App) teamProjectNotes(p proto.TeamProject) int {
	n := 0
	for _, note := range a.notesOf().list {
		if note.Team == p.Team && slices.ContainsFunc(note.Projects, func(np proto.NoteProject) bool { return np.Ref == p.Ref }) {
			n++
		}
	}
	return n
}

// cloneTeamProject opens the clone dialog on a team's project's link.
func (a *App) cloneTeamProject(p proto.TeamProject) {
	a.cloudSet.teams.open = ""
	a.openClone()
	if a.clone != nil {
		a.clone.url = p.Repo
	}
}

// teamProjectsToAttach are the teams' projects a note of team attaches
// to, as a note's projects.
func (a *App) teamProjectsToAttach(team string) []proto.NoteProject {
	var out []proto.NoteProject
	for _, p := range a.teamProjectsOf().list {
		if p.Team == team {
			out = append(out, proto.NoteProject{Ref: p.Ref, Name: p.Name, Remote: p.Repo})
		}
	}
	return out
}

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
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// A team kit's definitions (docs/cloud.md): pipelines' stages, automations
// of its projects, and project templates. A team's project's pipeline
// dialog offers the team's pipelines and shares its own; the Kit tab uses
// the others: an automation set on this host's copy of the project, a
// template cloned as a new project.

// kitOfKind are the team's kit items of kind, as last read.
func (a *App) kitOfKind(team, kind string) []proto.TeamKitItem {
	var out []proto.TeamKitItem
	for _, it := range a.teamKitOf().list {
		if it.Team == team && it.Kind == kind {
			out = append(out, it)
		}
	}
	return out
}

// pipelineTeamRow offers, in a team's project's pipeline dialog, the
// team's pipelines to start from, and sharing the stages shown.
func (a *App) pipelineTeamRow(c *ui.Context, f *pipelineForm) {
	tp, ok := a.teamOfProject(f.project.Path)
	if !ok {
		return
	}
	team := a.spaceName(tp.Team)
	widget.Field(c, team+"'s pipelines", func() {
		ui.Row(c).FillWidth().Gap(6).Wrap().Children(func() {
			for _, it := range a.kitOfKind(tp.Team, proto.KitPipeline) {
				var stages []proto.PipelineStage
				if json.Unmarshal(it.Data, &stages) != nil || len(stages) == 0 {
					continue
				}
				b := widget.Button(c.Key("tp:"+it.Name), widget.Secondary, icon.List, it.Name, "")
				if it.About != "" {
					b.Tooltip(it.About)
				}
				if b.Clicked() {
					f.stages, f.editing = stages, -1
				}
			}
		})
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Grow(1).Children(func() { widget.Input(c, &f.shareName, "Share these stages as…").FillWidth() })
			b := widget.Button(c, widget.Secondary, icon.Plus, "Share with "+team, "")
			name := strings.TrimSpace(f.shareName)
			if name == "" {
				b.Disabled(true)
			} else if b.Clicked() {
				data, _ := json.Marshal(f.stages)
				a.saveKitDefinition(proto.TeamKitSaveParams{Team: tp.Team, Kind: proto.KitPipeline, Name: name, Data: data}, func(err error) {
					if err != nil {
						f.err = err.Error()
						return
					}
					f.shareName = ""
				})
			}
		})
	})
}

func (a *App) saveKitDefinition(p proto.TeamKitSaveParams, done func(error)) {
	cl := a.client
	go func() {
		_, err := cl.SaveToTeamKit(p)
		a.onMain(func() {
			if err != nil {
				done(errorText(strings.TrimPrefix(client.Explain(err), "teams: ")))
				return
			}
			done(nil)
		})
	}()
}

type errorText string

func (e errorText) Error() string { return string(e) }

// kitDefinitions is the Kit tab's part for pipelines, automations and
// templates.
func (a *App) kitDefinitions(c *ui.Context, cs *cloudSettings) {
	tk := a.teamKitOf()
	team := cs.teams.team
	can := team.Role != "viewer"
	widget.Field(c, "Pipelines, automations and templates", func() {
		var defs []proto.TeamKitItem
		for _, it := range tk.list {
			if it.Team == team.ID && slices.Contains([]string{proto.KitPipeline, proto.KitAutomation, proto.KitTemplate}, it.Kind) {
				defs = append(defs, it)
			}
		}
		if len(defs) == 0 {
			ui.Text(c, "A team project's pipeline dialog shares its stages; an automation of one of the team's projects shares from Settings › Automations; templates are added here.").FontSize(theme.TextLabel).TextColor(theme.Muted)
		}
		for _, it := range defs {
			ui.Row(c.Key("def:"+it.Kind+":"+it.Name)).FillWidth().Gap(10).AlignItems(ui.Center).Padding(8, 10).Radius(theme.RadiusControl).Background(theme.Sunken).Children(func() {
				widget.Tag(c, it.Kind, theme.TextSoft)
				ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(1).Children(func() {
					ui.Text(c, it.Name).FontSize(theme.TextUI).SingleLine()
					if sub := a.kitDefinitionAbout(it); sub != "" {
						ui.Text(c, sub).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine()
					}
				})
				switch it.Kind {
				case proto.KitAutomation:
					if widget.Button(c, widget.Secondary, nil, "Use on "+a.host.Name, "").Clicked() {
						a.useKitAutomation(it, tk)
					}
				case proto.KitTemplate:
					if widget.Button(c, widget.Secondary, icon.Download, "New project", "").Clicked() {
						var t proto.KitTemplateData
						json.Unmarshal(it.Data, &t)
						cs.teams.open = ""
						a.openClone()
						if a.clone != nil {
							a.clone.url = t.Repo
						}
					}
				}
				if can && widget.IconButton(c, icon.Close, "Take "+it.Name+" out of the kit", false).Clicked() {
					item := it
					a.confirm("Take "+item.Name+" out of the team's kit?", "Members stop seeing it. What they set up from it stays theirs.", "Take it out", func() {
						cl := a.client
						go func() { _ = cl.RemoveFromTeamKit(item.Team, item.Kind, item.Name) }()
					})
				}
			})
		}
		if !can {
			return
		}
		ui.Row(c).FillWidth().Gap(8).Children(func() {
			ui.Column(c).Width(140).Children(func() { widget.Input(c, &tk.templateName, "Template name").FillWidth() })
			ui.Column(c).Grow(1).Children(func() {
				widget.Input(c, &tk.templateRepo, "Its Git link").FillWidth().Font(theme.MonoFont)
			})
			b := widget.Button(c, widget.Secondary, icon.Plus, "Add template", "")
			if strings.TrimSpace(tk.templateName) == "" || strings.TrimSpace(tk.templateRepo) == "" {
				b.Disabled(true)
			} else if b.Clicked() {
				data, _ := json.Marshal(proto.KitTemplateData{Repo: strings.TrimSpace(tk.templateRepo)})
				a.saveKitDefinition(proto.TeamKitSaveParams{Team: team.ID, Kind: proto.KitTemplate, Name: strings.TrimSpace(tk.templateName), Data: data}, func(err error) {
					if err != nil {
						tk.err = err.Error()
						return
					}
					tk.templateName, tk.templateRepo = "", ""
				})
			}
		})
	})
}

// kitDefinitionAbout says what a definition is.
func (a *App) kitDefinitionAbout(it proto.TeamKitItem) string {
	if it.About != "" {
		return it.About
	}
	switch it.Kind {
	case proto.KitPipeline:
		var stages []proto.PipelineStage
		json.Unmarshal(it.Data, &stages)
		var names []string
		for _, s := range stages {
			names = append(names, s.Name)
		}
		return strings.Join(names, " → ")
	case proto.KitAutomation:
		var au proto.Automation
		json.Unmarshal(it.Data, &au)
		return au.Schedule + " · " + strings.TrimPrefix(au.Project, "git:")
	case proto.KitTemplate:
		var t proto.KitTemplateData
		json.Unmarshal(it.Data, &t)
		return t.Repo
	}
	return ""
}

// useKitAutomation sets a team's automation up on this host, in its copy
// of the automation's project.
func (a *App) useKitAutomation(it proto.TeamKitItem, tk *hostTeamKit) {
	var au proto.Automation
	if json.Unmarshal(it.Data, &au) != nil {
		return
	}
	path := ""
	for _, p := range a.teamProjectsOf().list {
		if p.Team == it.Team && p.Ref == au.Project {
			for _, h := range p.Here {
				if h.Project {
					path = h.Path
				}
			}
		}
	}
	if path == "" {
		tk.err = "Clone " + strings.TrimPrefix(au.Project, "git:") + " on " + a.host.Name + " first (the team's Projects tab)"
		return
	}
	// What a member shared, as the kit keeps it: off, the agent asking
	// before it acts, whatever their host wrote.
	au = proto.Automation{Name: au.Name, Project: path, Agent: au.Agent, Prompt: au.Prompt, Schedule: au.Schedule, Branch: au.Branch}
	if au.Name == "" {
		au.Name = it.Name
	}
	where := "in " + path
	if au.Branch {
		where = "on a new branch of " + path + " each time"
	}
	msg := fmt.Sprintf("%s runs on %s %s, %s, with this prompt:\n\n%s\n\nIt is set up off, the agent asking before it acts: turn it on in Settings › Automations once you trust it.",
		au.Agent, a.host.Name, au.Schedule, where, au.Prompt)
	a.confirm("Set up “"+au.Name+"” from the team's kit?", msg, "Set it up", func() {
		cl := a.client
		go func() {
			_, err := cl.SaveAutomation(au)
			a.onMain(func() {
				if err != nil {
					tk.err = client.Explain(err)
					return
				}
				a.notify("“" + au.Name + "” is set up on " + a.host.Name + ", off: turn it on in Settings › Automations")
				a.loadAutomations()
			})
		}()
	})
}

// shareAutomation shares one of the host's automations with the team
// whose project it runs in.
func (a *App) shareAutomation(au proto.Automation) {
	tp, ok := a.teamOfProject(au.Project)
	if !ok {
		return
	}
	// Never on, nor skipping permissions, on a teammate's host.
	data, _ := json.Marshal(proto.Automation{Name: au.Name, Project: tp.Ref, Agent: au.Agent, Prompt: au.Prompt, Schedule: au.Schedule, Branch: au.Branch})
	name := strings.Map(func(r rune) rune {
		if r == ' ' {
			return '-'
		}
		return r
	}, strings.ToLower(strings.TrimSpace(au.Name)))
	a.saveKitDefinition(proto.TeamKitSaveParams{Team: tp.Team, Kind: proto.KitAutomation, Name: name, Data: data}, func(err error) {
		if err != nil {
			a.automationsOf().err = err.Error()
			return
		}
		a.notify("“" + au.Name + "” shared with " + a.spaceName(tp.Team))
	})
}

// canvasProjects are what a canvas of the space shown belongs to: the
// host's projects for the user's own, the team's projects (by ref) for a
// team's.
func (a *App) canvasProjects() []proto.ProjectInfo {
	team := a.space()
	if team == "" {
		return a.projects
	}
	var out []proto.ProjectInfo
	for _, p := range a.teamProjectsOf().list {
		if p.Team == team {
			out = append(out, proto.ProjectInfo{Path: p.Ref, Name: p.Name})
		}
	}
	return out
}

// projectFolder is a canvas's project's folder on the host: a team's
// project's (its ref) copy here, "" when there is none.
func (a *App) projectFolder(project string) string {
	if !strings.HasPrefix(project, "git:") {
		return project
	}
	for _, p := range a.teamProjectsOf().list {
		if p.Ref == project {
			for _, h := range p.Here {
				return h.Path
			}
		}
	}
	return ""
}

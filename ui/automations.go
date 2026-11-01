package ui

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// Settings → Automations: prompts an agent gets on a schedule, kept and
// run by the connected host's daemon (proto.Automation).

// automationsState is the section's: the host's automations and the form
// adding or changing one. It belongs to the main thread.
type automationsState struct {
	host    string
	asked   bool // the list was asked for, of host
	loaded  bool
	list    []proto.Automation
	err     string
	editing *proto.Automation // the form's, nil when it's closed
	formErr string
	busy    bool
}

func (a *App) automationsOf() *automationsState {
	if a.autos.host != a.host.ID || !a.autos.asked {
		a.autos = automationsState{host: a.host.ID, asked: true}
		a.loadAutomations()
	}
	return &a.autos
}

func (a *App) loadAutomations() {
	cl, host := a.client, a.host.ID
	if cl == nil {
		return
	}
	go func() {
		list, err := cl.Automations()
		a.onMain(func() {
			if a.autos.host != host {
				return
			}
			a.autos.loaded, a.autos.list, a.autos.err = true, list, ""
			if err != nil {
				a.autos.err = client.Explain(err)
			}
		})
	}()
}

func (a *App) automationsSection(c *ui.Context) {
	st := a.automationsOf()
	sectionIntro(c, "Automations", "Prompts an agent gets on a schedule, each run in a session of its own on "+a.host.Name+", whether or not this app is open. Times are "+a.host.Name+"'s.")
	ui.Row(c).FillWidth().Children(func() {
		ui.Spacer(c)
		if widget.PrimaryButton(c, icon.Plus, "Add an automation", "").Disabled(len(a.projects) == 0).Clicked() {
			p, _ := a.currentProject()
			st.editing = &proto.Automation{Project: p.Path, Agent: a.defaultAgent(), Schedule: "weekdays 09:00", Enabled: true}
			st.formErr = ""
		}
	})
	switch {
	case st.err != "":
		widget.Notice(c, icon.Alert, theme.Error, st.err, "", nil)
	case !st.loaded:
		widget.Spinner(c, 14)
	case len(st.list) == 0 && st.editing == nil:
		ui.Text(c, "None yet. Weekday triage of new issues, a nightly dependency update, a weekly summary of what changed: an agent can do them while you are away.").
			FontSize(12).TextColor(theme.Muted)
	}
	if st.editing != nil {
		a.automationForm(c, st)
	}
	for _, au := range st.list {
		a.automationRow(c.Key(au.ID), st, au)
	}
}

func (a *App) automationRow(c *ui.Context, st *automationsState, au proto.Automation) {
	widget.Card(c).FillWidth().Children(func() {
		ui.Row(c).FillWidth().Gap(10).Children(func() {
			agentTile(c, au.Agent, 26)
			ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
				ui.Text(c, au.Name).FontSize(theme.TextUI).SingleLine()
				detail := au.Schedule + " · " + a.agentName(au.Agent) + " · " + filepath.Base(au.Project)
				if au.Enabled && !au.NextRun.IsZero() {
					detail += " · next " + au.NextRun.Local().Format("Mon Jan 2 15:04")
				}
				ui.Text(c, detail).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine()
				switch {
				case au.LastError != "":
					ui.Text(c, "Last run failed: "+au.LastError).FontSize(theme.TextLabel).TextColor(theme.Error).MaxLines(2)
				case !au.LastRun.IsZero():
					ui.Text(c, "Last ran "+au.LastRun.Local().Format("Jan 2 15:04")).FontSize(theme.TextLabel).TextColor(theme.Muted)
				}
			})
			on := au.Enabled
			if widget.Switch(c, &on, "Enabled") {
				au.Enabled = on
				a.saveAutomation(st, au, false)
			}
			if widget.GhostButton(c, "Run now").Clicked() {
				a.runAutomation(au)
			}
			widget.MoreMenu(c, &a.menu, "automation:"+au.ID, func(done func()) {
				if widget.MenuItem(c, icon.Pencil, "Edit…", false) {
					edit := au
					st.editing, st.formErr = &edit, ""
					done()
				}
				if tp, ok := a.teamOfProject(au.Project); ok && widget.MenuItem(c, icon.Plus, "Share with "+a.spaceName(tp.Team), false) {
					a.shareAutomation(au)
					done()
				}
				if widget.MenuItem(c, icon.Trash, "Remove", true) {
					a.askRemoveAutomation(au)
					done()
				}
			})
		})
	})
}

// automationForm adds an automation, or changes the one being edited.
func (a *App) automationForm(c *ui.Context, st *automationsState) {
	e := st.editing
	widget.Card(c).FillWidth().Border(1, theme.AccentLine).Children(func() {
		ui.Column(c).FillWidth().Gap(10).Children(func() {
			if st.formErr != "" {
				widget.Notice(c, icon.Alert, theme.Error, st.formErr, "", nil)
			}
			widget.Field(c, "Name", func() { widget.Input(c, &e.Name, "Weekday triage").FillWidth().AutoFocus() })
			widget.Field(c, "Project", func() {
				sel := ui.SelectBase(c, &e.Project)
				widget.Trigger(sel.Trigger, sel.Open()).FillWidth().Label("Project").Children(func() {
					ui.Text(c, filepath.Base(e.Project)).FontSize(theme.TextUI).SingleLine()
					ui.Spacer(c)
					ui.Icon(c, icon.ChevronDown).FontSize(12).TextColor(theme.Muted)
				})
				sel.Popup(func(panel ui.Element) {
					widget.Popup(panel.Margin(6, 0, 0, 0).Width(360))
					for _, p := range a.projects {
						item := sel.Item(p.Path)
						widget.PopupItem(item, item.Highlighted()).Children(func() { ui.Text(c, p.Name).FontSize(theme.TextUI) })
					}
				})
			})
			widget.Field(c, "Agent", func() {
				sel := ui.SelectBase(c, &e.Agent)
				widget.Trigger(sel.Trigger, sel.Open()).FillWidth().Label("Agent").Children(func() {
					agentTile(c, e.Agent, 20)
					ui.Text(c, a.agentName(e.Agent)).FontSize(theme.TextUI).SingleLine()
					ui.Spacer(c)
					ui.Icon(c, icon.ChevronDown).FontSize(12).TextColor(theme.Muted)
				})
				sel.Popup(func(panel ui.Element) {
					widget.Popup(panel.Margin(6, 0, 0, 0).Width(300))
					ui.Scroll(c).FillWidth().MaxHeight(300).Children(func() {
						ui.Column(c).FillWidth().Children(func() {
							for _, ag := range a.agents {
								if ag.Command == "" || !ag.Installed {
									continue
								}
								item := sel.Item(ag.ID)
								widget.PopupItem(item, item.Highlighted()).Children(func() {
									agentTile(c, ag.ID, 20)
									ui.Text(c, ag.Name).FontSize(theme.TextUI)
								})
							}
						})
					})
				})
			})
			widget.Field(c, "When", func() {
				widget.Input(c, &e.Schedule, "weekdays 09:00").FillWidth().Font(theme.MonoFont)
				ui.Text(c, "hourly (or hourly :15), daily 09:00, weekdays 18:30, weekly mon 08:00").FontSize(theme.TextLabel).TextColor(theme.Muted)
			})
			widget.Field(c, "Prompt", func() {
				ui.TextAreaBase(c, &e.Prompt).FillWidth().MinHeight(70).MaxHeight(180).Padding(9, 11).FontSize(theme.TextUI).
					Label("Prompt").Radius(theme.RadiusControl).Background(theme.Sunken).Border(1, theme.Border).
					Placeholder("Triage the issues opened since yesterday and summarize them")
			})
			switchRow(c, &e.Branch, "A new branch each run", "In a worktree of its own, so runs don't touch each other or your work; otherwise in the project folder.")
			switchRow(c, &e.SkipPermissions, "Skip permission prompts", "Nobody may be there to answer them.")
			ui.Row(c).FillWidth().Gap(8).Children(func() {
				ui.Spacer(c)
				if widget.GhostButton(c, "Cancel").Clicked() {
					st.editing = nil
				}
				ok := strings.TrimSpace(e.Name) != "" && strings.TrimSpace(e.Prompt) != "" && !st.busy
				if widget.PrimaryButton(c, icon.Check, "Save", "").Disabled(!ok).Clicked() && ok {
					a.saveAutomation(st, *e, true)
				}
			})
		})
	})
}

// saveAutomation sends au to the daemon; from the form, it closes it, or
// says why the daemon refused.
func (a *App) saveAutomation(st *automationsState, au proto.Automation, fromForm bool) {
	cl := a.client
	if cl == nil {
		return
	}
	st.busy = true
	go func() {
		_, err := cl.SaveAutomation(au)
		a.onMain(func() {
			st.busy = false
			switch {
			case err != nil && fromForm:
				st.formErr = client.Explain(err)
			case err != nil:
				a.status = client.Explain(err)
			case fromForm:
				st.editing = nil
			}
			a.loadAutomations()
		})
	}()
}

func (a *App) runAutomation(au proto.Automation) {
	cl := a.client
	if cl == nil {
		return
	}
	go func() {
		info, err := cl.RunAutomation(au.ID)
		a.onMain(func() {
			if err != nil {
				a.status = "Couldn't run " + au.Name + ": " + client.Explain(err)
				return
			}
			a.upsert(info)
			a.loadAutomations()
			a.notify(au.Name + " started at " + time.Now().Format("15:04"))
		})
	}()
}

// askRemoveAutomation removes au once the user agrees.
func (a *App) askRemoveAutomation(au proto.Automation) {
	a.confirm("Remove “"+au.Name+"”?", "It stops running on its schedule, and its settings go. Sessions it already started stay.", "Remove", func() { a.removeAutomation(au) })
}

func (a *App) removeAutomation(au proto.Automation) {
	cl := a.client
	if cl == nil {
		return
	}
	go func() {
		err := cl.RemoveAutomation(au.ID)
		a.onMain(func() {
			if err != nil {
				a.status = client.Explain(err)
			}
			a.loadAutomations()
		})
	}()
}

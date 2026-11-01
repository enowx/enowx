package ui

import (
	"path/filepath"
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
	widget.PageHeader(c, "Automations", "Prompts on a schedule, in "+a.host.Name+"'s time", func() {
		if widget.PrimaryButton(c, icon.Plus, "Add an automation", "").Disabled(len(a.projects) == 0).Clicked() {
			p, _ := a.currentProject()
			st.editing = &proto.Automation{Project: p.Path, Agent: a.defaultAgent(), Schedule: "weekdays 09:00", Enabled: true}
			st.formErr = ""
		}
	})
	now := time.Now()
	week := automationWeek(st.list, now)
	widget.FigsBand(c, automationFigs(st.list, week))
	// The week takes what the cards leave of the frame; the frame scrolls only
	// when there are more cards than fit with a week still readable.
	scroll := ui.Scroll(c.Key("automations")).Grow(1).FillWidth().MinHeight(0)
	height := scroll.Bounds().H
	scroll.Children(func() {
		ui.Column(c).FillWidth().MinHeight(height).Padding(theme.S4, theme.S5, theme.S5, theme.S5).Gap(theme.S3).Children(func() {
			switch {
			case st.err != "":
				widget.Notice(c, icon.Alert, theme.Error, st.err, "", nil)
			case !st.loaded:
				widget.Spinner(c, 14)
			case len(st.list) == 0:
				widget.EmptyState(c, icon.History, "None yet", "Weekday triage of new issues, a nightly dependency update, a weekly summary of what changed: an agent can do them while you are away.", nil)
			}
			if len(st.list) == 0 {
				return
			}
			rows := make([]ui.Track, (len(st.list)+1)/2)
			for i := range rows {
				rows[i] = ui.Fixed(automationCardHeight)
			}
			widget.Cols(c.Key("cards"), 2).Shrink(0).RowTracks(rows...).Children(func() {
				for _, au := range st.list {
					a.automationRow(c.Key(au.ID), st, au)
				}
			})
			automationWeekBoard(c, week, now)
		})
	})
	if st.editing != nil {
		a.automationForm(c, st)
	}
}

// automationCardHeight holds a card whatever it says: a long prompt or a
// failure is cut to its lines rather than growing it.
const automationCardHeight = 248

func (a *App) automationRow(c *ui.Context, st *automationsState, au proto.Automation) {
	widget.GridCard(c).Children(func() {
		widget.CardTitle(c, nil, au.Name, func() {
			on := au.Enabled
			if widget.Switch(c, &on, "Enabled") {
				au.Enabled = on
				a.saveAutomation(st, au, false)
			}
		})
		ui.Row(c).FillWidth().Shrink(0).Gap(theme.S2).Children(func() {
			ui.Box(c).Shrink(0).Tooltip(a.agentName(au.Agent)).Children(func() { agentTile(c, au.Agent, 32) })
			ui.Row(c).Grow(1).MinWidth(0).Children(func() {
				widget.Running(c, au.Schedule, widget.RunStyle{Mono: true})
			})
			automationProject(c, filepath.Base(au.Project))
		})
		lines := 2
		if au.LastError != "" {
			lines = 1
			ui.Text(c, "Last run failed: "+au.LastError).FontSize(theme.TextLabel).TextColor(theme.Error).FillWidth().MinWidth(0).MaxLines(2).Shrink(0)
		}
		widget.CardNote(c, au.Prompt, lines).Shrink(0)
		automationFacts(c, au)
		widget.CardFoot(c, func() {
			if widget.Button(c, widget.Secondary, icon.Play, "Run now", "").Clicked() {
				a.runAutomation(au)
			}
			ui.Spacer(c)
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
				if widget.MenuItem(c, icon.Trash, "Remove…", true) {
					a.askRemoveAutomation(au)
					done()
				}
			})
		})
	})
}

// automationProject is a project's name in a chip that keeps inside the row,
// running when it's too long.
func automationProject(c *ui.Context, name string) {
	ui.Row(c.Key("project")).Height(24).MinWidth(0).MaxWidth(160).Shrink(1).PaddingX(theme.S2).Radius(theme.RadiusFull).Background(theme.Sunken).Children(func() {
		widget.Running(c, name, widget.RunStyle{Size: theme.TextLabel})
	})
}

// automationFacts are a card's figures: its next run, only while enabled, its
// last run and where it runs.
func automationFacts(c *ui.Context, au proto.Automation) {
	next := "Off"
	if au.Enabled {
		next = "Not set"
		if !au.NextRun.IsZero() {
			next = automationWhen(au.NextRun, time.Now())
		}
	}
	last := "Not yet"
	if !au.LastRun.IsZero() {
		last = automationWhen(au.LastRun, time.Now())
	}
	where := "Project folder"
	if au.Branch {
		where = "Worktree"
	}
	ui.Row(c.Key("facts")).FillWidth().Shrink(0).Gap(theme.S2).Children(func() {
		for _, f := range [][2]string{{next, "next run"}, {last, "last run"}, {where, "runs in"}} {
			ui.Column(c.Key(f[1])).Grow(1).Basis(0).MinWidth(0).Gap(2).Children(func() {
				widget.Running(c.Key("v"), f[0], widget.RunStyle{Weight: 500})
				widget.Running(c.Key("l"), f[1], widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
			})
		}
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
	a.confirm("Remove this automation?", "“"+au.Name+"” stops running. Its past sessions stay.", "Remove", func() { a.removeAutomation(au) })
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

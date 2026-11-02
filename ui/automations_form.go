package ui

import (
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// automationForm is the dialog adding an automation, or changing the one being
// edited.
func (a *App) automationForm(c *ui.Context, st *automationsState) {
	e := st.editing
	open, title := true, "Add an automation"
	if e.ID != "" {
		title = "Edit the automation"
	}
	about := "Prompts an agent gets on a schedule, each run in a session of its own on " + a.host.Name + ", whether or not this app is open."
	widget.FormSheet(c, &open, 480, title, about, func() {
		if st.formErr != "" {
			widget.Notice(c, icon.Alert, theme.Error, st.formErr, "", nil)
		}
		widget.Field(c, "Name", func() { widget.Input(c, &e.Name, "Weekday triage").FillWidth().AutoFocus() })
		widget.FieldCols(c, func() {
			widget.Field(c, "Project", func() { a.automationProjectPick(c, &e.Project) })
		}, func() {
			widget.Field(c, "Agent", func() { a.automationAgentPick(c, &e.Agent) })
		})
		widget.Field(c, "Schedule", func() {
			widget.Input(c, &e.Schedule, "weekdays 09:00").FillWidth().Font(theme.MonoFont)
			ui.Text(c, "hourly, hourly :15, daily 09:00, weekdays 18:30, weekly mon 08:00. Times are "+a.host.Name+"'s.").FontSize(theme.TextLabel).TextColor(theme.TextSoft)
		})
		widget.Field(c, "Prompt", func() {
			ui.TextAreaBase(c, &e.Prompt).FillWidth().MinHeight(72).MaxHeight(180).Padding(theme.S2, theme.S3).FontSize(theme.TextUI).
				Label("Prompt").Radius(theme.RadiusSmall).Background(theme.Sunken).Border(1, theme.Border).
				Placeholder("Triage the issues opened since yesterday and summarize them")
		})
		switchRow(c, &e.Branch, "Own branch", "In a worktree of its own, so runs don't touch each other or your work; otherwise in the project folder.")
		switchRow(c, &e.SkipPermissions, "Skip permission prompts", "Nobody may be there to answer them.")
	}, func() {
		ok := strings.TrimSpace(e.Name) != "" && strings.TrimSpace(e.Prompt) != "" && !st.busy
		if widget.PrimaryButton(c, icon.Check, "Save", "").Disabled(!ok).Clicked() && ok {
			a.saveAutomation(st, *e, true)
		}
	})
	if !open {
		st.editing = nil
	}
}

func (a *App) automationProjectPick(c *ui.Context, project *string) {
	sel := ui.SelectBase(c, project)
	widget.Trigger(sel.Trigger, sel.Open()).FillWidth().Label("Project").Children(func() {
		ui.Row(c).Grow(1).MinWidth(0).Children(func() { widget.Running(c, filepath.Base(*project), widget.RunStyle{}) })
		ui.Icon(c, icon.ChevronDown).FontSize(12).TextColor(theme.TextSoft)
	})
	sel.Popup(func(panel ui.Element) {
		widget.Popup(panel.Margin(6, 0, 0, 0).Width(300))
		ui.Scroll(c).FillWidth().MaxHeight(300).Children(func() {
			ui.Column(c).FillWidth().Children(func() {
				for _, p := range a.projects {
					item := sel.Item(p.Path)
					widget.PopupItem(item, item.Highlighted()).Children(func() { ui.Text(c, p.Name).FontSize(theme.TextUI) })
				}
			})
		})
	})
}

// automationAgentPick offers the agents installed on the host.
func (a *App) automationAgentPick(c *ui.Context, id *string) {
	sel := ui.SelectBase(c, id)
	widget.Trigger(sel.Trigger, sel.Open()).FillWidth().Label("Agent").Children(func() {
		agentTile(c, *id, 20)
		ui.Row(c).Grow(1).MinWidth(0).Children(func() { widget.Running(c, a.agentName(*id), widget.RunStyle{}) })
		ui.Icon(c, icon.ChevronDown).FontSize(12).TextColor(theme.TextSoft)
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
}

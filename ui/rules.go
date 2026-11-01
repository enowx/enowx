package ui

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// A project's rules (proto.ProjectRules): Markdown every session in it
// reads first, edited from the project's menu. A team's project's are the
// team's, edited by its members or only its admins.

type rulesForm struct {
	open    bool
	project proto.ProjectInfo
	rules   proto.ProjectRules
	text    string
	editors int // of rulesEditors
	loaded  bool
	saving  bool
	err     string
	draft   rulesDraft
}

var rulesEditors = []string{proto.RulesByMembers, proto.RulesByAdmins}

func (a *App) openRules(p proto.ProjectInfo) {
	f := &rulesForm{open: true, project: p}
	a.rulesEditing = f
	cl := a.client
	go func() {
		r, err := cl.ProjectRules(p.Path)
		a.onMain(func() {
			if err != nil {
				// Not read: nothing here may write over them.
				f.err = client.Explain(err)
				return
			}
			f.loaded, f.rules, f.text = true, r, r.Rules
			if r.Editors == proto.RulesByAdmins {
				f.editors = 1
			}
		})
	}()
}

func (a *App) rulesDialog(c *ui.Context) {
	f := a.rulesEditing
	if f == nil {
		return
	}
	if !f.open {
		a.rulesEditing = nil
		return
	}
	about := "Every session in " + f.project.Name + " reads them first, whatever its agent. Yours: they follow the project to your other hosts."
	if f.rules.Team != "" {
		about = "Every session in " + f.project.Name + " reads them first, on every member's host: they are " + a.spaceName(f.rules.Team) + "'s."
	}
	widget.FormSheet(c, &f.open, 600, "Rules of "+f.project.Name, about, func() {
		if !f.loaded {
			rulesNotRead(c, f.err)
			return
		}
		if f.rules.Team != "" {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				widget.TeamTag(c, f.rules.Team, a.spaceName(f.rules.Team))
				if !f.rules.CanEdit {
					ui.Text(c, "Edited by the team's admins").FontSize(theme.TextLabel).TextColor(theme.Muted)
				}
			})
		}
		area := ui.TextArea(c, &f.text).Height(260).Padding(10, 12).Radius(theme.RadiusControl).Font(theme.MonoFont).FontSize(theme.TextUI).
			Background(theme.Sunken).Border(1, theme.BorderStrong).Placeholder("- Run the tests before saying it's done\n- Never push to main")
		if !f.rules.CanEdit {
			area.Disabled(true)
		}
		ui.Text(c, "No credentials here: agents read these, and rules that look like they hold a secret are refused.").FontSize(theme.TextLabel).TextColor(theme.Muted)
		if f.rules.CanEdit {
			a.rulesDraftBox(c, &f.draft, proto.RulesForProject, f.project.Path, &f.text)
		}
		if f.rules.Team != "" && a.isAdminOf(f.rules.Team) {
			widget.Field(c, "Who edits them", func() {
				ui.Row(c).FillWidth().Children(func() {
					if i := widget.Segmented(c, []string{"Members", "Admins only"}, f.editors, nil); i >= 0 {
						f.editors = i
					}
				})
			})
		}
		if f.err != "" {
			widget.Notice(c, icon.Alert, theme.Error, f.err, "", nil)
		}
	}, func() {
		b := widget.PrimaryButton(c, nil, "Save", "")
		if f.saving || !f.loaded || f.draft.drafting || !f.rules.CanEdit && !a.isAdminOf(f.rules.Team) {
			b.Disabled(true)
		} else if b.Clicked() {
			a.saveRules(f, false)
		}
	})
}

// isAdminOf says the account is one of team's admins.
func (a *App) isAdminOf(team string) bool {
	for _, sp := range a.spacesOf().list {
		if sp.Team == team && team != "" {
			return canAdmin(sp.Role)
		}
	}
	return false
}

// rulesNotRead is a rules dialog before it has them: still reading, or why it
// couldn't, with nothing to edit or save over them.
func rulesNotRead(c *ui.Context, err string) {
	if err != "" {
		widget.Notice(c, icon.Alert, theme.Error, "The rules couldn't be read", err+" Nothing can change them until they are: close this and open it again.", nil)
		return
	}
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		widget.Spinner(c, 12)
		ui.Text(c, "Reading them…").FontSize(theme.TextLabel).TextColor(theme.Muted)
	})
}

func (a *App) saveRules(f *rulesForm, clear bool) {
	cl := a.client
	r := proto.ProjectRules{Path: f.project.Path, Rules: strings.TrimSpace(f.text)}
	if r.Rules == "" && strings.TrimSpace(f.rules.Rules) != "" && !clear {
		a.confirm("Delete every rule?", "Saving them empty removes all of "+f.project.Name+"'s rules for every session in it. An earlier version stays to bring back.", "Delete them", func() { a.saveRules(f, true) })
		return
	}
	r.Clear = clear
	if f.rules.Team != "" && a.isAdminOf(f.rules.Team) {
		r.Editors = rulesEditors[f.editors]
	}
	f.saving, f.err = true, ""
	go func() {
		_, err := cl.SaveProjectRules(r)
		a.onMain(func() {
			f.saving = false
			if err != nil {
				f.err = strings.TrimPrefix(client.Explain(err), "rules: ")
				return
			}
			f.open = false
			a.status = "Saved: new sessions in " + f.project.Name + " read them"
		})
	}()
}

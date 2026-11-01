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

// goaRulesForm is the user's rules for GOD of Agents being edited.
type goaRulesForm struct {
	open   bool
	text   string
	saved  string // the rules as read, to tell emptying them
	loaded bool
	saving bool
	err    string
	draft  rulesDraft
}

// openGOARules reads the rules into their dialog.
func (a *App) openGOARules() {
	cl := a.client
	if cl == nil {
		return
	}
	f := &goaRulesForm{open: true}
	a.goa.rules = f
	go func() {
		var r proto.GOARules
		err := cl.Call(proto.MethodGOARules, nil, &r)
		a.onMain(func() {
			if err != nil {
				// Not read: nothing here may write over them.
				f.err = client.Explain(err)
				return
			}
			f.loaded, f.saved, f.text = true, r.Rules, r.Rules
		})
	}()
}

func (a *App) goaRulesDialog(c *ui.Context) {
	f := a.goa.rules
	if f == nil {
		return
	}
	if !f.open {
		a.goa.rules = nil
		return
	}
	widget.FormSheet(c, &f.open, 640, "Rules of GOD of Agents", "How it delegates, what it leaves to you, what it tells its workers. Yours: every host's GOD of Agents follows them.", func() {
		if !f.loaded {
			rulesNotRead(c, f.err)
			return
		}
		ui.TextArea(c.Key("goa-rules"), &f.text).Height(240).Padding(10, 12).Radius(theme.RadiusControl).Font(theme.MonoFont).FontSize(theme.TextUI).
			Background(theme.Sunken).Border(1, theme.BorderStrong).
			Placeholder("- Give frontend tasks to Codex, the rest to Claude Code\n- Ask me before merging to main")
		ui.Text(c, "No credentials here: agents read these, and rules that look like they hold a secret are refused.").FontSize(theme.TextLabel).TextColor(theme.Muted)
		a.rulesDraftBox(c, &f.draft, proto.RulesForGOA, "", &f.text)
		if f.err != "" {
			widget.Notice(c, icon.Alert, theme.Error, f.err, "", nil)
		}
	}, func() {
		b := widget.PrimaryButton(c, nil, "Save", "")
		if f.saving || !f.loaded || f.draft.drafting {
			b.Disabled(true)
		} else if b.Clicked() {
			a.saveGOARules(f, false)
		}
	})
}

func (a *App) saveGOARules(f *goaRulesForm, clear bool) {
	cl := a.client
	if cl == nil {
		return
	}
	r := proto.GOARules{Rules: strings.TrimSpace(f.text)}
	if r.Rules == "" && strings.TrimSpace(f.saved) != "" && !clear {
		a.confirm("Delete every rule?", "Saving them empty removes all of GOD of Agents' rules. An earlier version stays to bring back.", "Delete them", func() { a.saveGOARules(f, true) })
		return
	}
	r.Clear = clear
	f.saving, f.err = true, ""
	go func() {
		err := cl.Call(proto.MethodGOARulesSave, r, nil)
		a.onMain(func() {
			f.saving = false
			if err != nil {
				f.err = strings.TrimPrefix(client.Explain(err), "rules: ")
				return
			}
			f.open = false
			a.status = "Saved: GOD of Agents follows them from its next prompt"
		})
	}()
}

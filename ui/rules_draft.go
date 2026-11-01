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

// rulesDraft is a rules dialog's asking an AI to write them: nothing is
// saved until the user reads the result and saves it.
type rulesDraft struct {
	ask      string
	drafting bool
	by       string // who wrote the text shown, "" when the user did
	before   string // the text before it, for Undo
	err      string
}

// rulesDraftBox lets the user say what they want and has an AI put it in
// text (rules of forWhat; path a project's folder).
func (a *App) rulesDraftBox(c *ui.Context, d *rulesDraft, forWhat, path string, text *string) {
	widget.Field(c, "Write them with AI", func() {
		ui.TextArea(c.Key("rules-ask"), &d.ask).Height(64).Padding(8, 10).Radius(theme.RadiusControl).FontSize(12.5).
			Background(theme.Sunken).Border(1, theme.BorderStrong).
			Placeholder("Say what you want in your own words, in any language: it writes them clearly and keeps the rules above.")
	})
	ui.Row(c.Key("rules-ai")).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
		b := widget.Button(c, widget.Secondary, icon.Sparkle, "Write with AI", "")
		if d.drafting || strings.TrimSpace(d.ask) == "" {
			b.Disabled(true)
		} else if b.Clicked() {
			a.draftRules(d, forWhat, path, text)
		}
		switch {
		case d.drafting:
			widget.Spinner(c, 12)
			ui.Text(c, "Writing…").FontSize(11.5).TextColor(theme.Muted)
		case d.by != "":
			ui.Text(c, "Written by "+d.by+": read them, then save.").FontSize(11.5).TextColor(theme.Muted)
			ui.Spacer(c)
			if widget.GhostButton(c, "Undo").Clicked() {
				*text, d.by = d.before, ""
			}
		}
	})
	if d.err != "" {
		widget.Notice(c, icon.Alert, theme.Error, d.err, "", nil)
	}
}

// draftRules asks the host's AI, on its own goroutine.
func (a *App) draftRules(d *rulesDraft, forWhat, path string, text *string) {
	cl := a.client
	if cl == nil {
		return
	}
	p := proto.RulesDraftParams{For: forWhat, Ask: d.ask, Current: *text, Path: path}
	d.drafting, d.err = true, ""
	go func() {
		r, err := cl.DraftRules(p)
		a.onMain(func() {
			d.drafting = false
			if err != nil {
				d.err = strings.TrimPrefix(strings.TrimPrefix(client.Explain(err), "rules: "), "decide: ")
				return
			}
			d.before, d.by, *text, d.ask = *text, r.By, r.Rules, ""
		})
	}()
}

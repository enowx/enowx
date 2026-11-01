package ui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// rulesDraft is a rules dialog's asking an AI for new rules. What comes back is
// only added after the text there, never in its place, and nothing is saved
// until the user reads it and saves. Changing or removing a rule is the user's,
// by hand.
type rulesDraft struct {
	ask       string
	drafting  bool
	by        string // who wrote what was added, "" when the user did
	before    string // the text before it, for Undo
	conflicts []string
	err       string
	versions  []proto.RulesVersion
	asked     bool
}

// appendRules is rules with added after them, those two exactly as they are,
// less lines of added that rules already hold. ok false: past the size rules
// may be, and nothing changed.
func appendRules(rules, added string) (string, bool) {
	have := map[string]bool{}
	for _, line := range strings.Split(rules, "\n") {
		have[strings.TrimSpace(line)] = true
	}
	var fresh []string
	for _, line := range strings.Split(added, "\n") {
		if t := strings.TrimSpace(line); t != "" && have[t] {
			continue
		}
		fresh = append(fresh, line)
	}
	add := strings.TrimSpace(strings.Join(fresh, "\n"))
	out := rules
	if add != "" {
		if strings.TrimSpace(rules) != "" {
			out = strings.TrimRight(rules, "\n") + "\n"
		}
		out += add
	}
	return out, len(out) <= proto.RulesMaxBytes
}

// rulesDraftBox lets the user say what they want and has an AI put it in
// text (rules of forWhat; path a project's folder).
func (a *App) rulesDraftBox(c *ui.Context, d *rulesDraft, forWhat, path string, text *string) {
	widget.Field(c, "Write them with AI", func() {
		ui.TextArea(c.Key("rules-ask"), &d.ask).Height(64).Padding(8, 10).Radius(theme.RadiusControl).FontSize(12.5).
			Background(theme.Sunken).Border(1, theme.BorderStrong).
			Placeholder("Say what to add, in your own words and any language: it writes clear rules and adds them after those above. To change or remove one, edit the text.")
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
				*text, d.by, d.conflicts = d.before, "", nil
			}
		}
	})
	for _, said := range d.conflicts {
		widget.Notice(c, icon.Alert, theme.Warning, "This may go against a rule above", said, nil)
	}
	if d.err != "" {
		widget.Notice(c, icon.Alert, theme.Error, d.err, "", nil)
	}
	a.rulesVersionsRow(c, d, forWhat, path, text)
}

// rulesVersionsRow brings back rules as they were before a save, into the
// editor, for the user to read and save.
func (a *App) rulesVersionsRow(c *ui.Context, d *rulesDraft, forWhat, path string, text *string) {
	if !d.asked {
		d.asked = true
		if cl := a.client; cl != nil {
			go func() {
				list, err := cl.RulesHistory(proto.RulesDraftParams{For: forWhat, Path: path})
				if err == nil { // an older daemon has none
					a.onMain(func() { d.versions = list })
				}
			}()
		}
	}
	if len(d.versions) == 0 {
		return
	}
	widget.Field(c, "Earlier versions", func() {
		ui.Column(c).FillWidth().Gap(2).Children(func() {
			for i, v := range d.versions[:min(len(d.versions), 5)] {
				b := ui.ButtonBase(c.Key("version:"+strconv.Itoa(i))).FillWidth().Padding(4, 6).Radius(theme.RadiusControl).Justify(ui.Start).Cursor(ui.CursorPointer).
					Label("Bring back the version of " + v.Time.Format("2 Jan 15:04"))
				b.Children(func() {
					ui.Text(c, v.Time.Format("2 Jan 15:04")+"  "+textfmt.FirstLine(v.Rules, 70)).FontSize(11.5).TextColor(theme.TextSoft).SingleLine()
				})
				if b.Clicked() {
					d.before, d.by, *text = *text, "an earlier version", v.Rules
				}
			}
		})
	})
}

// draftRules asks the host's AI, on its own goroutine.
func (a *App) draftRules(d *rulesDraft, forWhat, path string, text *string) {
	cl := a.client
	if cl == nil {
		return
	}
	p := proto.RulesDraftParams{For: forWhat, Ask: d.ask, Current: *text, Path: path}
	d.drafting, d.err, d.conflicts = true, "", nil
	go func() {
		r, err := cl.DraftRules(p)
		a.onMain(func() {
			d.drafting = false
			if err != nil {
				d.err = strings.TrimPrefix(strings.TrimPrefix(client.Explain(err), "rules: "), "decide: ")
				return
			}
			next, fits := appendRules(*text, r.Rules)
			if !fits {
				d.err = fmt.Sprintf("Added, the rules would be over %d KB, so nothing was: shorten them first.", proto.RulesMaxBytes>>10)
				return
			}
			d.before, d.by, *text, d.ask, d.conflicts = *text, r.By, next, "", r.Conflicts
		})
	}()
}

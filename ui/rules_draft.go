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

// rulesDraftBox is what asks an AI for rules, then their earlier versions, for a
// dialog that keeps the two together.
func (a *App) rulesDraftBox(c *ui.Context, d *rulesDraft, forWhat, path string, text *string) {
	a.rulesAskBox(c, d, forWhat, path, text)
	a.rulesVersionsRow(c, d, forWhat, path, text)
}

// rulesAskBox lets the user say what they want and has an AI put it in text
// (rules of forWhat; path a project's folder).
func (a *App) rulesAskBox(c *ui.Context, d *rulesDraft, forWhat, path string, text *string) {
	widget.Field(c, "What you want, in your words", func() {
		ask := ui.TextArea(c.Key("rules-ask"), &d.ask).Height(72).Padding(theme.S2, theme.S3).Radius(theme.RadiusCard).FontSize(theme.TextUI).
			Placeholder("Say what to add, in any language: it writes clear rules and adds them after the ones there. To change or remove one, edit the text.")
		widget.Well(ask, ask.Focused())
	})
	ui.Row(c.Key("rules-ai")).FillWidth().Shrink(0).Gap(theme.S2).Children(func() {
		b := widget.Button(c, widget.Secondary, icon.Sparkle, "Write with AI", "")
		if d.drafting || strings.TrimSpace(d.ask) == "" {
			b.Disabled(true)
		} else if b.Clicked() {
			a.draftRules(d, forWhat, path, text)
		}
		switch {
		case d.drafting:
			widget.Spinner(c, 12)
			ui.Text(c, "Writing…").FontSize(theme.TextLabel).TextColor(theme.TextSoft)
		case d.by != "":
			ui.Row(c).Grow(1).MinWidth(0).Children(func() {
				widget.Running(c, "Written by "+d.by+": read them, then save.", widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
			})
			if widget.GhostButton(c, "Undo").Clicked() {
				*text, d.by, d.conflicts = d.before, "", nil
			}
		}
	})
	for _, said := range d.conflicts {
		widget.Notice(c.Key(said), icon.Alert, theme.Warning, "This may go against a rule", said, nil)
	}
	if d.err != "" {
		widget.Notice(c, icon.Alert, theme.Error, d.err, "", nil)
	}
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
		ui.Column(c).FillWidth().Gap(theme.S1).Children(func() {
			for i, v := range d.versions[:min(len(d.versions), 5)] {
				b := ui.ButtonBase(c.Key("version:" + strconv.Itoa(i))).FillWidth().Height(28).PaddingX(theme.S2).Radius(theme.RadiusFull).Justify(ui.Start).Cursor(ui.CursorPointer).
					Label("Bring back the version of " + v.Time.Format("2 Jan 15:04"))
				if b.Hovered() {
					b.Background(theme.Hover)
				}
				b.Children(func() {
					ui.Row(c).Grow(1).MinWidth(0).Children(func() {
						widget.Running(c, v.Time.Format("2 Jan 15:04")+"  "+textfmt.FirstLine(v.Rules, 70), widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
					})
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

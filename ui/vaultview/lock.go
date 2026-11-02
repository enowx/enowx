package vaultview

import (
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// create is the page before there is a vault: a master password, twice.
func (v *View) create(c *ui.Context, o Options) {
	v.hero(c, icon.Shield, "One vault for your logins",
		"Keep tokens, keys and passwords for GitHub, your clouds, databases and AI APIs once, encrypted on this device. Sessions on any host get what they need without logging in again: the first time on each host, you allow it.", func() {
			widget.Field(c, "Master password", func() {
				widget.Input(c, &v.password, "At least 8 characters").FillWidth().Password().AutoFocus()
			})
			var submit bool
			widget.Field(c, "Again", func() {
				submit = widget.Input(c, &v.confirm, "The same").FillWidth().Password().Submitted()
			})
			widget.ToggleRow(c, "Remember on this device", rememberNote, &v.remember)
			v.errorLine(c)
			widget.CardNote(c, "There is no way back in without it: enowX can't recover a forgotten master password.", 0).TextColor(theme.Warning)
			ui.Row(c).FillWidth().Shrink(0).Justify(ui.End).Children(func() {
				if widget.PrimaryButton(c, nil, "Make the vault", "").Clicked() || submit {
					switch {
					case len(v.password) < 8:
						v.err = "The master password needs at least 8 characters."
					case v.password != v.confirm:
						v.err = "The two passwords differ."
					default:
						v.err = errText(o.Actions.Create(v.password, v.remember))
						if v.err == "" {
							v.password, v.confirm = "", ""
						}
					}
				}
			})
		})
}

const rememberNote = "Its key is kept in the system's keychain, so the vault opens without the password; it still locks after a while unused."

// unlock is the page while the vault is locked.
func (v *View) unlock(c *ui.Context, o Options) {
	if !v.remember && o.Remember {
		v.remember = true
	}
	widget.Card(c).Width(heroWidth).Shrink(1).Children(func() {
		ui.Row(c).FillWidth().Gap(theme.S2).Justify(ui.Center).Children(func() {
			ui.Text(c, "The vault is locked").FontSize(theme.TextRead).FontWeight(600)
			ui.Icon(c, icon.Lock).FontSize(16).TextColor(theme.TextSoft)
		})
		ui.Text(c, "Unlock it to see your credentials and lend them to sessions.").FontSize(theme.TextUI).TextColor(theme.TextSoft).FillWidth().TextAlign(ui.Center)
		submit := widget.Input(c, &v.password, "Your master password").FillWidth().Password().AutoFocus().Submitted()
		widget.ToggleRow(c, "Remember on this device", "", &v.remember)
		v.errorLine(c)
		if widget.PrimaryButton(c, nil, "Unlock", "").FillWidth().Clicked() || submit {
			v.err = errText(o.Actions.Unlock(v.password, v.remember))
			v.password = "" // wrong: typed again from the start
		}
	})
}

// heroWidth is how wide the vault's cards are while there's no list.
const heroWidth = 420

// hero is a centered card: its title and mark, a line and a form.
func (v *View) hero(c *ui.Context, svg *ui.SVG, title, about string, body func()) {
	widget.Card(c).Width(heroWidth).Shrink(1).Children(func() {
		widget.CardTitle(c, nil, title, func() { ui.Icon(c, svg).FontSize(16).TextColor(theme.TextSoft) })
		widget.CardNote(c, about, 0)
		body()
	})
}

func (v *View) errorLine(c *ui.Context) {
	if v.err != "" {
		widget.CardNote(c, v.err, 0).TextColor(theme.Error)
	}
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// change is the Change master password dialog.
type change struct {
	open                 bool
	old, password, again string
	err                  string
}

func (v *View) changeDialog(c *ui.Context, o Options) {
	f := v.change
	if f == nil || !f.open {
		v.change = nil
		return
	}
	widget.FormSheet(c, &f.open, 480, "Change master password", "The vault's key stays the same: hosts' copies open with the new password once they have the vault again.", func() {
		widget.Field(c, "Current password", func() { widget.Input(c, &f.old, "").FillWidth().Password().AutoFocus() })
		widget.Field(c, "New password", func() { widget.Input(c, &f.password, "At least 8 characters").FillWidth().Password() })
		widget.Field(c, "Again", func() { widget.Input(c, &f.again, "").FillWidth().Password() })
		if f.err != "" {
			ui.Text(c, f.err).FontSize(theme.TextLabel).TextColor(theme.Error)
		}
	}, func() {
		if widget.PrimaryButton(c, nil, "Change it", "").Clicked() {
			switch {
			case f.password != f.again:
				f.err = "The two new passwords differ."
			default:
				if f.err = errText(o.Actions.ChangePassword(f.old, f.password)); f.err == "" {
					v.change = nil
				}
			}
		}
	})
}

// Ask is a question the vault puts to the user: a host wants a
// credential, an agent wants to add or remove one, or the vault must be
// unlocked to answer.
type Ask struct {
	Title, About string
	Host         string // the host's name, for "Always on HOST"
	Locked       bool   // the vault must be unlocked first
	Always       bool   // offer Always on Host
	Danger       bool
}

// Answers to an Ask.
const (
	AnswerNone = iota
	AnswerDeny
	AnswerOnce
	AnswerAlways
)

// AskCard is ask as a card for the app to float: answer takes the user's
// answer. Unlocking waits for a click into the password field: the card must
// never take the keyboard (widget.AskCard).
func (v *View) AskCard(c *ui.Context, key string, ask Ask, unlock func(password string) error, answer func(int)) *widget.Ask {
	card := &widget.Ask{Key: key, Icon: icon.Key, Title: ask.Title, About: ask.About}
	refuse := widget.AskButton{Kind: widget.Secondary, Label: "Refuse", Do: func() { answer(AnswerDeny) }}
	try := func() {
		v.err = errText(unlock(v.password))
		if v.err == "" {
			v.password = ""
		}
	}
	switch {
	case ask.Locked:
		card.Body = func() {
			widget.Field(c, "Master password", func() {
				if widget.Input(c, &v.password, "Unlock the vault to answer").FillWidth().Password().Submitted() {
					try()
				}
			})
			v.errorLine(c)
		}
		card.Buttons = []widget.AskButton{refuse, {Kind: widget.Primary, Icon: icon.Lock, Label: "Unlock and answer", Do: try}}
	case ask.Always:
		card.Buttons = []widget.AskButton{refuse,
			{Kind: widget.Secondary, Label: "Allow once", Do: func() { answer(AnswerOnce) }},
			{Kind: widget.Primary, Label: "Always on " + ask.Host, Do: func() { answer(AnswerAlways) }}}
	case ask.Danger:
		card.Buttons = []widget.AskButton{refuse, {Kind: widget.Danger, Label: "Remove", Do: func() { answer(AnswerOnce) }}}
	default:
		card.Buttons = []widget.AskButton{refuse, {Kind: widget.Primary, Label: "Allow", Do: func() { answer(AnswerOnce) }}}
	}
	return card
}

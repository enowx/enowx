package ui

import (
	"errors"

	"github.com/egoist/mygo/ui"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// passphrasePrompt asks for the passphrase of an SSH key; the connecting
// goroutine waits on answer. Passphrases are never saved.
type passphrasePrompt struct {
	path   string
	value  string
	answer chan passphraseAnswer
}

type passphraseAnswer struct {
	value string
	err   error
}

// askPassphrase runs on the connecting goroutine and waits for the user.
func (a *App) askPassphrase(gen int, path string) (string, error) {
	answer := make(chan passphraseAnswer, 1)
	a.onMain(func() {
		if gen != a.gen {
			answer <- passphraseAnswer{err: errConnectCancelled}
			return
		}
		a.passphrase = &passphrasePrompt{path: path, answer: answer}
	})
	r := <-answer
	return r.value, r.err
}

func (a *App) answerPassphrase(ok bool) {
	p := a.passphrase
	if p == nil {
		return
	}
	if ok {
		p.answer <- passphraseAnswer{value: p.value}
	} else {
		p.answer <- passphraseAnswer{err: errors.New("no passphrase for " + p.path)}
	}
	a.passphrase = nil
}

func (a *App) passphraseDialog(c *ui.Context) {
	p := a.passphrase
	if p == nil || a.connectByApp {
		return
	}
	open := true
	widget.Dialog(c, &open, 480, 14, func() {
		widget.DialogHeader(c, "Unlock your SSH key", "", false, "Hosts", a.host.Name, "Unlock SSH key")
		passphraseFacts(c, p)
		submit := widget.Input(c, &p.value, "Passphrase").FillWidth().Password().AutoFocus().Submitted()
		widget.DialogFooter(c, "", func() {
			if widget.GhostButton(c, "Cancel").Clicked() {
				a.answerPassphrase(false)
			}
			if widget.PrimaryButton(c, nil, "Unlock and connect", "").Clicked() || submit {
				a.answerPassphrase(true)
			}
		})
	})
	if !open {
		a.answerPassphrase(false)
	}
}

// passphraseAsk is passphraseDialog on a connection the user didn't start: the
// field waits for their click.
func (a *App) passphraseAsk(c *ui.Context) *widget.Ask {
	p := a.passphrase
	if p == nil || !a.connectByApp {
		return nil
	}
	return &widget.Ask{Key: "passphrase:" + p.path, Icon: icon.Key, Title: "Unlock your SSH key",
		Body: func() {
			passphraseFacts(c, p)
			if widget.Input(c, &p.value, "Passphrase").FillWidth().Password().Submitted() {
				a.answerPassphrase(true)
			}
		},
		Buttons: []widget.AskButton{
			{Kind: widget.Secondary, Label: "Cancel", Do: func() { a.answerPassphrase(false) }},
			{Kind: widget.Primary, Label: "Unlock and connect", Do: func() { a.answerPassphrase(true) }},
		}}
}

func passphraseFacts(c *ui.Context, p *passphrasePrompt) {
	ui.Text(c, "The server accepts this key. Type its passphrase to sign in; enowX doesn't save it.").
		FontSize(12.5).TextColor(theme.TextSoft)
	ui.Text(c, textfmt.ShortHome(p.path)).Font(theme.MonoFont).FontSize(12).Padding(10).Radius(theme.RadiusControl).
		Background(theme.Sunken).Border(1, theme.Border).FillWidth()
}

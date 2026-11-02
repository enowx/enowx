package ui

import (
	"errors"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/hosts"
	"github.com/enowx/enowx/transport"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// signInPrompt asks again how to sign in after a server refused: another
// user, or a password, as ssh would prompt for.
type signInPrompt struct {
	host     hosts.Host
	refusal  *transport.AuthError
	user     string
	password string
	remember bool
	err      string
}

// promptSignIn opens the sign-in dialog when err is a refused sign-in.
func (a *App) promptSignIn(h hosts.Host, err error) {
	var refusal *transport.AuthError
	if h.ID == hosts.LocalID || !errors.As(err, &refusal) {
		return
	}
	a.signIn = &signInPrompt{host: h, refusal: refusal, user: refusal.User, remember: true}
}

func (a *App) submitSignIn() {
	p := a.signIn
	user := strings.TrimSpace(p.user)
	if user == "" || strings.ContainsAny(user, "@ ") {
		p.err = "Type the user to sign in as."
		return
	}
	if p.refusal.AcceptsPassword() && p.password == "" {
		p.err = "Type the password."
		return
	}
	t, err := transport.ParseSSHTarget(p.host.Address)
	if err != nil {
		p.err = err.Error()
		return
	}
	t.User = user
	h := p.host
	h.Address = t.String()
	password := p.password
	if password != "" && p.remember {
		h.PasswordAuth = true
	}
	if _, stored := a.storedHost(h.ID); stored {
		if err := a.opts.Hosts.Update(h); err != nil {
			p.err = err.Error()
			return
		}
		a.reloadHosts()
	}
	a.signIn = nil
	conn := h
	if password != "" && !p.remember {
		// Kept for this run of the app only.
		if a.passwords == nil {
			a.passwords = map[string]string{}
		}
		a.passwords[h.ID] = password
		conn.PasswordAuth = true
	}
	a.connect(conn)
	if password != "" && p.remember {
		// After connect, which clears the status bar: a warning about the
		// keychain must stay visible.
		a.savePassword(h, password)
	}
}

func (a *App) signInDialog(c *ui.Context) {
	p := a.signIn
	if p == nil || a.connectByApp {
		return
	}
	open := true
	submit := false
	widget.FormSheet(c, &open, 480, "Sign in to "+p.host.Name, "", func() {
		submit = signInFields(c, p, true)
	}, func() {
		if widget.PrimaryButton(c, nil, "Sign in", "").Clicked() || submit {
			a.submitSignIn()
		}
	})
	if !open {
		a.signIn = nil
	}
}

// signInAsk is signInDialog on a connection the user didn't start: the fields
// wait for their click.
func (a *App) signInAsk(c *ui.Context) *widget.Ask {
	p := a.signIn
	if p == nil || !a.connectByApp {
		return nil
	}
	return &widget.Ask{Key: "signin:" + p.host.ID, Title: "Sign in to " + p.host.Name,
		Body: func() {
			if signInFields(c, p, false) {
				a.submitSignIn()
			}
		},
		Buttons: []widget.AskButton{
			{Kind: widget.Secondary, Label: "Cancel", Do: func() { a.signIn = nil }},
			{Kind: widget.Primary, Label: "Sign in", Do: a.submitSignIn},
		}}
}

// signInFields is what signInDialog says and asks; focus has a field take the
// keyboard, in a dialog the user's own connecting opened. submit is Enter in a
// field.
func signInFields(c *ui.Context, p *signInPrompt, focus bool) (submit bool) {
	what := "your SSH keys"
	if p.refusal.TriedPassword {
		what = "your SSH keys or the saved password"
	}
	widget.CardNote(c.Key("refused"), p.refusal.Addr+" didn't accept "+what+" for "+p.refusal.User+".", 0).FontSize(theme.TextUI)
	widget.Field(c, "User", func() {
		in := widget.Input(c, &p.user, "root").FillWidth()
		if focus && !p.refusal.AcceptsPassword() {
			in.AutoFocus()
		}
		submit = in.Submitted()
	})
	if p.refusal.AcceptsPassword() {
		widget.Field(c, "Password", func() {
			in := widget.Input(c, &p.password, "The user's password").FillWidth().Password()
			if focus {
				in.AutoFocus()
			}
			submit = in.Submitted() || submit
		})
		widget.ToggleRow(c.Key("remember"), "Remember the password in the keychain", "", &p.remember)
	} else {
		widget.CardNote(c.Key("keys-only"), "This server accepts SSH keys only. Add your public key (~/.ssh/id_ed25519.pub) to the user's ~/.ssh/authorized_keys, or allow passwords in the server's /etc/ssh/sshd_config.", 0)
	}
	if p.err != "" {
		widget.Notice(c, icon.Alert, theme.Error, p.err, "", nil)
	}
	return submit
}

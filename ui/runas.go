package ui

import (
	"errors"
	"slices"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/hosts"
	"github.com/enowx/enowx/hostconn"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// defaultRunAs is the user offered to root logins.
const defaultRunAs = "enowx"

// runAsPrompt asks what to do when SSH signs in as root; the connecting
// goroutine waits on answer.
type runAsPrompt struct {
	host   hosts.Host
	users  []hostconn.ServerUser // the server's regular users
	choice string                // one of users, or "" for a new user
	user   string                // the new user's name
	err    string
	answer chan runAsAnswer
}

type runAsAnswer struct {
	host hosts.Host
	err  error
}

var errConnectCancelled = errors.New("connection cancelled")

// askRunAs runs on the connecting goroutine and waits for the user.
// A user enowX already ran as, from this device or another, is chosen
// without asking, so connecting again never makes another user.
func (a *App) askRunAs(gen int, h hosts.Host, users []hostconn.ServerUser) (hosts.Host, error) {
	answer := make(chan runAsAnswer, 1)
	a.onMain(func() {
		if gen != a.gen {
			answer <- runAsAnswer{err: errConnectCancelled}
			return
		}
		a.runAs = &runAsPrompt{host: h, users: users, user: defaultRunAs, answer: answer}
		if slices.ContainsFunc(users, func(u hostconn.ServerUser) bool { return u.Name == defaultRunAs }) {
			a.runAs.choice = defaultRunAs // made by enowX before, its daemon since removed
		}
		switch u, ok := knownServerUser(users); {
		case a.chooseRunAs == h.ID:
			// The user asked to choose: ask, even with a user set up.
			if ok {
				a.runAs.choice = u
			}
		case ok:
			a.answerRunAs(u, false)
			a.notify("Running as " + u + ", which enowX set up on " + h.Name + " before. To run as root: Hosts › ⋯ › Edit… › Run sessions as.")
		case a.chooseRunAs != h.ID:
			// Without asking: agents won't skip permission prompts as root,
			// so enowX runs as a user of its own, made the first time.
			a.answerRunAs(defaultRunAs, false)
			a.notify("Running as " + defaultRunAs + " on " + h.Name + ", a user enowX made for its sessions. To run as root: Hosts › ⋯ › Edit… › Run sessions as.")
		}
	})
	r := <-answer
	return r.host, r.err
}

// answerRunAs saves the choice for next time and lets the connection go on.
func (a *App) answerRunAs(user string, keepRoot bool) {
	p := a.runAs
	if p == nil {
		return
	}
	h := p.host
	h.RunAs, h.KeepRoot = user, keepRoot
	if !keepRoot && !hosts.ValidUserName(user) {
		p.err = "Use lowercase letters, digits, - and _, starting with a letter."
		return
	}
	if a.opts.Hosts != nil {
		if _, ok := a.opts.Hosts.Get(h.ID); ok {
			if err := a.opts.Hosts.Update(h); err != nil {
				p.err = err.Error()
				return
			}
			a.reloadHosts()
		}
	}
	a.host = h
	p.answer <- runAsAnswer{host: h}
	a.runAs, a.chooseRunAs = nil, ""
}

// knownServerUser picks the user enowX ran as before: enowx if it did,
// else the first one.
func knownServerUser(users []hostconn.ServerUser) (string, bool) {
	var known []string
	for _, u := range users {
		if u.HasDaemon {
			known = append(known, u.Name)
		}
	}
	if len(known) == 0 {
		return "", false
	}
	if slices.Contains(known, defaultRunAs) {
		return defaultRunAs, true
	}
	return known[0], true
}

func (a *App) cancelRunAs() {
	if a.runAs != nil {
		a.runAs.answer <- runAsAnswer{err: errConnectCancelled}
		a.runAs = nil
	}
}

func (a *App) runAsDialog(c *ui.Context) {
	p := a.runAs
	if p == nil || a.connectByApp {
		return
	}
	open := true
	widget.Dialog(c, &open, 540, 14, func() {
		widget.DialogHeader(c, "Run agents as a regular user?", "", false, "Hosts", a.host.Name, "Run as")
		submit := runAsFacts(c, p)
		widget.DialogFooter(c, "", func() {
			if widget.GhostButton(c, "Cancel").Clicked() {
				a.cancelRunAs()
			}
			if widget.GhostButton(c, "Keep root").Clicked() {
				a.answerRunAs("", true)
			}
			label, user := "Create user and connect", p.user
			if p.choice != "" {
				label, user = "Connect as "+p.choice, p.choice
			}
			if widget.PrimaryButton(c, nil, label, "").Clicked() || submit {
				a.answerRunAs(user, false)
			}
		})
	})
	if !open {
		a.cancelRunAs()
	}
}

// runAsAsk is runAsDialog on a connection the user didn't start.
func (a *App) runAsAsk(c *ui.Context) *widget.Ask {
	p := a.runAs
	if p == nil || !a.connectByApp {
		return nil
	}
	label, user := "Create user and connect", p.user
	if p.choice != "" {
		label, user = "Connect as "+p.choice, p.choice
	}
	return &widget.Ask{Key: "runas:" + p.host.ID, Title: "Run agents as a regular user?",
		Body: func() {
			if runAsFacts(c, p) {
				a.answerRunAs(user, false)
			}
		},
		Buttons: []widget.AskButton{
			{Kind: widget.Secondary, Label: "Cancel", Do: a.cancelRunAs},
			{Kind: widget.Secondary, Label: "Keep root", Do: func() { a.answerRunAs("", true) }},
			{Kind: widget.Primary, Label: label, Do: func() { a.answerRunAs(user, false) }},
		}}
}

// runAsFacts is what runAsDialog says and asks; submit is Enter in its field.
func runAsFacts(c *ui.Context, p *runAsPrompt) (submit bool) {
	ui.Text(c, "You signed in to "+p.host.Name+" as root. Claude Code won't skip permission prompts as root, and agents running as root can change anything on the server. "+
		"enowX can run its daemon and every session as a regular user instead. SSH still signs in as root.").
		FontSize(12.5).TextColor(theme.TextSoft)
	if len(p.users) > 0 {
		ui.Column(c).FillWidth().Gap(6).Children(func() {
			for _, u := range p.users {
				if widget.OptionCard(c.Key(u.Name), p.choice == u.Name, u.Name, u.Home) {
					p.choice = u.Name
				}
			}
			if widget.OptionCard(c.Key("+new"), p.choice == "", "New user", "A user of its own, without sudo") {
				p.choice = ""
			}
		})
	}
	if p.choice == "" {
		submit = widget.Input(c, &p.user, defaultRunAs).FillWidth().Font(theme.MonoFont).Submitted()
	} else {
		ui.Text(c, "Agents get "+p.choice+"'s permissions: if it can use sudo, so can they.").FontSize(12).TextColor(theme.Muted)
	}
	if p.err != "" {
		ui.Text(c, p.err).FontSize(12).TextColor(theme.Error)
	}
	ui.Text(c, "Afterwards, sign in to each agent once from a Shell session (it runs as that user), and keep projects in that user's home. "+
		"Agents installed only for root, for example with nvm, need installing again.").FontSize(12).TextColor(theme.Muted)
	return submit
}

// runAsNote says who enowX runs as on a root login, once the user chose.
func runAsNote(h hosts.Host) string {
	switch {
	case h.RunAs != "":
		return "Runs as " + h.RunAs + " when signed in as root"
	case h.KeepRoot:
		return "Runs as root"
	}
	return ""
}

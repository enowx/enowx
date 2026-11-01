package ui

import (
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// Signing in to enowX's cloud, in a dialog: opened from the rail's Sign in
// or Settings › Cloud, it asks for a code straight away and opens the
// browser on it, waits there, and says who is signed in. Closed while it
// waits, the sign-in is dropped.

// openCloudSignIn opens the dialog and starts signing the connected host
// in.
func (a *App) openCloudSignIn() {
	cs := a.cloudOf()
	a.cloudSigningIn = true
	if !cs.st.SignedIn && cs.st.Pending == nil {
		a.cloudSignIn()
	}
}

func (a *App) cloudSignInDialog(c *ui.Context) {
	if !a.cloudSigningIn {
		return
	}
	cs := a.cloudOf()
	open := true
	widget.Dialog(c, &open, 480, theme.S3, func() {
		st := cs.st
		switch {
		case st.SignedIn:
			if widget.DialogHeader(c, "You're signed in", "", true) {
				open = false
			}
			ui.Row(c).FillWidth().Gap(theme.S3).Padding(theme.S3, theme.S4).Radius(theme.RadiusCard).Background(theme.Sunken).Children(func() {
				ui.Box(c).Size(32, 32).Radius(theme.RadiusFull).Background(theme.Fade(theme.Done, 0.16)).Center().Shrink(0).Children(func() {
					ui.Icon(c, icon.Check).FontSize(16).TextColor(theme.Done)
				})
				ui.Column(c).Grow(1).MinWidth(0).Children(func() {
					who := st.Account
					if st.GitHub != "" {
						who += " · @" + st.GitHub
					}
					widget.Running(c, who, widget.RunStyle{Weight: 600})
					widget.Running(c.Key("device"), a.host.Name+", signed in as "+deviceKind(st.DeviceKind), widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
				})
			})
			widget.DialogFooter(c, "", func() {
				if widget.GhostButton(c, "Teams and devices").Clicked() {
					a.cloudSigningIn = false
					a.page, a.settingsSection = pageSettings, sectionCloud
				}
				if widget.PrimaryButton(c, nil, "Done", "").Clicked() {
					a.cloudSigningIn = false
				}
			})
		case st.Pending != nil:
			p := st.Pending
			a.openCloudPage(false) // once per code
			if widget.DialogHeader(c, "Sign in to enowX Cloud", "Your browser opened on this code: approve it there, signed in with GitHub.", true) {
				open = false
			}
			ui.Column(c).FillWidth().Gap(theme.S2).Padding(theme.S4).Radius(theme.RadiusCard).Background(theme.Sunken).AlignItems(ui.Center).Children(func() {
				ui.Text(c, p.UserCode).Font(theme.MonoFont).FontSize(24).FontWeight(500).LetterSpacing(2).SingleLine()
				ui.Row(c).Gap(theme.S2).Children(func() {
					widget.Spinner(c, 12)
					ui.Text(c, "Waiting for you, until "+p.Expires.Local().Format("15:04")).FontSize(theme.TextLabel).TextColor(theme.TextSoft)
				})
			})
			c.After(time.Second)
			widget.DialogFooter(c, "", func() {
				if widget.GhostButton(c, "Cancel").Clicked() {
					a.cloudCancel()
					a.cloudSigningIn = false
				}
				if p.VerifyURLComplete != "" && widget.Button(c, widget.Secondary, icon.External, "Open the page again", "").Clicked() {
					a.openCloudPage(true)
				}
			})
		default:
			if widget.DialogHeader(c, "Sign in to enowX Cloud", "Your teams in enowX: their members, the notes and canvases they share, their hosts.", true) {
				open = false
			}
			if cs.err != "" {
				widget.Notice(c, icon.Alert, theme.Error, cs.err, "", nil)
			}
			if cs.busy != "" {
				ui.Row(c).Gap(theme.S2).Children(func() {
					widget.Spinner(c, 14)
					ui.Text(c, cs.busy).FontSize(theme.TextLabel).TextColor(theme.TextSoft)
				})
			}
			widget.DialogFooter(c, "", func() {
				if widget.GhostButton(c, "Cancel").Clicked() {
					a.cloudSigningIn = false
				}
				if widget.PrimaryButton(c, icon.Account, "Continue with GitHub", "").Clicked() && cs.busy == "" {
					a.cloudSignIn()
				}
			})
		}
	})
	if !open {
		// Closed with Esc or outside it: a sign-in waiting goes with it.
		if cs.st.Pending != nil {
			a.cloudCancel()
		}
		a.cloudSigningIn = false
	}
}

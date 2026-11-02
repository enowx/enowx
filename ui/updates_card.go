package ui

import (
	"fmt"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
	"github.com/enowx/enowx/version"
)

// cornerCards are the cards over the window's bottom right corner: a new
// version, teams deleted, then ports a session opened.
func (a *App) cornerCards(c *ui.Context) {
	if !a.showsUpdateCard() && !a.spacesGoneShown() && !a.showsVideoCard() && (len(a.portOffers) == 0 || a.client == nil) {
		return
	}
	ui.Column(c.Key("corner-cards")).Absolute().Right(theme.S4).Bottom(40).Width(340).Gap(theme.S2).TextColor(theme.Text).Children(func() {
		a.videoCard(c.Key("video"))
		a.updateCard(c.Key("update"))
		a.spacesGoneCards(c)
		a.portOfferCards(c)
	})
}

// showsUpdateCard reports whether the corner shows the update: a version
// found, until put off to the next launch, and not while its dialog shows.
func (a *App) showsUpdateCard() bool {
	u := &a.updates
	return u.found != nil && !u.later && !u.dialog &&
		(u.state == updateAvailable || u.state == updateDownloading || u.state == updateReady)
}

// updateCard offers the version found: install it now, or later, which
// puts it off until the app starts again.
func (a *App) updateCard(c *ui.Context) {
	if !a.showsUpdateCard() {
		return
	}
	u := &a.updates
	v := u.found
	widget.Float(ui.Column(c).FillWidth().Padding(theme.S3).Gap(theme.S2).Radius(theme.RadiusCard).
		Background(theme.Dialog).Border(1, theme.BorderStrong)).Transition(widget.OverlayMotion).Children(func() {
		ui.Row(c).FillWidth().Gap(theme.S2).AlignItems(ui.Start).Children(func() {
			ui.Icon(c, icon.ArrowUp).FontSize(14).TextColor(theme.Now).Margin(2, 0, 0, 0)
			ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
				title, detail := "enowX "+v.Version+" is available", "You have "+version.String+"."
				switch u.state {
				case updateDownloading:
					title, detail = "Updating to "+v.Version+"…", "Downloading and checking its signature."
					if u.total > 0 {
						title = fmt.Sprintf("Updating to %s · %d%%", v.Version, u.done*100/u.total)
					}
				case updateReady:
					title, detail = "enowX "+v.Version+" is installed", "Restart to run it: your sessions come back, agents on their conversations."
				}
				widget.Running(c, title, widget.RunStyle{Weight: 500})
				ui.Text(c, detail).FontSize(theme.TextLabel).TextColor(theme.TextSoft)
				if u.err != "" {
					ui.Text(c, "The update failed: "+u.err).FontSize(theme.TextLabel).TextColor(theme.Error).MaxLines(3)
				}
			})
		})
		if u.state == updateDownloading {
			frac := 0.0
			if u.total > 0 {
				frac = float64(u.done) / float64(u.total)
			}
			widget.Meter(c, widget.Segment{Fraction: frac, Color: theme.Accent})
		}
		ui.Row(c).FillWidth().Gap(theme.S2).Justify(ui.End).Children(func() {
			switch u.state {
			case updateAvailable:
				if widget.GhostButton(c, "What's new").Clicked() {
					u.dialog = true
				}
				if widget.GhostButton(c, "Later").Clicked() {
					u.later = true
				}
				label := "Update now"
				if u.err != "" {
					label = "Try again"
				}
				if widget.PrimaryButton(c, icon.ArrowUp, label, "").Clicked() {
					a.installUpdate()
				}
			case updateDownloading:
				if widget.GhostButton(c, "Cancel").Clicked() && u.cancel != nil {
					u.cancel()
				}
			case updateReady:
				if widget.GhostButton(c, "Later").Clicked() {
					u.later = true
				}
				if widget.PrimaryButton(c, icon.Restore, "Restart now", "").Clicked() {
					u.source.Relaunch()
				}
			}
		})
	})
}

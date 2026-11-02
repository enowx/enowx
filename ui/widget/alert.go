package widget

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/theme"
)

// AlertDialog asks a question in a small dialog while *open, as ui.AlertDialog
// does, with this app's look: the last button is the action, red when it
// destroys something, and takes the keyboard. It returns the index of the
// button clicked in the frame it is, which closes it, ui.AlertDismissed in the
// frame Escape closes it, and -1 otherwise. A click outside does nothing.
func AlertDialog(c *ui.Context, open *bool, title, message string, buttons ...string) int {
	if !*open {
		return -1
	}
	chosen, dismissed := -1, false
	ui.Overlay(c, func() {
		back := ui.Box(c).Absolute().Left(0).Top(0).Right(0).Bottom(0).Center().Background(theme.Scrim).Modal()
		dismissed = back.OverlayShortcut(0, ui.KeyEscape)
		back.Children(func() {
			w, h := c.Size()
			panel := Float(ui.Column(c).Width(min(420, max(280, w-32))).MaxHeight(max(0, h-32)).Radius(theme.RadiusPanel).Clip().Background(theme.Dialog)).
				Label(title).Description(message).Transition(OverlayMotion)
			panel.Children(func() {
				ui.Column(c).FillWidth().Padding(theme.S4, theme.S5, theme.S2, theme.S5).Gap(theme.S1).Children(func() {
					ui.Text(c, title).FontSize(theme.TextTitle).FontWeight(600)
					if message != "" {
						ui.Text(c, message).FontSize(theme.TextUI).TextColor(theme.Text)
					}
				})
				ui.Row(c).FillWidth().Padding(theme.S3, theme.S4, theme.S4, theme.S4).Gap(theme.S2).Wrap().Justify(ui.End).Children(func() {
					for i, label := range buttons {
						kind := Secondary
						if i == len(buttons)-1 {
							kind = Primary
							if destroys(label) {
								kind = Danger
							}
						}
						b := Button(c.Key(label), kind, nil, label, "")
						if i == len(buttons)-1 {
							b.AutoFocus()
						}
						if b.Clicked() {
							chosen = i
						}
					}
				})
			})
		})
	})
	if dismissed && chosen < 0 {
		chosen = ui.AlertDismissed
	}
	if chosen != -1 {
		*open = false
	}
	return chosen
}

// destroys says an action's label is of one that can't be taken back.
func destroys(label string) bool {
	verb, _, _ := strings.Cut(label, " ")
	switch strings.TrimSuffix(verb, "…") {
	case "Delete", "Remove", "Discard", "Clear", "Stop", "Leave", "Revoke", "Forget", "Reset", "Overwrite", "Replace", "Sign":
		return true
	}
	return false
}

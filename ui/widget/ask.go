package widget

import (
	"strconv"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
)

const (
	askWidth = 460
	askTop   = 44 // under the title bar
)

// Ask is a question raised without the user's action: an agent's, a
// daemon's, or a connection the app started by itself.
type Ask struct {
	Key          string // the question's identity: another one is other controls
	Icon         *ui.SVG
	Title, About string
	Body         func() // what else it shows; a field in it is the user's to click into
	Buttons      []AskButton
}

// AskButton is one answer of an Ask. Those approving nothing come first,
// so a Tab into the card stops at them first.
type AskButton struct {
	Kind     ButtonKind
	Icon     *ui.SVG
	Label    string
	Disabled bool
	Do       func()
}

// AskCard floats ask over the window, with more waiting after it. It is not a
// dialog: MyGo moves the focus into a modal the frame it appears and holds the
// window's shortcuts, so a modal would take the keyboard from what the user is
// typing. The card takes nothing and blocks nothing: its controls are reached
// by a click, or with Tab, last in the window's focus order. While not armed,
// its answers do nothing: a card just shown may be under a click meant for
// what was there.
func AskCard(c *ui.Context, ask *Ask, more int, armed bool) {
	ui.Overlay(c, func() {
		w, h := c.Size()
		width := min(askWidth, max(280, w-32))
		card := ui.Column(c.Key(ask.Key)).Absolute().Left((w-width)/2).Top(askTop).Width(width).MaxHeight(max(0, h-askTop-16)).
			Radius(theme.RadiusPanel).Background(theme.Dialog).Border(1, theme.BorderStrong).TextColor(theme.Text).
			Role(ui.RoleAlertDialog).Label(ask.Title).Transition(OverlayMotion)
		Float(card).Children(func() {
			ui.Scroll(c).FillWidth().Shrink(1).MinHeight(0).Children(func() {
				ui.Column(c).FillWidth().Padding(14, 16, 12, 16).Gap(10).Children(func() {
					askHead(c, ask)
					if ask.Body != nil {
						ask.Body()
					}
				})
			})
			ui.Row(c).FillWidth().Shrink(0).Padding(0, 16, 14, 16).Gap(8).Wrap().AlignItems(ui.Center).Children(func() {
				if more > 0 {
					ui.Text(c, strconv.Itoa(more)+" more waiting").FontSize(11.5).TextColor(theme.Muted).SingleLine()
				}
				ui.Spacer(c)
				for _, b := range ask.Buttons {
					e := Button(c.Key(b.Label), b.Kind, b.Icon, b.Label, "").Disabled(b.Disabled)
					if e.Clicked() && armed && !b.Disabled {
						b.Do()
					}
				}
			})
		})
	})
}

func askHead(c *ui.Context, ask *Ask) {
	svg := ask.Icon
	if svg == nil {
		svg = icon.Alert
	}
	ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Start).Children(func() {
		ui.Icon(c, svg).FontSize(15).TextColor(theme.Accent).Margin(2, 0, 0, 0)
		ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
			ui.Text(c, ask.Title).FontSize(13.5).FontWeight(600).TextColor(theme.TextStrong)
			if ask.About != "" {
				ui.Text(c, ask.About).FontSize(12).TextColor(theme.TextSoft)
			}
		})
	})
}

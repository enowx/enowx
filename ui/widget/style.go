package widget

import (
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/theme"
)

// Backdrop dims what is under a dialog.
var Backdrop = ui.RGBA(0, 0, 0, 0.5)

// tint animates one of e's colors toward target.
func tint(e ui.Element, key string, target ui.Color) ui.Color {
	return e.AnimateColor(key, target, ease)
}

// Tint eases e's color named key toward target, at the pace the widgets
// change theirs.
func Tint(e ui.Element, key string, target ui.Color) ui.Color { return tint(e, key, target) }

// Rim gives e the edge of what lies over something else: a thin, quiet line
// around it and a small soft shadow under it.
func Rim(e ui.Element) ui.Element {
	return e.InsetShadow(0, 0, 0, 0.5, theme.Border).Shadow(0, 4, 16, 0, ui.RGBA(0, 0, 0, 0.18))
}

// Pane makes e one of the window's flat tinted surfaces, parted from the ground
// by its tint and a thin line: the bars' pieces, the session list, the tools.
func Pane(e ui.Element) ui.Element {
	return e.Background(theme.Panel).InsetShadow(0, 0, 0, 0.5, theme.Border)
}

// BrightFrame makes e the frame the user works in.
func BrightFrame(e ui.Element) ui.Element {
	return e.Radius(theme.RadiusPanel).Clip().Background(theme.Frame).InsetShadow(0, 0, 0, 0.5, theme.Border)
}

// Raise gives e the look of a card on a frame: a thin, quiet line around it.
func Raise(e ui.Element) ui.Element {
	return e.InsetShadow(0, 0, 0, 0.5, theme.Border)
}

// Bento makes e one of the window's panels: a pane of the given color.
func Bento(e ui.Element, bg ui.Color) ui.Element {
	return e.Radius(theme.RadiusBento).Clip().Background(bg).InsetShadow(0, 0, 0, 0.5, theme.Border)
}

// Float gives e the edge of an overlay: menus, pop-ups, dialogs.
func Float(e ui.Element) ui.Element { return Rim(e) }

// Popup styles the panel of a drop-down or pop-over, which grows in.
func Popup(panel ui.Element) ui.Element {
	return Float(panel.Padding(theme.S1).Gap(0).Radius(theme.RadiusCard).Background(theme.Popover)).
		TransformOrigin(0.5, 0).Transition(OverlayMotion)
}

// PopupItem styles one entry of a pop-up: highlighted while hovered or
// picked with the keyboard.
func PopupItem(item ui.Element, highlighted bool) ui.Element {
	item.FillWidth().Gap(theme.S2).MinHeight(theme.ControlHeight).Padding(theme.S1, theme.S3).Radius(theme.RadiusSmall)
	bg := theme.Fade(theme.Selected, 0)
	if highlighted {
		bg = theme.Selected
	}
	return item.Background(QuickTint(item, "bg", bg))
}

// PopupLabel is the title of a group of pop-up entries.
func PopupLabel(c *ui.Context, label string) {
	SectionLabel(c, label, theme.TextSoft).Padding(theme.S2, theme.S3, theme.S1, theme.S3)
}

// Divider is a hairline between groups.
func Divider(c *ui.Context) {
	ui.Row(c).FillWidth().Shrink(0).Padding(theme.S1, theme.S2).Children(func() {
		ui.Box(c).Height(0.5).FillWidth().Background(theme.Border)
	})
}

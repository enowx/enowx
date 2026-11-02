package widget

import (
	"github.com/egoist/mygo/plugins/glass"
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

// Rim gives e glass's edge: a hairline around it, lit along its top, and
// a soft shadow under it.
func Rim(e ui.Element) ui.Element {
	return e.InsetShadow(0, 0.5, 0, 0, theme.Highlight).InsetShadow(0, 0, 0, 0.5, theme.Border).
		Shadow(0, 10, 30, 0, ui.RGBA(0, 0, 0, 0.35))
}

// Frosted says glass frosts what is under it. Off, as in tests where the CPU
// draws every frame, it is its tint alone.
var Frosted = true

// SurfaceEdge is Rim for a main surface: its shadow and light along the top
// fade with the slider over the user's own image, its hairline stays, for
// panels to be told apart when nothing else of them shows.
func SurfaceEdge(e ui.Element) ui.Element {
	o := float64(theme.SurfaceOpacity)
	return e.InsetShadow(0, 0.5, 0, 0, theme.Fade(theme.Highlight, float64(theme.Highlight.A)/255*o)).InsetShadow(0, 0, 0, 0.5, theme.Border).
		Shadow(0, 10, 30, 0, ui.RGBA(0, 0, 0, float32(0.35*o)))
}

// glassLeastOpacity is the slider's value under which glass no longer frosts:
// the user's image shows as it is.
const glassLeastOpacity = 0.25

// Glass makes e a piece of glass over the wallpaper: what is under it shows
// through, frosted, tinted by theme.Panel. For the bars' capsules, the
// session list and the tools. Over the user's own image it frosts lightly, and
// not at all near the slider's lowest.
func Glass(e ui.Element) ui.Element {
	switch {
	case !Frosted, theme.SurfaceOpacity < glassLeastOpacity:
		return SurfaceEdge(e.Background(theme.Panel))
	case theme.SurfaceOpacity < 1:
		return SurfaceEdge(e.Material(glass.Glass{Style: glass.Clear, Tint: theme.Panel}))
	}
	return SurfaceEdge(e.Material(glass.Glass{Tint: theme.Panel}))
}

// BrightFrame makes e the one frame that draws the eye, the strongest thing
// on screen: nearly opaque, with the deepest shadow and the brightest rim.
func BrightFrame(e ui.Element) ui.Element {
	o := float32(theme.SurfaceOpacity)
	return e.Radius(theme.RadiusPanel).Clip().Background(theme.Frame).
		InsetShadow(0, 1, 0, 0, ui.RGBA(255, 255, 255, 0.6*o)).InsetShadow(0, 0, 0, 0.5, ui.RGBA(255, 255, 255, 0.3)).
		Shadow(0, 30, 80, 0, ui.RGBA(0, 0, 0, 0.55*o))
}

// Raise gives e the look of a card on a frame: a faint light along its top.
func Raise(e ui.Element) ui.Element {
	return e.InsetShadow(0, 0.5, 0, 0, ui.RGBA(255, 255, 255, 0.3))
}

// Bento makes e one of the window's panels: a pane of the given color with
// glass's edge.
func Bento(e ui.Element, bg ui.Color) ui.Element {
	return SurfaceEdge(e.Radius(theme.RadiusBento).Clip().Background(bg))
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
	item.FillWidth().Gap(theme.S2).MinHeight(theme.ControlHeight).Padding(theme.S1, theme.S3).Radius(theme.RadiusCard)
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

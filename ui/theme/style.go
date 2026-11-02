package theme

import "github.com/egoist/mygo/ui"

// Style is how the app is built, apart from its colors: its corners, spacing,
// control sizes, type, and whether it has any material. Regions, navigation and
// behavior are the same in every style.
type Style struct {
	ID, Name string

	RadiusFull, RadiusControl, RadiusTail, RadiusSmall, RadiusCard, RadiusPanel float32
	BentoGap, WindowInset, ControlHeight, BarHeight, ScrollWidth                float32
	S1, S2, S3, S4, S5, S6                                                      float32
	TextLabel, TextUI, TextRead, TextTitle                                      float32

	// Flat takes every blur, shadow and light off the app's surfaces, and its
	// wallpaper; Hairline is the width of the line a flat surface has for an edge,
	// 0 for none. Scrim is what lies over the window under a dialog.
	Flat     bool
	Hairline float32
	Scrim    ui.Color

	// surfaces is what the style makes of a palette's surfaces; nil keeps them.
	surfaces func(Palette) Palette
}

// LiquidGlass is the style of who hasn't picked one: lit glass over a wallpaper.
var LiquidGlass = Style{
	ID: "liquid-glass", Name: "Liquid Glass",
	RadiusFull: 999, RadiusControl: 16, RadiusTail: 4, RadiusSmall: 12, RadiusCard: 20, RadiusPanel: 28,
	BentoGap: 8, WindowInset: 12, ControlHeight: 32, BarHeight: 40, ScrollWidth: 8,
	S1: 4, S2: 8, S3: 12, S4: 16, S5: 24, S6: 32,
	TextLabel: 12, TextUI: 13, TextRead: 15, TextTitle: 17,
	Scrim: ui.RGBA(0, 0, 0, 0.5),
}

// Styles is every style, in the order the app offers them.
var Styles = []Style{LiquidGlass}

// style is the style in use.
var style Style

// CurrentStyle is the style in use.
func CurrentStyle() Style { return style }

// StyleByID is the style with the given ID, else the default, so a setting from
// another version still loads.
func StyleByID(id string) Style {
	for _, s := range Styles {
		if s.ID == id {
			return s
		}
	}
	return LiquidGlass
}

// UseStyle sets the tokens of shape, spacing, type and effects from s. Apply a
// palette after it, as a style can change what the surfaces take from one.
func UseStyle(s Style) {
	style = s
	RadiusFull, RadiusControl, RadiusTail, RadiusSmall, RadiusCard = s.RadiusFull, s.RadiusControl, s.RadiusTail, s.RadiusSmall, s.RadiusCard
	RadiusPanel, RadiusBento = s.RadiusPanel, s.RadiusPanel
	BentoGap, WindowInset, ControlHeight, BarHeight, ScrollWidth = s.BentoGap, s.WindowInset, s.ControlHeight, s.BarHeight, s.ScrollWidth
	S1, S2, S3, S4, S5, S6 = s.S1, s.S2, s.S3, s.S4, s.S5, s.S6
	TextLabel, TextUI, TextRead, TextTitle = s.TextLabel, s.TextUI, s.TextRead, s.TextTitle
	Flat, Hairline, Scrim = s.Flat, s.Hairline, s.Scrim
}

// styled is p as the style in use draws it.
func styled(p Palette) Palette {
	if style.surfaces == nil {
		return p
	}
	return style.surfaces(p)
}

// StyleColors are what a style makes of palette p, for a sample of it: its ground,
// its panes and its accent, whichever style is in use.
func StyleColors(s Style, p Palette) (ground, pane, accent ui.Color) {
	if s.surfaces != nil {
		p = s.surfaces(p)
	}
	return p.Ground, p.Panel, p.Accent
}

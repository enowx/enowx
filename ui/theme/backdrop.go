package theme

import "github.com/egoist/mygo/ui"

// BackdropScrim is how much of the palette's ground lies over the user's own
// background image: what keeps words on the panels above it readable, whatever
// the image holds, with panels still letting it show.
const BackdropScrim = 0.4

// WithBackdrop is p over the user's own image, its panels and frame as opaque
// as the user asks, down to what still lets words read on them over the
// lightest and darkest image: terminals, code and diffs have their own near
// opaque ground, and menus and dialogs stay as they are.
func WithBackdrop(p Palette, opacity float64) Palette {
	p.Panel = Fade(p.Panel, max(opacity, backdropFloor(p, p.Panel)))
	p.Frame = Fade(p.Frame, max(opacity, backdropFloor(p, p.Frame)))
	return p
}

// backdropFloor is the least alpha of surface at which p's words read on it,
// over a white or a black image under the scrim.
func backdropFloor(p Palette, surface ui.Color) float64 {
	for a := 0.0; a < 1; a += 0.01 {
		if backdropReads(p, Fade(surface, a)) {
			return a
		}
	}
	return 1
}

// backdropReads says p's words read on surface over the lightest and darkest
// image, a selection's text too.
func backdropReads(p Palette, surface ui.Color) bool {
	for _, image := range []ui.Color{ui.Hex("#FFFFFF"), ui.Hex("#000000")} {
		bg := over(surface, over(Fade(p.Ground, BackdropScrim), image))
		for _, fg := range []ui.Color{p.Text, p.TextStrong, p.TextSoft, p.Muted} {
			if contrast(fg, bg) < 4.5 {
				return false
			}
		}
		if contrast(p.Text, over(p.Selected, bg)) < 4.5 {
			return false
		}
	}
	return true
}

// over is translucent c as it reads on the opaque color under.
func over(c, under ui.Color) ui.Color {
	at := func(v, u uint8) uint8 { return uint8(float64(u) + (float64(v)-float64(u))*float64(c.A)/255 + 0.5) }
	return ui.Color{R: at(c.R, under.R), G: at(c.G, under.G), B: at(c.B, under.B), A: 255}
}

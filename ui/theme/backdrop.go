package theme

import "github.com/egoist/mygo/ui"

// BackdropScrim is how much of the palette's ground lies over the user's own
// background image with the panels fully opaque. It fades with them, so that at
// the slider's lowest the image shows at full strength.
const BackdropScrim = 0.4

// WithBackdrop is p over the user's own image, every main surface as opaque as
// the user asks. How readable the low end is, is the user's choice: only the
// default opacity is held to reading well. Menus, dialogs and pop-overs stay as
// they are, as they float over what they would otherwise be read against.
func WithBackdrop(p Palette, opacity float64) Palette {
	p.backdrop, p.opacity = true, opacity
	p.Panel = Fade(p.Panel, opacity)
	p.Frame = Fade(p.Frame, opacity)
	p.Raised = Fade(p.Raised, float64(p.Raised.A)/255*opacity)
	p.Sunken = Fade(p.Sunken, float64(p.Sunken.A)/255*opacity)
	return p
}

// Surface is c, a main surface's color, as opaque as the backdrop has it: c
// itself with no image of the user's.
func Surface(c ui.Color) ui.Color {
	return Fade(c, float64(c.A)/255*SurfaceOpacity)
}

// over is translucent c as it reads on the opaque color under.
func over(c, under ui.Color) ui.Color {
	at := func(v, u uint8) uint8 { return uint8(float64(u) + (float64(v)-float64(u))*float64(c.A)/255 + 0.5) }
	return ui.Color{R: at(c.R, under.R), G: at(c.G, under.G), B: at(c.B, under.B), A: 255}
}

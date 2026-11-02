package theme

import (
	"math"

	"github.com/egoist/mygo/ui"
)

// LookupAppearance returns a palette with surfaces and contrast composed
// for the requested appearance. The palette ID and chosen accent survive.
func LookupAppearance(id string, accent ui.Color, light bool) Palette {
	if !light {
		return Lookup(id, accent)
	}
	dark := Lookup(id, ui.Color{})
	// The surfaces take their tint from the light accent: the dark tint is too
	// grey to show once it is this pale.
	base, tint, white := dark.Panel, dark.lightAccent, ui.Hex("#FFFFFF")
	overBlack := func(a float32) ui.Color { return ui.RGBA(0, 0, 0, a) }
	p := Palette{
		ID: dark.ID, Name: dark.Name, Light: true,
		Ground: Mix(tint, white, 0.82), Panel: Mix(tint, white, 0.88), Frame: Mix(tint, white, 0.94),
		Background: Mix(tint, white, 0.95), Popover: Mix(tint, white, 0.97), Dialog: Mix(tint, white, 0.97),
		Raised: overBlack(0.025), Sunken: overBlack(0.05),
		Hover: overBlack(0.04), Selected: overBlack(0.08),
		Border: overBlack(0.09), BorderStrong: Mix(base, white, 0.26),
		Text: Mix(base, ui.Hex("#000000"), 0.55), TextStrong: Mix(base, ui.Hex("#000000"), 0.7),
		TextSoft: Mix(base, white, 0.1), Muted: Mix(base, white, 0.14),
		Accent: dark.lightAccent, Working: ui.Hex("#2B6580"), Done: ui.Hex("#2C6B4B"), Warning: ui.Hex("#7A5716"),
		Error: ui.Hex("#A23E36"), Idle: ui.Hex("#5F6A6D"),
		Add: ui.Hex("#2C6B4B"), Del: ui.Hex("#A23E36"),
	}
	if accent.A != 0 {
		p.Accent = accent
	}
	// An accent the user gave may be pale. Keep its hue, with enough depth for
	// selected labels and links on these surfaces.
	for contrast(p.Accent, over(p.Sunken, p.Panel)) < 4.5 || contrast(p.Accent, over(p.Selected, p.Panel)) < 4.5 {
		p.Accent = Mix(p.Accent, ui.Hex("#000000"), 0.06)
	}
	p.OnAccent = contrastInk(p.Accent)
	p.Now = p.Accent
	return p
}

// over is translucent c as it reads on the opaque color under.
func over(c, under ui.Color) ui.Color {
	at := func(v, u uint8) uint8 { return uint8(float64(u) + (float64(v)-float64(u))*float64(c.A)/255 + 0.5) }
	return ui.Color{R: at(c.R, under.R), G: at(c.G, under.G), B: at(c.B, under.B), A: 255}
}

func onWhite(c ui.Color) ui.Color {
	at := func(v uint8) uint8 { return uint8(255 - (255-float64(v))*float64(c.A)/255 + 0.5) }
	return ui.Color{R: at(c.R), G: at(c.G), B: at(c.B), A: 255}
}

func contrastInk(background ui.Color) ui.Color {
	dark, light := ui.Hex("#101828"), ui.Hex("#FFFFFF")
	if contrast(dark, background) >= contrast(light, background) {
		return dark
	}
	return light
}

// contrast is the ratio of a and b, each a translucent color as it reads on
// white.
func contrast(a, b ui.Color) float64 {
	a, b = onWhite(a), onWhite(b)
	luminance := func(c ui.Color) float64 {
		linear := func(v uint8) float64 {
			x := float64(v) / 255
			if x <= 0.04045 {
				return x / 12.92
			}
			return math.Pow((x+0.055)/1.055, 2.4)
		}
		return 0.2126*linear(c.R) + 0.7152*linear(c.G) + 0.0722*linear(c.B)
	}
	x, y := luminance(a), luminance(b)
	return (max(x, y) + 0.05) / (min(x, y) + 0.05)
}

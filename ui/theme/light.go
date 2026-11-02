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
	black := func(a float32) ui.Color { return ui.RGBA(0, 0, 0, a) }
	white := func(a float32) ui.Color { return ui.RGBA(255, 255, 255, a) }
	wall := dark.Wall
	for i := range wall {
		wall[i].A = 70
	}
	p := Palette{
		ID: dark.ID, Name: dark.Name, Light: true,
		Ground: ui.Hex("#E9EDF5"), Wall: wall,
		Background: ui.Hex("#F4F6F9"), Panel: white(0.62), Raised: black(0.035), Sunken: black(0.06),
		Frame: white(0.88), Popover: ui.RGBA(252, 252, 254, 0.98),
		Hover: black(0.05), Selected: black(0.1), Dialog: ui.RGBA(252, 252, 254, 0.98),
		Border: black(0.12), BorderStrong: ui.Hex("#778497"),
		Text: ui.Hex("#1C2130"), TextStrong: ui.Hex("#0C101C"), TextSoft: ui.Hex("#475467"), Muted: ui.Hex("#536176"),
		Accent: ui.Hex("#0A63C9"), Working: ui.Hex("#075E8C"), Done: ui.Hex("#166B42"), Warning: ui.Hex("#855300"),
		Error: ui.Hex("#B3261E"), Idle: ui.Hex("#5F6673"),
		Add: ui.Hex("#166B42"), Del: ui.Hex("#B3261E"), Now: ui.Hex("#0A63C9"),
	}
	switch p.ID {
	case Champagne.ID:
		p.Ground, p.Accent = ui.Hex("#F3EFE7"), ui.Hex("#826235")
	case Obsidian.ID:
		p.Accent = ui.Hex("#4E5A6B")
	case Midnight.ID:
		p.Accent = ui.Hex("#355CAD")
	case Copper.ID:
		p.Ground, p.Accent = ui.Hex("#F3ECE6"), ui.Hex("#9C5429")
	case Teal.ID:
		p.Ground, p.Accent = ui.Hex("#E6F0F0"), ui.Hex("#285F63")
	}
	if accent.A != 0 {
		p.Accent = accent
	}
	// The dark palettes use pale accents. Keep their hue in light mode,
	// with enough depth for selected labels and links on white surfaces.
	for contrast(p.Accent, p.Sunken) < 4.5 || contrast(p.Accent, p.Panel) < 4.5 || contrast(p.Accent, p.Selected) < 4.5 {
		p.Accent = Mix(p.Accent, ui.Hex("#000000"), 0.06)
	}
	p.OnAccent = contrastInk(p.Accent)
	return p
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

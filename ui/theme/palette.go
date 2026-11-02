package theme

import (
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
)

// Palette is one theme's colors in a light or dark appearance: flat, muted
// surfaces of one tint on a ground of that tint, parted by quiet lines.
type Palette struct {
	ID, Name string
	Light    bool

	Ground                            ui.Color // what the panels lie on
	Background, Panel, Raised, Sunken ui.Color
	Frame, Popover                    ui.Color
	Hover, Selected, Dialog           ui.Color
	Border, BorderStrong              ui.Color
	Text, TextStrong, TextSoft, Muted ui.Color
	Accent, OnAccent                  ui.Color
	Working, Done, Warning, Error     ui.Color
	Idle                              ui.Color
	Add, Del, Now                     ui.Color

	lightAccent ui.Color // its accent in the light appearance: a pale one only turns grey when deepened
}

// soft is a dark palette: every palette has the same recipe, from a muted tint
// the surfaces are lighter or darker shades of, with an accent of its own. What
// lies on a surface is a faint overlay of white or black, so it keeps the tint.
func soft(id, name, tint, accent, lightAccent string) Palette {
	base := ui.Hex(tint)
	white, black := ui.Hex("#FFFFFF"), ui.Hex("#000000")
	overWhite := func(a float32) ui.Color { return ui.RGBA(255, 255, 255, a) }
	overBlack := func(a float32) ui.Color { return ui.RGBA(0, 0, 0, a) }
	p := Palette{
		ID: id, Name: name,
		Ground: Mix(base, black, 0.22), Panel: base, Frame: Mix(base, white, 0.03),
		Background: Mix(base, black, 0.6), Popover: Mix(base, black, 0.5), Dialog: Mix(base, black, 0.38),
		Raised: overWhite(0.04), Sunken: overBlack(0.18),
		Hover: overWhite(0.05), Selected: overWhite(0.1),
		Border: overWhite(0.09), BorderStrong: overWhite(0.18),
		Text: Mix(white, base, 0.08), TextStrong: Mix(white, base, 0.02), TextSoft: Mix(white, base, 0.18), Muted: Mix(white, base, 0.2),
		Working: ui.Hex("#8CCBE0"), Done: ui.Hex("#8DCFA6"), Warning: ui.Hex("#E2C47C"), Error: ui.Hex("#EE9A92"), Idle: ui.Hex("#B4BEC0"),
		Add: ui.Hex("#9BD6B2"), Del: ui.Hex("#F0ADA6"),
		lightAccent: ui.Hex(lightAccent),
	}
	p.setDarkAccent(ui.Hex(accent))
	return p
}

// setDarkAccent gives a dark palette its accent, lifted as far as it takes to
// read as words on the surfaces, a selection included.
func (p *Palette) setDarkAccent(accent ui.Color) {
	for contrast(accent, p.Panel) < 4.5 || contrast(accent, over(p.Selected, p.Frame)) < 4.5 {
		accent = Mix(accent, ui.Hex("#FFFFFF"), 0.06)
	}
	p.Accent, p.OnAccent = accent, contrastInk(accent)
	p.Now = Mix(accent, ui.Hex("#FFFFFF"), 0.2)
}

// The built-in palettes. Status colors keep their meaning in every one:
// blue works, yellow waits, green is done, red failed.
var (
	Teal      = soft("teal", "Soft Teal", "#2A4B4E", "#8AD4D0", "#2F7273")
	Champagne = soft("champagne", "Champagne", "#3B352C", "#D0BC98", "#7A6238")
	Obsidian  = soft("obsidian", "Obsidian Silver", "#303238", "#C5C9D1", "#525B6B")
	Midnight  = soft("midnight", "Midnight", "#2A3350", "#A3B6E0", "#3D5A9E")
	Copper    = soft("copper", "Copper", "#3F312C", "#D2A484", "#96583A")
	Graphite  = soft("graphite", "Graphite", "#2E3442", "#8FB0DC", "#3E6AA3")
)

// Palettes is every built-in palette. Users have the IDs saved: they only grow.
var Palettes = []Palette{Teal, Champagne, Obsidian, Midnight, Copper, Graphite}

// Default is the palette of who hasn't picked one.
var Default = Teal

// Accents are the colors a user can give any palette instead of its own.
var Accents = []ui.Color{
	ui.Hex("#7FA8D9"), // blue
	ui.Hex("#83BD9C"), // green
	ui.Hex("#A99BDC"), // violet
	ui.Hex("#D9A37C"), // orange
	ui.Hex("#D596AC"), // rose
	ui.Hex("#86C0BF"), // teal
}

// Lookup is the palette with the given ID, else the default; a non-zero
// accent replaces the palette's own.
func Lookup(id string, accent ui.Color) Palette {
	p := Default
	for _, q := range Palettes {
		if q.ID == id {
			p = q
		}
	}
	if accent.A != 0 {
		p.setDarkAccent(accent)
	}
	return p
}

// Terminal keeps a dark ground even in light appearance: CLI programs often
// draw their own pale input text without adapting to the app's colors.
func Terminal() *terminal.Theme {
	p := current
	if p.Light {
		p = Lookup(p.ID, ui.Color{})
	}
	t := terminal.DarkTheme()
	fg := ui.Hex("#E8EAF2")
	t.Foreground, t.Background = fg, p.Background
	t.Cursor = p.Accent
	t.Selection = Fade(p.Accent, 0.25)
	t.Palette = [16]ui.Color{
		ui.Hex("#3A3C4A"), p.Error, p.Done, p.Warning,
		p.Working, ui.Hex("#C7A4E8"), ui.Hex("#7CCBD3"), fg,
		p.Idle, Mix(p.Error, p.TextStrong, 0.2), Mix(p.Done, p.TextStrong, 0.2), Mix(p.Warning, p.TextStrong, 0.2),
		Mix(p.Working, p.TextStrong, 0.2), ui.Hex("#DAB8F0"), ui.Hex("#9DDCE3"), p.TextStrong,
	}
	return t
}

package theme

import (
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
)

// Palette is one theme's colors in a light or dark appearance. A palette is
// a lit wallpaper with glass over it: Ground and Wall are the wallpaper, the
// surfaces are translucent over them.
type Palette struct {
	ID, Name string
	Light    bool

	Ground                            ui.Color    // the wallpaper's base
	Wall                              [3]ui.Color // the wallpaper's three lights, their alpha how strong
	Background, Panel, Raised, Sunken ui.Color
	Frame, Popover                    ui.Color
	Hover, Selected, Dialog           ui.Color
	Border, BorderStrong              ui.Color
	Text, TextStrong, TextSoft, Muted ui.Color
	Accent, OnAccent                  ui.Color
	Working, Done, Warning, Error     ui.Color
	Idle                              ui.Color
	Add, Del, Now                     ui.Color
}

// glass is a dark palette: every palette has the same glass, over a
// wallpaper and with an accent of its own.
func glass(id, name, ground string, wall [3]ui.Color, accent, onAccent string) Palette {
	white := func(a float32) ui.Color { return ui.RGBA(255, 255, 255, a) }
	return Palette{
		ID: id, Name: name,
		Ground: ui.Hex(ground), Wall: wall,
		Background: ui.Hex("#0D0E16"), Panel: ui.RGBA(38, 40, 54, 0.62), Raised: white(0.07), Sunken: ui.RGBA(0, 0, 0, 0.25),
		Frame: ui.RGBA(70, 72, 88, 0.92), Popover: ui.RGBA(30, 32, 44, 0.98),
		Hover: white(0.08), Selected: white(0.2), Dialog: ui.RGBA(48, 50, 66, 0.98),
		Border: white(0.2), BorderStrong: white(0.2),
		Text: ui.Hex("#F5F6FA"), TextStrong: ui.Hex("#FFFFFF"), TextSoft: ui.Hex("#D8DAE5"), Muted: ui.Hex("#D8DAE5"),
		Accent: ui.Hex(accent), OnAccent: ui.Hex(onAccent),
		Working: ui.Hex("#64D2FF"), Done: ui.Hex("#30D158"), Warning: ui.Hex("#FFD60A"), Error: ui.Hex("#FF6961"), Idle: ui.Hex("#AEAEB2"),
		Add: ui.Hex("#8BF0B0"), Del: ui.Hex("#FFB1AB"), Now: ui.Hex("#BCDCFF"),
	}
}

func light(col string, a float32) ui.Color {
	c := ui.Hex(col)
	return ui.RGBA(c.R, c.G, c.B, a)
}

// The built-in palettes. Status colors keep their meaning in every one:
// blue works, yellow waits, green is done, red failed.
var (
	Champagne = glass("champagne", "Champagne", "#1D1913",
		[3]ui.Color{light("#C9A15A", 0.45), light("#B5653C", 0.35), light("#6E5A3A", 0.45)}, "#D8C3A0", "#1A150E")
	Obsidian = glass("obsidian", "Obsidian Silver", "#16171B",
		[3]ui.Color{light("#8D97A8", 0.35), light("#5C6370", 0.35), light("#3E4450", 0.45)}, "#E2E4E9", "#0B0C0E")
	Midnight = glass("midnight", "Midnight", "#0E1428",
		[3]ui.Color{light("#3158C9", 0.55), light("#2A7E9C", 0.35), light("#4B3AA8", 0.45)}, "#9DB6F5", "#0A0F1E")
	Copper = glass("copper", "Copper", "#1E1614",
		[3]ui.Color{light("#C4743C", 0.45), light("#A83C32", 0.35), light("#6B3A5C", 0.45)}, "#D49A6A", "#1A0F07")
	Graphite = glass("graphite", "Graphite", "#15182B",
		[3]ui.Color{light("#2C7BE0", 0.55), light("#D9543C", 0.45), light("#7A3FBF", 0.45)}, "#0A6FE0", "#FFFFFF")
)

// Palettes is every built-in palette.
var Palettes = []Palette{Champagne, Obsidian, Midnight, Copper, Graphite}

// Default is the palette of who hasn't picked one.
var Default = Graphite

// Accents are the colors a user can give any palette instead of its own.
var Accents = []ui.Color{
	ui.Hex("#0A6FE0"), // blue
	ui.Hex("#2B7A5B"), // green
	ui.Hex("#8A5CF6"), // violet
	ui.Hex("#C2410C"), // orange
	ui.Hex("#B4235A"), // rose
	ui.Hex("#0E7490"), // teal
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
		p.Accent = accent
		p.OnAccent = contrastInk(accent)
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

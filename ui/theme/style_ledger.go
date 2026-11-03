package theme

import "github.com/egoist/mygo/ui"

// Ledger is real black with flat panes on one-pixel lines: a working tool, as many
// rows as the window holds.
var Ledger = Style{
	ID: "ledger", Name: "Ledger",
	RadiusFull: 4, RadiusControl: 4, RadiusTail: 4, RadiusSmall: 4, RadiusCard: 4, RadiusPanel: 0,
	BentoGap: 1, WindowInset: 0, ControlHeight: 26, BarHeight: 36, ScrollWidth: 6,
	BarRadius: 0, RailWidth: 40, ReadWidth: 800, HeaderHeight: 40,
	RowHeight: 26, RowTwo: 36, RowTall: 32, TabHeight: 22, Rule: 1,
	S1: 4, S2: 6, S3: 8, S4: 12, S5: 16, S6: 24,
	TextLabel: 12, TextUI: 12, TextRead: 13, TextTitle: 14,
	Flat: true, Hairline: 1, Scrim: ui.RGBA(0, 0, 0, 0.55),
	Butted: true, FullDock: true, DockedComposer: true,
	Family:   "IBM Plex Sans, system-ui",
	surfaces: ledgerSurfaces,
}

// ledgerAccent is the accent Ledger and Bento open on, with the default palette.
var ledgerAccent = ui.Hex("#2F6FED")

// ledgerSurfaces is p on real black, or on white in the light appearance. The
// surfaces are steps from the ground toward the words; a palette other than the
// default tints those steps, and gives its accent, while the ground stays.
func ledgerSurfaces(p Palette) Palette {
	base, ink, soft, bright := ui.Hex("#000000"), ui.Hex("#E8E8E8"), ui.Hex("#A6A6A6"), ui.Hex("#FFFFFF")
	if p.Light {
		base, ink, soft, bright = ui.Hex("#FFFFFF"), ui.Hex("#1A1A1A"), ui.Hex("#595959"), ui.Hex("#000000")
	}
	if p.own && p.ID == Default.ID {
		p.Accent = ledgerAccent
	}
	tint := ink
	if p.ID != Default.ID {
		tint = Mix(ink, p.Accent, 0.4)
	}
	step := func(t float64) ui.Color { return Mix(base, tint, t) }
	p.Ground, p.Background, p.Frame = base, base, base
	p.Wall = [3]ui.Color{}
	p.Panel, p.Raised = step(0.04), step(0.04)
	p.Sunken = base
	p.Dialog, p.Popover = step(0.06), step(0.08)
	p.composer = p.Panel
	p.Hover = Fade(ink, 0.07)
	p.Selected = Mix(base, p.Accent, 0.4)
	p.Border, p.BorderStrong = step(0.16), step(0.16)
	p.floatLine = step(0.16)
	p.Text, p.TextStrong, p.TextSoft, p.Muted = ink, bright, soft, soft
	p.OnAccent = contrastInk(p.Accent)
	p.code = ui.Hex("#000000")
	if !p.Light {
		p.Working, p.Done, p.Warning, p.Error, p.Idle = ui.Hex("#55B5E6"), ui.Hex("#46B06E"), ui.Hex("#E5B931"), ui.Hex("#E8645A"), ui.Hex("#8C8C8C")
		p.Add, p.Del, p.Now = ui.Hex("#7FD49B"), ui.Hex("#F09A92"), ui.Hex("#D6D6D6")
	}
	return holdWords(p)
}

package theme

import "github.com/egoist/mygo/ui"

// Tonal is one deep hue in a few flat tones: regions separate by color alone, with
// no line or shadow, and shape does the rest.
var Tonal = Style{
	ID: "tonal", Name: "Tonal",
	RadiusFull: 999, RadiusControl: 12, RadiusTail: 4, RadiusSmall: 12, RadiusCard: 14, RadiusPanel: 24,
	BentoGap: 12, WindowInset: 12, ControlHeight: 32, BarHeight: 40, ScrollWidth: 8,
	BarRadius: 14, RailWidth: 48, ReadWidth: 740, HeaderHeight: 56,
	RowHeight: 36, RowTwo: 40, RowTall: 44, TabHeight: 28, Rule: 0,
	S1: 4, S2: 8, S3: 12, S4: 16, S5: 24, S6: 32,
	TextLabel: 12, TextUI: 13, TextRead: 15, TextTitle: 20,
	Flat: true, Hairline: 0, Scrim: ui.RGBA(0, 0, 0, 0.55),
	Family:   "Rubik, system-ui",
	surfaces: tonalSurfaces,
}

// tonalSurfaces is every surface a tone between the palette's ground and the words.
// The default palette opens on its deep green and coral; another's ground and accent
// change the whole field, as this style means them to.
func tonalSurfaces(p Palette) Palette {
	base, accent := p.Ground, p.Accent
	white, black := ui.Hex("#FFFFFF"), ui.Hex("#000000")
	own := p.own && p.ID == Default.ID
	var ink, soft ui.Color
	switch {
	case p.Light:
		if own {
			accent = ui.Hex("#B4502B")
		}
		ink = Mix(accent, black, 0.75)
		soft = Mix(ink, base, 0.2)
	case p.ID == Default.ID:
		base, ink, soft = ui.Hex("#0D2A25"), ui.Hex("#EEF7F2"), ui.Hex("#BCD3CA")
		if own {
			accent = ui.Hex("#FF9F7A")
		}
	default:
		ink = Mix(white, accent, 0.06)
		soft = Mix(ink, base, 0.2)
	}
	tone := func(t float64) ui.Color { return Mix(base, ink, t) }
	t1, t2, t3 := tone(0.07), tone(0.13), tone(0.23)
	toward := white // an accent that reads as words on the tones
	if p.Light {
		toward = black
	}
	for i := 0; i < 20 && contrast(accent, t1) < 4.5; i++ {
		accent = Mix(accent, toward, 0.06)
	}
	shade := Mix(base, black, 0.14)
	if p.Light {
		shade = Mix(base, black, 0.05)
	}
	p.Ground, p.Accent, p.OnAccent = base, accent, contrastInk(accent)
	p.Wall = [3]ui.Color{}
	p.Background = Mix(base, black, 0.3)
	p.Panel, p.Frame, p.Raised = t1, t2, t1
	p.Sunken = shade
	p.Dialog, p.Popover = t2, t2 // a menu's hover is the selection: on t3 the two are one tone
	p.Hover = Fade(ink, 0.09)
	p.Selected = Mix(base, accent, 0.3)
	p.picked = Mix(base, accent, 0.84)
	p.onPicked = contrastInk(p.picked)
	p.Border, p.BorderStrong = t3, t3
	p.Text, p.TextStrong, p.TextSoft, p.Muted = ink, Mix(ink, toward, 0.5), soft, soft
	p.code = Mix(base, black, 0.3)
	p.dock = Mix(base, accent, 0.22)
	if own && !p.Light {
		p.Working, p.Done, p.Warning, p.Error, p.Idle = ui.Hex("#7FD6E8"), ui.Hex("#86E3A8"), ui.Hex("#F6CE5B"), ui.Hex("#FF8C7A"), ui.Hex("#A9C0B7")
		p.Add, p.Del, p.Now = ui.Hex("#A4EDBE"), ui.Hex("#FFB4A6"), ui.Hex("#FFD5C2")
	}
	return holdWords(p)
}

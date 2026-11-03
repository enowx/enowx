package onboardview

import (
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/theme"
)

// look is everything a style decides of what onboarding draws. Onboarding paints from
// these rather than the theme's tokens, one set for the whole app, so that each region
// can be partway from one style to another on its own as the wave crosses the window.
type look struct {
	m, g, bar, rail, r, rc, rt, rr, rb, rowh, lw, sha, blur, wall, band, rf, cf, cr, hd, tabh, ctl, capr, railr float32
	tlL, tlW, ctxL, ctxW, pgL, pgW, fdL, fdW, actL, actW, fs                                                    float32

	desk, side, main, panel, comp, rim, rim2, gl, ink, ink2, ink3, accent, accent2, ona, sel, onsel ui.Color
	well, fill, fill0, fill2, focus, idle, tile, wire, node, nodel, nodet, bandc                    ui.Color
}

func (l *look) nums() []*float32 {
	return []*float32{&l.m, &l.g, &l.bar, &l.rail, &l.r, &l.rc, &l.rt, &l.rr, &l.rb, &l.rowh, &l.lw, &l.sha, &l.blur, &l.wall, &l.band, &l.rf, &l.cf, &l.cr, &l.hd, &l.tabh, &l.ctl, &l.capr, &l.railr,
		&l.tlL, &l.tlW, &l.ctxL, &l.ctxW, &l.pgL, &l.pgW, &l.fdL, &l.fdW, &l.actL, &l.actW}
}

func (l *look) colors() []*ui.Color {
	return []*ui.Color{&l.desk, &l.side, &l.main, &l.panel, &l.comp, &l.rim, &l.rim2, &l.gl, &l.ink, &l.ink2, &l.ink3, &l.accent, &l.accent2, &l.ona, &l.sel, &l.onsel,
		&l.well, &l.fill, &l.fill0, &l.fill2, &l.focus, &l.idle, &l.tile, &l.wire, &l.node, &l.nodel, &l.nodet, &l.bandc}
}

func rgba(r, g, b uint8, a float32) ui.Color { return ui.RGBA(r, g, b, a) }

// The four styles, as design-2's script has them.
var (
	lookGlass = look{m: 12, g: 8, bar: 40, rail: 44, r: 24, rc: 14, rt: 12, rr: 12, rb: 16, rowh: 40, lw: 1, sha: .35, blur: 30, wall: 1, rf: 1, cf: 1, cr: 24, hd: 48, tabh: 36, ctl: 28, capr: 20, railr: 22,
		tlL: 12, tlW: 76, ctxL: 96, ctxW: 160, pgL: 268, pgW: 424, fdL: 700, fdW: 340, actL: 1196, actW: 232, fs: 13,
		desk: rgba(21, 24, 43, 1), side: rgba(38, 40, 54, .62), main: rgba(70, 72, 88, .9), panel: rgba(38, 40, 54, .94), comp: rgba(20, 22, 32, .9), rim: rgba(255, 255, 255, .2), rim2: rgba(255, 255, 255, .16), gl: rgba(255, 255, 255, .3),
		ink: rgba(245, 246, 250, 1), ink2: rgba(216, 218, 229, 1), ink3: rgba(150, 152, 166, 1), accent: rgba(10, 111, 224, 1), accent2: rgba(140, 192, 255, 1), ona: rgba(255, 255, 255, 1), sel: rgba(255, 255, 255, .2), onsel: rgba(255, 255, 255, 1),
		well: rgba(0, 0, 0, .25), fill: rgba(255, 255, 255, .1), fill0: rgba(255, 255, 255, .05), fill2: rgba(255, 255, 255, .18), focus: rgba(188, 220, 255, 1), idle: rgba(174, 174, 178, 1), tile: rgba(255, 255, 255, .1), wire: rgba(140, 192, 255, 1),
		node: rgba(34, 36, 50, 1), nodel: rgba(255, 255, 255, .22), nodet: rgba(255, 255, 255, .1), bandc: rgba(10, 10, 10, 1)}
	lookLedger = look{m: 0, g: -1, bar: 36, rail: 40, r: 0, rc: 4, rt: 3, rr: 4, rb: 4, rowh: 36, lw: 1, band: 1, hd: 40, tabh: 28, ctl: 26,
		tlL: 0, tlW: 96, ctxL: 95, ctxW: 153, pgL: 247, pgW: 454, fdL: 700, fdW: 341, actL: 1040, actW: 400, fs: 12,
		desk: rgba(0, 0, 0, 1), side: rgba(10, 10, 10, 1), main: rgba(0, 0, 0, 1), panel: rgba(10, 10, 10, 1), comp: rgba(10, 10, 10, 1), rim: rgba(37, 37, 37, 1), rim2: rgba(37, 37, 37, 1), gl: rgba(255, 255, 255, 0),
		ink: rgba(232, 232, 232, 1), ink2: rgba(166, 166, 166, 1), ink3: rgba(110, 110, 110, 1), accent: rgba(47, 111, 237, 1), accent2: rgba(130, 170, 255, 1), ona: rgba(255, 255, 255, 1), sel: rgba(19, 44, 95, 1), onsel: rgba(255, 255, 255, 1),
		well: rgba(0, 0, 0, 1), fill: rgba(38, 38, 38, 1), fill0: rgba(16, 16, 16, 1), fill2: rgba(52, 52, 52, 1), focus: rgba(214, 214, 214, 1), idle: rgba(140, 140, 140, 1), tile: rgba(26, 26, 26, 1), wire: rgba(110, 155, 255, 1),
		node: rgba(14, 14, 14, 1), nodel: rgba(48, 48, 48, 1), nodet: rgba(30, 30, 30, 1), bandc: rgba(10, 10, 10, 1)}
	lookTonal = look{m: 12, g: 12, bar: 40, rail: 48, r: 24, rc: 12, rt: 10, rr: 12, rb: 14, rowh: 40, rf: 1, cf: 1, cr: 24, hd: 48, tabh: 36, ctl: 28, capr: 14, railr: 24,
		tlL: 12, tlW: 76, ctxL: 96, ctxW: 164, pgL: 272, pgW: 416, fdL: 700, fdW: 340, actL: 1196, actW: 232, fs: 13,
		desk: rgba(13, 42, 37, 1), side: rgba(29, 57, 52, 1), main: rgba(42, 71, 65, 1), panel: rgba(29, 57, 52, 1), comp: rgba(65, 90, 85, 1), rim: rgba(65, 90, 85, 0), rim2: rgba(238, 247, 242, .12), gl: rgba(255, 255, 255, 0),
		ink: rgba(238, 247, 242, 1), ink2: rgba(188, 211, 202, 1), ink3: rgba(130, 158, 148, 1), accent: rgba(255, 159, 122, 1), accent2: rgba(255, 184, 155, 1), ona: rgba(43, 18, 8, 1), sel: rgba(216, 140, 108, 1), onsel: rgba(43, 18, 8, 1),
		well: rgba(11, 36, 32, 1), fill: rgba(65, 90, 85, 1), fill0: rgba(38, 66, 60, 1), fill2: rgba(84, 108, 102, 1), focus: rgba(255, 213, 194, 1), idle: rgba(169, 192, 183, 1), tile: rgba(52, 80, 74, 1), wire: rgba(255, 184, 155, 1),
		node: rgba(29, 57, 52, 1), nodel: rgba(65, 90, 85, 1), nodet: rgba(52, 80, 74, 1), bandc: rgba(10, 10, 10, 1)}
	lookBento = func() look {
		l := lookLedger
		l.m, l.g, l.bar, l.rail, l.r, l.band, l.capr, l.railr = 8, 8, 36, 40, 6, 0, 6, 6
		l.tlL, l.tlW, l.ctxL, l.ctxW, l.pgL, l.pgW, l.fdL, l.fdW, l.actL, l.actW = 8, 64, 80, 176, 264, 428, 700, 340, 1048, 384
		l.main = rgba(10, 10, 10, 1)
		return l
	}()
)

// Words of the wallpaper's lights, as the design has them.
var (
	wallLight = []struct {
		col        ui.Color
		x, y, w, h float32
	}{
		{ui.Hex("#2C7BE0"), 160, 0, 640, 340},
		{ui.Hex("#D9543C"), 900, 220, 540, 520},
		{ui.Hex("#7A3FBF"), 0, 480, 520, 420},
	}
	dotWork = ui.Hex("#64D2FF")
	dotWait = ui.Hex("#FFD60A")
	dotDone = ui.Hex("#30D158")
	addInk  = ui.Hex("#8BF0B0")
	lightsR = ui.Hex("#FF5F57")
	lightsY = ui.Hex("#FEBC2E")
	lightsG = ui.Hex("#28C840")
)

// palettes are the palettes the Style step offers: their accent and the ground they tint.
var palettes = []struct {
	id     string
	name   string
	accent ui.Color
	base   ui.Color
}{
	{theme.Champagne.ID, "Champagne", ui.Hex("#D8C3A0"), ui.Hex("#121110")},
	{theme.Obsidian.ID, "Obsidian Silver", ui.Hex("#E2E4E9"), ui.Hex("#111215")},
	{theme.Midnight.ID, "Midnight", ui.Hex("#9DB6F5"), ui.Hex("#0F1218")},
	{theme.Copper.ID, "Copper", ui.Hex("#D49A6A"), ui.Hex("#131417")},
	{theme.Graphite.ID, "Graphite", ui.Hex("#7FA6FF"), ui.Hex("#121418")},
}

// lookOf is style id's look, tinted by palette paletteID ("" the style's own) and with
// the panels' opacity and blur the user set (only with an image of theirs, as in the app).
func lookOf(id, paletteID string, o Options) look {
	s := theme.StyleByID(id)
	var v look
	switch s.ID {
	case theme.Ledger.ID:
		v = lookLedger
	case theme.Tonal.ID:
		v = lookTonal
	case theme.Bento.ID:
		v = lookBento
	default:
		v = lookGlass
	}
	v.tint(s.ID, paletteID)
	if s.ID == theme.LiquidGlass.ID {
		if o.BackgroundMode == BackgroundPlain {
			v.wall = 0
		}
		if o.BackgroundMode == BackgroundImage {
			k := float64(o.PanelOpacity)
			for _, col := range []*ui.Color{&v.side, &v.main, &v.panel, &v.comp} {
				*col = theme.Fade(*col, float64(col.A)/255*k)
			}
			v.blur = o.BlurValue
		}
	}
	return v
}

// tint gives the look a palette: any style takes any, as the app's choice says.
func (v *look) tint(style, id string) {
	for _, p := range palettes {
		if p.id != id {
			continue
		}
		pa, pb := p.accent, p.base
		white, black, dark := rgba(255, 255, 255, 1), rgba(0, 0, 0, 1), rgba(10, 10, 12, 1)
		y := luminance(pa)
		v.accent, v.accent2, v.wire = pa, pa, pa
		v.ona = dark
		if 1.05/(y+.05) >= (y+.05)/.05 {
			v.ona = white
		}
		switch style {
		case theme.Tonal.ID:
			v.desk, v.side = pb, mixColor(pb, v.ink, .07)
			v.panel, v.node = v.side, v.side
			v.main, v.comp = mixColor(pb, v.ink, .13), mixColor(pb, v.ink, .23)
			v.fill, v.fill0, v.fill2 = v.comp, mixColor(pb, v.ink, .1), mixColor(pb, v.ink, .3)
			v.tile, v.nodet = mixColor(pb, v.ink, .18), mixColor(pb, v.ink, .18)
			v.nodel, v.well = v.comp, mixColor(pb, black, .2)
			v.sel, v.onsel = mixColor(pb, pa, .84), v.ona
		case theme.LiquidGlass.ID:
			v.desk = pb
		default:
			surf := mixColor(pb, black, .3)
			v.side, v.panel, v.comp = surf, surf, surf
			if style == theme.Bento.ID {
				v.main = surf
			}
			v.sel = mixColor(black, pa, .42)
			v.onsel = dark
			if 1.05/(luminance(v.sel)+.05) >= 4.5 {
				v.onsel = white
			}
		}
		return
	}
}

// luminance is how light c is, its alpha aside.
func luminance(c ui.Color) float32 {
	return .2126*float32(linear(c.R)) + .7152*float32(linear(c.G)) + .0722*float32(linear(c.B))
}

// lookBetween is looks a and b part of the way. Text on a fill takes whichever ink reads
// on what the fill is by then, and a font's size can't be between: it goes over halfway.
func lookBetween(a, b look, x float32) look {
	if x <= 0 {
		return a
	}
	if x >= 1 {
		return b
	}
	out := a
	an, bn, on := a.nums(), b.nums(), out.nums()
	for i := range on {
		*on[i] = lerp(*an[i], *bn[i], x)
	}
	ac, bc, oc := a.colors(), b.colors(), out.colors()
	for i := range oc {
		*oc[i] = mixColor(*ac[i], *bc[i], x)
	}
	if x >= .5 {
		out.fs = b.fs
	}
	out.ona = inkOn(out.accent, rgba(0, 0, 0, 1))
	out.onsel = inkOn(out.sel, out.main)
	return out
}

func inkOn(fill, under ui.Color) ui.Color {
	if luminance(mixColor(under, rgba(fill.R, fill.G, fill.B, 1), float32(fill.A)/255)) > .2 {
		return rgba(20, 12, 8, 1)
	}
	return rgba(255, 255, 255, 1)
}

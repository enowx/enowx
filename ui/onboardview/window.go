package onboardview

import (
	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/widget"
)

// frameLine is the hairline a region is before it has its material.
var frameLine = rgba(236, 236, 240, .2)

// window draws everything under the opening, in the design's order.
func (v *View) window(c *ui.Context, o Options, s scene) {
	fill := func(key string) ui.Element {
		return ui.Box(c.Key(key)).Absolute().Left(0).Top(0).Right(0).Bottom(0).PassThrough()
	}
	fill("ob-desk").Background(s.v.desk)
	if s.wall > .004 {
		fill("ob-wall").Opacity(s.wall).Clip().Children(func() { v.wallpaper(c, o, s) })
	}
	if s.band > .004 {
		ui.Box(c.Key("ob-band")).Absolute().Left(0).Top(0).Right(0).Height(s.ctx.v.bar).Opacity(s.band).PassThrough().
			Background(s.v.bandc).InsetShadow(0, -1, 0, 0, s.v.rim)
	}
	if s.ground > .004 {
		fill("ob-ground").Opacity(s.ground).Background(openingGround)
	}
	if s.t < tOpeningEnd {
		return
	}
	fill("ob-frames").Draw(func(p *ui.Painter, _ ui.Rect) { drawFrames(p, s) })
	fill("ob-lights").Draw(func(p *ui.Painter, _ ui.Rect) { drawLights(p, s) })
	v.barRegions(c, s, o)
	v.docRegion(c, o, s)
	v.railRegion(c, s)
	v.sideRegion(c, o, s)
	v.panelRegion(c, o, s)
	v.serverNode(c, o, s)
	v.goaNode(c, o, s)
	fill("ob-wires").Draw(func(p *ui.Painter, _ ui.Rect) {
		v.drawHostWire(p, o, s)
		v.drawGOAWires(p, o, s)
	})
	fill("ob-fly").Draw(func(p *ui.Painter, _ ui.Rect) {
		v.drawRing(p, o, s)
		v.drawFlights(p, o, s)
	})
}

// wallpaper is what lies under glass: the user's own image when they chose one, or the
// design's three lights.
func (v *View) wallpaper(c *ui.Context, o Options, s scene) {
	if o.BackgroundMode == BackgroundImage && o.Wallpaper != nil {
		o.Wallpaper(c)
		return
	}
	sx, sy := s.w/1440, s.h/900
	for i, l := range wallLight {
		e := ui.Box(c.Key(i)).Absolute().Left(l.x*sx).Top(l.y*sy).Size(l.w*sx, l.h*sy).Radius(9999).Opacity(.42).Background(l.col)
		if widget.Frosted {
			e.Blur(70)
		}
	}
}

// drawFrames has each region draw itself as a hairline, both ways round from one point
// on a side until the two halves meet, giving way to its material.
func drawFrames(p *ui.Painter, s scene) {
	if s.t > tStyle || s.t < tOpeningEnd {
		return
	}
	outline := func(r rect, radius, mat, from float32, side byte) {
		e := seg(s.t, from, from+860, easeInOut)
		a := 1 - mat
		if a <= .004 || e <= 0 {
			return
		}
		one, two := halves(r.x+.5, r.y+.5, r.w-1, r.h-1, radius, side)
		one.Trim(0, e)
		two.Trim(0, e)
		col := fadeColor(frameLine, a)
		p.StrokePath(one, 1, col)
		p.StrokePath(two, 1, col)
	}
	outline(s.doc.r, s.doc.radius, s.doc.mat, 7050, 't')
	outline(s.panel.r, s.panel.radius, s.panel.mat, 7200, 'l')
	outline(s.sb.r, s.sb.radius, s.sb.mat, 7260, 'r')
	outline(s.rail.r, s.rail.radius, s.rail.mat, 7520, 'l')
	for i, cap := range []part{s.ctx, s.pg, s.fd, s.act} {
		outline(cap.r, cap.radius, cap.mat, 7320+float32(i)*70, 'b')
	}
}

// drawLights is the window's three controls, one after another.
func drawLights(p *ui.Painter, s scene) {
	r := s.lights
	const size, gap = 12, 8
	x := r.cx() - (3*size+2*gap)/2
	for i, col := range []ui.Color{lightsR, lightsY, lightsG} {
		at := 7500 + float32(i)*70
		a := seg(s.t, at, at+320, easeOut)
		if a <= .004 {
			continue
		}
		o := float32(size) * (.7 + .3*easeBack(clamp((s.t-at)/320, 0, 1)))
		cx, cy := x+float32(i)*(size+gap)+size/2, r.cy()
		p.Fill(ui.Rect{X: cx - o/2, Y: cy - o/2, W: o, H: o}, fadeColor(col, a), o/2)
	}
}

// regionBox is a region's surface: its fill, its rim and light along the top and its
// shadow come with its material; what's in it goes in a box inside its rim.
func regionBox(c *ui.Context, key string, p part, surf ui.Color, inner func()) {
	if p.appear <= .004 {
		return
	}
	v := p.v
	col := fadeColor(surf, p.mat)
	e := ui.Box(c.Key(key)).Absolute().Left(p.r.x).Top(p.r.y).Size(p.r.w, p.r.h).Radius(p.radius).Clip().Opacity(p.appear).TextColor(v.ink).FontSize(v.fs)
	if p.family != "" {
		e.Font(p.family)
	}
	// The design's glass: what's under it blurred, more as its material comes, tinted.
	frosted := v.blur > .5 && p.mat > .3 && widget.Frosted
	if frosted {
		e.Material(glass.Blur{Radius: v.blur * p.mat})
	} else {
		e.Background(col)
	}
	e.InsetShadow(0, 1, 0, 0, fadeColor(v.gl, p.mat)).InsetShadow(0, 0, 0, v.lw, fadeColor(v.rim, p.mat))
	if sha := v.sha * p.mat; sha > .004 {
		e.Shadow(0, 10, 30, 0, rgba(0, 0, 0, sha))
	}
	e.Children(func() {
		if frosted {
			ui.Box(c.Key("tint")).Absolute().Left(0).Top(0).Right(0).Bottom(0).Background(col)
		}
		ui.Box(c.Key("inner")).Absolute().Left(v.lw).Top(v.lw).Right(v.lw).Bottom(v.lw).Opacity(p.text * p.content).Children(inner)
	})
}

// line is one of the design's two sizes of words on their line heights.
func line(c *ui.Context, text string, size float32, col ui.Color) ui.Element {
	lead := float32(lead12)
	if size >= 13 {
		lead = lead13
	}
	return ui.Text(c, text).FontSize(size).LineHeight(lead).TextColor(col).SingleLine()
}

// running is a line that may not fit: it runs under the pointer rather than being cut.
func running(c *ui.Context, text string, size float32, weight int, col ui.Color) {
	ui.Row(c).Grow(1).MinWidth(0).Children(func() {
		widget.Running(c, text, widget.RunStyle{Size: size, Weight: weight, Color: col})
	})
}

// tile is the rounded square an icon sits on.
func tile(c *ui.Context, svg *ui.SVG, size float32, col, ink ui.Color, opacity float32) {
	ui.Box(c).Size(28, 28).Radius(8).Background(col).Center().Shrink(0).Opacity(opacity).Children(func() {
		ui.Icon(c, svg).FontSize(size).TextColor(ink)
	})
}

// segmentedShown is the segmented control as a picture of the real one, sharing the
// room it's given, for what isn't to be used here.
func segmentedShown(c *ui.Context, v look, labels []string, fill bool) {
	row := ui.Row(c).Height(v.ctl).Padding(2).Radius(v.rc).Background(v.well).Shrink(0)
	if fill {
		row.FillWidth()
	}
	row.Children(func() {
		for i, label := range labels {
			bg, fg := rgba(0, 0, 0, 0), v.ink2
			if i == 0 {
				bg, fg = v.sel, v.onsel
			}
			one := ui.Row(c.Key(label)).Height(v.ctl - 4).PaddingX(4).Radius(v.rt).Background(bg).AlignItems(ui.Center).Justify(ui.Center)
			if fill {
				one.Grow(1).Basis(0).MinWidth(0)
			} else {
				one.PaddingX(8)
			}
			one.Children(func() { line(c, label, 12, fg) })
		}
	})
}

// segmented is the design's segmented control, saying which was clicked.
func segmented(c *ui.Context, v look, labels []string, on int, live, disabled bool) int {
	clicked := -1
	ui.Row(c).Height(v.ctl).Padding(2).Radius(v.rc).Background(v.well).Shrink(0).Children(func() {
		for i, label := range labels {
			bg, fg := rgba(0, 0, 0, 0), v.ink2
			switch {
			case disabled && i == on:
				bg, fg = v.fill, v.ink2
			case disabled:
				fg = v.ink3
			case i == on:
				bg, fg = v.sel, v.onsel
			}
			b := ui.ButtonBase(c.Key(label)).Height(v.ctl - 4).PaddingX(8).Radius(v.rt).Background(bg).Label(label).Center().Disabled(!live || disabled)
			if live && !disabled {
				b.Cursor(ui.CursorPointer)
			}
			b.Children(func() { line(c, label, 12, fg) })
			if live && !disabled && b.Clicked() {
				clicked = i
			}
		}
	})
	return clicked
}

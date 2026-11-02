package onboardview

import (
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/theme"
)

// rect is a place in the window.
type rect struct{ x, y, w, h float32 }

func (r rect) ui() ui.Rect { return ui.Rect{X: r.x, Y: r.y, W: r.w, H: r.h} }

func (r rect) cx() float32 { return r.x + r.w/2 }

func (r rect) cy() float32 { return r.y + r.h/2 }

func (r rect) right() float32 { return r.x + r.w }

// Widths the regions keep, whatever the window: only the conversation changes with it.
const (
	sideWidth  = 248
	panelWidth = 320
)

// geometry is where a style puts the regions in a window this large, as the app does.
type geometry struct {
	side, doc, panel, rail, ctx, pg, fd, act rect
	radius, barRadius, railRadius, rim       float32
}

// layoutFor is geometry for style s.
func layoutFor(w, h float32, s theme.Style) geometry {
	m, gap, bar := s.WindowInset, s.BentoGap, s.BarHeight
	top := m + bar + gap
	g := geometry{radius: s.RadiusPanel, barRadius: s.BarRadius, railRadius: s.RadiusFull, rim: s.Hairline}
	if s.FullDock {
		g.railRadius = s.RadiusPanel
	}
	g.side = rect{m, top, sideWidth, h - top - m}
	railH := h - top - m
	railY := top
	if !s.FullDock {
		railH = min(railH, 244) // it floats in its region, as a capsule
		railY = top + (h-top-m-railH)/2
	}
	g.rail = rect{w - m - s.RailWidth, railY, s.RailWidth, railH}
	g.panel = rect{g.rail.x - gap - panelWidth, top, panelWidth, h - top - m}
	docX := g.side.right() + gap
	g.doc = rect{docX, top, g.panel.x - gap - docX, h - top - m}
	// The bar's cells, the middle ones shrinking first in a small window.
	left := m + 84
	right := w - m - 232
	mid := max(right-left-gap*3-160, 200)
	g.ctx = rect{left, m, 160, bar}
	g.pg = rect{g.ctx.right() + gap, m, mid * .55, bar}
	g.fd = rect{g.pg.right() + gap, m, mid * .45, bar}
	g.act = rect{right, m, 232, bar}
	return g
}

// mixGeometry is a and b part of the way.
func mixGeometry(a, b geometry, x float32) geometry {
	r := func(p, q rect) rect {
		return rect{lerp(p.x, q.x, x), lerp(p.y, q.y, x), lerp(p.w, q.w, x), lerp(p.h, q.h, x)}
	}
	return geometry{
		side: r(a.side, b.side), doc: r(a.doc, b.doc), panel: r(a.panel, b.panel), rail: r(a.rail, b.rail),
		ctx: r(a.ctx, b.ctx), pg: r(a.pg, b.pg), fd: r(a.fd, b.fd), act: r(a.act, b.act),
		radius: lerp(a.radius, b.radius, x), barRadius: lerp(a.barRadius, b.barRadius, x), railRadius: lerp(a.railRadius, b.railRadius, x), rim: lerp(a.rim, b.rim, x),
	}
}

// roundedPath is a rectangle's outline as a path, from the middle of its side a,
// going round both ways by the caller's trims: drawing itself from one point.
func roundedPath(r rect, radius float32) *ui.Path {
	radius = min(radius, r.w/2, r.h/2)
	x0, y0, x1, y1 := r.x, r.y, r.right(), r.y+r.h
	p := &ui.Path{}
	p.MoveTo(r.cx(), y0)
	p.LineTo(x1-radius, y0).QuadTo(x1, y0, x1, y0+radius)
	p.LineTo(x1, y1-radius).QuadTo(x1, y1, x1-radius, y1)
	p.LineTo(x0+radius, y1).QuadTo(x0, y1, x0, y1-radius)
	p.LineTo(x0, y0+radius).QuadTo(x0, y0, x0+radius, y0)
	p.Close()
	return p
}

// wirePath is a route through legs with rounded corners.
func wirePath(points [][2]float32, radius float32) *ui.Path {
	p := &ui.Path{}
	p.MoveTo(points[0][0], points[0][1])
	for i := 1; i < len(points)-1; i++ {
		q, at, n := points[i-1], points[i], points[i+1]
		in := dist(q, at)
		out := dist(at, n)
		a, b := min(radius, in/2), min(radius, out/2)
		p.LineTo(at[0]+(q[0]-at[0])/max(in, 1)*a, at[1]+(q[1]-at[1])/max(in, 1)*a)
		p.QuadTo(at[0], at[1], at[0]+(n[0]-at[0])/max(out, 1)*b, at[1]+(n[1]-at[1])/max(out, 1)*b)
	}
	last := points[len(points)-1]
	p.LineTo(last[0], last[1])
	return p
}

func dist(a, b [2]float32) float32 {
	dx, dy := b[0]-a[0], b[1]-a[1]
	return float32(sqrt(float64(dx*dx + dy*dy)))
}

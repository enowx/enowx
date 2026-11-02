package onboardview

import (
	"github.com/egoist/mygo/ui"
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

// halves is a rounded rectangle as two paths from the middle of one side ('t', 'r', 'b'
// or 'l'), going round it opposite ways to meet on the far side: a frame drawing itself
// from one point.
func halves(x, y, w, h, r float32, side byte) (*ui.Path, *ui.Path) {
	r = min(r, w/2, h/2)
	x1, y1 := x+w, y+h
	type corner struct{ ax, ay, cx, cy, bx, by float32 }
	corners := [4]corner{
		{x1 - r, y, x1, y, x1, y + r},    // top right, going clockwise
		{x1, y1 - r, x1, y1, x1 - r, y1}, // bottom right
		{x + r, y1, x, y1, x, y1 - r},    // bottom left
		{x, y + r, x, y, x + r, y},       // top left
	}
	mids := map[byte][2]float32{'t': {x + w/2, y}, 'r': {x1, y + h/2}, 'b': {x + w/2, y1}, 'l': {x, y + h/2}}
	order := []byte{'t', 'r', 'b', 'l'}
	at := 0
	for i, c := range order {
		if c == side {
			at = i
		}
	}
	start, end := mids[side], mids[order[(at+2)%4]]
	one, two := (&ui.Path{}).MoveTo(start[0], start[1]), (&ui.Path{}).MoveTo(start[0], start[1])
	for k := range 2 {
		c := corners[(at+k)%4]
		one.LineTo(c.ax, c.ay).QuadTo(c.cx, c.cy, c.bx, c.by)
		c = corners[(at+3-k)%4]
		two.LineTo(c.bx, c.by).QuadTo(c.cx, c.cy, c.ax, c.ay)
	}
	return one.LineTo(end[0], end[1]), two.LineTo(end[0], end[1])
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

// polyline is a curve cut into straight legs, to find a point along it by distance and
// to draw part of it.
type polyline struct {
	points [][2]float32
	lens   []float32 // from the start, at each point
	total  float32
}

func polylineOf(points [][2]float32) polyline {
	l := polyline{points: points, lens: make([]float32, len(points))}
	for i := 1; i < len(points); i++ {
		l.total += dist(points[i-1], points[i])
		l.lens[i] = l.total
	}
	return l
}

// cubicThenLine is a cubic from (x0, y0) to (x3, y3), then a straight leg to (x4, y4).
func cubicThenLine(x0, y0, x1, y1, x2, y2, x3, y3, x4, y4 float32) polyline {
	const legs = 48
	points := make([][2]float32, 0, legs+2)
	for i := range legs + 1 {
		u := float32(i) / legs
		s := 1 - u
		points = append(points, [2]float32{
			s*s*s*x0 + 3*s*s*u*x1 + 3*s*u*u*x2 + u*u*u*x3,
			s*s*s*y0 + 3*s*s*u*y1 + 3*s*u*u*y2 + u*u*u*y3,
		})
	}
	return polylineOf(append(points, [2]float32{x4, y4}))
}

// at is the point this far along.
func (l polyline) at(d float32) (float32, float32) {
	d = clamp(d, 0, l.total)
	for i := 1; i < len(l.points); i++ {
		if d <= l.lens[i] {
			leg := max(l.lens[i]-l.lens[i-1], .001)
			u := (d - l.lens[i-1]) / leg
			a, b := l.points[i-1], l.points[i]
			return lerp(a[0], b[0], u), lerp(a[1], b[1], u)
		}
	}
	last := l.points[len(l.points)-1]
	return last[0], last[1]
}

// upTo is the curve up to this far along.
func (l polyline) upTo(d float32) [][2]float32 {
	out := [][2]float32{l.points[0]}
	for i := 1; i < len(l.points) && l.lens[i-1] < d; i++ {
		if l.lens[i] <= d {
			out = append(out, l.points[i])
			continue
		}
		x, y := l.at(d)
		out = append(out, [2]float32{x, y})
	}
	return out
}

func dist(a, b [2]float32) float32 {
	dx, dy := b[0]-a[0], b[1]-a[1]
	return float32(sqrt(float64(dx*dx + dy*dy)))
}

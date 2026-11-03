package onboardview

import (
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/theme"
)

// part is one of the window's regions at this moment.
type part struct {
	v       look // the style it has got to, the wave passing each region in turn
	r       rect
	radius  float32
	mat     float32 // its material, after being a hairline frame
	text    float32 // how much what's in it shows: it dips as the wave crosses
	content float32 // how far its content has arrived
	appear  float32 // the region itself, as its frame starts
	family  string
}

// scene is everything on screen as a function of the clock, as in the design.
type scene struct {
	t, w, h   float32
	returning bool
	a, b      look
	p         float32 // how far the style has changed
	same      bool
	v         look // the style, without the wave's delays: what lies between regions

	ctx, pg, fd, act, sb, doc, panel, rail part
	lights                                 rect

	wall, ground, band, fold float32
	matS, matD, matP, matR   float32
	family                   string
}

// How far behind the first region each one starts, and the last's start of the move.
const (
	delayCtx, delayPg, delayFd, delayAct = .02, .04, .06, .09
	delaySide, delayDoc                  = .05, .11
	delayPanel, delayRail                = .17, .22
	waveSpread                           = .22
)

func familyOf(id string) string { return theme.StyleByID(id).Family }

// sceneAt works out the moment t of a window w by h.
func (v *View) sceneAt(o Options, w, h, t float32) scene {
	from, to, p := v.morph.at(o.StyleID)
	s := scene{t: t, w: w, h: h, returning: v.flow == FlowReturning, p: p, same: from == to}
	s.a = v.lookFor(from, o)
	s.b = v.lookFor(to, o)
	r := s.returning
	m := func(a, b float32) float32 {
		if r {
			return seg(t, 8200+(a-17300)/2, 9000+(a-17300)/2, easeInOut)
		}
		return seg(t, a, b, easeInOut)
	}
	s.matP = seg(t, 8200, 8900, easeInOut)
	s.matS, s.matD, s.matR = m(17300, 18100), m(17450, 18250), m(17650, 18300)
	s.fold = seg(t, 45650, 46750, easeInOut)
	caps := [4]float32{m(17500, 18200), m(17580, 18280), m(17660, 18360), m(17740, 18440)}

	xOf := func(delay float32) float32 {
		if s.same {
			return 1
		}
		return easeInOut(clamp((p-delay)/(1-waveSpread), 0, 1))
	}
	s.v = lookBetween(s.a, s.b, easeInOut(p))
	s.family = familyOf(from)
	if p >= .5 {
		s.family = familyOf(to)
	}
	make1 := func(delay, mat, appear, content float32) part {
		x := xOf(delay)
		pt := part{v: lookBetween(s.a, s.b, x), mat: mat, appear: appear, content: content, text: 1, family: familyOf(from)}
		if x >= .5 {
			pt.family = familyOf(to)
		}
		if !s.same {
			pt.text = 1 - .42*max(0, 1-abs(x-.5)/.05)
		}
		return pt
	}
	ca := func(a, b, ra, rb float32, curve func(float32) float32) float32 {
		if r {
			return seg(t, ra, rb, easeInOut)
		}
		return seg(t, a, b, curve)
	}
	s.ctx = make1(delayCtx, caps[0], seg(t, 7300, 7500, easeInOut), seg(t, 8500, 8900, easeOut))
	s.pg = make1(delayPg, caps[1], seg(t, 7350, 7500, easeInOut), ca(17900, 18400, 8400, 9000, easeInOut))
	s.fd = make1(delayFd, caps[2], seg(t, 7400, 7500, easeInOut), ca(18000, 18500, 8450, 9050, easeInOut))
	s.act = make1(delayAct, caps[3], seg(t, 7450, 7600, easeInOut), ca(18100, 18600, 8500, 9100, easeInOut))
	s.sb = make1(delaySide, s.matS, seg(t, 7200, 7400, easeInOut), 1)
	s.doc = make1(delayDoc, s.matD, seg(t, 7050, 7250, easeInOut), 1)
	s.panel = make1(delayPanel, s.matP, seg(t, 7150, 7350, easeInOut), 1)
	first := seg(t, 8700, 9100, easeInOut)
	if r {
		first = seg(t, 8000, 8500, easeInOut)
	}
	s.rail = make1(delayRail, s.matR, seg(t, 7400, 7500, easeInOut), lerp(first, 1, s.matR)*lerp(.28, 1, s.matR))
	s.place(w, h)

	// The wallpaper leaves first as a change starts, and comes in last as it ends.
	wall := s.a.wall
	if !s.same {
		if s.a.wall > s.b.wall {
			wall = lerp(s.a.wall, s.b.wall, seg(p, 0, .55, easeInOut))
		} else {
			wall = lerp(s.a.wall, s.b.wall, seg(p, .45, 1, easeInOut))
		}
	}
	s.wall = wall * m(17600, 18800)
	if r {
		s.ground = 1 - seg(t, 8300, 9300, easeInOut)
	} else {
		s.ground = 1 - seg(t, 17500, 18900, easeInOut)
	}
	s.band = s.v.band * max(caps[0], caps[1], caps[2], caps[3])
	return s
}

// place puts the regions where each one's style has them. Widths the design keeps stay
// whatever the window: the conversation and the search take what changes.
func (s *scene) place(w, h float32) {
	vs, vd, vp, vr := s.sb.v, s.doc.v, s.panel.v, s.rail.v
	sbT := vs.m + vs.bar + vs.g
	sbR := vs.m + sideWidth
	s.sb.r, s.sb.radius = rect{vs.m, sbT, sideWidth, h - sbT - vs.m}, vs.r
	railL := w - vr.m - vr.rail
	panL := railL - vp.g - panelWidth
	docL := max(vd.m+sideWidth+vd.g, sbR+min(vd.g, -1))
	docR := lerp(min(panL-vd.g, panL+1), railL-vd.g, s.fold)
	dT := vd.m + vd.bar + vd.g
	s.doc.r, s.doc.radius = rect{docL, dT, docR - docL, h - dT - vd.m}, vd.r
	pT := vp.m + vp.bar + vp.g
	s.panel.r, s.panel.radius = rect{panL, pT, panelWidth, h - pT - vp.m}, vp.r
	rT := vr.m + vr.bar + vr.g
	mid := (rT + h - vr.m) / 2
	top, bot := lerp(rT, mid-122, vr.rf), lerp(h-vr.m, mid+122, vr.rf)
	s.rail.r, s.rail.radius = rect{railL, top, vr.rail, bot - top}, vr.railr

	grow := w - 1440
	cap1 := func(p *part, x, width float32) {
		p.r, p.radius = rect{x, p.v.m, width, p.v.bar}, p.v.capr
	}
	cap1(&s.ctx, s.ctx.v.ctxL, s.ctx.v.ctxW)
	cap1(&s.pg, s.pg.v.pgL, s.pg.v.pgW)
	cap1(&s.fd, s.fd.v.fdL, max(s.fd.v.fdW+grow, 120))
	cap1(&s.act, s.act.v.actL+grow, s.act.v.actW)
	vt := s.ctx.v
	s.lights = rect{vt.tlL, vt.m, vt.tlW, vt.bar}
}

func abs(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

// dot is a status dot.
func dot(c *ui.Context, size float32, col ui.Color) ui.Element {
	return ui.Box(c).Size(size, size).Radius(size / 2).Background(col).Shrink(0)
}

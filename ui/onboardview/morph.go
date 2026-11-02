package onboardview

import (
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/theme"
)

// The hero: picking a style plays one change across the window, a wave, each region
// starting a beat after the last, its place, corners and color crossing together. The
// theme's tokens are one set for the whole app, so what's in the regions can't be in
// two styles at once: it fades out in the old style, the style changes, and it fades
// back in, while the surfaces carry the change.

const (
	morphFade = 120 * time.Millisecond // what's in the regions leaves
	morphMove = 1100 * time.Millisecond
	// morphSpread is how far behind the first region the last one starts, of the move.
	morphSpread = .22
)

// What each region's start lags by, in the design's order.
const (
	delayCtx, delayPg, delayFd, delayAct = .02, .04, .06, .09
	delaySide, delayDoc                  = .05, .11
	delayPanel, delayRail                = .17, .22
)

// tokens are the theme's colors the surfaces use.
type tokens struct{ ground, main, panel, border ui.Color }

func tokensNow() tokens { return tokens{theme.Ground, theme.Background, theme.Panel, theme.Border} }

// morph is a change of style under way. It belongs to the main thread.
type morph struct {
	began    time.Time // when it was asked for, zero for none going
	moved    time.Time // when the style changed
	to       string
	queued   string // picked meanwhile: next
	a, b     tokens
	ga, gb   geometry
	progress float32 // of the move, as of this frame
}

func (m *morph) running() bool { return !m.began.IsZero() }

// start changes to style id, at once with less motion.
func (m *morph) start(o Options, id string) {
	switch {
	case o.Reduce:
		o.SetStyle(id)
	case m.running():
		m.queued = id
	default:
		m.begin(id)
	}
}

func (m *morph) begin(id string) {
	m.began, m.moved, m.to, m.progress = time.Now(), time.Time{}, id, 0
}

// step moves it on to now, changing the style when what's in the regions has left.
func (m *morph) step(o Options, w, h float32, now time.Time) {
	if !m.running() {
		return
	}
	if m.moved.IsZero() {
		if now.Sub(m.began) < morphFade {
			return
		}
		m.a, m.ga = tokensNow(), layoutFor(w, h, theme.StyleByID(o.StyleID))
		o.SetStyle(m.to)
		m.b, m.gb = tokensNow(), layoutFor(w, h, theme.StyleByID(m.to))
		m.moved = now
	}
	m.progress = clamp(float32(now.Sub(m.moved))/float32(morphMove), 0, 1)
	if m.progress < 1 {
		return
	}
	m.began, m.progress = time.Time{}, 0
	if m.queued != "" {
		id := m.queued
		m.queued = ""
		m.begin(id)
	}
}

// x is how far a region starting after delay has got.
func (m *morph) x(delay float32) float32 {
	if !m.running() || m.moved.IsZero() {
		if m.running() {
			return 0
		}
		return 1
	}
	return easeInOut(clamp((m.progress-delay)/(1-morphSpread), 0, 1))
}

// geometry is g with each region where the wave has it.
func (m *morph) geometry(g geometry) geometry {
	if !m.running() || m.moved.IsZero() {
		return g
	}
	r := func(delay float32, a, b rect) rect {
		x := m.x(delay)
		return rect{lerp(a.x, b.x, x), lerp(a.y, b.y, x), lerp(a.w, b.w, x), lerp(a.h, b.h, x)}
	}
	a, b := m.ga, m.gb
	x := m.x(delayDoc)
	return geometry{
		ctx: r(delayCtx, a.ctx, b.ctx), pg: r(delayPg, a.pg, b.pg), fd: r(delayFd, a.fd, b.fd), act: r(delayAct, a.act, b.act),
		side: r(delaySide, a.side, b.side), doc: r(delayDoc, a.doc, b.doc), panel: r(delayPanel, a.panel, b.panel), rail: r(delayRail, a.rail, b.rail),
		radius: lerp(a.radius, b.radius, x), barRadius: lerp(a.barRadius, b.barRadius, x), railRadius: lerp(a.railRadius, b.railRadius, m.x(delayRail)), rim: lerp(a.rim, b.rim, x),
	}
}

// color is a surface's color where the wave has it: pick says which of the tokens.
func (m *morph) color(pick func(tokens) ui.Color, delay float32) ui.Color {
	if !m.running() || m.moved.IsZero() {
		return pick(tokensNow())
	}
	return mixColor(pick(m.a), pick(m.b), m.x(delay))
}

// content is how much what's in the regions shows: gone before the style changes, back
// as the wave passes.
func (m *morph) content(now time.Time) float32 {
	switch {
	case !m.running():
		return 1
	case m.moved.IsZero():
		return 1 - clamp(float32(now.Sub(m.began))/float32(morphFade), 0, 1)
	}
	return seg(m.progress, .1, .55, easeOut)
}

// wall is how much of the wallpaper shows: it comes in last when glass is arrived at.
func (m *morph) wall(base float32) float32 {
	if !m.running() || m.moved.IsZero() {
		return base
	}
	return base * seg(m.progress, .45, 1, easeInOut)
}

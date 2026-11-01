package ui

import (
	"math"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"

	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// floatingFan draws the children of the open wedge as slices pulled out of
// the pie: an annulus of the pie's own shape, set above the rim so it never
// covers a slice, cut into one part a child. They bloom out of the wedge's
// angle and slide further out as they grow. It reports the child under the
// pointer (-1 for none) and the name the hub shows for it: no name floats over
// the pie, so two of them can never collide.
func (a *App) floatingFan(c *ui.Context, panel ui.Element, st *floatingState, f, t float32) (int, string) {
	kids := a.floatingFanActs(st.fan)
	at := 0
	for i, act := range a.floatingActions() {
		if act.fan == st.fan {
			at = i
		}
	}
	pie := floatingSlicesFor(floatingWedges)
	subs := floatingFanSlices(pie, at, len(kids), f)
	// The whole annulus sits past the pie's rim, and slides further out as
	// it blooms: inner starts at floatingPizzaOut+floatingFanGap, so no
	// child can ever be drawn over the pie.
	lift := floatingFanLift * f
	inner, outer := float32(floatingFanIn)+lift, float32(floatingFanOut)+lift
	iconAt := float64(floatingFanIn+floatingFanOut)/2 + float64(lift)
	hovers := make([]float32, len(kids))
	hovered, name := -1, ""
	for i, act := range kids {
		on := a.floatingShown && st.fan != "" && act.enabled && a.fanSliceAt(panel, subs, i, inner, outer)
		hovers[i] = a.floatingAnimate(panel, "hover:fan:"+act.key, float01(on), widget.Spring)
		if on {
			hovered, name = i, act.short
		}
	}
	box := ui.Box(c).Absolute().Left(0).Top(0).Size(floatingSize, floatingSize).PassThrough().
		Opacity(t * f)
	box.Draw(func(p *ui.Painter, r ui.Rect) {
		cx, cy := r.X+r.W/2, r.Y+r.H/2
		for i, act := range kids {
			p.FillPath(chart.Sector(cx, cy, inner, outer, subs[i]), a.floatingFanFill(act, hovers[i]))
		}
	})
	box.Children(func() {
		for i, act := range kids {
			angle := floatingMidOf(subs, i)
			x := float64(floatingSize)/2 + math.Cos(angle)*iconAt
			y := float64(floatingSize)/2 - math.Sin(angle)*iconAt
			// The aim: the sub-slice's own icon box, as a ring slice has
			// its own. The box paints nothing, so the slice behind it is
			// the button; hover and the lit fill come from chart.SliceAt
			// (fanSliceAt), so the whole sub-slice answers the pointer.
			b := ui.ButtonBase(c.Key("fan:"+act.key)).Absolute().
				Left(float32(x-floatingFanTarget/2)).Top(float32(y-floatingFanTarget/2)).
				Size(floatingFanTarget, floatingFanTarget).Radius(floatingFanTarget / 2).Column().Center().Gap(3).
				Label(act.label).Tooltip(act.label).Cursor(ui.CursorPointer)
			// Dimmed by colour, not by MyGo's disabled state, which halves
			// what the element paints: the sub-slice behind is opaque and
			// its icon must be too. act.enabled guards the click.
			icon := theme.Text
			if !act.enabled {
				icon = theme.Mix(theme.Text, theme.Background, floatingDim)
			}
			b.Children(func() {
				ui.Icon(c, act.svg).FontSize(16).TextColor(icon)
				ui.Text(c, act.short).FontSize(theme.TextLabel).TextColor(icon).
					MaxWidth(floatingFanTarget).MaxLines(2).TextAlign(ui.Center).PassThrough()
			})
			if b.Clicked() && act.enabled && a.floatingShown && st.fan != "" {
				if st.fan == floatingActiveFan {
					// Source selection starts the host/project drill-down.
					a.floatingChooseSource(st, i)
					a.restartPie(panel)
					continue
				}
				st.fan = ""
				a.hideFloating()
				a.runFloating(act)
			}
		}
	})
	return hovered, name
}

// floatingFanSlices cuts the wedge at index at of pie into n sub-slices,
// each floatingFanPad from its neighbour, and collapses them all onto the
// wedge's own mid angle as f goes to 0: they fan out of it as they bloom,
// and their span grows with f, so at f = 0 they are a point at that angle.
func floatingFanSlices(pie []chart.Slice, at, n int, f float32) []chart.Slice {
	out := make([]chart.Slice, n)
	if n == 0 || at < 0 || at >= len(pie) {
		return out
	}
	parent := pie[at]
	// The wedge's visible span: chart insets each slice by half its pad.
	from := parent.Start + parent.Pad/2
	to := parent.End - parent.Pad/2
	share := (to - from) / float64(n)
	mid := parent.Mid()
	k := float64(f)
	for i := range out {
		start := from + float64(i)*share
		out[i] = chart.Slice{
			Start: mid + (start-mid)*k,
			End:   mid + (start+share-mid)*k,
			Pad:   floatingFanPad * k,
		}
	}
	return out
}

// fanSliceAt reports whether the pointer is in sub-slice i of subs, whose
// annulus is inner..outer: the same test chart.SliceAt makes, with the ring's
// radii widened as a pie menu's are, so the whole sub-slice answers the
// pointer and not only its icon. It compares the angle from the sub-pie's own
// start rather than from zero, because a wedge's sub-slices can straddle
// twelve o'clock, where chart.SliceAt's angle begins.
func (a *App) fanSliceAt(panel ui.Element, subs []chart.Slice, i int, inner, outer float32) bool {
	if len(subs) == 0 {
		return false
	}
	x, y, over := panel.PointerPosition()
	if !over {
		return false
	}
	cx, cy := float32(floatingSize)/2, float32(floatingSize)/2
	dx, dy := float64(x-cx), float64(y-cy)
	if d := math.Hypot(dx, dy); d < float64(inner-2) || d > float64(outer+2) {
		return false
	}
	// chart's angle: from twelve o'clock, clockwise.
	angle := math.Atan2(dx, -dy)
	origin := subs[0].Start
	rel := math.Mod(angle-origin, 2*math.Pi)
	if rel < 0 {
		rel += 2 * math.Pi
	}
	for j, s := range subs {
		if rel >= s.Start-origin && rel < s.End-origin {
			return j == i
		}
	}
	return false
}

// floatingFanFill is a fan child's colour: the theme's raised surface, dimmed
// toward the background where the child can't run, and lit toward the
// selection as it hovers, as a ring slice's is. Opaque, like the ring's.
func (a *App) floatingFanFill(act floatingAction, hover float32) ui.Color {
	fill := a.floatingSurface()
	if !act.enabled {
		fill = theme.Mix(fill, theme.Background, floatingDim)
	}
	return theme.Mix(fill, a.floatingAccent(), float64(hover)*0.24)
}

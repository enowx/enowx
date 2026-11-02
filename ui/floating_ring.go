package ui

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"

	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// floatingPizza paints the slices, the selector band under the hovered one,
// the thin decorator ring, and a button over each slice's icon. The slices
// are drawn by the panel itself, so there is no disc behind them: what the
// wedges leave empty shows the desktop. It returns the slice under the
// pointer, or -1, which names it in the hub.
func (a *App) floatingPizza(c *ui.Context, panel ui.Element, st *floatingState, acts []floatingAction, t, sweep, spin float32) int {
	pie := floatingSlicesFor(len(acts))
	// One hover value a slice, kept on the panel so it survives the frame,
	// and so a slice that lights eases rather than snaps.
	hovers := make([]float32, len(acts))
	hovered := -1
	for i, act := range acts {
		on := a.floatingShown && act.enabled && sweep > 0.9 && a.sliceAt(panel, pie, i)
		hovers[i] = a.floatingAnimate(panel, "lit:"+act.key, float01(on), widget.Spring)
		if on {
			hovered = i
		}
	}

	// The pie turns into place and grows, and turns again as its slices
	// swap: both are the same transform, so nothing fights over it.
	turn := -floatingSweep*(1-sweep) + floatingSpin*(1-spin)
	scale := floatingOpenScale + (1-floatingOpenScale)*sweep
	if spin < 1 {
		scale *= 0.9 + 0.1*spin
	}
	box := ui.Box(c).Absolute().Left(0).Top(0).Size(floatingSize, floatingSize).PassThrough().
		Opacity(t).TransformOrigin(0.5, 0.5).Rotate(turn).Scale(scale, scale)
	box.Draw(func(p *ui.Painter, r ui.Rect) {
		cx, cy := r.X+r.W/2, r.Y+r.H/2
		a.floatingDecorator(p, cx, cy)
		for i, act := range acts {
			sector := chart.Sector(cx, cy, floatingPizzaIn, floatingPizzaOut, pie[i])
			p.FillPath(sector, a.sliceFill(st, act, hovers[i]))
			p.StrokePath(sector, 1, theme.Mix(theme.Border, a.floatingAccent(), float64(hovers[i])*0.5))
		}
	})
	box.DrawOver(func(p *ui.Painter, r ui.Rect) {
		// The selector: a band just inside the ring under the hovered
		// slice, growing out of its inner edge as the slice lights.
		selected := hovered
		if selected < 0 && st.fan != "" {
			for i, act := range acts {
				if act.fan == st.fan {
					selected = i
				}
			}
		}
		if selected < 0 {
			return
		}
		cx, cy := r.X+r.W/2, r.Y+r.H/2
		hv := hovers[selected]
		if st.fan != "" && acts[selected].fan == st.fan {
			hv = 1
		}
		width := floatingSelectorWidth * (0.92 + 0.08*hv)
		p.FillPath(chart.Sector(cx, cy, floatingSelectorIn, floatingSelectorIn+width, pie[selected]),
			theme.Fade(a.floatingAccent(), 0.55*float64(hv)))
	})
	box.Children(func() {
		// The catcher: one element over the whole ring, under every icon's
		// own button, so a press anywhere in a slice picks that slice. The
		// visible shape is the sector; an icon's box alone would
		// swallow presses on the rest of it. It paints nothing and takes
		// the pointer only inside the pie's own radii (the same widened
		// band sliceAt uses), so the hub and the air around the ring stay
		// free.
		a.floatingCatcher(c, st, acts, pie, t)
		for i, act := range acts {
			a.floatingSlice(c, act, i, floatingMidOf(pie, i), hovers[i], t, st)
		}
	})
	return hovered
}

// floatingCatcher is the press catcher for the whole pie: an invisible button
// over the ring's band, which turns a press on any part of a slice into that
// slice's own click. It is built before the slices' buttons and the hub, so
// they sit over it and keep their own presses; the catcher only sees what they
// do not cover, and it ignores the corners of its square and the ring's hole.
func (a *App) floatingCatcher(c *ui.Context, st *floatingState, acts []floatingAction, pie []chart.Slice, t float32) {
	inner := float32(floatingPizzaIn) - floatingSelectIn*floatingRingWidth
	outer := float32(floatingPizzaOut) + floatingSelectOut*floatingRingWidth
	size := 2 * outer
	// It paints nothing and names nothing: the slices behind it are the
	// shape and the labels, and a background, a radius or a label here
	// would be a disc and a text under the pie.
	b := ui.ButtonBase(c.Key("pie")).Absolute().
		Left(float32(floatingSize)/2-outer).Top(float32(floatingSize)/2-outer).
		Size(size, size).Radius(outer).Opacity(t).FocusRing(false).
		Cursor(ui.CursorPointer)
	if !a.floatingShown {
		b.Disabled(true)
	}
	if !b.Clicked() {
		return
	}
	// The click is somewhere in the square: only a slice of the ring takes
	// it, so the hole in the middle and the corners do nothing.
	x, y, over := b.PointerPosition()
	if !over {
		return
	}
	cx, cy := float32(floatingSize)/2, float32(floatingSize)/2
	j, ok := chart.SliceAt(pie, x-cx, y-cy, inner, outer)
	if !ok || j < 0 || j >= len(acts) {
		return
	}
	a.floatingPick(acts[j], j, st)
}

// floatingDim is how far a slice that can't run is mixed toward the
// background: dim enough to read as unavailable, and still solid. It is a mix,
// not a fade — a slice is the panel's body, and its alpha must stay 255 or the
// desktop shows through it.
const floatingDim = 0.45

// sliceFill is a slice's colour: the theme's raised surface, a session's
// status tint, the open fan's own wedge in the accent, and the hover mix.
// Its alpha is always 255: the slices are opaque, and only what the pie does
// not paint shows the desktop.
func (a *App) sliceFill(st *floatingState, act floatingAction, hover float32) ui.Color {
	fill := a.floatingSurface()
	if act.status != nil {
		// A session's slice carries its status colour, so one waiting reads
		// amber from across the screen.
		fill = theme.Mix(fill, act.status.color, 0.2)
	}
	if !act.enabled {
		// Dim toward the background, which keeps the alpha: theme.Fade
		// would make the slice see-through.
		fill = theme.Mix(fill, theme.Background, floatingDim)
	}
	if st.fan != "" && act.fan == st.fan {
		fill = theme.Mix(fill, a.floatingAccent(), 0.18)
	}
	return theme.Mix(fill, a.floatingAccent(), float64(hover)*0.24)
}

// floatingDecorator is the thin ring just outside the pie: the only outline
// the panel has, and it follows the arc, not the window's square.
func (a *App) floatingDecorator(p *ui.Painter, cx, cy float32) {
	r := float32(floatingPizzaOut + floatingDecoratorGap)
	ring := chart.Sector(cx, cy, r, r+floatingDecoratorWidth,
		chart.Slice{Start: floatingCenterAngle + math.Pi/2, End: floatingCenterAngle + math.Pi/2 + 2*math.Pi})
	p.FillPath(ring, theme.Fade(theme.BorderStrong, 0.6))
}

// floatingTitle cross-fades the centre circle's title to name: the old one
// fades out as the new one fades in, so it never pops. The name is the
// hovered slice's short label, a fan child's, or what an empty source says.
func (a *App) floatingTitle(panel ui.Element, st *floatingState, name string) {
	st.title = name
	st.fade = a.floatingAnimate(panel, "title", float01(name != ""), ui.Animation{Duration: 120 * time.Millisecond, Ease: widget.Out})
}

// floatingSlice places an icon and caption in slice i, or a session's icon
// and status dot. The hub names a hovered session without crowding the ring.
//
// The button is the aim for the icon, not the target: a click anywhere in the
// slice picks it, through the catcher floatingPizza puts under the whole ring
// and the same chart.SliceAt test that lights it (sliceAt), because the visible
// shape is the sector and an icon's box inside it would swallow clicks on the
// rest of it.
func (a *App) floatingSlice(c *ui.Context, act floatingAction, i int, angle float64, hover, t float32, st *floatingState) {
	x := float64(floatingSize)/2 + math.Cos(angle)*floatingIconAt
	y := float64(floatingSize)/2 - math.Sin(angle)*floatingIconAt
	// The icon lifts outward along its own slice as it grows.
	lift := 2 * hover
	x += math.Cos(angle) * float64(lift)
	y -= math.Sin(angle) * float64(lift)
	b := ui.ButtonBase(c.Key(act.key)).Absolute().
		Left(float32(x-floatingSliceTarget/2)).Top(float32(y-floatingSliceTarget/2)).
		Size(floatingSliceTarget, floatingSliceTarget).Radius(floatingSliceTarget / 2).Column().Center().Gap(theme.S1).
		Opacity(t).Label(act.label).Tooltip(act.label).Cursor(ui.CursorPointer)
	if !a.floatingShown {
		b.Disabled(true)
	}
	// A slice that can't run is not disabled at the element level: MyGo's
	// disabled state halves what the element paints (ui/paint.go, "if
	// e.flags&flagDisabled != 0 { p.opacity *= 0.5 }"), which would make its
	// icon see-through on an opaque slice. It takes no click anyway
	// (act.enabled guards floatingPick), and its icon is dimmed by colour.
	ink := theme.Mix(theme.Text, a.floatingAccent(), float64(hover)*0.4)
	if !act.enabled {
		ink = theme.Mix(theme.Text, theme.Background, floatingDim)
	}
	b.Children(func() {
		size := float32(21)
		if act.status != nil {
			size = 26
		}
		ic := ui.Icon(c, act.svg).FontSize(size).TextColor(ink)
		ic.Scale(1+0.08*hover, 1+0.08*hover)
		if act.status == nil {
			ui.Text(c, floatingCaption(act)).FontSize(theme.TextLabel).TextColor(ink).
				MaxWidth(floatingSliceTarget).MaxLines(2).TextAlign(ui.Center).PassThrough()
		}
		if act.status != nil {
			// The session's state, outward of the icon.
			dot := ui.Box(c).Absolute().
				Left(float32(floatingSliceTarget/2+math.Cos(angle)*18-5)).
				Top(float32(floatingSliceTarget/2-math.Sin(angle)*18-5)).
				Size(10, 10).Center().PassThrough()
			dot.Children(func() { widget.Dot(c, act.status.color, act.status.hollow, 8) })
		}
	})
	if act.fan != "" {
		const cueAt = floatingPizzaOut - 13
		ui.Icon(c, icon.ChevronRight).Absolute().
			Left(float32(float64(floatingSize)/2+math.Cos(angle)*cueAt-5)).
			Top(float32(float64(floatingSize)/2-math.Sin(angle)*cueAt-5)).
			Size(10, 10).FontSize(theme.TextLabel).TextColor(ink).
			Rotate(float32(-angle * 180 / math.Pi)).Opacity(t).PassThrough()
	}
	if b.Clicked() && act.enabled {
		a.floatingPick(act, i, st)
	}
}

// floatingPick runs what a slice does: it opens or folds its fan, or runs its
// action and folds the panel away. One place, so the icon's box and the
// sector around it do exactly the same thing.
func (a *App) floatingPick(act floatingAction, i int, st *floatingState) {
	if !act.enabled {
		return
	}
	if act.navigate != nil {
		act.navigate(st)
		return
	}
	if act.fan != "" {
		// One fan at a time: another closes as this one opens.
		if st.fan == act.fan {
			st.fan = ""
		} else {
			st.fan = act.fan
		}
		return
	}
	a.hideFloating()
	a.runFloating(act)
}

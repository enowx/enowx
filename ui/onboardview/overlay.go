package onboardview

import (
	"math"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/ui/icon"
)

// What travels between regions: agents and the folder flying to their slots, the host's
// wire and card, GOD of Agents' node and branches, the ring where things arrive. Each is
// measured from what it joins, every frame, so nothing floats free of it.

func wirePort(p *ui.Painter, s scene, x, y, a, scale, ring, centre float32) {
	port(p, x, y, a, scale, ring, centre, s.v.node, s.v.wire, s.v.wire)
}

// panelRowsTop is the top of the first row of the agents' list, in the window.
func panelRowsTop(s scene) float32 {
	return s.panel.r.y + s.panel.v.lw + 40 + 12 + 18 + 8 + 16 + 8
}

// drawFlights has each agent found go from its row to its slot along an arc, with weight,
// one after the next, and the folder after them.
func (v *View) drawFlights(p *ui.Painter, o Options, s scene) {
	if s.returning {
		return
	}
	t := s.t
	flight := func(svg *ui.SVG, iconSize, x0, y0, x1, y1, from, to, plate0, plate1 float32) {
		x := seg(t, from, to, easeInOut)
		if t < from-20 || t >= to+40 {
			return
		}
		cx, cy := (x0+x1)/2, min(y0, y1)-64
		u := 1 - x
		px, py := u*u*x0+2*u*x*cx+x*x*x1, u*u*y0+2*u*x*cy+x*x*y1
		box := ui.Rect{X: px - 14, Y: py - 14, W: 28, H: 28}
		if plate := 1 - seg(t, plate0, plate1, easeInOut); plate > .004 {
			p.Fill(box, fadeColor(s.v.tile, plate), 8)
			p.Stroke(box, fadeColor(s.v.nodel, plate), 8, 1)
		}
		p.Icon(svg, ui.Rect{X: px - iconSize/2, Y: py - iconSize/2, W: iconSize, H: iconSize}, s.v.ink)
	}
	if t > 11700 && t < 13300 {
		rows := listRows(o)
		x0 := s.panel.r.x + s.panel.v.lw + 12 + 14
		top := panelRowsTop(s)
		for n, ag := range rows {
			if !ag.Installed {
				continue
			}
			from := 11900 + float32(n)*80
			x1, y1 := slotIcon(s, float32(n))
			flight(icon.Agent(agent.Base(ag.ID)), 16, x0, top+float32(n)*48+24, x1, y1, from, from+800, from+320, from+720)
		}
	}
	if t > 13850 && t < 14850 && v.projectName(o) != "" {
		x0 := s.panel.r.x + s.panel.v.lw + 12 + 1 + 8 + 14
		y0 := s.panel.r.y + s.panel.v.lw + 40 + 12 + 18 + 8 + 32 + 8 + float32(v.pick)*60 + 26
		found := foundOf(listRows(o))
		vs := s.sb.v
		x1, y1 := s.sb.r.x+vs.lw+8+8+7, s.sb.r.y+vs.lw+projectY(s, found)+vs.rowh/2
		flight(icon.Folder, 14, x0, y0, x1, y1, 13900, 14750, 14220, 14670)
	}
}

// serverAt is where the host's card is: a little under the project's row, then shrinking
// away to the host menu at the top. The second value is its scale.
func (v *View) serverAt(o Options, s scene) (rect, float32) {
	vs := s.sb.v
	rowCy := s.sb.r.y + vs.lw + projectY(s, foundOf(listRows(o))) + vs.rowh/2
	cardX, cardY := s.doc.r.x+128, rowCy+84
	out := seg(s.t, 17420, 18150, easeInOut)
	scale := lerp(1, .62, out)
	ox, oy := s.ctx.r.x+8, s.ctx.r.cy()-30*.62
	x := lerp(cardX, ox, out)
	y := lerp(cardY, oy, out) - float32(math.Sin(float64(out)*math.Pi))*24
	return rect{x, y, 264 * scale, 60 * scale}, scale
}

// drawHostWire is the moment: a wire leaving a port on the project's row, turning two
// rounded corners and landing on a port on the server as it connects.
func (v *View) drawHostWire(p *ui.Painter, o Options, s scene) {
	t := s.t
	if !v.wireOn || s.returning || t <= tHost || t >= 18300 {
		return
	}
	vs := s.sb.v
	ax := s.sb.r.right()
	ay := s.sb.r.y + vs.lw + projectY(s, foundOf(listRows(o))) + vs.rowh/2
	bx, by := s.doc.r.x+128, ay+84+30
	mx := s.doc.r.x + 52
	drawn, gone := seg(t, 15850, 16620, easeInOut), seg(t, 17100, 17480, easeIn)
	if drawn > gone {
		path := wirePath([][2]float32{{ax, ay}, {mx, ay}, {mx, by}, {bx, by}}, 10)
		path.Trim(gone, drawn)
		p.StrokePath(path, 1.5, s.v.wire)
	}
	pa := seg(t, 15650, 15880, easeBack) * (1 - seg(t, 17380, 17500, easeInOut))
	pb := seg(t, 16540, 16760, easeBack) * (1 - seg(t, 17150, 17300, easeInOut))
	wirePort(p, s, ax, ay, clamp(pa, 0, 1), .6+.4*pa, 4, 1.5)
	wirePort(p, s, bx, by, clamp(pb, 0, 1), .6+.4*pb, 4, 1.5)
}

// serverNode is the host the user added, connecting, then gone up to the host menu.
func (v *View) serverNode(c *ui.Context, o Options, s scene) {
	t := s.t
	if !v.wireOn || s.returning || t <= tHost || t >= 18300 {
		return
	}
	in := seg(t, 15350, 15800, easeOut)
	a := in * (1 - seg(t, 17950, 18200, easeIn))
	if a <= .004 {
		return
	}
	r, scale := v.serverAt(o, s)
	status, col := "Connecting…", dotWait
	if t >= 16760 {
		status, col = "Saved", dotDone
	}
	vv := s.v
	ui.Row(c.Key("ob-server")).Absolute().Left(r.x).Top(r.y+(1-in)*8).Size(264, 60).TransformOrigin(0, 0).Scale(scale, scale).Opacity(a).
		PaddingX(12).Gap(8).AlignItems(ui.Center).Radius(min(8, vv.rc)).Background(vv.node).InsetShadow(0, 0, 0, 1, vv.nodel).Clip().Children(func() {
		tile(c, icon.Server, 14, vv.nodet, vv.ink, 1)
		ui.Column(c.Key("words")).Grow(1).MinWidth(0).Children(func() {
			line(c, v.hostName(o), vv.fs, vv.ink).FontWeight(500)
			ui.Row(c.Key("state")).Gap(4).AlignItems(ui.Center).Children(func() {
				dot(c, 6, col)
				line(c, status, 12, vv.ink2)
			})
		})
	})
}

// goaNodeAt is the node's place: out of its chip in the bar, into the head of the list.
func (v *View) goaNodeAt(s scene) rect {
	t := s.t
	vs := s.sb.v
	chip := rect{v.goaChip.X, v.goaChip.Y, v.goaChip.W, v.goaChip.H}
	if chip.w == 0 {
		chip = rect{s.pg.r.x + 180, s.pg.r.y + 6, 108, vs.ctl}
	}
	nl, nt := s.sb.r.x+vs.lw+8, s.sb.r.y+vs.lw+vs.ctl+14
	in, out := seg(t, 37000, 37760, easeInOut), seg(t, 39800, 40450, easeInOut)
	var lx, ly float32
	if out > 0 {
		lx, ly = 1-seg(out, .42, 1, easeInOut), 1-seg(out, 0, .62, easeInOut)
	} else {
		lx, ly = seg(in, .3, 1, easeInOut), seg(in, 0, .62, easeInOut)
	}
	return rect{lerp(chip.x, nl, lx), lerp(chip.y, nt, ly), lerp(chip.w, 230, lx), lerp(chip.h, 36, ly)}
}

// goaNode is GOD of Agents arriving in the list's head, the sessions under it.
func (v *View) goaNode(c *ui.Context, o Options, s scene) {
	t := s.t
	if s.returning || t <= 36860 || t >= 40500 {
		return
	}
	a := seg(t, 36980, 37120, easeInOut) * (1 - seg(t, 40330, 40470, easeInOut))
	if a <= .004 {
		return
	}
	r := v.goaNodeAt(s)
	vv := s.v
	name := seg(t, 37650, 37900, easeOut) * (1 - seg(t, 39800, 39950, easeInOut))
	ui.Row(c.Key("ob-goa")).Absolute().Left(r.x).Top(r.y).Size(r.w, r.h).Opacity(a).PaddingX(8).Gap(8).AlignItems(ui.Center).
		Radius(min(8, vv.rc)).Background(vv.node).InsetShadow(0, 0, 0, 1, vv.nodel).Clip().Children(func() {
		ui.Box(c.Key("tile")).Size(24, 24).Radius(8).Background(vv.nodet).Center().Shrink(0).Children(func() {
			ui.Icon(c, icon.Sparkle).FontSize(14).TextColor(vv.ink)
		})
		line(c.Key("title"), "GOD of Agents", vv.fs, vv.ink).FontWeight(500)
		ui.Spacer(c)
		ui.Box(c.Key("on")).Opacity(name).Children(func() { line(c, nonEmpty(v.goaName(o), "Claude Code"), 12, vv.ink2) })
	})
}

// drawGOAWires is one trunk from the node, growing at one speed, a branch leaving it for
// each session as it passes, ending on a port on the session's row.
func (v *View) drawGOAWires(p *ui.Painter, o Options, s scene) {
	t := s.t
	if s.returning || t <= 36860 || t >= 40500 {
		return
	}
	r := v.goaNodeAt(s)
	tx, ty := r.x+10, r.y+36
	vs := s.sb.v
	rows := listRows(o)
	found := foundOf(rows)
	_, ind := listPush(s)
	rowL := s.sb.r.x + vs.lw + 8 + ind
	length := func(cy float32) float32 { return (cy - ty) + (rowL - tx) - 3 }
	var most float32
	for n := range found {
		most = max(most, length(s.sb.r.y+vs.lw+rowY(s, float32(n))+vs.rowh/2))
	}
	d := seg(t, 37780, 38620, easeInOut) * (1 - seg(t, 39420, 39880, easeIn)) * most
	wirePort(p, s, tx, ty, seg(t, 37700, 37860, easeOut)*(1-seg(t, 39840, 39960, easeInOut)), 1, 4, 1.5)
	for n := range found {
		cy := s.sb.r.y + vs.lw + rowY(s, float32(n)) + vs.rowh/2
		l := length(cy)
		if vis := min(l, d); vis > .25 {
			path := wirePath([][2]float32{{tx, ty}, {tx, cy}, {rowL, cy}}, 7)
			path.Trim(0, vis/max(l, 1))
			p.StrokePath(path, 1.5, s.v.wire)
		}
		arr := clamp((d-(l-10))/12, 0, 1)
		wirePort(p, s, rowL, cy, arr, .6+.4*easeBack(arr), 3.5, 1.25)
	}
}

// drawRing marks where things arrive: one ring, opening and fading.
func (v *View) drawRing(p *ui.Painter, o Options, s scene) {
	t := s.t
	if s.returning {
		return
	}
	var to rect
	var e float32
	chip := rect{v.goaChip.X, v.goaChip.Y, v.goaChip.W, v.goaChip.H}
	switch {
	case v.wireOn && t >= 18120 && t < 18700:
		to, e = s.ctx.r, seg(t, 18120, 18700, easeOut)
	case t >= 36820 && t < 37300:
		to, e = chip, seg(t, 36820, 37300, easeOut)
	case t >= 40360 && t < 40900:
		to, e = chip, seg(t, 40360, 40900, easeOut)
	case v.task.sent && t >= 41900 && t < 42500:
		vs := s.sb.v
		to = rect{s.sb.r.x + vs.lw + 8 + 230 - 16, s.sb.r.y + vs.lw + rowY(s, 0) + vs.rowh/2 - 4, 8, 8}
		e = seg(t, 41900, 42500, easeOut)
	default:
		return
	}
	g := 2 + e*6
	box := ui.Rect{X: to.x - g, Y: to.y - g, W: to.w + 2*g, H: to.h + 2*g}
	p.Stroke(box, fadeColor(s.v.wire, (1-e)*.9), min(to.h/2, 14)+g, 1.5)
}

package onboardview

import (
	"math"
	"runtime"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/icon"
)

func sqrt(x float64) float64 { return math.Sqrt(x) }

// The opening: one cursor divides into agents, each goes out along a wire to a slot on a
// host's card, and they all come home to make the logo. Its colors are its own: the same
// dark room in every style and palette.
var (
	openingGround = ui.Hex("#0B0C10")
	openingCardBG = ui.Hex("#121319")
	openingSlotBG = ui.Hex("#0C0D12")
	openingLine   = ui.Hex("#474A58")
	openingRim    = ui.Hex("#2A2B34")
	openingPort   = ui.Hex("#6E7080")
	openingDot    = ui.Hex("#9A9CAA")
	openingInk    = ui.Hex("#ECECF0")
	openingInk2   = ui.Hex("#A3A5B0")
	openingTile   = ui.Hex("#17181F")
)

// What goes where, as measured from the room's middle: each agent has an offset in the
// row, a host and a slot on it, a delay out from the middle, and a wave its wire leaves in.
var openingAgents = []struct {
	id, name string
	off      float32
	host     int
	slot     int
	delay    float32
	wave     float32
}{
	{"claude", "Claude Code", -192, 0, 0, 120, 0},
	{"codex", "Codex", -96, 0, 1, 60, 1},
	{"opencode", "OpenCode", 0, 1, 0, 0, 2},
	{"gemini", "Gemini CLI", 96, 2, 0, 60, 1},
	{"kiro", "Kiro CLI", 192, 2, 1, 120, 0},
}

// The line heights the design sets its two sizes on.
const (
	lead13 = 18.0 / 13
	lead12 = 16.0 / 12
)

const (
	openingCardW = 248
	openingCardH = 64
	openingSlot  = 26
	// The room's middle lies a little under the window's: the tiles' row.
	openingDrop = 50
)

// openingCardX is where host i's card starts, from the middle.
var openingCardX = []float32{-508, -124, 260}

// room is the opening's place in a window this large.
type room struct {
	cx, cy, spread float32
}

func roomOf(w, h float32) room {
	return room{w / 2, h/2 + openingDrop, min(1, (w-40)/1016)}
}

func (r room) card(i int) rect {
	x := openingCardX[i]
	return rect{r.cx + (x+openingCardW/2)*r.spread - openingCardW/2, r.cy - 236, openingCardW, openingCardH}
}

// openingWire is an agent's way out: the curve up to its card's foot, where the wire
// ends at a port, and on into its slot, where only the tile goes.
type openingWire struct {
	line     polyline
	x0, y0   float32
	x1, foot float32
}

func (r room) wire(i int) openingWire {
	ag := openingAgents[i]
	card := r.card(ag.host)
	slot := card.right() - 8 - openingSlot - 4 - openingSlot + openingSlot/2 + float32(ag.slot)*30
	x0, y0 := r.cx+ag.off*r.spread, r.cy-22
	foot, centre := card.y+card.h, card.cy()
	mid := (y0 + foot) / 2
	return openingWire{line: cubicThenLine(x0, y0, x0, mid, slot, mid, slot, foot, slot, centre), x0: x0, y0: y0, x1: slot, foot: foot}
}

// wave and home are when an agent's wire goes out and is taken up, by how far its host is.
func wave(i int) (out, home float32) {
	w := openingAgents[i].wave
	return w * 130, (2 - w) * 70
}

// opening draws the stage: the opening, then its giving way.
func (v *View) opening(c *ui.Context, o Options, w, h, t float32) {
	if t >= tOpeningEnd+450 || v.flow == FlowDevice {
		return
	}
	stage := 1 - seg(t, tOpeningEnd, tOpeningEnd+450, easeIn)
	if v.reduce {
		stage = 1 - seg(t, tOpeningEnd, tOpeningEnd+1, easeIn)
	}
	r := roomOf(w, h)
	ui.Box(c.Key("stage")).Absolute().Left(0).Top(0).Right(0).Bottom(0).Opacity(stage).Children(func() {
		layer := func(key string, draw func(p *ui.Painter)) {
			ui.Box(c.Key(key)).Absolute().Left(0).Top(0).Right(0).Bottom(0).PassThrough().Draw(func(p *ui.Painter, _ ui.Rect) { draw(p) })
		}
		layer("wires", func(p *ui.Painter) { drawOpeningWires(p, r, t) })
		for i := range openingCardX {
			openingCard(c.Key(i), o, r.card(i), i, t)
		}
		layer("tiles", func(p *ui.Painter) {
			drawOpeningTiles(p, r, t)
			drawLogo(p, r, t)
		})
		for i, ag := range openingAgents {
			in := seg(t, 1820+ag.delay, 2180+ag.delay, easeOut)
			if a := in * (1 - seg(t, 2640, 2820, easeIn)); a > .004 {
				ui.Row(c.Key(ag.id)).Absolute().Left(r.cx + ag.off*r.spread - 48).Top(r.cy + 28 + (1-in)*4).Width(96).Justify(ui.Center).Opacity(a).Children(func() {
					ui.Text(c, openingAgents[i].name).FontSize(12).TextColor(openingInk2).SingleLine()
				})
			}
		}
		in := seg(t, 5350, 5800, easeOut)
		if a := in * (1 - seg(t, 6850, 7150, easeIn)); a > .004 {
			ui.Column(c.Key("name")).Absolute().Left(0).Right(0).Top(r.cy-16).Center().Opacity(a).Translate(0, (1-in)*6).Children(func() {
				ui.Text(c, "enowX").FontSize(13).LineHeight(lead13).FontWeight(600).TextColor(openingInk).SingleLine()
				ui.Text(c, "One place for agents on every host.").FontSize(12).LineHeight(lead12).TextColor(openingInk2).SingleLine()
			})
		}
	})
}

// openingCard is a host's card, its two empty slots waiting for agents at its right.
func openingCard(c *ui.Context, o Options, r rect, i int, t float32) {
	in := seg(t, 2340+float32(i)*90, 2800+float32(i)*90, easeOut)
	a := in * (1 - seg(t, 4560+float32(i)*40, 4900+float32(i)*40, easeIn))
	if a <= .004 {
		return
	}
	name, sub, svg := nonEmpty(o.Device, "this device"), systemName()+", this device", icon.Monitor
	switch i {
	case 1:
		name, sub, svg = "enowxlabs", "Linux, over SSH", icon.Server
	case 2:
		name, sub, svg = "studio-mac", "macOS, tailnet", icon.Monitor
	}
	ui.Row(c).Absolute().Left(r.x).Top(r.y).Size(r.w, r.h).Opacity(a).Translate(0, (1-in)*10).Gap(8).AlignItems(ui.Center).
		Padding(0, 8, 0, 12).Radius(8).Background(openingCardBG).InsetShadow(0, 0, 0, 1, openingRim).Children(func() {
		ui.Icon(c, svg).FontSize(18).TextColor(openingInk).Shrink(0)
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			ui.Text(c, name).FontSize(13).LineHeight(lead13).FontWeight(500).TextColor(openingInk).SingleLine()
			ui.Text(c, sub).FontSize(12).LineHeight(lead12).TextColor(openingInk2).SingleLine()
		})
		ui.Row(c).Gap(4).Shrink(0).Children(func() {
			for s := range 2 {
				ui.Box(c.Key(s)).Size(openingSlot, openingSlot).Radius(8).Background(openingSlotBG).InsetShadow(0, 0, 0, 1, openingRim)
			}
		})
	})
}

func systemName() string {
	switch runtime.GOOS {
	case "darwin":
		return "macOS"
	case "windows":
		return "Windows"
	}
	return "Linux"
}

// drawOpeningWires draws each wire from a port on its tile to a port on its card: ahead
// of the tile going out, taken up behind it coming home.
func drawOpeningWires(p *ui.Painter, r room, t float32) {
	for i := range openingAgents {
		out, home := wave(i)
		w := r.wire(i)
		back := seg(t, 4040+home, 4640+home, easeInOut)
		toFoot := w.line.total - (w.foot - (r.card(openingAgents[i].host).cy()))
		to := min(toFoot*seg(t, 2640+out, 3360+out, easeInOut), w.line.total*(1-back))
		if to > .25 {
			p.StrokePolyline(w.line.upTo(to), 1.5, openingLine)
		}
		a0 := clamp(seg(t, 2520+out, 2720+out, easeBack), 0, 1) * (1 - seg(t, 4600, 4760, easeInOut))
		port(p, w.x0, w.y0, a0, .6+.4*a0, 3.5, 1.25, openingGround, openingPort, openingDot)
		a1 := clamp(seg(t, 3240+out, 3440+out, easeBack), 0, 1) * (1 - seg(t, 3980+home, 4120+home, easeInOut))
		port(p, w.x1, w.foot, a1, .6+.4*a1, 3.5, 1.25, openingGround, openingPort, openingDot)
	}
}

// port is the small ring on what a wire joins.
func port(p *ui.Painter, x, y, a, scale, ring, dot float32, ground, line, centre ui.Color) {
	if a <= .004 {
		return
	}
	// A ring's line lies on its radius, half of it outside; Stroke draws within the box.
	o := ring*scale + .75
	box := ui.Rect{X: x - o, Y: y - o, W: 2 * o, H: 2 * o}
	p.Fill(box, fadeColor(ground, a), o)
	p.Stroke(box, fadeColor(line, a), o, 1.5)
	d := dot * scale
	p.Fill(ui.Rect{X: x - d, Y: y - d, W: 2 * d, H: 2 * d}, fadeColor(centre, a), d)
}

func drawOpeningTiles(p *ui.Painter, r room, t float32) {
	for i, ag := range openingAgents {
		delay := ag.delay
		grow := seg(t, 1120+delay, 1720+delay, easeInOut)
		spread := easeBack(clamp((t-1060-delay)/760, 0, 1))
		x, y := r.cx+ag.off*r.spread*spread, r.cy
		tw, th := lerp(14, 44, grow), lerp(28, 44, grow)
		if i == 2 { // the cursor gathers a little before it divides
			g := max(0, 1-float32(math.Abs(float64(t-1000)))/180)
			tw, th = tw+g*6, th-g*3
		}
		out, home := wave(i)
		ride := seg(t, 2780+out, 3560+out, easeInOut) * (1 - seg(t, 4040+home, 4640+home, easeInOut))
		if ride > 0 {
			line := r.wire(i).line
			x, y = line.at(line.total * ride)
			y += (1 - ride) * 22 * (1 - seg(ride, 0, .25, easeInOut))
			tw, th = lerp(44, 26, ride), lerp(44, 26, ride)
		}
		late := float32(60)
		if i == 2 {
			late = 0
		}
		if conv := seg(t, 4620+late, 5060+late, easeInOut); conv > 0 {
			x, y = lerp(r.cx+ag.off*r.spread, r.cx, conv), lerp(r.cy, r.cy-80, conv)
			tw, th = lerp(44, 20, conv), lerp(44, 20, conv)
		}
		a := seg(t, 1060, 1240, easeInOut)
		if i == 2 {
			a = 1
			if t >= 400 && t <= 620 {
				a = 0 // the cursor's one blink
			}
		}
		a *= 1 - seg(t, 4900, 5120, easeIn)
		if a <= .004 {
			continue
		}
		box := ui.Rect{X: x - tw/2, Y: y - th/2, W: tw, H: th}
		radius := lerp(1, 10, grow)
		p.Fill(box, fadeColor(mixColor(openingInk, openingTile, grow), a), radius)
		if grow > .5 {
			p.Stroke(box, fadeColor(openingLine, a), radius, 1)
		}
		if ia := seg(t, 1460+delay, 1820+delay, easeOut) * a; ia > .004 {
			p.Clip(box, radius, func() {
				p.Icon(icon.Agent(ag.id), ui.Rect{X: x - 11, Y: y - 11, W: 22, H: 22}, fadeColor(openingInk, ia))
			})
		}
	}
}

// drawLogo assembles the logo from sixteen bands, coming in from either side as the
// agents reach it.
func drawLogo(p *ui.Painter, r room, t float32) {
	if t < 4760 {
		return
	}
	const size, bands = 96, 16
	leave := seg(t, 6950, 7400, easeIn)
	fade := 1 - leave
	if fade <= .004 {
		return
	}
	scale := 1 - .08*leave
	s := size * scale
	x0, y0 := r.cx-s/2, r.cy-80-s/2
	for i := range bands {
		off := float32(math.Abs(float64(i) - 7.5))
		e := seg(t, 4760+off*38, 5460+off*38, easeOutQuint)
		if e <= 0 {
			continue
		}
		dir := float32(1)
		if i%2 == 0 {
			dir = -1
		}
		k := s / 128
		dx, dy := (1-e)*dir*7*k, (1-e)*(float32(i)-7.5)*.9*k
		band := ui.Rect{X: x0 - 16 + dx, Y: y0 + float32(i)*s/bands + dy, W: s + 32, H: s/bands + .5}
		p.Clip(band, 0, func() {
			p.Icon(icon.Logo, ui.Rect{X: x0 + dx, Y: y0 + dy, W: s, H: s}, fadeColor(openingInk, e*fade))
		})
	}
}

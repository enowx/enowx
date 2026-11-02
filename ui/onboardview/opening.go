package onboardview

import (
	"math"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
)

func sqrt(x float64) float64 { return math.Sqrt(x) }

// What the opening shows: one cursor dividing into agents, each going out to a host
// and coming home to make the logo. All colors here are the opening's own: it's the
// same dark room in every style and palette.
var (
	openingGround = ui.Hex("#0B0C10")
	openingTile   = ui.Hex("#17181F")
	openingLine   = ui.Hex("#2A2B34")
	openingWire   = ui.Hex("#0A6FE0")
	openingInk    = ui.Hex("#FFFFFF")
	openingSoft   = ui.Hex("#9A9DAB")
	openingCursor = ui.Hex("#ECECF0")
)

var openingAgents = []struct {
	id, name string
	off      float32 // from the middle
	to       int     // the host it goes to
}{
	{"claude", "Claude Code", -152, 0}, {"codex", "Codex", -76, 1}, {"opencode", "OpenCode", 0, 1}, {"gemini", "Gemini CLI", 76, 2}, {"kiro", "Kiro CLI", 152, 2},
}

const (
	openingCardW = 248
	openingCardH = 64
)

// opening draws the stage: the opening, then its giving way.
func (v *View) opening(c *ui.Context, o Options, w, h, t float32) {
	if t >= tOpeningEnd+450 || v.flow != FlowNew {
		return
	}
	fade := 1 - seg(t, tOpeningEnd, tOpeningEnd+450, easeIn)
	cx, cy := w/2, h/2
	hosts := []string{o.Device + "\nthis device", "A server\nLinux, over SSH", "Another computer\nover SSH"}
	still := o.Reduce
	ui.Box(c.Key("stage")).Absolute().Left(0).Top(0).Right(0).Bottom(0).Opacity(fade).Background(openingGround).Draw(func(p *ui.Painter, _ ui.Rect) {
		if !still {
			drawOpeningWires(p, cx, cy, t)
			drawOpeningTiles(p, cx, cy, t)
		}
		drawLogo(p, cx, cy, t, still)
	}).Children(func() {
		if !still {
			for i, ag := range openingAgents {
				a := seg(t, 1820+float32(i)*60, 2180+float32(i)*60, easeOut) * (1 - seg(t, 2640, 2820, easeIn))
				if a > .01 {
					ui.Text(c.Key(ag.id), ag.name).FontSize(11).TextColor(openingSoft).SingleLine().Absolute().
						Left(cx + ag.off - 36).Top(cy + 30).Width(72).Opacity(a)
				}
			}
			for i, name := range hosts {
				a := seg(t, 2340+float32(i)*90, 2800+float32(i)*90, easeOut) * (1 - seg(t, 4560+float32(i)*40, 4900+float32(i)*40, easeIn))
				if a > .01 {
					openingCard(c.Key(i), name, cardPlace(cx, cy, i), a)
				}
			}
		}
		name := seg(t, 5300, 5900, easeOut)
		if still {
			name = 1
		}
		if name > .01 {
			ui.Column(c.Key("name")).Absolute().Left(0).Right(0).Top(cy+52).Center().Gap(theme.S2).Opacity(name).Translate(0, (1-name)*8).Children(func() {
				ui.Text(c, "enowX").FontSize(28).FontWeight(600).TextColor(openingInk).SingleLine()
				ui.Text(c, "One place for agents on every host.").FontSize(14).TextColor(openingSoft).SingleLine()
			})
		}
	})
}

// cardPlace is where host i's card lies.
func cardPlace(cx, cy float32, i int) rect {
	return rect{cx + float32(i-1)*(openingCardW+24) - openingCardW/2, cy - 240, openingCardW, openingCardH}
}

func openingCard(c *ui.Context, name string, r rect, a float32) {
	first, second := name, ""
	for i := range name {
		if name[i] == '\n' {
			first, second = name[:i], name[i+1:]
			break
		}
	}
	ui.Row(c).Absolute().Left(r.x).Top(r.y).Size(r.w, r.h).Opacity(a).Translate(0, (1-a)*10).Padding(0, theme.S3).Gap(theme.S3).AlignItems(ui.Center).
		Radius(10).Background(openingTile).Border(1, openingLine).Children(func() {
		ui.Icon(c, icon.Server).FontSize(16).TextColor(openingSoft).Shrink(0)
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			ui.Text(c, first).FontSize(13).FontWeight(500).TextColor(openingInk).SingleLine()
			ui.Text(c, second).FontSize(11).TextColor(openingSoft).SingleLine()
		})
	})
}

// bezier is a point of the curve from a tile up to its host's card.
func bezier(x0, y0, x1, y1, u float32) (float32, float32) {
	mid := (y0 + y1) / 2
	s := 1 - u
	return s*s*s*x0 + 3*s*s*u*x0 + 3*s*u*u*x1 + u*u*u*x1, s*s*s*y0 + 3*s*s*u*mid + 3*s*u*u*mid + u*u*u*y1
}

// openingRoute is where agent i goes: from its tile, up to its card's foot.
func openingRoute(cx, cy float32, i int) (x0, y0, x1, y1 float32) {
	ag := openingAgents[i]
	card := cardPlace(cx, cy, ag.to)
	slot := float32(i-ag.to*2+1) * 30
	return cx + ag.off, cy - 22, card.cx() + slot, card.y + card.h
}

func drawOpeningWires(p *ui.Painter, cx, cy, t float32) {
	for i := range openingAgents {
		out := float32(openingAgents[i].to) * 130
		home := float32(2-openingAgents[i].to) * 70
		drawn := seg(t, 2640+out, 3360+out, easeInOut)
		taken := seg(t, 4040+home, 4640+home, easeInOut)
		if drawn <= 0 || taken >= 1 {
			continue
		}
		x0, y0, x1, y1 := openingRoute(cx, cy, i)
		mid := (y0 + y1) / 2
		path := (&ui.Path{}).MoveTo(x0, y0).CubeTo(x0, mid, x1, mid, x1, y1)
		path.Trim(0, min(drawn, 1-taken))
		p.StrokePath(path, 1.5, openingWire)
		a := clamp(seg(t, 2520+out, 2720+out, easeBack), 0, 1) * (1 - seg(t, 4600, 4760, easeInOut))
		port(p, x0, y0, a, openingWire, openingGround)
		b := clamp(seg(t, 3240+out, 3440+out, easeBack), 0, 1) * (1 - seg(t, 3980+home, 4120+home, easeInOut))
		port(p, x1, y1, b, openingWire, openingGround)
	}
}

// port is the small ring on what a wire joins.
func port(p *ui.Painter, x, y, a float32, ring, ground ui.Color) {
	if a <= .01 {
		return
	}
	r := 4 * (.6 + .4*a)
	box := ui.Rect{X: x - r, Y: y - r, W: 2 * r, H: 2 * r}
	p.Fill(box, fadeColor(ground, a), r)
	p.Stroke(box, fadeColor(ring, a), r, 1.5)
}

func drawOpeningTiles(p *ui.Painter, cx, cy, t float32) {
	for i, ag := range openingAgents {
		delay := float32(math.Abs(float64(i-2))) * 40
		spread := easeBack(clamp((t-1060-delay)/760, 0, 1))
		grow := seg(t, 1120+delay, 1720+delay, easeInOut)
		x, y := cx+ag.off*spread, cy
		w, h := lerp(14, 44, grow), lerp(28, 44, grow)
		if i == 2 { // the cursor gathers a little before it divides
			g := max(0, 1-float32(math.Abs(float64(t-1000)))/180)
			w, h = w+g*6, h-g*3
		}
		out := float32(ag.to) * 130
		home := float32(2-ag.to) * 70
		ride := seg(t, 2780+out, 3560+out, easeInOut) * (1 - seg(t, 4040+home, 4640+home, easeInOut))
		if ride > 0 {
			x0, y0, x1, y1 := openingRoute(cx, cy, i)
			x, y = bezier(x0, y0, x1, y1, ride)
			w, h = lerp(44, 26, ride), lerp(44, 26, ride)
		}
		conv := seg(t, 4620, 5060, easeInOut)
		if conv > 0 {
			x, y = lerp(cx+ag.off, cx, conv), lerp(cy, cy-24, conv)
			w, h = lerp(44, 20, conv), lerp(44, 20, conv)
		}
		a := seg(t, 1060, 1240, easeInOut)
		if i == 2 {
			a = 1
			if t < 1000 && t >= 400 && t <= 620 {
				a = 0 // the cursor's one blink
			}
		}
		a *= 1 - seg(t, 4900, 5120, easeIn)
		if a <= .01 {
			continue
		}
		box := ui.Rect{X: x - w/2, Y: y - h/2, W: w, H: h}
		radius := lerp(1, 10, grow)
		p.Fill(box, fadeColor(mixColor(openingCursor, openingTile, grow), a), radius)
		if grow > .5 {
			p.Stroke(box, fadeColor(openingLine, a), radius, 1)
		}
		if ia := seg(t, 1460+delay, 1820+delay, easeOut) * a; ia > .01 {
			size := min(w, h) * .55
			p.Icon(icon.Agent(ag.id), ui.Rect{X: x - size/2, Y: y - size/2, W: size, H: size}, fadeColor(openingInk, ia))
		}
	}
}

// drawLogo assembles the logo from bands that slide in from either side.
func drawLogo(p *ui.Painter, cx, cy, t float32, still bool) {
	const size, bands = 64, 16
	x0, y0 := cx-size/2, cy-size/2-24
	for i := range bands {
		delay := float32(math.Abs(float64(i)-7.5)) * 38
		e := seg(t, 4760+delay, 5460+delay, easeOutQuint)
		if still {
			e = 1
		}
		if e <= 0 {
			continue
		}
		dir := float32(1 - 2*(i%2))
		band := ui.Rect{X: x0 - 16, Y: y0 + float32(i)*size/bands, W: size + 32, H: size/bands + .5}
		p.Clip(band, 0, func() {
			p.Icon(icon.Logo, ui.Rect{X: x0 + (1-e)*dir*7, Y: y0, W: size, H: size}, fadeColor(openingInk, e))
		})
	}
}

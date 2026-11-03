package onboardview

import (
	"fmt"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/icon"
)

// The panel holding the side panel's place: a head counting the steps, and one view at a
// time, each coming in on the clock, what's inside it one piece after another.

// viewRange is when view id shows on the clock, nil when it doesn't in this flow.
func (v *View) viewRange(id string, s scene) (from, to float32, ok bool) {
	if v.detour != nil {
		return 0, tEnd, id == v.detour.id
	}
	if s.returning {
		switch id {
		case "back":
			return 8200, 45000, true
		case "community":
			return 44700, 45800, true
		}
		return 0, 0, false
	}
	if v.flow == FlowDevice && id != "agents" && id != "host" {
		return 0, 0, false
	}
	switch id {
	case "agents":
		return 8700, 13300, true
	case "project":
		return 13200, 15000, true
	case "host":
		return 14900, 19500, true
	case "style":
		return 19300, 36950, true
	case "goa":
		return 36900, 40350, true
	case "task":
		return 40250, 44800, true
	case "community":
		return 44700, 45800, true
	}
	return 0, 0, false
}

// viewShown says how far a view is: its alpha, its rise, and whether it's taking clicks.
func viewShown(t, from, to float32) (alpha, rise, in float32) {
	in = 1
	if from > 0 {
		in = seg(t, from, from+320, easeOut)
	}
	out := seg(t, to-200, to, easeIn)
	return min(in, 1-out), (1-in)*10 - out*6, in
}

func (v *View) panelRegion(c *ui.Context, o Options, s scene) {
	if s.fold >= 1 {
		return
	}
	p := s.panel
	// It folds under the main region as that takes its room.
	cut := max(0, s.doc.r.right()+p.v.g-p.r.x)
	regionBox(c, "ob-panel", p, p.v.panel, func() {
		vp := p.v
		ui.Box(c.Key("fold")).Absolute().Left(cut).Top(0).Right(0).Bottom(0).Clip().Children(func() {
			ui.Box(c.Key("all")).Absolute().Left(-cut).Top(0).Width(panelWidth).Bottom(0).Children(func() {
				v.panelHead(c, s)
				for _, id := range []string{"agents", "project", "host", "style", "goa", "task", "community", "back"} {
					from, to, ok := v.viewRange(id, s)
					if !ok {
						continue
					}
					a, rise, in := viewShown(s.t, from, to)
					if a <= .004 {
						continue
					}
					ui.Column(c.Key(id)).Absolute().Left(0).Right(0).Top(40).Bottom(0).Opacity(a).Translate(0, rise).Children(func() {
						pane := pane{c: c, v: vp, t: s.t, from: from, stagger: in < 1, live: a >= .6}
						v.stepView(pane, id, o, s)
					})
				}
			})
		})
	})
}

func (v *View) panelHead(c *ui.Context, s scene) {
	t := s.t
	vp := s.panel.v
	a := seg(t, 8500, 8900, easeOut) * (1 - seg(t, 45500, 45700, easeInOut))
	if a <= .004 {
		return
	}
	cur := v.current()
	word := ""
	switch {
	case s.returning && v.detour != nil:
		word = v.detour.name
	case s.returning && t < 44800:
		word = "Welcome back"
	case v.flow == FlowDevice:
		word = fmt.Sprintf("%d of 2", max(cur.step, 1))
	case cur.step > 0:
		word = fmt.Sprintf("%d of 7", cur.step)
	}
	done := 7 * float32(1)
	total := 7
	switch {
	case s.returning:
	case v.flow == FlowDevice:
		total, done = 2, 1+seg(t, 14900, 15300, easeInOut)
	default:
		done = 1 + seg(t, 8700, 9100, easeInOut) + seg(t, 13200, 13600, easeInOut) + seg(t, 19300, 19700, easeInOut) +
			seg(t, 36950, 37350, easeInOut) + seg(t, 40300, 40700, easeInOut) + seg(t, 44700, 45100, easeInOut)
	}
	ui.Row(c.Key("head")).Absolute().Left(0).Right(0).Top(0).Height(40).Gap(8).Padding(0, 12, 2, 12).AlignItems(ui.Center).InsetShadow(0, -1, 0, 0, vp.rim2).Opacity(a).Children(func() {
		line(c, "Set up enowX", 13, vp.ink).FontWeight(600).Shrink(0)
		line(c, word, 12, vp.ink2).Shrink(0)
		ui.Spacer(c)
		ui.Row(c.Key("pips")).Gap(4).Shrink(0).Children(func() {
			for i := range total {
				part := clamp(done-float32(i), 0, 1)
				ui.Box(c.Key(i)).Size(14, 3).Radius(4).Background(vp.fill).Clip().Children(func() {
					if part > 0 {
						ui.Box(c).Size(14*part, 3).Background(vp.accent)
					}
				})
			}
		})
	})
}

// pane is what a step's view draws with: the panel's look, and the clock for what comes
// in one after another.
type pane struct {
	c       *ui.Context
	v       look
	t, from float32
	stagger bool
	live    bool // it's far enough in to take clicks
	kid     int
}

// next is the next piece's way in.
func (p *pane) next(e ui.Element) ui.Element {
	i := min(p.kid, 5)
	p.kid++
	if !p.stagger {
		return e
	}
	at := p.from + float32(i)*40
	q := seg(p.t, at, at+280, easeOut)
	return e.Opacity(q).Translate(0, (1-q)*6)
}

// body is a step's words and controls, its buttons staying at the foot.
func (p *pane) body(fn func()) {
	ui.Column(p.c.Key("body")).Grow(1).FillWidth().MinHeight(0).Padding(12).Gap(8).Clip().Children(fn)
}

func (p *pane) title(text string) {
	p.next(ui.Row(p.c.Key(text)).FillWidth().Height(18).AlignItems(ui.Center).Shrink(0)).Children(func() {
		line(p.c, text, 13, p.v.ink).FontWeight(600)
	})
}

func (p *pane) about(text string) {
	p.next(ui.Text(p.c.Key(text), text).FontSize(12).LineHeight(lead12).TextColor(p.v.ink2).FillWidth().Shrink(0))
}

func (p *pane) label(text, right string) {
	p.next(ui.Row(p.c.Key("lab"+text)).FillWidth().Margin(4, 0, 0, 0).AlignItems(ui.Center).Shrink(0)).Children(func() {
		line(p.c, text, 12, p.v.ink2).FontWeight(500)
		ui.Spacer(p.c)
		if right != "" {
			line(p.c, right, 12, p.v.ink2)
		}
	})
}

// foot is a step's buttons, with Back at its left when there's somewhere to go back to.
func (p *pane) foot(back func(), fn func()) {
	c, v := p.c, p.v
	ui.Row(c.Key("foot")).FillWidth().Height(44).Gap(8).Padding(0, 8, 0, 4).AlignItems(ui.Center).Shrink(0).InsetShadow(0, 1, 0, 0, v.rim2).Children(func() {
		if back != nil && p.bare("Back") {
			back()
		}
		ui.Spacer(c)
		fn()
	})
}

// button is the design's, and says it was clicked.
func (p *pane) button(label string, bg, fg ui.Color, weight int, after func()) bool {
	c, v := p.c, p.v
	b := ui.ButtonBase(c.Key(label)).Height(26).PaddingX(8).Gap(4).Radius(v.rc).Background(bg).Label(label).Center().Cursor(ui.CursorPointer).Shrink(0)
	b.Disabled(!p.live)
	b.Children(func() {
		line(c, label, 12, fg).FontWeight(weight)
		if after != nil {
			after()
		}
	})
	return p.live && b.Clicked()
}

func (p *pane) plain(label string) bool {
	return p.button(label, p.v.fill, p.v.ink, 400, nil)
}

func (p *pane) bare(label string) bool {
	return p.button(label, rgba(0, 0, 0, 0), p.v.ink2, 400, nil)
}

// primary is the step's main button, its Enter key shown.
func (p *pane) primary(label string, enabled bool) bool {
	c, v := p.c, p.v
	if !enabled {
		b := ui.Row(c.Key(label)).Height(26).PaddingX(8).Radius(v.rc).Background(v.well).AlignItems(ui.Center).Shrink(0)
		b.Children(func() { line(c, label, 12, v.ink2).FontWeight(500) })
		return false
	}
	clicked := p.button(label, v.accent, v.ona, 500, func() {
		ui.Icon(c, icon.EnterKey).FontSize(14).TextColor(v.ona).Opacity(.75)
	})
	return clicked || p.live && c.Shortcut(0, ui.KeyEnter)
}

// card is a choice with a tile: something to click, in the design's card.
func (p *pane) card(key string, svg *ui.SVG, name, sub string, height float32) bool {
	c, v := p.c, p.v
	b := ui.ButtonBase(c.Key(key)).FillWidth().MinHeight(height).Gap(8).PaddingX(8).Radius(v.rr).Background(v.fill0).InsetShadow(0, 0, 0, 1, v.rim2).Label(name).Cursor(ui.CursorPointer).AlignItems(ui.Center).Shrink(0)
	if b.Hovered() {
		b.Background(v.fill)
	}
	p.next(b).Children(func() {
		if svg != nil {
			tile(c, svg, 14, v.tile, v.ink, 1)
		}
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			ui.Row(c.Key("name")).FillWidth().MinWidth(0).Clip().Children(func() { line(c, name, v.fs, v.ink) })
			if sub != "" {
				ui.Row(c.Key("sub")).FillWidth().MinWidth(0).Clip().Children(func() { line(c, sub, 12, v.ink2) })
			}
		})
		if svg != nil {
			ui.Icon(c.Key("go"), icon.ChevronRight).FontSize(14).TextColor(v.ink2).Shrink(0)
		} else {
			ui.Icon(c.Key("go"), icon.ArrowUp).FontSize(12).TextColor(v.ink2).Shrink(0)
		}
	})
	return p.live && b.Clicked()
}

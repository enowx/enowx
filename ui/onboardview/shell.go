package onboardview

import (
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// The window as it assembles itself: every region a hairline frame first, drawing
// itself out from one point, then given its material, then filled.

// region is one of the window's parts on the clock.
type region struct {
	r      rect
	radius float32
	from   float32 // when its frame starts drawing
	mat    [2]float32
	fill   func(tokens) ui.Color
	delay  float32 // of the wave of a change of style
}

// regionsAt are the regions, each with its material so far.
func (v *View) regionsAt(g geometry) []region {
	short := v.flow != FlowNew && v.flow != FlowDevice // a returning user's come in at once
	m := func(a, b float32) [2]float32 {
		if short {
			return [2]float32{tPanelFrom + (a-tMaterial)/2, tPanelTo + (a-tMaterial)/2}
		}
		return [2]float32{a, b}
	}
	main := func(t tokens) ui.Color { return t.main }
	side := func(t tokens) ui.Color { return t.panel }
	return []region{
		{g.doc, g.radius, 7050, m(17450, 18250), main, delayDoc},
		{g.panel, g.radius, 7200, [2]float32{tPanelFrom, tPanelTo}, side, delayPanel},
		{g.side, g.radius, 7260, m(17300, 18100), side, delaySide},
		{g.ctx, g.barRadius, 7320, m(17500, 18200), side, delayCtx},
		{g.pg, g.barRadius, 7390, m(17580, 18280), side, delayPg},
		{g.fd, g.barRadius, 7460, m(17660, 18360), side, delayFd},
		{g.act, g.barRadius, 7530, m(17740, 18440), side, delayAct},
		{g.rail, g.railRadius, 7520, m(17650, 18300), side, delayRail},
	}
}

// ground is what lies under the regions: bare at first, the style's own by the end.
func (v *View) ground(c *ui.Context, o Options, t float32) {
	ui.Box(c.Key("ob-bare")).Absolute().Left(0).Top(0).Right(0).Bottom(0).Background(openingGround)
	wall := seg(t, tMaterial+300, tMaterialTo, easeInOut)
	if v.flow == FlowReturning {
		wall = seg(t, tPanelFrom, tPanelTo, easeInOut)
	}
	if wall > .004 {
		ground := v.morph.color(func(t tokens) ui.Color { return t.ground }, 0)
		ui.Box(c.Key("ob-wall")).Absolute().Left(0).Top(0).Right(0).Bottom(0).Opacity(wall).Background(ground).Children(func() {
			if o.Wallpaper != nil {
				ui.Box(c.Key("paper")).Absolute().Left(0).Top(0).Right(0).Bottom(0).Opacity(v.morph.wall(1)).Children(func() { o.Wallpaper(c) })
			}
		})
	}
}

// shell draws the regions and what's in them.
func (v *View) shell(c *ui.Context, o Options, g geometry, t float32) {
	if t < tOpeningEnd {
		return
	}
	g = v.morph.geometry(g)
	regions := v.regionsAt(g)
	ink := openingLine
	ui.Box(c.Key("ob-frames")).Absolute().Left(0).Top(0).Right(0).Bottom(0).PassThrough().Draw(func(p *ui.Painter, _ ui.Rect) {
		for _, r := range regions {
			mat := seg(t, r.mat[0], r.mat[1], easeInOut)
			if mat > .004 {
				p.Fill(r.r.ui(), fadeColor(v.morph.color(r.fill, r.delay), mat), r.radius)
			}
			if g.rim > 0 && mat > .004 {
				edge := v.morph.color(func(t tokens) ui.Color { return t.border }, r.delay)
				p.Stroke(r.r.ui(), fadeColor(edge, mat), r.radius, g.rim)
			}
			drawn := seg(t, r.from, r.from+860, easeInOut)
			if frame := 1 - mat; drawn > 0 && frame > .004 {
				// Both ways round from one point, closing on the far side.
				half := drawn / 2
				a := roundedPath(r.r, r.radius)
				a.Trim(0, half)
				b := roundedPath(r.r, r.radius)
				b.Trim(1-half, 1)
				p.StrokePath(a, 1, fadeColor(ink, frame))
				p.StrokePath(b, 1, fadeColor(ink, frame))
			}
		}
		v.hostWire(p, g, t)
	})
	ui.Box(c.Key("ob-content")).Absolute().Left(0).Top(0).Right(0).Bottom(0).PassThrough().Opacity(v.morph.content(time.Now())).Children(func() {
		if over := seg(t, tFold, tEnd, easeInOut); over < 1 {
			v.panelContent(c, o, g, t, over)
		}
		v.barContent(c, o, g, t)
		v.sideContent(c, o, g, t)
		v.docContent(c, o, g, t)
		v.railContent(c, g, t)
		v.serverCard(c, o, g, t)
	})
}

// barContent is the title bar's cells, once they have their material.
func (v *View) barContent(c *ui.Context, o Options, g geometry, t float32) {
	a := seg(t, 18200, 18500, easeOut)
	if v.flow == FlowReturning {
		a = seg(t, tPanelTo, tPanelTo+300, easeOut)
	}
	if a < .01 {
		return
	}
	cell := func(key string, r rect, fn func()) {
		ui.Row(c.Key(key)).Absolute().Left(r.x).Top(r.y).Size(r.w, r.h).Opacity(a).Padding(0, theme.S3).Gap(theme.S2).AlignItems(ui.Center).Clip().Children(fn)
	}
	label := func(text string, col ui.Color) {
		ui.Text(c, text).FontSize(theme.TextUI).TextColor(col).SingleLine()
	}
	cell("ob-ctx", g.ctx, func() { label(o.HostName, theme.Text) })
	cell("ob-pg", g.pg, func() {
		for _, name := range []string{"Sessions", "Notes", "Canvas", "GOD of Agents"} {
			label(name, theme.TextSoft)
		}
	})
	cell("ob-fd", g.fd, func() { label("Search sessions, hosts and actions", theme.Muted) })
	cell("ob-act", g.act, func() { label("New session", theme.Text) })
}

// sideContent is the session list: slots waiting, filling as agents are found, then
// the user's project.
func (v *View) sideContent(c *ui.Context, o Options, g geometry, t float32) {
	if t < 8600 {
		return
	}
	r := g.side
	ui.Column(c.Key("ob-side")).Absolute().Left(r.x).Top(r.y).Size(r.w, r.h).Padding(theme.S3).Gap(theme.S1).Clip().Children(func() {
		ui.Text(c, "Sessions").FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine().Opacity(seg(t, 8600, 8900, easeOut))
		found := installed(o)
		for i := range 6 {
			from := 9000 + float32(i)*220
			slot := ui.Row(c.Key(i)).FillWidth().Height(theme.RowHeight).Gap(theme.S2).AlignItems(ui.Center).PaddingX(theme.S2).Radius(theme.RadiusControl)
			if i >= len(found) {
				slot.Border(1, fadeColor(theme.Border, seg(t, 8800, 9200, easeOut)*.6))
				continue
			}
			a := seg(t, from, from+360, easeOut)
			slot.Opacity(max(a, .15)).Translate(0, (1-a)*6).Children(func() {
				ui.Icon(c, icon.Agent(agent.Base(found[i].ID))).FontSize(14).TextColor(theme.TextSoft).Shrink(0)
				ui.Row(c).Grow(1).MinWidth(0).Children(func() { widget.Running(c, found[i].Name, widget.RunStyle{Size: theme.TextUI}) })
				ui.Text(c, "Ready").FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine().Shrink(0)
			})
		}
		if a := seg(t, tProject, tProject+500, easeOut); a > .01 && len(o.Projects) > 0 {
			ui.Text(c, "Projects").FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine().Opacity(a).Padding(theme.S2, 0, 0, 0)
			for i, p := range o.Projects[:min(len(o.Projects), 4)] {
				ui.Row(c.Key(p.Path)).FillWidth().Height(theme.RowHeight).Gap(theme.S2).AlignItems(ui.Center).PaddingX(theme.S2).Opacity(a).Translate((1-a)*-12, 0).Children(func() {
					ui.Icon(c, icon.Folder).FontSize(14).TextColor(theme.TextSoft).Shrink(0)
					ui.Row(c).Grow(1).MinWidth(0).Children(func() { widget.Running(c, nonEmpty(p.Name, p.Path), widget.RunStyle{Size: theme.TextUI}) })
				})
				_ = i
			}
		}
	})
}

// railContent is the tools dock's icons, each a beat after the last.
func (v *View) railContent(c *ui.Context, g geometry, t float32) {
	if t < 18300 {
		return
	}
	r := g.rail
	ui.Column(c.Key("ob-rail")).Absolute().Left(r.x).Top(r.y).Size(r.w, r.h).Center().Gap(theme.S2).Clip().Children(func() {
		for i, svg := range []*ui.SVG{icon.Terminal, icon.Folder, icon.Globe, icon.Server, icon.Gauge} {
			a := seg(t, 18300+float32(i)*60, 18620+float32(i)*60, easeOut)
			ui.Icon(c.Key(i), svg).FontSize(15).TextColor(theme.TextSoft).Opacity(a).Translate(0, (1-a)*6)
		}
	})
}

// docContent is the main region: a setup log, a sample of how chats read, the first task.
func (v *View) docContent(c *ui.Context, o Options, g geometry, t float32) {
	if t < 8600 {
		return
	}
	r := g.doc
	id := v.current().id
	ui.Column(c.Key("ob-doc")).Absolute().Left(r.x).Top(r.y).Size(r.w, r.h).Padding(theme.S4).Gap(theme.S2).Clip().Children(func() {
		switch id {
		case "style":
			sampleChat(c)
		case "goa":
			sampleGOA(c)
		case "task":
			v.taskComposer(c.Key("task"), o)
		case "community", "back", "end":
		default:
			v.setupLog(c, o, t)
		}
	})
}

// setupLog is what setup has found so far, as it comes.
func (v *View) setupLog(c *ui.Context, o Options, t float32) {
	line := func(key string, from float32, text string, col ui.Color) {
		a := seg(t, from, from+260, easeOut)
		if a < .01 {
			return
		}
		ui.Text(c.Key(key), text).FontSize(theme.TextUI).Font(theme.MonoFont).TextColor(col).SingleLine().Opacity(a)
	}
	line("head", 8700, "Looking for agents on "+o.Device, theme.TextSoft)
	if !o.AgentsLoaded {
		return
	}
	for i, ag := range installed(o) {
		line(ag.ID, 9000+float32(i)*220, ag.Command, theme.Text)
	}
	if len(installed(o)) == 0 {
		line("none", 9000, "none yet", theme.TextSoft)
	}
	if t >= tProject && len(o.Projects) > 0 {
		line("project", tProject+200, "project  "+o.Projects[0].Path, theme.Text)
	}
}

// sampleChat shows the style on a conversation, marked a sample.
func sampleChat(c *ui.Context) {
	ui.Text(c, "Sample conversation, to show the style").FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine()
	ui.Row(c.Key("q")).FillWidth().Justify(ui.End).Children(func() {
		ui.Text(c, "Does it hold for a small screen?").FontSize(theme.TextUI).TextColor(theme.OnAccent).Padding(theme.S2, theme.S3).Radius(theme.RadiusCard).Background(theme.Accent)
	})
	ui.Text(c.Key("a"), "It does: the regions stay in their places, and only the conversation gets narrower.").FontSize(theme.TextUI).TextColor(theme.Text).MaxWidth(520)
}

// sampleGOA shows how GOD of Agents reads, marked a sample.
func sampleGOA(c *ui.Context) {
	ui.Text(c, "Sample").FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine()
	ui.Row(c.Key("q")).FillWidth().Justify(ui.End).Children(func() {
		ui.Text(c, "Fix the failing check, then update the README.").FontSize(theme.TextUI).TextColor(theme.OnAccent).Padding(theme.S2, theme.S3).Radius(theme.RadiusCard).Background(theme.Accent)
	})
	ui.Text(c.Key("a"), "I gave the check to Codex and the README to Claude Code. I'll tell you when they're done.").FontSize(theme.TextUI).TextColor(theme.Text).MaxWidth(520)
	for _, row := range [][2]string{{"Delegated to Codex", "fix the failing check"}, {"Delegated to Claude Code", "update the README"}, {"Codex asks to run git push", "yours to answer"}} {
		ui.Row(c.Key(row[0])).FillWidth().Gap(theme.S2).Children(func() {
			ui.Text(c, row[0]).FontSize(theme.TextUI).TextColor(theme.Text).SingleLine().Shrink(0)
			ui.Text(c, row[1]).FontSize(theme.TextUI).TextColor(theme.TextSoft).SingleLine()
		})
	}
}

// hostWire is a wire from the project's row to the server's card, as it connects. It
// is measured from what it joins each frame, so it never floats free of them.
func (v *View) hostWire(p *ui.Painter, g geometry, t float32) {
	if !v.wireOn || t < tHost || t > 18300 {
		return
	}
	card := serverRect(g)
	ax, ay := g.side.right(), g.side.y+theme.RowHeight*2+theme.S4
	bx, by := card.x, card.cy()
	mx := g.doc.x + 52
	path := wirePath([][2]float32{{ax, ay}, {mx, ay}, {mx, by}, {bx, by}}, 10)
	drawn := seg(t, 15850, 16600, easeInOut)
	gone := seg(t, 17420, 18150, easeInOut)
	if drawn <= 0 || gone >= 1 {
		return
	}
	path.Trim(gone, drawn)
	wire := fadeColor(theme.Accent, 1-gone)
	p.StrokePath(path, 1.5, wire)
	port(p, ax, ay, seg(t, 15700, 15900, easeBack)*(1-gone), theme.Accent, theme.Background)
	port(p, bx, by, seg(t, 16500, 16700, easeBack)*(1-gone), theme.Accent, theme.Background)
}

func serverRect(g geometry) rect {
	return rect{g.doc.x + 128, g.side.y + theme.RowHeight*2 + 72, 220, 56}
}

// serverCard is the host the user just added, connecting, then gone up to the bar.
func (v *View) serverCard(c *ui.Context, o Options, g geometry, t float32) {
	if !v.wireOn || t < tHost || t > 18200 {
		return
	}
	in := seg(t, 15350, 15800, easeOut)
	out := seg(t, 17420, 18150, easeInOut)
	r := serverRect(g)
	x := lerp(r.x, g.ctx.x+8, out)
	y := lerp(r.y, g.ctx.y, out) + (1-in)*8
	status := "Connecting…"
	if t >= 16700 {
		status = "Saved"
	}
	name := "Your server"
	if len(o.Hosts) > 0 {
		name = o.Hosts[len(o.Hosts)-1]
	}
	ui.Column(c.Key("ob-server")).Absolute().Left(x).Top(y).Size(r.w, r.h).Opacity(in*(1-seg(t, 17950, 18200, easeIn))).Scale(lerp(1, .62, out), lerp(1, .62, out)).
		Padding(theme.S2, theme.S3).Radius(theme.RadiusCard).Background(theme.Popover).Border(1, theme.Border).Children(func() {
		ui.Text(c, name).FontSize(theme.TextUI).FontWeight(500).TextColor(theme.Text).SingleLine()
		ui.Text(c, status).FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine()
	})
}

func nonEmpty(s, other string) string {
	if s != "" {
		return s
	}
	return other
}

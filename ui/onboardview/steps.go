package onboardview

import (
	"fmt"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
)

// The steps. Each reads real settings and acts through Options: nothing here needs an
// agent or the network to go on, and what's optional has Later.

// stepView draws view id.
func (v *View) stepView(p pane, id string, o Options, s scene) {
	switch id {
	case "agents":
		v.agentsStep(&p, o, s)
	case "project":
		v.projectStep(&p, o)
	case "host":
		v.hostStep(&p, o, s)
	case "style":
		v.styleStep(&p, o)
	case "goa":
		v.goaStep(&p, o)
	case "task":
		v.taskStep(&p, o, s)
	case "community":
		v.communityStep(&p, o)
	case "back":
		v.backStep(&p, o)
	}
}

// backFn is what Back does here, nil where it can't go back.
func (v *View) backFn(o Options) func() {
	if v.at == 0 && v.detour == nil {
		return nil
	}
	return func() { v.back(o) }
}

func (v *View) agentsStep(p *pane, o Options, s scene) {
	c, vp := p.c, p.v
	rows := listRows(o)
	p.body(func() {
		p.title("Agents")
		about := "Looking for agents on " + o.Device + "."
		if o.AgentsLoaded && (s.t >= 11500 || s.returning) {
			about = fmt.Sprintf("Found %d of %d on %s.", foundOf(rows), len(rows), o.Device)
			if len(rows) == 0 {
				about = "None found yet. You can go on without one, and install one whenever you like."
			}
		}
		p.about(about)
		ui.Column(c.Key("rows")).FillWidth().Shrink(0).Children(func() {
			for j, ag := range rows {
				v.agentRow(p, o, s, j, ag)
			}
		})
		if len(rows) == 0 && vp.fs > 0 {
			return
		}
	})
	p.foot(v.backFn(o), func() {
		if p.primary("Continue", true) {
			v.next(o)
		}
	})
}

// agentRow looks, then answers, then lends its icon to the agent flying to the list.
func (v *View) agentRow(p *pane, o Options, s scene, j int, ag proto.AgentInfo) {
	c, vp, t := p.c, p.v, s.t
	at := 9100 + float32(j)*420
	a, res := seg(t, at, at+260, easeOut), seg(t, at+260, at+480, easeOut)
	if s.returning || v.detour != nil {
		a, res = 1, 1
	}
	dim := float32(1)
	if !s.returning && t > 11700 && t < 13300 && ag.Installed {
		if from := 11900 + float32(j)*80; t >= from && t < from+800+500 {
			dim = .28
		}
	}
	status, col := "Looking…", vp.idle
	if res >= .02 {
		if ag.Installed {
			status, col = "Installed", dotDone
		} else {
			status = "Not installed"
		}
	}
	ui.Column(c.Key(ag.ID)).FillWidth().Height(48).Opacity(a).Translate(0, (1-a)*6).Shrink(0).Children(func() {
		if j > 0 {
			ui.Box(c.Key("rule")).Absolute().Left(0).Right(0).Top(0).Height(1).Background(vp.rim2)
		}
		ui.Row(c.Key("row")).FillWidth().Grow(1).MinHeight(0).Gap(8).AlignItems(ui.Center).Children(func() {
			tile(c, icon.Agent(agent.Base(ag.ID)), 16, vp.tile, vp.ink, dim)
			ui.Column(c.Key("words")).Grow(1).MinWidth(0).Children(func() {
				ui.Row(c.Key("name")).FillWidth().MinWidth(0).Clip().Children(func() { line(c, ag.Name, vp.fs, vp.ink) })
				ui.Row(c.Key("state")).Gap(4).AlignItems(ui.Center).Children(func() {
					dot(c, 6, col)
					line(c, status, 12, vp.ink2)
				})
			})
			if !ag.Installed && ag.Homepage != "" && res >= .02 {
				if p.plain("Install…") {
					o.OpenLink(ag.Homepage)
				}
			}
		})
	})
}

func (v *View) projectStep(p *pane, o Options) {
	if len(o.Projects) > v.projectsAtStart && v.current().id == "project" && !v.projectMoved {
		v.projectMoved = true
		v.next(o) // one was just added: it flies to the list
	}
	p.body(func() {
		p.title("Project")
		p.about("Agents work in a folder. Choose one on " + o.Device + ", or clone one here.")
		if p.card("open", icon.Folder, "Open a folder…", "On "+o.Device, 52) {
			v.pick = 0
			o.OpenFolder()
		}
		if p.card("clone", icon.Git, "Clone from Git…", "From a Git address", 52) {
			v.pick = 1
			o.OpenClone()
		}
	})
	p.foot(v.backFn(o), func() {
		if len(o.Projects) > 0 {
			if p.primary("Continue", true) {
				v.next(o)
			}
			return
		}
		if p.plain("Later") {
			v.next(o)
		}
	})
}

// hostName is the host the user just added, as the app has it.
func (v *View) hostName(o Options) string {
	if len(o.Hosts) > 0 {
		return o.Hosts[len(o.Hosts)-1]
	}
	return "Your server"
}

func (v *View) hostStep(p *pane, o Options, s scene) {
	c, vp, t := p.c, p.v, s.t
	if len(o.Hosts) > v.hostsAtStart && !v.wireOn && v.current().id == "host" {
		v.wireOn = true // one was just added: its wire plays on the way on
		v.next(o)
	}
	formShown := t < 15400 || v.detour != nil
	dip := clamp(abs(t-15400)/110, 0, 1)
	if v.detour != nil {
		dip = 1
	}
	p.body(func() {
		p.title("Host")
		about := o.Device + " is your first host. A server can run sessions too. You can add one any time from Hosts."
		switch {
		case formShown:
		case v.wireOn && t < 16760:
			about = "Connecting over SSH."
		case v.wireOn:
			about = "It is in the host menu at the top, with " + o.Device + "."
		default:
			about = "You can add a server any time from Hosts."
		}
		ui.Box(c.Key("dip")).FillWidth().Opacity(dip).Shrink(0).Children(func() {
			ui.Text(c, about).FontSize(12).LineHeight(lead12).TextColor(vp.ink2).FillWidth()
		})
		if formShown {
			p.label("Server", "SSH, with your keys")
			ui.TextInput(c.Key("addr"), &v.hostAddress).FillWidth().Height(28).PaddingX(8).Radius(vp.rc).Background(vp.well).InsetShadow(0, 0, 0, 1, vp.rim2).
				Font(theme.MonoFont).FontSize(12).TextColor(vp.ink).Placeholder("root@server").Label("Server address").Shrink(0)
			return
		}
		name, state := "No server yet", "Skipped for now"
		if v.wireOn {
			name, state = v.hostName(o), "Connecting…"
			if t >= 16760 {
				state = "Saved"
			}
		}
		ui.Row(c.Key("result")).FillWidth().Padding(8).Gap(8).Radius(vp.rr).Background(vp.fill0).InsetShadow(0, 0, 0, 1, vp.rim2).AlignItems(ui.Center).Opacity(dip).Shrink(0).Children(func() {
			tile(c, icon.Server, 14, vp.tile, vp.ink, 1)
			ui.Column(c.Key("words")).Grow(1).MinWidth(0).Children(func() {
				ui.Row(c.Key("name")).FillWidth().MinWidth(0).Clip().Children(func() { line(c, name, vp.fs, vp.ink) })
				line(c.Key("state"), state, 12, vp.ink2)
			})
		})
	})
	p.foot(v.backFn(o), func() {
		if formShown {
			if p.plain("Later") {
				v.wireOn = false
				v.next(o)
				v.clock.t = v.clock.target // nothing to watch: straight to what became of it
			}
			if p.primary("Add host", true) {
				o.AddHost(strings.TrimSpace(v.hostAddress))
			}
			return
		}
		if (!v.wireOn || t >= 16750) && p.primary("Continue", true) {
			v.next(o)
		}
	})
}

func (v *View) styleStep(p *pane, o Options) {
	c, vp := p.c, p.v
	_, to, _ := v.morph.at(o.StyleID)
	isGlass := theme.StyleByID(to).ID == theme.LiquidGlass.ID
	p.body(func() {
		p.title("Style")
		p.about("The window changes as you choose. The regions stay in their places.")
		p.next(ui.Box(c.Key("ths")).FillWidth().Height(200).Shrink(0)).Children(func() { v.styleCards(p, o) })
		name := "Style's own"
		for _, pal := range palettes {
			if pal.id == o.PaletteID {
				name = pal.name
			}
		}
		p.label("Palette", name)
		p.next(ui.Row(c.Key("pals")).Height(24).Gap(8).AlignItems(ui.Center).PaddingX(2).Shrink(0)).Children(func() {
			if paletteDot(c.Key("own"), p, vp.desk, vp.accent, "Style's own", !knownPalette(o.PaletteID)) {
				o.SetPalette("")
			}
			for _, pal := range palettes {
				if paletteDot(c.Key(pal.id), p, pal.base, pal.accent, pal.name, pal.id == o.PaletteID) {
					o.SetPalette(pal.id)
				}
			}
		})
		p.label("Background", "")
		p.next(ui.Row(c.Key("bg")).Shrink(0)).Children(func() {
			at := 0
			switch o.BackgroundMode {
			case BackgroundPlain:
				at = 1
			case BackgroundImage:
				at = 2
			}
			switch segmented(c, vp, []string{"Default", "Plain", "Image…"}, at, p.live, !isGlass) {
			case 0:
				o.SetBackground(BackgroundDefault)
			case 1:
				o.SetBackground(BackgroundPlain)
			case 2:
				o.SetBackground(BackgroundImage)
			}
		})
		mine := isGlass && o.BackgroundMode == BackgroundImage
		v.styleSlider(p, "Panels' opacity", fmt.Sprintf("%.0f%%", o.PanelOpacity*100), o.PanelOpacity, 0, 1, mine, o.SetPanelOpacity)
		v.styleSlider(p, "Blur", fmt.Sprintf("%.0f", o.BlurValue), o.BlurValue, 0, o.BlurMost, mine, o.SetBlur)
		switch {
		case !isGlass:
			p.about("Background, opacity and blur apply to Liquid Glass.")
		case !mine:
			p.about("Opacity and blur apply to an image of yours.")
		}
	})
	p.foot(v.backFn(o), func() {
		if p.primary("Continue", true) {
			v.next(o)
		}
	})
}

func knownPalette(id string) bool {
	return slices.ContainsFunc(palettes, func(p struct {
		id     string
		name   string
		accent ui.Color
		base   ui.Color
	}) bool {
		return p.id == id
	})
}

// styleSlider is a slider with its name and value, in the design's three columns.
func (v *View) styleSlider(p *pane, name, value string, at, lo, hi float32, on bool, set func(float32)) {
	c, vp := p.c, p.v
	col := vp.ink2
	if !on {
		col = vp.ink3
	}
	p.next(ui.Row(c.Key(name)).FillWidth().Height(22).Gap(8).AlignItems(ui.Center).Shrink(0)).Children(func() {
		ui.Box(c.Key("name")).Width(96).Shrink(0).Children(func() { line(c, name, 12, col) })
		x := float64(at)
		if ui.Slider(c.Key("slider"), &x, float64(lo), float64(hi)).Grow(1).MinWidth(0).Label(name).Disabled(!on).Changed() && on {
			set(float32(x))
		}
		ui.Row(c.Key("value")).Width(34).Justify(ui.End).Shrink(0).Children(func() { line(c, value, 12, col) })
	})
}

// paletteDot is a palette to pick, half ground and half accent, and says it was clicked.
func paletteDot(c *ui.Context, p *pane, base, accent ui.Color, name string, on bool) bool {
	const size = 22
	b := ui.ButtonBase(c).Size(size, size).Radius(size / 2).Cursor(ui.CursorPointer).Label(name).Tooltip(name).Shrink(0).Disabled(!p.live)
	if on {
		b.Shadow(0, 0, 0, 4, p.v.ink).Shadow(0, 0, 0, 2, p.v.panel)
	}
	b.Children(func() {
		ui.Box(c.Key("d")).Fill().Draw(func(pa *ui.Painter, r ui.Rect) {
			pa.Clip(r, size/2, func() {
				pa.Fill(ui.Rect{X: r.X, Y: r.Y, W: r.W / 2, H: r.H}, base, 0)
				pa.Fill(ui.Rect{X: r.X + r.W/2, Y: r.Y, W: r.W / 2, H: r.H}, accent, 0)
			})
			pa.Stroke(r, p.v.rim2, size/2, 1)
		})
	})
	return p.live && b.Clicked()
}

// The four small windows, in the styles' own colors whatever the window is in now.
var miniatures = []struct {
	id, name         string
	bg, pane, main   ui.Color
	accent, edge     ui.Color
	gap, radius, rim float32
	lights           bool
}{
	{theme.LiquidGlass.ID, "Liquid Glass", ui.Hex("#15182B"), ui.Hex("#3A3C50"), ui.Hex("#4C4E62"), ui.Hex("#0A84FF"), rgba(255, 255, 255, .22), 3, 5, 1, true},
	{theme.Ledger.ID, "Ledger", ui.Hex("#000000"), ui.Hex("#0F0F0F"), ui.Hex("#000000"), ui.Hex("#2F6FED"), ui.Hex("#2E2E2E"), 0, 0, 1, false},
	{theme.Tonal.ID, "Tonal", ui.Hex("#0D2A25"), ui.Hex("#1D3934"), ui.Hex("#2A4741"), ui.Hex("#FF9F7A"), rgba(0, 0, 0, 0), 3, 6, 0, false},
	{theme.Bento.ID, "Bento", ui.Hex("#000000"), ui.Hex("#0F0F0F"), ui.Hex("#0F0F0F"), ui.Hex("#2F6FED"), ui.Hex("#2E2E2E"), 3, 2, 1, false},
}

// styleCards are the four live cards, and one ring moving from the old choice to the new.
func (v *View) styleCards(p *pane, o Options) {
	c, vp := p.c, p.v
	const cardW, cardH, stepX, stepY = 144, 76, 152, 104
	from, to, x := v.morph.at(o.StyleID)
	indexOf := func(id string) int {
		id = theme.StyleByID(id).ID
		return slices.IndexFunc(miniatures, func(m struct {
			id, name         string
			bg, pane, main   ui.Color
			accent, edge     ui.Color
			gap, radius, rim float32
			lights           bool
		}) bool {
			return m.id == id
		})
	}
	for i, m := range miniatures {
		left, top := float32(i%2)*stepX, float32(i/2)*stepY
		b := ui.ButtonBase(c.Key(m.id)).Absolute().Left(left).Top(top).Width(cardW).Label(m.name).Cursor(ui.CursorPointer).Disabled(!p.live)
		b.Children(func() {
			ui.Column(c).Gap(4).Children(func() {
				miniature(c, i, vp)
				ui.Box(c.Key("name")).PaddingX(2).Children(func() { line(c, m.name, 12, vp.ink) })
			})
		})
		if p.live && b.Clicked() && m.id != theme.StyleByID(to).ID {
			v.morph.start(o, m.id)
		}
	}
	ia, ib := indexOf(from), indexOf(to)
	e := easeInOut(x)
	ui.Box(c.Key("ring")).Absolute().PassThrough().Left(lerp(float32(ia%2)*stepX, float32(ib%2)*stepX, e)).Top(lerp(float32(ia/2)*stepY, float32(ib/2)*stepY, e)).
		Size(cardW, cardH).Radius(8).InsetShadow(0, 0, 0, 2, vp.accent2)
}

// miniature is a style's window, small.
func miniature(c *ui.Context, i int, vp look) {
	m := miniatures[i]
	ui.Box(c.Key("mini")).Size(144, 76).Radius(8).Clip().Background(m.bg).InsetShadow(0, 0, 0, 1, vp.rim2).Draw(func(p *ui.Painter, r ui.Rect) {
		if m.lights {
			p.Fill(ui.Rect{X: r.X + 6, Y: r.Y, W: 60, H: 40}, fadeColor(wallLight[0].col, .4), 30)
			p.Fill(ui.Rect{X: r.X + r.W - 60, Y: r.Y + 20, W: 60, H: 50}, fadeColor(wallLight[1].col, .4), 30)
		}
		g := m.gap
		panel := func(x, y, w, h float32, col ui.Color) {
			box := ui.Rect{X: r.X + x, Y: r.Y + y, W: w, H: h}
			p.Fill(box, col, m.radius)
			if m.rim > 0 {
				p.Stroke(box, m.edge, m.radius, m.rim)
			}
		}
		panel(g, g, r.W-2*g, 10, m.pane)
		panel(g, 2*g+10, 34, r.H-3*g-10, m.pane)
		panel(2*g+34, 2*g+10, r.W-3*g-34, r.H-3*g-10, m.main)
		p.Fill(ui.Rect{X: r.X + 3*g + 34, Y: r.Y + r.H - g - 3 - 10, W: r.W - 5*g - 56, H: 10}, m.accent, m.radius)
		p.Fill(ui.Rect{X: r.X + g + 3, Y: r.Y + 2*g + 14, W: 26, H: 6}, m.accent, 4)
	})
}

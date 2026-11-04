package onboardview

import (
	"fmt"
	"slices"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
)

// communityLink is a row of the Community step.
type communityLink struct {
	name, sub, url string
	svg            *ui.SVG
}

// communityLinks are the rows that have somewhere to go.
func communityLinks() []communityLink {
	all := []communityLink{
		{"X", "News from the owner", LinkX, icon.AtSign},
		{"Discord", "Join the server", LinkDiscord, icon.Discord},
		{"GitHub", "Star the repository", LinkGitHub, icon.Star},
	}
	return slices.DeleteFunc(all, func(l communityLink) bool { return l.url == "" })
}

// goaAgents are the agents that can hold GOD of Agents here.
func goaAgents(o Options) []proto.GOAAgent {
	return slices.DeleteFunc(slices.Clone(o.GOAAgents), func(a proto.GOAAgent) bool { return !a.Installed || a.NotYet != "" })
}

// goaName is the agent it would run on, by name.
func (v *View) goaName(o Options) string {
	can := goaAgents(o)
	for _, a := range can {
		if a.Agent == v.goaAgent {
			return a.Name
		}
	}
	if len(can) > 0 {
		return can[0].Name
	}
	return ""
}

func (v *View) goaStep(p *pane, o Options) {
	c, vp := p.c, p.v
	can := goaAgents(o)
	if v.goaAgent == "" && len(can) > 0 {
		v.goaAgent = can[0].Agent
	}
	rule := func(svg *ui.SVG, title, words string) {
		p.next(ui.Column(c.Key(title)).FillWidth().Shrink(0)).Children(func() {
			ui.Box(c.Key("rule")).FillWidth().Height(1).Background(vp.rim2)
			ui.Row(c.Key("row")).FillWidth().Gap(8).Padding(8, 0).AlignItems(ui.Start).Children(func() {
				ui.Icon(c, svg).FontSize(14).TextColor(vp.ink2).Shrink(0).Margin(1, 0, 0, 0)
				ui.Column(c.Key("words")).Grow(1).MinWidth(0).Children(func() {
					line(c, title, 12, vp.ink).FontWeight(500)
					ui.Text(c, words).FontSize(12).LineHeight(lead12).TextColor(vp.ink2).FillWidth()
				})
			})
		})
	}
	p.body(func() {
		p.title("GOD of Agents")
		p.about("One session you drive. It hands each task to a session, follows it, checks what came back and reports.")
		if len(can) > 0 {
			p.label("Run it on", "")
			p.next(ui.Row(c.Key("on")).Shrink(0)).Children(func() {
				labels := make([]string, len(can))
				at := 0
				for i, a := range can {
					labels[i] = a.Name
					if a.Agent == v.goaAgent {
						at = i
					}
				}
				if i := segmented(c, vp, labels, at, p.live, false); i >= 0 {
					v.goaAgent = can[i].Agent
				}
			})
		} else {
			p.about("It needs an agent. Turn it on from the title bar once you have one.")
		}
		p.label("Its rules", "")
		rule(icon.Lock, "These stay yours", "What can't be undone, money, publishing and secrets always ask you.")
		rule(icon.Shield, "These it handles", "Edits in a worker's own folder are allowed. It answers the rest of its workers' prompts.")
		rule(icon.Languages, "Language", "It answers you in the language you write in.")
		if v.goaErr != "" {
			ui.Text(c.Key("err"), v.goaErr).FontSize(12).LineHeight(lead12).TextColor(dotWait).FillWidth()
		}
	})
	p.foot(v.backFn(o), func() {
		if o.GOAOn {
			if p.primary("Continue", true) {
				v.next(o)
			}
			return
		}
		if p.plain("Later") {
			v.next(o)
		}
		if len(can) == 0 {
			return
		}
		label := "Turn on"
		if v.goaBusy {
			label = "Starting…"
		}
		if p.primary(label, !v.goaBusy) {
			v.goaBusy, v.goaErr = true, ""
			o.StartGOA(v.goaAgent, func(err error) {
				v.goaBusy = false
				if err != nil {
					v.goaErr = err.Error()
					return
				}
				v.next(o)
			})
		}
	})
}

func (v *View) taskStep(p *pane, o Options, s scene) {
	c, vp := p.c, p.v
	id, name := firstAgent(o)
	_, hasProject := v.project(o)
	running := v.task.sent && s.t >= 41300
	p.body(func() {
		p.title("First task")
		switch {
		case id == "":
			p.about("It needs an agent. Start one with New session once you have one.")
		case !hasProject:
			p.about("It needs a project. Start one with New session once you've added one.")
		case running:
			ui.Row(c.Key("now")).FillWidth().Height(20).Gap(8).AlignItems(ui.Center).Shrink(0).Children(func() {
				dot(c, 8, dotWork)
				code(c, name+" is working", vp.focus)
			})
			p.about("It runs in a session of its own. You'll find it in Sessions, and it tells you when it's done.")
		default:
			p.about("Type a small prompt in the composer and send it. It runs in a session of its own.")
			p.next(ui.Box(c.Key("or")).Height(16).Shrink(0)).Children(func() { line(c, "Or start from one of these", 12, vp.ink2).FontWeight(500) })
			for _, sug := range []string{"Add a short section on running it to the README.", "List what changed since the last commit."} {
				if p.card(sug, nil, sug, "", 40) {
					v.task.text = sug
				}
			}
		}
		if v.task.err != "" {
			ui.Text(c.Key("err"), v.task.err).FontSize(12).LineHeight(lead12).TextColor(dotWait).FillWidth()
		}
	})
	p.foot(v.backFn(o), func() {
		if !v.task.sent && p.plain("Later") {
			v.skipTask(o)
		}
		if !v.task.sent || s.t < 44500 {
			p.primary("Continue", false)
			return
		}
		if p.button("Continue", vp.accent, vp.ona, 500, nil) {
			v.next(o)
		}
	})
}

// skipTask goes on without a first task: nothing plays of one.
func (v *View) skipTask(o Options) {
	for i, st := range v.stops {
		if st.id == "community" {
			v.at = i
		}
	}
	v.clock.t = tRunning
	v.goTo(o)
}

func (v *View) communityStep(p *pane, o Options) {
	c, vp := p.c, p.v
	p.body(func() {
		p.title("Community")
		p.about("Follow along, if you like. Each opens in your browser.")
		ui.Column(c.Key("rows")).FillWidth().Shrink(0).Children(func() {
			for i, l := range communityLinks() {
				p.next(ui.Column(c.Key(l.name)).FillWidth().Height(48)).Children(func() {
					if i > 0 {
						ui.Box(c.Key("rule")).FillWidth().Height(1).Background(vp.rim2).Shrink(0)
					}
					ui.Row(c.Key("row")).FillWidth().Grow(1).MinHeight(0).Gap(8).AlignItems(ui.Center).Children(func() {
						tile(c, l.svg, 16, vp.tile, vp.ink, 1)
						ui.Column(c.Key("words")).Grow(1).MinWidth(0).Children(func() {
							line(c, l.name, vp.fs, vp.ink)
							line(c, l.sub, 12, vp.ink2)
						})
						if p.button("Open", vp.fill, vp.ink, 400, func() { ui.Icon(c, icon.External).FontSize(12).TextColor(vp.ink) }) {
							o.OpenLink(l.url)
						}
					})
				})
			}
		})
		p.about("You can continue without any of these. enowX doesn't check what you do there.")
	})
	p.foot(v.backFn(o), func() {
		if p.primary("Continue", true) {
			v.next(o)
		}
	})
}

// backStep is a returning user's settings, filled in, to confirm or change.
func (v *View) backStep(p *pane, o Options) {
	c, vp := p.c, p.v
	goa := "Off"
	if o.GOAOn {
		goa = "On"
	}
	project := nonEmpty(v.projectName(o), "None yet")
	hosts := o.Device
	for _, h := range o.Hosts {
		hosts += ", " + h
	}
	rows := []struct{ name, value, to string }{
		{"Agents", fmt.Sprintf("%d found", foundOf(listRows(o))), "agents"},
		{"Project", project, "project"},
		{"Hosts", hosts, "host"},
		{"Style", theme.StyleByID(o.StyleID).Name, "style"},
		{"GOA", goa, "goa"},
	}
	p.body(func() {
		p.title("Your settings")
		p.about("Filled in from this account. Confirm them, or change what you like.")
		ui.Column(c.Key("rows")).FillWidth().Shrink(0).Children(func() {
			for _, r := range rows {
				p.next(ui.Column(c.Key(r.name)).FillWidth().Height(30)).Children(func() {
					ui.Box(c.Key("rule")).FillWidth().Height(1).Background(vp.rim2)
					ui.Row(c.Key("row")).FillWidth().Grow(1).MinHeight(0).Gap(8).AlignItems(ui.Center).Children(func() {
						ui.Box(c.Key("name")).Width(62).Shrink(0).Children(func() { line(c, r.name, 12, vp.ink2) })
						ui.Row(c.Key("value")).Grow(1).MinWidth(0).Clip().Children(func() { line(c, r.value, 12, vp.ink) })
						b := ui.ButtonBase(c.Key("change")).Height(22).PaddingX(4).Radius(4).Label("Change " + r.name).Cursor(ui.CursorPointer).Center().Disabled(!p.live)
						b.Children(func() { line(c, "Change", 12, vp.accent2) })
						if p.live && b.Clicked() {
							v.changeStep(o, r.to)
						}
					})
				})
			}
		})
		p.label("New in this version", "")
		p.about("Four UI styles: Liquid Glass, Ledger, Tonal and Bento.")
	})
	p.foot(nil, func() {
		if p.primary("Continue", true) {
			v.next(o)
		}
	})
}

// changeStep shows step id of the full flow, coming back here after.
func (v *View) changeStep(o Options, id string) {
	i := slices.IndexFunc(stopsOf(FlowNew, o.Reduce), func(s stop) bool { return s.id == id })
	if i < 0 {
		return
	}
	s := stopsOf(FlowNew, o.Reduce)[i]
	s.step = 0 // a detour isn't one of the seven
	s.name = map[string]string{"agents": "Agents", "project": "Project", "host": "Host", "style": "Style", "goa": "GOD of Agents"}[id]
	v.detour = &s
}

func nonEmpty(s, other string) string {
	if s != "" {
		return s
	}
	return other
}

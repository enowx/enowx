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
	"github.com/enowx/enowx/ui/widget"
)

// The panel holding the side panel's place: one step at a time, with what can wait
// keeping Later, and nothing that needs an agent or the network to go on.

// installed are the agents found here.
func installed(o Options) []proto.AgentInfo {
	return slices.DeleteFunc(slices.Clone(o.Agents), func(a proto.AgentInfo) bool {
		return !a.Installed || a.Command == ""
	})
}

// communityLink is a card of the Community step.
type communityLink struct {
	name, sub, url string
	svg            *ui.SVG
}

// communityLinks are the cards that have somewhere to go.
func communityLinks() []communityLink {
	all := []communityLink{
		{"X", "News from the owner", LinkX, icon.Globe},
		{"Discord", "Join the server", LinkDiscord, icon.Discord},
		{"GitHub", "Star the repository", LinkGitHub, icon.Star},
	}
	return slices.DeleteFunc(all, func(l communityLink) bool { return l.url == "" })
}

// panelContent is the panel's step, gone as the flow folds up.
func (v *View) panelContent(c *ui.Context, o Options, g geometry, t, over float32) {
	a := seg(t, tPanelTo-300, tPanelTo+100, easeOut) * (1 - over)
	if a < .01 {
		return
	}
	r := g.panel
	s := v.current()
	ui.Column(c.Key("ob-panel")).Absolute().Left(r.x).Top(r.y).Size(r.w, r.h).Opacity(a).Padding(theme.S4).Gap(theme.S3).Clip().Children(func() {
		v.panelHead(c, s)
		ui.Column(c.Key(s.id)).Grow(1).FillWidth().MinHeight(0).Gap(theme.S3).Transition(widget.RowMotion(0)).Children(func() {
			switch s.id {
			case "agents":
				v.agentsStep(c, o)
			case "project":
				v.projectStep(c, o)
			case "host":
				v.hostStep(c, o)
			case "style":
				v.styleStep(c, o)
			case "goa":
				v.goaStep(c, o)
			case "task":
				v.taskStep(c, o)
			case "community":
				v.communityStep(c, o)
			case "back":
				v.backStep(c, o)
			}
		})
	})
}

// panelHead says where in the seven steps this is.
func (v *View) panelHead(c *ui.Context, s stop) {
	total := stepsShown(v.stops)
	ui.Row(c.Key("head")).FillWidth().Gap(theme.S2).AlignItems(ui.Center).Shrink(0).Children(func() {
		ui.Text(c, "Set up enowX").FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine()
		ui.Spacer(c)
		if s.step == 0 {
			return
		}
		ui.Text(c, fmt.Sprintf("%d of %d", s.step, total)).FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine().Shrink(0)
		ui.Row(c).Gap(3).Shrink(0).Children(func() {
			for i := 1; i <= total; i++ {
				col := theme.Border
				if i <= s.step {
					col = theme.Accent
				}
				ui.Box(c.Key(i)).Size(5, 5).Radius(theme.RadiusFull).Background(col)
			}
		})
	})
}

func stepTitle(c *ui.Context, title, about string) {
	ui.Text(c, title).FontSize(theme.TextTitle).FontWeight(600).TextColor(theme.Text)
	if about != "" {
		ui.Text(c, about).FontSize(theme.TextUI).TextColor(theme.TextSoft)
	}
}

// stepBody scrolls what a step holds, its buttons staying at the foot.
func stepBody(c *ui.Context, fn func()) {
	ui.Scroll(c.Key("body")).Grow(1).FillWidth().MinHeight(0).Children(func() {
		ui.Column(c).FillWidth().Gap(theme.S2).Children(fn)
	})
}

// stepFoot is a step's buttons: Back unless first; Later when it can wait; the main one.
func (v *View) stepFoot(c *ui.Context, o Options, later bool, main string) {
	ui.Row(c.Key("foot")).FillWidth().Gap(theme.S2).AlignItems(ui.Center).Shrink(0).Children(func() {
		if v.at > 0 || v.detour != nil {
			if widget.Button(c.Key("back"), widget.Secondary, nil, "Back", "").Clicked() {
				v.back(o)
			}
		}
		ui.Spacer(c)
		if later && widget.Button(c.Key("later"), widget.Secondary, nil, "Later", "").Clicked() {
			v.next(o)
		}
		if main != "" && widget.PrimaryButton(c.Key("next"), nil, main, "").Clicked() {
			v.next(o)
		}
	})
}

// listRow is a step's row: an icon, a name and a line, and what's at its right.
func listRow(c *ui.Context, svg *ui.SVG, name, sub string, right func()) {
	ui.Row(c).FillWidth().MinHeight(theme.RowTwo).Gap(theme.S2).AlignItems(ui.Center).Children(func() {
		if svg != nil {
			ui.Icon(c, svg).FontSize(16).TextColor(theme.TextSoft).Shrink(0)
		}
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			widget.Running(c, name, widget.RunStyle{Size: theme.TextUI})
			if sub != "" {
				widget.Running(c.Key("sub"), sub, widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
			}
		})
		if right != nil {
			right()
		}
	})
}

func (v *View) agentsStep(c *ui.Context, o Options) {
	found := installed(o)
	about := "Looking for agents on " + o.Device + "."
	switch {
	case o.AgentsLoaded && len(found) == 0:
		about = "None found yet. You can go on without one, and install one whenever you like."
	case o.AgentsLoaded:
		about = fmt.Sprintf("%d found on %s.", len(found), o.Device)
	}
	stepTitle(c, "Agents", about)
	stepBody(c, func() {
		for _, ag := range o.Agents {
			if ag.Command == "" {
				continue
			}
			sub, id := "Not installed", ag.ID
			if ag.Installed {
				sub = "Installed"
			}
			listRow(c.Key(id), icon.Agent(agent.Base(id)), ag.Name, sub, func() {
				if !ag.Installed && ag.Homepage != "" && widget.GhostButton(c.Key("get"), "Get it").Label("Get "+ag.Name).Clicked() {
					o.OpenLink(ag.Homepage)
				}
			})
		}
		if !o.AgentsLoaded {
			widget.Spinner(c, 14)
		}
	})
	if o.Rescan != nil && o.AgentsLoaded && widget.Button(c.Key("rescan"), widget.Secondary, icon.Sync, "Look again", "").Clicked() {
		o.Rescan()
	}
	v.stepFoot(c, o, false, "Continue")
}

func (v *View) projectStep(c *ui.Context, o Options) {
	stepTitle(c, "Project", "Agents work in a folder. Choose one on "+o.Device+", or clone one here.")
	stepBody(c, func() {
		if widget.Button(c.Key("open"), widget.Secondary, icon.Folder, "Open a folder…", "").FillWidth().Clicked() {
			o.OpenFolder()
		}
		if widget.Button(c.Key("clone"), widget.Secondary, icon.Download, "Clone from Git…", "").FillWidth().Clicked() {
			o.OpenClone()
		}
		for _, p := range o.Projects {
			listRow(c.Key(p.Path), icon.Check, nonEmpty(p.Name, p.Path), p.Path, nil)
		}
	})
	if len(o.Projects) > 0 {
		v.stepFoot(c, o, false, "Continue")
	} else {
		v.stepFoot(c, o, true, "")
	}
}

func (v *View) hostStep(c *ui.Context, o Options) {
	stepTitle(c, "Host", o.Device+" is ready. Add a server to run agents there too, over SSH with your keys.")
	if len(o.Hosts) > v.hostsAtStart && !v.wireOn {
		v.wireOn = true // one was just added: its wire plays on the way on
		v.clock.target = tHostDone
		if v.flow == FlowDevice {
			v.clock.target = tHostDone
		}
	}
	stepBody(c, func() {
		for _, h := range o.Hosts {
			listRow(c.Key(h), icon.Server, h, "Saved", nil)
		}
		if widget.Button(c.Key("add"), widget.Secondary, icon.Plus, "Add host", "").FillWidth().Clicked() {
			o.AddHost()
		}
	})
	if len(o.Hosts) > 0 {
		v.stepFoot(c, o, false, "Continue")
	} else {
		v.stepFoot(c, o, true, "")
	}
}

func (v *View) styleStep(c *ui.Context, o Options) {
	stepTitle(c, "Style", "The window changes as you choose. The regions stay in their places.")
	stepBody(c, func() {
		ui.Grid(c.Key("styles")).FillWidth().Columns(2).Gap(theme.S2).Children(func() {
			for _, s := range theme.Styles {
				on := theme.StyleByID(o.StyleID).ID == s.ID
				b := ui.ButtonBase(c.Key(s.ID)).FillWidth().Height(64).Radius(theme.RadiusCard).Cursor(ui.CursorPointer).Label(s.Name).Center().Background(theme.Sunken)
				if on {
					b.Border(2, theme.Accent)
				} else {
					b.Border(1, theme.Border)
				}
				b.Children(func() {
					ui.Text(c, s.Name).FontSize(theme.TextUI).FontWeight(500).TextColor(theme.Text).SingleLine()
				})
				if b.Clicked() && !on {
					v.morph.start(o, s.ID)
				}
			}
		})
		ui.Text(c.Key("pal"), "Palette").FontSize(theme.TextLabel).TextColor(theme.TextSoft).Padding(theme.S2, 0, 0, 0)
		ui.Row(c.Key("pals")).FillWidth().Gap(theme.S2).Wrap().Children(func() {
			if paletteDot(c.Key("own"), theme.Fade(theme.TextSoft, .4), "Style's own", o.PaletteID == "") {
				o.SetPalette("")
			}
			for _, p := range theme.Palettes[:min(len(theme.Palettes), 5)] {
				if paletteDot(c.Key(p.ID), p.Accent, p.Name, o.PaletteID == p.ID) {
					o.SetPalette(p.ID)
				}
			}
		})
		widget.CardNote(c.Key("more"), "The background, opacity and blur of Liquid Glass are in Settings, Appearance.", 0)
	})
	v.stepFoot(c, o, false, "Continue")
}

// paletteDot is a palette to pick, and says whether it was clicked.
func paletteDot(c *ui.Context, col ui.Color, name string, on bool) bool {
	b := ui.ButtonBase(c).Size(26, 26).Radius(theme.RadiusFull).Cursor(ui.CursorPointer).Label(name).Tooltip(name).Background(col)
	if on {
		b.Border(2, theme.Text)
	}
	return b.Clicked()
}

func (v *View) goaStep(c *ui.Context, o Options) {
	stepTitle(c, "GOD of Agents", "One session you drive. It hands each task to a session, follows it, checks what came back and reports.")
	can := slices.DeleteFunc(slices.Clone(o.GOAAgents), func(a proto.GOAAgent) bool { return !a.Installed })
	if v.goaAgent == "" && len(can) > 0 {
		v.goaAgent = can[0].Agent
	}
	stepBody(c, func() {
		if len(can) > 0 {
			ui.Text(c.Key("on"), "Run it on").FontSize(theme.TextLabel).TextColor(theme.TextSoft)
			for _, ag := range can {
				b := widget.Button(c.Key(ag.Agent), widget.Secondary, icon.Agent(agent.Base(ag.Agent)), ag.Name, "").FillWidth()
				if v.goaAgent == ag.Agent {
					b.Border(2, theme.Accent)
				}
				if b.Clicked() {
					v.goaAgent = ag.Agent
				}
			}
		} else {
			widget.CardNote(c.Key("none"), "It needs an agent. Turn it on from the title bar once you have one.", 0)
		}
		ui.Text(c.Key("rules"), "Its rules").FontSize(theme.TextLabel).TextColor(theme.TextSoft).Padding(theme.S2, 0, 0, 0)
		for _, r := range [][2]string{
			{"These stay yours", "What can't be undone, money, publishing and secrets always ask you."},
			{"These it handles", "Edits in a worker's own folder are allowed. It answers the rest of its workers' prompts."},
			{"Language", "It answers you in the language you write in."},
		} {
			ui.Column(c.Key(r[0])).FillWidth().Children(func() {
				ui.Text(c, r[0]).FontSize(theme.TextUI).FontWeight(500).TextColor(theme.Text)
				ui.Text(c, r[1]).FontSize(theme.TextLabel).TextColor(theme.TextSoft)
			})
		}
		if v.goaErr != "" {
			ui.Text(c.Key("err"), v.goaErr).FontSize(theme.TextLabel).TextColor(theme.Error)
		}
	})
	ui.Row(c.Key("foot")).FillWidth().Gap(theme.S2).AlignItems(ui.Center).Shrink(0).Children(func() {
		if widget.Button(c.Key("back"), widget.Secondary, nil, "Back", "").Clicked() {
			v.back(o)
		}
		ui.Spacer(c)
		if widget.Button(c.Key("later"), widget.Secondary, nil, "Later", "").Clicked() {
			v.next(o)
		}
		if len(can) == 0 || o.GOAOn {
			return
		}
		label := "Turn on"
		if v.goaBusy {
			label = "Starting…"
		}
		if widget.PrimaryButton(c.Key("on"), nil, label, "").Clicked() && !v.goaBusy {
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

// taskState is the first task's: what's typed, and how it went.
type taskState struct {
	text    string
	sent    bool
	sending bool
	err     string
}

func (v *View) taskStep(c *ui.Context, o Options) {
	can := installed(o)
	switch {
	case len(can) == 0:
		stepTitle(c, "First task", "It needs an agent. Start one with New session once you have one.")
	case len(o.Projects) == 0:
		stepTitle(c, "First task", "It needs a project. Start one with New session once you've added one.")
	case v.task.sent:
		stepTitle(c, "First task", can[0].Name+" is working on it in a session of its own. You'll find it in Sessions.")
	default:
		stepTitle(c, "First task", "Type a small prompt in the composer and send it. It runs in a session of its own.")
	}
	stepBody(c, func() {
		if len(can) == 0 || len(o.Projects) == 0 || v.task.sent {
			return
		}
		ui.Text(c.Key("or"), "Or start from one of these").FontSize(theme.TextLabel).TextColor(theme.TextSoft)
		for _, s := range []string{"List what changed since the last commit.", "Add a short section on running it to the README."} {
			if widget.Button(c.Key(s), widget.Secondary, nil, s, "").FillWidth().Clicked() {
				v.task.text = s
			}
		}
	})
	if v.task.sent {
		v.stepFoot(c, o, false, "Continue")
	} else {
		v.stepFoot(c, o, true, "")
	}
}

// taskComposer is where the first task is typed: a real message to a real session.
func (v *View) taskComposer(c *ui.Context, o Options) {
	can := installed(o)
	if len(can) == 0 || len(o.Projects) == 0 {
		return
	}
	ui.Spacer(c)
	if v.task.err != "" {
		ui.Text(c.Key("err"), v.task.err).FontSize(theme.TextLabel).TextColor(theme.Error)
	}
	ui.Row(c.Key("box")).FillWidth().Gap(theme.S2).AlignItems(ui.Center).Children(func() {
		in := widget.Input(c.Key("in"), &v.task.text, "Ask "+can[0].Name+" something small").Grow(1).MinWidth(0)
		send := in.Submitted()
		if widget.PrimaryButton(c.Key("send"), nil, "Send", "").Clicked() {
			send = true
		}
		if send && strings.TrimSpace(v.task.text) != "" && !v.task.sending && !v.task.sent {
			v.task.sending, v.task.err = true, ""
			o.StartTask(can[0].ID, o.Projects[0].Path, v.task.text, func(err error) {
				v.task.sending = false
				if err != nil {
					v.task.err = err.Error()
					return
				}
				v.task.sent = true
			})
		}
	})
}

func (v *View) communityStep(c *ui.Context, o Options) {
	stepTitle(c, "Community", "Follow along, if you like. Each opens in your browser.")
	stepBody(c, func() {
		for _, l := range communityLinks() {
			listRow(c.Key(l.name), l.svg, l.name, l.sub, func() {
				if widget.GhostButton(c.Key("open"), "Open").Label("Open " + l.name).Clicked() {
					o.OpenLink(l.url)
				}
			})
		}
		widget.CardNote(c.Key("note"), "You can continue without any of these. enowX doesn't check what you do there.", 0)
	})
	v.stepFoot(c, o, false, "Continue")
}

// backStep is a returning user's settings, filled in, to confirm or change.
func (v *View) backStep(c *ui.Context, o Options) {
	stepTitle(c, "Your settings", "Filled in from this account. Confirm them, or change what you like.")
	style := theme.StyleByID(o.StyleID).Name
	goa := "Off"
	if o.GOAOn {
		goa = "On"
	}
	rows := []struct {
		name, value, to string
	}{
		{"Agents", fmt.Sprintf("%d found", len(installed(o))), "agents"},
		{"Projects", fmt.Sprintf("%d", len(o.Projects)), "project"},
		{"Hosts", fmt.Sprintf("%d besides %s", len(o.Hosts), o.Device), "host"},
		{"Style", style, "style"},
		{"GOD of Agents", goa, "goa"},
	}
	stepBody(c, func() {
		for _, r := range rows {
			listRow(c.Key(r.name), nil, r.name, r.value, func() {
				if widget.GhostButton(c.Key("change"), "Change").Label("Change " + r.name).Clicked() {
					v.changeStep(o, r.to)
				}
			})
		}
	})
	v.stepFoot(c, o, false, "Confirm")
}

// changeStep shows step id of the full flow, coming back here after.
func (v *View) changeStep(o Options, id string) {
	i := slices.IndexFunc(stopsOf(FlowNew, o.Reduce), func(s stop) bool { return s.id == id })
	if i < 0 {
		return
	}
	s := stopsOf(FlowNew, o.Reduce)[i]
	s.step = 0 // a detour isn't one of the seven
	v.detour = &s
	v.goTo(o)
}

package ui

import (
	"strconv"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// goaNeedsYou counts tasks whose session waits on the user.
func (a *App) goaNeedsYou() int {
	n := 0
	for _, t := range a.goa.board.Tasks {
		if t.Urgent != "" && t.Status == proto.GOATaskWaiting {
			n++
		}
	}
	return n
}

// goaMain is GOA mode's main view, as a session's: its header with what it
// runs on and the way out, its tab strip, then what is in front of it.
func (a *App) goaMain(c *ui.Context) {
	s, ok := a.goaSession()
	if !ok {
		widget.PageHeader(c, "GOD of Agents", a.host.Name, nil)
		widget.EmptyState(c, icon.Sparkle, "GOD of Agents is starting", "Its session shows here once the host lists it.", nil)
		return
	}
	a.selected, a.maximized = s.ID, ""
	if a.goa.chatSet != s.ID {
		// GOA is talked to: its chat first, where its agent has one.
		a.goa.chatSet = s.ID
		if canChat(s) && a.currentView(s) == viewTerminal {
			a.showView(s, viewChat)
		}
	}
	front := a.frontTab(s)
	a.goaHeader(c, s, front)
	a.tabStrip(c, s)
	if tab := a.centerFront(s); tab != "" {
		a.centerBody(c, s, tab)
		return
	}
	a.sessionBanner(c, front)
	a.sessionBody(c.Key("front:"+front.ID), front)
}

// goaHeader is GOA's top: its name, what it runs on, its views and the mode's
// own actions.
func (a *App) goaHeader(c *ui.Context, s, front proto.SessionInfo) {
	a.viewHeader(c, func() {
		ui.Row(c.Key("title")).Grow(1).MinWidth(0).Gap(theme.S3).Children(func() {
			ui.Row(c).MinWidth(0).Shrink(1).Children(func() {
				widget.Running(c, "GOD of Agents", widget.RunStyle{Size: theme.TextTitle, Weight: 600, Always: true})
			})
			ui.Row(c).MinWidth(0).Shrink(100).Children(func() {
				widget.Running(c.Key("facts"), a.agentName(s.Agent)+" · "+a.host.Name, widget.RunStyle{Color: theme.TextSoft})
			})
		})
		if n := a.goaNeedsYou(); n > 0 {
			widget.Tag(c, strconv.Itoa(n)+" "+textfmt.Plural(n, "task needs you", "tasks need you"), theme.Warning)
		}
		a.viewToggle(c, front)
		if widget.Button(c, widget.Secondary, nil, "Rules", "").Tooltip("What GOD of Agents follows, written by you or with AI").Clicked() {
			a.openGOARules()
		}
		if widget.Button(c, widget.Secondary, nil, "Leave", "").Tooltip("Back to the projects and their sessions; GOD of Agents keeps running").Clicked() {
			a.confirmLeaveGOA(nil)
		}
		more := widget.IconButton(c, icon.More, "More for GOD of Agents", false)
		a.menu.OpenBelow(more, "goa-more", theme.ControlHeight)
		a.menu.Show(c, more, "goa-more", func(done func()) {
			if widget.MenuItem(c, nil, "Rules…", false) {
				a.openGOARules()
				done()
			}
			if widget.MenuItem(c, nil, "Stall limits…", false) {
				a.openGOAStall()
				done()
			}
			widget.Divider(c)
			if widget.MenuItem(c, nil, "Leave GOD of Agents…", false) {
				a.confirmLeaveGOA(nil)
				done()
			}
		})
	})
}

// goaStartForm is the dialog starting GOA: which agent, each with how far
// it holds GOA to delegating.
type goaStartForm struct {
	open     bool
	agent    string
	starting bool
}

func (a *App) openGOAStart() {
	a.goa.start = &goaStartForm{open: true, agent: "claude"}
	cl := a.client
	if cl == nil {
		return
	}
	go func() {
		var list []proto.GOAAgent
		err := cl.Call(proto.MethodGOAAgents, nil, &list)
		a.onMain(func() {
			if err != nil {
				a.goa.err = "This host's enowX is too old for GOD of Agents: update it."
				return
			}
			a.goa.err, a.goa.agents = "", list
		})
	}()
}

// goaLevelColor is how a level reads: enforced calm, watched a warning.
func goaLevelColor(level string) ui.Color {
	switch level {
	case proto.GOAEnforced:
		return theme.Done
	case proto.GOALimited:
		return theme.Accent
	}
	return theme.Warning
}

func (a *App) goaStartDialog(c *ui.Context) {
	f := a.goa.start
	if f == nil {
		return
	}
	if !f.open {
		a.goa.start = nil
		return
	}
	start := false
	widget.FormSheet(c, &f.open, 480, "Start GOD of Agents", "One GOA per host at a time.", func() {
		if a.goa.err != "" {
			ui.Text(c, a.goa.err).FontSize(theme.TextUI).TextColor(theme.Error)
			return
		}
		if a.goa.agents == nil {
			ui.Text(c, "Reading the agents…").FontSize(theme.TextUI).TextColor(theme.TextSoft)
			return
		}
		ui.Column(c).FillWidth().Gap(theme.S2).Children(func() {
			for _, ag := range a.goa.agents {
				if !ag.Installed {
					continue
				}
				if goaAgentRow(c.Key("goa-agent:"+ag.Agent), ag, f.agent == ag.Agent) {
					f.agent = ag.Agent
				}
			}
		})
	}, func() {
		label := "Start"
		if f.starting {
			label = "Starting…"
		}
		if widget.PrimaryButton(c, nil, label, "").Label("Start GOA").Clicked() && !f.starting {
			start = true
		}
	})
	if !start {
		return
	}
	f.starting = true
	var info proto.SessionInfo
	a.goaCall(proto.MethodSessionCreate, proto.SessionCreateParams{Agent: f.agent, GOA: true, SkipUnheard: a.skipPermissions, Cols: 120, Rows: 40}, &info, func(err error) {
		f.starting = false
		if err != nil {
			return
		}
		f.open = false
		a.upsert(info)
		a.goa.host = a.host.ID
		a.goa.board.Session, a.goa.board.Agent = info.ID, info.Agent
		for _, ag := range a.goa.agents {
			if ag.Agent == info.Agent {
				a.goa.board.Level = ag.Level
			}
		}
		a.enterGOA()
	})
}

// goaAgentRow is an agent to start GOA on, with how far it holds GOA to
// delegating; it reports a click.
func goaAgentRow(c *ui.Context, ag proto.GOAAgent, on bool) bool {
	row := widget.ListItem(c, ui.Color{}, ag.Name, ag.Why, on, func() {
		widget.Legend(c, goaLevelColor(ag.Level), goaSentence(ag.Level))
	})
	return row.Clicked()
}

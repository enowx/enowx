package ui

import (
	"slices"
	"strconv"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// goaBrowser stands for GOA session's browser in the strip: not a page,
// the session shown on its Browser view.
const goaBrowser = "goa-browser"

// goaPages are what GOA mode's strip reaches, in its order: GOA itself
// (""), then the pages the session list's rail would.
var goaPages = []struct{ page, label string }{
	{"", "GOD of Agents"},
	{goaBrowser, "Browser"},
	{pageNotes, "Notes"},
	{pageCanvas, "Canvas"},
	{pageUsage, "Usage"},
	{pageHosts, "Hosts"},
	{pageSettings, "Settings"},
}

// goaChip is the title bar's way into GOA mode, and out of it.
func (a *App) goaChip(c *ui.Context) {
	if a.client == nil {
		return
	}
	a.goaLoad()
	on := a.goaOn()
	label := "GOD of Agents"
	tip := "One session controlling every session on every host"
	if on {
		label, tip = "Leave GOD of Agents", "Back to the projects and their sessions (GOD of Agents keeps running)"
	}
	b := titleChip(c, "goa", tip)
	if on {
		b.Background(theme.AccentSoft).Border(1, theme.AccentLine)
	}
	b.Children(func() {
		ui.Icon(c, icon.Sparkle).FontSize(12).TextColor(theme.Accent)
		ui.Text(c, label).FontSize(11.5).SingleLine()
	})
	if b.Clicked() {
		a.toggleGOA()
	}
}

// goaNav is GOA mode's strip on top of the main view: GOA, the pages, and
// the way out.
func (a *App) goaNav(c *ui.Context) {
	ui.Row(c.Key("goa-nav")).Height(headerHeight).Shrink(0).FillWidth().PaddingX(14).Gap(10).
		BorderWidth(0, 0, 1, 0).BorderColor(theme.Border).Background(theme.Panel).Children(func() {
		ui.Icon(c, icon.Sparkle).FontSize(14).TextColor(theme.Accent)
		labels := make([]string, len(goaPages))
		for i, p := range goaPages {
			labels[i] = p.label
		}
		shown := a.page
		if s, ok := a.goaSession(); ok && shown == "" && a.state.BrowserViews[s.ID] {
			shown = goaBrowser
		}
		if a.goaShownTab() != nil {
			shown = "a tab" // none of the strip's
		}
		current := slices.IndexFunc(goaPages, func(p struct{ page, label string }) bool { return p.page == shown })
		if i := widget.Segmented(c.Key("goa-pages"), labels, current, nil); i >= 0 {
			a.goaShow(goaPages[i].page)
		}
		ui.Spacer(c)
		if n := a.goaNeedsYou(); n > 0 {
			widget.Tag(c, strconv.Itoa(n)+" "+textfmt.Plural(n, "task needs you", "tasks need you"), theme.Warning)
		}
		if widget.GhostButton(c, "Rules").Tooltip("What GOD of Agents follows, written by you or with AI").Clicked() {
			a.openGOARules()
		}
		if widget.GhostButton(c, "Leave").Tooltip("Back to the projects and their sessions; GOD of Agents keeps running").Clicked() {
			a.confirmLeaveGOA(nil)
		}
	})
}

// goaShow shows what the strip's item page stands for: a page, GOA's
// session on its Browser view, or GOA's session on its chat (or terminal).
func (a *App) goaShow(page string) {
	a.goa.shown = ""
	s, ok := a.goaSession()
	switch {
	case page == goaBrowser && ok:
		a.page = ""
		a.showView(s, viewBrowser)
	case page == "" && ok:
		a.page = ""
		if a.state.BrowserViews[s.ID] {
			view := viewTerminal
			if canChat(s) {
				view = viewChat
			}
			a.showView(s, view)
		}
	default:
		a.page = page
	}
}

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

// goaMain is GOA mode's main view: GOA's session, alone.
func (a *App) goaMain(c *ui.Context) {
	s, ok := a.goaSession()
	if !ok {
		a.viewHeader(c, func() { widget.Breadcrumb(c, a.host.Name, "GOD of Agents") })
		widget.EmptyState(c, icon.Sparkle, "GOD of Agents is starting", "Its session shows here once the host lists it.", nil)
		return
	}
	a.selected, a.maximized = s.ID, ""
	if t := a.goaShownTab(); t != nil {
		a.goaTabView(c, t)
		return
	}
	if a.goa.chatSet != s.ID {
		// GOA is talked to: its chat first, where its agent has one.
		a.goa.chatSet = s.ID
		if canChat(s) && a.currentView(s) == viewTerminal {
			a.showView(s, viewChat)
		}
	}
	a.focusView(c, proto.ProjectInfo{Name: "its own folder", Path: s.Dir}, s)
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
	widget.FormSheet(c, &f.open, 520, "Start GOD of Agents",
		"One session that controls every session on every host: it hands each task to a session, reading files and writing what is small itself. It runs in a folder of its own on "+a.host.Name+". Pick its agent: how far each holds it to delegating is shown.", func() {
			if a.goa.err != "" {
				ui.Text(c, a.goa.err).FontSize(12).TextColor(theme.Error)
				return
			}
			if a.goa.agents == nil {
				ui.Text(c, "Reading the agents…").FontSize(12).TextColor(theme.Muted)
				return
			}
			ui.Column(c).FillWidth().Gap(4).Children(func() {
				for _, ag := range a.goa.agents {
					if !ag.Installed {
						continue
					}
					row := ui.ButtonBase(c.Key("goa-agent:"+ag.Agent)).FillWidth().Padding(8, 10).Gap(10).Radius(theme.RadiusControl).
						Cursor(ui.CursorPointer).Label(ag.Name)
					if f.agent == ag.Agent {
						row.Background(theme.AccentSoft).Border(1, theme.AccentLine)
					} else if row.Hovered() {
						row.Background(theme.Hover)
					}
					row.Children(func() {
						ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
							ui.Text(c, ag.Name).FontSize(12.5).SingleLine()
							ui.Text(c, ag.Why).FontSize(11.5).TextColor(theme.Muted)
						})
						widget.Tag(c, ag.Level, goaLevelColor(ag.Level))
					})
					if row.Clicked() {
						f.agent = ag.Agent
					}
				}
			})
		}, func() {
			label := "Start GOA"
			if f.starting {
				label = "Starting…"
			}
			if widget.PrimaryButton(c, icon.Sparkle, label, "").Clicked() && !f.starting {
				start = true
			}
		})
	if !start {
		return
	}
	f.starting = true
	var info proto.SessionInfo
	a.goaCall(proto.MethodSessionCreate, proto.SessionCreateParams{Agent: f.agent, GOA: true, Cols: 120, Rows: 40}, &info, func(err error) {
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

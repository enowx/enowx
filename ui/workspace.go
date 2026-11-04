package ui

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/chatview"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/tokenview"
	"github.com/enowx/enowx/ui/widget"
)

// headerHeight is the height of every view's top bar, so the breadcrumbs
// of the rail's views line up.
const headerHeight = 56

// minWorkspaceWidth keeps the session readable: the side panel narrows,
// down to its own minimum, before the workspace does.
const minWorkspaceWidth = 400

// With the panel beside them, the Notes and Canvas pages narrow first, down to
// these: what is left of the smallest window beside the list and the panels,
// and on Notes room for its agent, which keeps its width inside the frame.
const (
	minNotesWidth  = 480
	minCanvasWidth = 270
)

// The window's smallest size: the wide rail, the session list, the
// workspace and the side panel at their narrowest, with the gaps between.
const (
	minWindowWidth  = 1200
	minWindowHeight = 720
)

// The rail's views, shown instead of the sessions.
const (
	pageUsage      = "usage"
	pageSync       = "sync"
	pageHosts      = "hosts"
	pageAppearance = "appearance"
	pageCredits    = "credits"
)

// workspace is the main area: one focused session, or the project's
// sessions side by side in split mode, or a rail view.
func (a *App) workspace(c *ui.Context) {
	if a.page != pageHosts {
		a.hostsPageShown = false
	}
	if a.page != pageTeam {
		a.leftTeamPage()
	}
	if a.page != pageTeamHost && a.visit != nil {
		a.closeVisit() // left the shared host
	}
	// A new view (a rail page, another session) rises in as it replaces
	// the last; the panel itself moves with the rail.
	view := "page:" + a.page
	if a.page == "" {
		view = "session:" + a.selected
		if a.splitOn() {
			view = "split:" + a.project
		}
	}
	frame := ui.Column(c.Key("workspace")).Grow(1).FillHeight().MinWidth(minWorkspaceWidth)
	if a.pageHasDock() && a.panelShown() {
		frame.Basis(0).MinWidth(minCanvasWidth)
		if a.page == pageNotes {
			frame.MinWidth(minNotesWidth)
		}
	}
	widget.SheetPage = a.pageIsSheet()
	widget.SheetFrame(frame).Transition(widget.LayoutMotion).Children(func() {
		widget.PageGap(ui.Column(c.Key(view)).Grow(1).FillWidth().MinHeight(0).Transition(widget.PageMotion)).Children(func() { a.workspaceView(c) })
	})
}

// pageIsSheet says the page shown is a header over sections, which in a style of
// tiles are tiles themselves. Notes, Canvas and sessions keep their one frame.
func (a *App) pageIsSheet() bool {
	switch a.page {
	case pageSettings, pageAppearance, pageUsage, pageHosts, pageSync, pageCredits:
		return true
	}
	return false
}

// workspaceView fills the workspace with what it shows now.
func (a *App) workspaceView(c *ui.Context) {
	switch {
	case a.page == pageAppearance:
		a.appearancePage(c)
		return
	case a.page == pageVault:
		a.vaultPage(c)
		return
	case a.page == pageHosts:
		a.hostsPage(c)
		return
	case a.page == pageSync && a.sync != nil:
		a.syncPage(c)
		return
	case a.page == pageUsage && a.client != nil:
		a.usagePage(c)
		return
	case a.page == pageNotes && a.client != nil:
		a.notesPage(c)
		return
	case a.page == pageCanvas && a.client != nil:
		a.canvasPage(c)
		return
	case a.page == pageTeam && a.client != nil:
		a.teamPage(c)
		return
	case a.page == pageTeamHost && a.visit != nil:
		a.teamHostPage(c)
		return
	case a.page == pageCredits && a.client != nil:
		a.creditsPage(c)
		return
	case a.page == pageSettings && a.client != nil:
		a.settingsPage(c)
		return
	case a.page == pageSetup && a.setup != nil && a.client != nil:
		a.setupPage(c)
		return
	}
	// The connection dropping isn't a reason to leave: the page is back with it.
	if a.client != nil || a.retryHost != a.host.ID {
		a.page = ""
	}
	if a.connectionView(c) {
		return
	}
	if a.goaOn() {
		a.goaMain(c)
		return
	}
	p, ok := a.currentProject()
	if !ok {
		widget.PageHeader(c, "Start", a.host.Name, nil)
		a.emptyState(c, "Open a folder to start", "Each folder is a project. Sessions run in it or in their own git worktrees.", "Open folder", a.openFolder)
		return
	}
	sessions := a.projectSessions(p.Path)
	if a.selected != "" {
		a.emptyProject = ""
	}
	if len(sessions) > 0 && a.selected == "" && a.emptyProject == p.Path {
		// A session an agent started here doesn't take the user there.
		widget.PageHeader(c, p.Name, a.host.Name, nil)
		a.emptyState(c, "Pick a session of "+p.Name, "It has sessions now: they're in the list.", "New session", func() { a.openNewSession(p) })
		return
	}
	if len(sessions) == 0 {
		a.emptyProject = p.Path
		widget.PageHeader(c, p.Name, a.host.Name, nil)
		a.emptyState(c, "No sessions in "+p.Name, "Start an agent or a shell. On a git project it can get its own branch and worktree.", "New session", func() { a.openNewSession(p) })
		return
	}
	if a.splitOn() && a.maximized == "" && len(sessions) > 1 {
		a.splitView(c, p, sessions)
		return
	}
	a.focusView(c, p, a.focused(sessions))
}

// focused is the session the focus view shows: the maximized one, else
// the selected one, else the project's first.
func (a *App) focused(sessions []proto.SessionInfo) proto.SessionInfo {
	for _, id := range []string{a.maximized, a.selected} {
		for _, s := range sessions {
			if id != "" && s.ID == id {
				return s
			}
		}
	}
	return sessions[0]
}

// viewHeader is the top bar of a view; fn fills it, breadcrumb first.
func (a *App) viewHeader(c *ui.Context, fn func()) {
	ui.Row(c).Height(headerHeight).Shrink(0).FillWidth().Padding(0, theme.S4, 0, theme.S5).Gap(theme.S3).
		BorderWidth(0, 0, theme.Rule, 0).BorderColor(theme.Border).Children(fn)
}

// sessionsHidden reports whether the sidebar gives way: to a maximized
// session, or to the room the user asked for it to leave.
func (a *App) sessionsHidden() bool {
	return a.maximized != "" || a.state.HideSessions
}

// toggleSessionsPanel folds the sidebar away, or brings it back.
func (a *App) toggleSessionsPanel() { a.state.HideSessions = !a.state.HideSessions }

func (a *App) emptyState(c *ui.Context, title, body, action string, do func()) {
	widget.EmptyState(c, icon.FolderPlus, title, body, func() {
		if widget.PrimaryButton(c, icon.Plus, action, "").Clicked() {
			do()
		}
	})
}

// focusView shows one session: its title and what it is, its terminal's tabs
// while the terminal shows, what stands out about it, then its terminal or
// chat.
func (a *App) focusView(c *ui.Context, p proto.ProjectInfo, s proto.SessionInfo) {
	if a.selected == "" {
		a.selected = s.ID
	}
	front := a.frontTab(s)
	a.viewHeader(c, func() {
		ui.Row(c.Key("title")).Grow(1).MinWidth(0).Gap(theme.S3).Children(func() {
			// The frame keeps its width: the title runs when it can't fit, and
			// the facts give way first.
			ui.Row(c).MinWidth(0).Shrink(1).Children(func() {
				widget.Running(c, a.sessionName(s), widget.RunStyle{Size: theme.TextTitle, Weight: 600, Always: true})
			})
			ui.Row(c).MinWidth(0).Shrink(100).Children(func() {
				t := widget.Running(c.Key("facts"), a.sessionFactsLine(front), widget.RunStyle{Color: theme.TextSoft})
				t.Tooltip(a.sessionFactsTip(front))
			})
		})
		a.sessionPresence(c, s)
		a.viewToggle(c, front)
		more := widget.IconButton(c, icon.More, "More", false)
		a.menu.OpenBelow(more, "more:"+s.ID, theme.ControlHeight)
		a.menu.Show(c, more, "more:"+s.ID, func(done func()) { a.sessionMenuItems(c, s, done) })
	})
	a.tabStrip(c, s)
	if tab := a.centerFront(s); tab != "" {
		a.centerBody(c, s, tab)
		return
	}
	a.sessionBanner(c, front)
	a.sessionBody(c.Key("front:"+front.ID), front)
}

// sessionFactsLine is what a session's header says after its title: its agent,
// branch and tokens.
func (a *App) sessionFactsLine(s proto.SessionInfo) string {
	parts := []string{a.agentName(s.Agent)}
	if s.Branch != "" {
		parts = append(parts, s.Branch)
	}
	if t, ok := a.sessionTokens(s.ID); ok {
		parts = append(parts, tokenview.Spent(t))
	}
	return strings.Join(parts, " · ")
}

// sessionFactsTip is the rest, on hover: state, where it runs, its age, and
// what it uses.
func (a *App) sessionFactsTip(s proto.SessionInfo) string {
	parts := []string{statusOf(s).label}
	switch {
	case s.Worktree && s.Branch != "":
		parts = append(parts, s.Branch+" · worktree")
	case s.Branch != "":
		parts = append(parts, s.Branch+" · project folder")
	}
	if !s.Created.IsZero() {
		parts = append(parts, textfmt.Age(s.Created, time.Now()))
	}
	if t, ok := a.sessionTokens(s.ID); ok {
		parts = append(parts, tokenview.SpentTip(t))
	}
	if u, ok := a.usage.Session(s.ID); ok {
		parts = append(parts, fmt.Sprintf("%s · %s · %d %s", textfmt.Bytes(u.RSS), textfmt.CPU(u.CPU), u.Procs, textfmt.Plural(u.Procs, "process", "processes")))
	}
	return strings.Join(parts, "\n")
}

// sessionBanner says what stands out about a session: it skips permission
// prompts, its time budget, or why it ended, with the way to go on.
func (a *App) sessionBanner(c *ui.Context, s proto.SessionInfo) {
	text, col := "", theme.Warning
	if s.SkipPermissions {
		text = "Skips permission prompts"
	}
	if t, tc := budgetFact(s); t != "" {
		text, col = t, tc
	}
	can := canContinue(s)
	if s.ExitWhy != "" {
		text, col = s.ExitWhy, theme.Error
	} else if can && text == "" {
		text, col = statusOf(s).label, statusOf(s).color
	}
	if text == "" || a.state.BannersClosed[s.ID] == text {
		return
	}
	ui.Row(c.Key("banner")).Shrink(0).FillWidth().Padding(theme.S3, theme.S5, 0, theme.S5).Children(func() {
		ui.Row(c).Grow(1).MinWidth(0).Height(40).Padding(0, theme.S1, 0, theme.S4).Gap(theme.S2).Radius(theme.RadiusFull).
			Background(theme.Fade(col, 0.14)).Children(func() {
			ui.Row(c).Grow(1).MinWidth(0).Children(func() { widget.Running(c, text, widget.RunStyle{}) })
			if can && widget.GhostButton(c, "Continue").Clicked() {
				a.continueSession(s)
			}
			// Gone until it says something else: a banner is news, not a fixture.
			if widget.IconButton(c.Key("close"), icon.Close, "Close this notice", false).Clicked() {
				if a.state.BannersClosed == nil {
					a.state.BannersClosed = map[string]string{}
				}
				a.state.BannersClosed[s.ID] = text
				a.saveState()
			}
		})
	})
}

// viewToggle switches a session between its terminal and its chat, for
// agents whose conversation enowX can read.
func (a *App) viewToggle(c *ui.Context, s proto.SessionInfo) {
	views := a.sessionViews(s)
	if len(views) < 2 {
		return
	}
	// With a viewer in front, neither is lit: picking one comes back to it.
	current := slices.Index(views, a.currentView(s))
	if a.centerFront(s) != "" {
		current = -1
	}
	if i := widget.Segmented(c.Key("view"), views, current, nil); i >= 0 {
		a.centerOf(a.parentOf(s).ID).front = ""
		a.showView(s, views[i])
	}
}

func canChat(s proto.SessionInfo) bool { return chatview.Supported(s) }

// sessionBody is a session's chat or terminal, filling what is left.
func (a *App) sessionBody(c *ui.Context, s proto.SessionInfo) {
	switch p := a.wantPane(s.ID); {
	case a.state.BrowserViews[s.ID]:
		a.browserTab(c.Key("browser-view"), s)
	case a.state.TrafficViews[s.ID]:
		a.trafficTab(c.Key("traffic-view"), s)
	case a.showsChat(s.ID):
		a.chatView(c.Key("chat"), s)
	case p != nil:
		v := terminal.View(c.Key("term"), p.term).Grow(1).FillWidth().Background(theme.TermGround).Padding(theme.S2, theme.S4)
		if text, ok := widget.TakeTextDrag(v); ok {
			p.term.Paste(text + " ")
			v.Focus()
		}
		if top := a.parentOf(s); v.Focused() && a.selected != top.ID {
			a.selected = top.ID
		}
	default:
		ui.Column(c).Grow(1).FillWidth().Center().Children(func() {
			ui.Text(c, "Connecting…").TextColor(theme.TextSoft)
		})
	}
}

// splitView lays the project's sessions out side by side, in the columns
// its layout says (as square a grid as fits them by default), leaving out
// those the user took out of it.
func (a *App) splitView(c *ui.Context, p proto.ProjectInfo, all []proto.SessionInfo) {
	sessions := a.splitSessions(all)
	l := a.layout()
	a.viewHeader(c, func() {
		ui.Row(c).Grow(1).MinWidth(0).Children(func() {
			widget.Running(c, p.Name, widget.RunStyle{Size: theme.TextTitle, Weight: 600, Always: true})
			ui.Box(c).Width(theme.S3).Shrink(0)
			widget.Running(c.Key("facts"), fmt.Sprintf("Side by side · %d sessions", len(sessions)), widget.RunStyle{Color: theme.TextSoft})
		})
		if left := len(all) - len(sessions); left > 0 &&
			widget.Button(c, widget.Secondary, nil, fmt.Sprintf("Show %d more", left), "").Clicked() {
			a.setLayout(func(l *layout) { l.Hidden = nil })
		}
		labels := make([]string, len(splitColumns))
		for i, n := range splitColumns {
			labels[i] = strconv.Itoa(n)
		}
		labels[0] = "Auto"
		if i := widget.Segmented(c.Key("columns"), labels, slices.Index(splitColumns, l.Columns), nil); i >= 0 {
			a.setLayout(func(l *layout) { l.Columns = splitColumns[i] })
		}
		if widget.IconButton(c, icon.Split, "Show one session", true).Clicked() {
			a.setSplit(false)
		}
	})
	cols := l.Columns
	if cols <= 0 {
		cols = int(math.Ceil(math.Sqrt(float64(len(sessions)))))
	}
	cols = min(cols, len(sessions))
	rows := (len(sessions) + cols - 1) / cols
	tracks := make([]ui.Track, rows)
	for i := range tracks {
		tracks[i] = ui.Fr(1)
	}
	ui.Grid(c).Grow(1).FillWidth().Columns(cols).RowTracks(tracks...).Gap(theme.S2).Padding(theme.S2).Children(func() {
		for _, s := range sessions {
			a.splitPane(c.Key(s.ID), s)
		}
	})
}

// splitPane is one session of the split view: a slim header over its
// terminal or chat. The selected one is marked by an accent line.
func (a *App) splitPane(c *ui.Context, s proto.SessionInfo) {
	st := statusOf(s)
	active := s.ID == a.selected
	pane := ui.Column(c).MinHeight(0).MinWidth(0).Radius(theme.RadiusCard).Clip().Background(theme.Sunken)
	if active {
		pane.Border(2, theme.Now)
	}
	pane.Children(func() {
		head := ui.Row(c).Height(44).Shrink(0).FillWidth().Padding(0, theme.S2, 0, theme.S4).Gap(theme.S2)
		head.Children(func() {
			title := ui.ButtonBase(c).Grow(1).Gap(theme.S2).MinWidth(0).Justify(ui.Start).Cursor(ui.CursorPointer).Label(a.sessionName(s))
			title.Children(func() {
				widget.Dot(c, st.color, st.hollow, 8)
				widget.Running(c, a.sessionName(s), widget.RunStyle{Weight: 500})
				widget.Running(c.Key("agent"), a.agentName(s.Agent), widget.RunStyle{Color: theme.TextSoft})
			})
			if title.Clicked() {
				a.selectSession(s)
			}
			a.viewToggle(c, s)
			if widget.IconButton(c, icon.Maximize, "Focus this session", false).Clicked() {
				a.selectSession(s)
				a.setSplit(false)
			}
			more := widget.IconButton(c, icon.More, "More", false)
			a.menu.OpenBelow(more, "more:"+s.ID, theme.ControlHeight)
			a.menu.Show(c, more, "more:"+s.ID, func(done func()) { a.sessionMenuItems(c, s, done) })
		})
		a.sessionBody(c, s)
	})
}

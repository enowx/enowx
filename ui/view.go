package ui

import (
	"runtime"
	"slices"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// modKey is how shortcut hints write ui.Cmd.
var modKey = func() string {
	if runtime.GOOS == "darwin" {
		return "⌘"
	}
	return "Ctrl+"
}()

func (a *App) view(c *ui.Context) {
	c.SetTheme(a.theme)
	a.winW, _ = c.Size()
	a.shortcuts(c)
	a.watchUsageShown()
	a.discordPresence()
	a.keepHere(c)
	if hc := a.teamChats; hc != nil {
		hc.wasShown, hc.shown = hc.shown, false // chatTab tells it again
	}
	root := ui.Box(c).Fill().Background(theme.Ground).TextColor(theme.Text)
	root.Children(func() {
		ui.Column(c).Absolute().Left(0).Top(0).Right(0).Bottom(0).Padding(theme.WindowInset).Gap(theme.BentoGap).Children(func() {
			a.titleBar(c)
			ui.Row(c).Grow(1).FillWidth().MinHeight(0).AlignItems(ui.Stretch).Gap(theme.BentoGap).Children(func() {
				switch {
				case !a.sessionListShows():
					// Pages, and GOD of Agents: no session list.
					a.pageSide(c)
				case a.navShown():
					a.navSidebar(c)
				default:
					a.navSidebarNarrow(c)
				}
				a.workspace(c)
				a.sidePanelView(c)
				a.toolsDock(c)
				a.canvasAgentPanel(c)
			})
		})
		a.statusToast(c)
	})
	a.cornerCards(c)
	a.chatToastsView(c)
	a.asksView(c)
	a.commandPalette(c)
	a.updateDialog(c)
	a.budgetDialog(c)
	a.commandDialog(c)
	a.copyDialog(c)
	a.cloneToDialog(c)
	a.projectSyncDialog(c)
	a.linksDialog(c)
	a.pipelineDialog(c)
	a.createServerDialog(c)
	a.quickOpenDialog(c)
	a.historyDialogView(c)
	a.compareDialogView(c)
	a.localPick.View(c)
	a.deleteServerDialog(c)
	a.clearHostDialog(c)
	a.agentLimitDialog(c)
	a.newSessionDialog(c)
	a.addProjectDialog(c)
	a.hostFileDialog(c)
	a.addHostDialog(c)
	a.trustDialog(c)
	a.runAsDialog(c)
	a.passphraseDialog(c)
	a.signInDialog(c)
	a.cloudSignInDialog(c)
	a.spaceMoveDialog(c)
	a.rulesDialog(c)
	a.confirmDialog(c)
	a.renameDialog(c)
	a.goaStartDialog(c)
	a.goaDetailDialog(c)
	a.goaOverrideDialog(c)
	a.goaRulesDialog(c)
	a.goaStallDialog(c)
	a.unslopCompareDialog(c)
	a.unslopChoiceDialog(c)
	a.replayDialog(c)
	a.deleteDialog(c)
	a.hibernateDialog(c)
	a.removeProjectDialog(c)
	a.groupDialog(c)
	a.cloneDialog(c)
	a.noteImportDialog(c)
	a.noteExportDialog(c)
	a.saveState()
}

func (a *App) shortcuts(c *ui.Context) {
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyH) && a.page == "" {
		if s, ok := a.sessionByID(a.selected); ok && canHibernate(s) {
			a.hibernate(a.hibernateTargets(s))
		}
	}
	if c.Shortcut(ui.Cmd, ui.KeyN) {
		if p, ok := a.currentProject(); ok {
			a.openNewSession(p)
		}
	}
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyEnter) {
		a.toggleMaximized(a.selected)
	}
	if c.Shortcut(ui.Cmd, ui.KeyO) {
		a.openFolder()
	}
	if c.Shortcut(ui.Cmd, ui.KeyK) {
		switch {
		case a.page == pageNotes && !a.palette.open && a.notes.OpenJump():
			// On a page of notes, it jumps to a section of it.
		case a.palette.open:
			a.palette.open = false
		default:
			a.openPalette()
		}
	}
	if c.Shortcut(ui.Cmd, ui.KeyP) {
		if a.quick.open {
			a.quick.open = false
		} else {
			a.openQuickOpen()
		}
	}
	if c.Shortcut(ui.Cmd, ui.KeyB) {
		a.state.RailWide = !a.state.RailWide
	}
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyBracketRight) {
		a.stepSession(1)
	}
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyBracketLeft) {
		a.stepSession(-1)
	}
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyL) {
		a.setSplit(!a.splitOn())
	}
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyD) {
		a.togglePanel()
	}
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyP) {
		a.toggleTool(panelBrowser)
	}
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyS) {
		a.toggleSessionsPanel()
	}
}

// selectedSession is the session the workspace shows, if any.
func (a *App) selectedSession() (proto.SessionInfo, bool) {
	for _, s := range a.sessions {
		if s.ID == a.selected {
			return s, true
		}
	}
	return proto.SessionInfo{}, false
}

// stepSession selects the project's next session (by is 1) or previous
// one (-1), wrapping around.
func (a *App) stepSession(by int) {
	sessions := a.projectSessions(a.project)
	if len(sessions) == 0 {
		return
	}
	i := slices.IndexFunc(sessions, func(s proto.SessionInfo) bool { return s.ID == a.selected })
	if i < 0 && by < 0 {
		i = 0
	}
	a.selectSession(sessions[(i+by+len(sessions))%len(sessions)])
}

func (a *App) toggleMaximized(id string) {
	switch {
	case a.maximized != "":
		a.maximized = ""
	case id != "":
		a.maximized = id
		a.selected = id
	}
}

// statusToast says what the status holds, over the window's foot: an error
// until dismissed, news for a few seconds.
func (a *App) statusToast(c *ui.Context) {
	if a.status == "" {
		a.statusSeen = ""
		return
	}
	news := a.status == a.notice
	if a.status != a.statusSeen {
		a.statusSeen, a.statusAt = a.status, c.Now()
	}
	if news {
		const lasts = 6 * time.Second
		left := lasts - c.Now().Sub(a.statusAt)
		if left <= 0 {
			a.status = ""
			return
		}
		c.After(left)
	}
	ui.Row(c.Key("status-toast")).Absolute().Left(0).Right(0).Bottom(theme.S5).Justify(ui.Center).PassThrough().Children(func() {
		col := theme.Del
		if news {
			col = theme.Text
		}
		// As wide as it says, up to its limit; longer, it runs inside that.
		natural, _ := c.MeasureText(0, ui.Span{Text: a.status, Size: theme.TextUI})
		toast := widget.Float(ui.Row(c).Width(min(natural+theme.S4+theme.S1+theme.S2+theme.ControlHeight+2, 640)).Shrink(1).MinWidth(0).Height(40).
			Padding(0, theme.S1, 0, theme.S4).Gap(theme.S2).Radius(theme.RadiusFull).Background(theme.Popover)).Transition(widget.OverlayMotion)
		toast.Children(func() {
			ui.Row(c).Grow(1).MinWidth(0).Children(func() { widget.Running(c, a.status, widget.RunStyle{Color: col, Always: true}) })
			if widget.IconButton(c, icon.Close, "Dismiss", false).Clicked() {
				a.status = ""
			}
		})
	})
}

// dot draws a session status mark: filled, or a ring when hollow.
// notify shows news that isn't a problem in the status bar.
func (a *App) notify(msg string) { a.status, a.notice = msg, msg }

// deleteDialog confirms deleting a session.
func (a *App) deleteDialog(c *ui.Context) {
	if a.deleting == nil {
		return
	}
	s := *a.deleting
	open := true
	msg := "Its worktree and branch stay on disk."
	if !s.Exited {
		msg = "The agent stops. " + msg
	}
	switch widget.AlertDialog(c, &open, "Delete “"+a.sessionName(s)+"”?", msg, "Cancel", "Delete") {
	case 1:
		a.killSession(s.ID)
		a.deleting = nil
	case 0, ui.AlertDismissed:
		a.deleting = nil
	}
}

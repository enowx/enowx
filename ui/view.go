package ui

import (
	"fmt"
	"runtime"
	"slices"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/textfmt"
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
	ui.Column(c).Fill().Background(theme.Ground).TextColor(theme.Text).Children(func() {
		a.titleBar(c)
		ui.Row(c).Grow(1).FillWidth().MinHeight(0).AlignItems(ui.Stretch).Padding(0, theme.BentoGap).Gap(theme.BentoGap).Children(func() {
			switch {
			case a.navShown():
				a.navSidebar(c)
			case !a.sessionsHidden():
				a.navSidebarNarrow(c)
			}
			a.workspace(c)
			a.sidePanelView(c)
			a.notesAgentPanel(c)
			a.canvasAgentPanel(c)
		})
		a.statusBar(c)
	})
	a.cornerCards(c)
	a.chatToastsView(c)
	a.vaultAskDialog(c)
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
	a.featureAskDialog(c)
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
	a.takeoverDialog(c)
	a.confirmDialog(c)
	a.renameDialog(c)
	a.accountAskDialog(c)
	a.ragOfferDialog(c)
	a.decisionDialog(c)
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

func (a *App) statusBar(c *ui.Context) {
	working, waiting, idle, queued := 0, 0, 0, 0
	for _, s := range a.sessions {
		switch {
		case s.Exited:
		case s.Queued:
			queued++
		case s.State == proto.StateWaiting:
			waiting++
		case isWorking(s):
			working++
		default:
			idle++
		}
	}
	ui.Row(c).Height(28).Shrink(0).FillWidth().PaddingX(theme.BentoGap + 8).Gap(16).Children(func() {
		ui.Row(c).Gap(6).Shrink(0).Children(func() {
			col, state := a.connectionState()
			widget.Dot(c, col, false, 6)
			ui.Text(c, a.host.Name+" · "+state).FontSize(11).TextColor(theme.Muted).SingleLine()
		})
		counts := fmt.Sprintf("%d working · %d idle", working, idle)
		if waiting > 0 {
			counts = fmt.Sprintf("%d need input · ", waiting) + counts
		}
		if queued > 0 {
			counts += fmt.Sprintf(" · %d queued", queued)
		}
		ui.Text(c, counts).FontSize(11).TextColor(theme.Muted).SingleLine()
		if a.status != "" {
			col := theme.Error
			if a.status == a.notice {
				col = theme.TextSoft
			}
			ui.Text(c, a.status).FontSize(11).TextColor(col).SingleLine().Grow(1).Shrink(1)
		} else {
			ui.Spacer(c)
		}
		if h, ok := a.usage.Host(); ok && h.MemTotal > 0 {
			col := theme.Muted
			if float64(h.MemUsed) > 0.85*float64(h.MemTotal) {
				col = theme.Warning
			}
			ui.Text(c, "Host "+textfmt.Bytes(h.MemUsed)+" / "+textfmt.Bytes(h.MemTotal)).FontSize(11).TextColor(col).SingleLine()
		}
		if a.client != nil {
			if s := a.credits.Summary(a.creditsHost()); s != "" {
				ui.Text(c, s).FontSize(11).TextColor(theme.Muted).SingleLine().Shrink(1)
			}
		}
		ui.Text(c, modKey+"K commands · "+modKey+"N new session").FontSize(11).TextColor(theme.Muted).SingleLine()
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
	switch ui.AlertDialog(c, &open, "Delete “"+a.sessionName(s)+"”?", msg, "Cancel", "Delete") {
	case 1:
		a.killSession(s.ID)
		a.deleting = nil
	case 0, ui.AlertDismissed:
		a.deleting = nil
	}
}

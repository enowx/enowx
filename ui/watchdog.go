package ui

import (
	"fmt"
	"path/filepath"

	"github.com/egoist/mygo"

	"github.com/enowx/enowx/internal/watchdog"
	"github.com/enowx/enowx/transport"
)

// startWatchdog has the main thread watched (package watchdog), its reports in the
// data folder's watchdog folder.
func (a *App) startWatchdog() {
	dir, err := transport.Dir()
	if err != nil {
		return
	}
	a.watchdog = watchdog.Start(filepath.Join(dir, "watchdog"), "app", mygo.RunOnMain, a.watchdogScreen)
	a.watchdog.SetDoing(a.diag.Doing)
}

// watchdogScreen says what the app shows, for a report: never what's in it.
func (a *App) watchdogScreen() string {
	page := a.page
	if page == "" {
		page = "sessions"
	}
	chat := 0
	if ch := a.chats[a.selected]; ch != nil {
		chat = ch.Entries()
	}
	return fmt.Sprintf("page=%s host=%s session=%s agent=%s style=%s sessions=%d panes=%d chats=%d chatEntries=%d window=%.0f",
		page, a.host.ID, a.selected, a.selectedAgent(), a.state.UIStyle, len(a.sessions), len(a.panes), len(a.chats), chat, a.winW)
}

// selectedAgent is the agent of the session selected.
func (a *App) selectedAgent() string {
	if s, ok := a.selectedSession(); ok {
		return s.Agent
	}
	return ""
}

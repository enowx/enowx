package ui

import (
	"slices"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/diffview"
)

// The Changes tab of the side panel (see package diffview): what the
// selected session's branch changed, and committing, pushing, merging or
// discarding it.

// canReview reports whether session s runs in a git repository.
func (a *App) canReview(s proto.SessionInfo) bool {
	if s.Branch != "" || s.Worktree {
		return true
	}
	i := slices.IndexFunc(a.projects, func(p proto.ProjectInfo) bool { return p.Path == s.Project })
	return i >= 0 && a.projects[i].IsGit
}

// Session views, as the header's toggle names them.
const (
	viewTerminal = "Terminal"
	viewChat     = "Chat"
	viewBrowser  = "Browser"
	viewTraffic  = "Traffic"
)

// sessionViews are the views session s can show, its chat first; Browser and
// Traffic are tools that open as tabs.
func (a *App) sessionViews(s proto.SessionInfo) []string {
	if canChat(s) {
		return []string{viewChat, viewTerminal}
	}
	return []string{viewTerminal}
}

// currentView is what session s shows now.
func (a *App) currentView(s proto.SessionInfo) string {
	switch {
	case a.state.BrowserViews[s.ID]:
		return viewBrowser
	case a.state.TrafficViews[s.ID]:
		return viewTraffic
	case a.showsChat(s.ID):
		return viewChat
	}
	return viewTerminal
}

// showView shows session s's terminal or chat.
func (a *App) showView(s proto.SessionInfo, view string) {
	if a.showsChat(s.ID) != (view == viewChat) {
		a.toggleChat(s)
	}
	if a.state.BrowserViews == nil {
		a.state.BrowserViews = map[string]bool{}
	}
	if view == viewBrowser {
		a.state.BrowserViews[s.ID] = true
	} else {
		delete(a.state.BrowserViews, s.ID)
	}
	if a.state.TrafficViews == nil {
		a.state.TrafficViews = map[string]bool{}
	}
	if view == viewTraffic {
		a.state.TrafficViews[s.ID] = true
	} else {
		delete(a.state.TrafficViews, s.ID)
	}
}

// diffOptions connect session s's diff to the app; wanted tells whether it
// still shows.
func (a *App) diffOptions(s proto.SessionInfo, wanted func() bool) diffview.Options {
	return diffview.Options{
		Client:    a.client,
		Session:   s,
		Update:    a.onMain,
		Wanted:    wanted,
		OpenURL:   func(url string) { go func() { _ = mygo.Shell.OpenExternal(url) }() },
		Notify:    a.notify,
		Send:      func(text string) { a.sendToSession(s.ID, text) },
		Conflicts: a.conflictNotes(s),
		ShowSession: func(id string) {
			for _, o := range a.sessions {
				if o.ID == id {
					a.selectSession(o)
				}
			}
		},
		OpenDiff: func(p string) { a.openCenter(s, centerDiff+p) },
		OpenFile: func(p string) { a.openFileTab(s, p) },
		Current:  func() string { p, _ := strings.CutPrefix(a.centerFront(s), centerDiff); return p },
	}
}

// changesTab draws the Git tool for session s: the changed files.
func (a *App) changesTab(c *ui.Context, s proto.SessionInfo) {
	v := a.diffOf(s.ID)
	gen := a.gen
	v.Draw(c, a.diffOptions(s, func() bool {
		return gen == a.gen && a.panelShows(panelChanges) && a.selected == s.ID && a.diffs[s.ID] == v
	}))
}

// diffTab draws the changes of the file at p, in the center.
func (a *App) diffTab(c *ui.Context, s proto.SessionInfo, p string) {
	v := a.diffOf(s.ID)
	gen := a.gen
	tab := centerDiff + p
	v.DrawFile(c, a.diffOptions(s, func() bool {
		return gen == a.gen && a.centerShows(s.ID, tab) && a.selected == s.ID && a.diffs[s.ID] == v
	}), p)
}

func (a *App) diffOf(id string) *diffview.View {
	if a.diffs == nil {
		a.diffs = map[string]*diffview.View{}
	}
	v := a.diffs[id]
	if v == nil {
		v = diffview.New()
		a.diffs[id] = v
	}
	return v
}

// review shows session s's changes and takes action there (diffview.Act*).
func (a *App) review(s proto.SessionInfo, action string) {
	a.selectSession(s)
	a.openPanel(panelChanges)
	a.diffOf(s.ID).Ask(action)
}

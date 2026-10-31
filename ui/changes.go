package ui

import (
	"slices"

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

// sessionViews are the views session s can show, terminal first.
func (a *App) sessionViews(s proto.SessionInfo) []string {
	views := []string{viewTerminal}
	if canChat(s) {
		views = append(views, viewChat)
	}
	// GOA's session reaches its browser from GOD of Agents' strip, not
	// twice from its header too.
	if webViewAvailable() && !(a.goaOn() && s.ID == a.goa.board.Session) {
		views = append(views, viewBrowser)
	}
	return append(views, viewTraffic)
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

// changesTab draws the Changes tab for session s.
func (a *App) changesTab(c *ui.Context, s proto.SessionInfo) {
	v := a.diffOf(s.ID)
	gen := a.gen
	v.Draw(c, diffview.Options{
		Client:  a.client,
		Session: s,
		Update:  a.onMain,
		Wanted: func() bool {
			return gen == a.gen && a.panelShows(panelChanges) && a.selected == s.ID && a.diffs[s.ID] == v
		},
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
	})
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

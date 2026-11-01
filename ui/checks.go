package ui

import (
	"encoding/json"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/checkview"
)

// The Checks tab of the side panel (see package checkview).

func (a *App) checkViewOf(id string) *checkview.View {
	if a.checkViews == nil {
		a.checkViews = map[string]*checkview.View{}
	}
	v := a.checkViews[id]
	if v == nil {
		v = checkview.New()
		a.checkViews[id] = v
	}
	return v
}

// checkOptions connect session s's checks to the app.
func (a *App) checkOptions(s proto.SessionInfo, v *checkview.View) checkview.Options {
	gen := a.gen
	return checkview.Options{
		Client:  a.client,
		Session: s,
		Update:  a.onMain,
		Wanted: func() bool {
			if gen != a.gen || a.selected != s.ID || a.checkViews[s.ID] != v {
				return false
			}
			return a.panelShows(panelChecks) || a.page == "" && strings.HasPrefix(a.centerFront(s), centerCheck)
		},
		OpenOutput: func(name string) { a.openCenter(s, centerCheck+name) },
		Current: func() string {
			name, _ := strings.CutPrefix(a.centerFront(s), centerCheck)
			return name
		},
	}
}

// checksTab draws the Checks tool for session s: the checks.
func (a *App) checksTab(c *ui.Context, s proto.SessionInfo) {
	v := a.checkViewOf(s.ID)
	v.Draw(c, a.checkOptions(s, v))
}

// checkTab draws a check's output, in the center.
func (a *App) checkTab(c *ui.Context, s proto.SessionInfo, name string) {
	v := a.checkViewOf(s.ID)
	v.DrawOutput(c, a.checkOptions(s, v), name)
}

// checksEvent pokes the Checks view of the session whose checks changed.
func (a *App) checksEvent(data json.RawMessage) func() {
	var ref proto.SessionRef
	if json.Unmarshal(data, &ref) != nil {
		return nil
	}
	return func() {
		if v := a.checkViews[ref.ID]; v != nil {
			v.Poke()
		}
	}
}

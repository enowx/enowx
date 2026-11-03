package ui

import (
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/activityview"
)

// The Activity tab of the side panel (see package activityview).

// activityTab draws the Activity tab for session s.
func (a *App) activityTab(c *ui.Context, s proto.SessionInfo) {
	if a.activityViews == nil {
		a.activityViews = map[string]*activityview.View{}
	}
	v := a.activityViews[s.ID]
	if v == nil {
		v = activityview.New()
		a.activityViews[s.ID] = v
	}
	gen := a.gen
	v.Draw(c, activityview.Options{
		Client:  a.client,
		Session: s,
		Update:  a.onMain,
		Wanted: func() bool {
			return gen == a.gen && a.panelShows(panelActivity) && a.selected == s.ID && a.activityViews[s.ID] == v
		},
		OpenFile: func(p string) { a.showFile(s, p) },
		OpenDiff: func(p string) { a.openCenter(s, centerDiff+a.diffOf(s.ID).PathOf(p)) },
	})
}

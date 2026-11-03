package ui

import "github.com/egoist/mygo/ui"

// pageSide draws the list a page keeps beside its frame, as Settings' sections:
// the same place the session list has on the Sessions page. Pages without one
// give their frame the whole width.
func (a *App) pageSide(c *ui.Context) {
	if a.page == pageSettings && a.client != nil {
		a.settingsSide(c)
	}
	if a.page == pageTeam && a.client != nil {
		a.teamSide(c)
	}
}

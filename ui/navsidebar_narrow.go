package ui

import (
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// The sidebar folded (Cmd+B): the sessions as their agents' icons with their
// status. Notes and Canvas draw their own lists while it is folded.

// navNarrow is the folded sidebar's width.
const navNarrow = 64

func (a *App) navSidebarNarrow(c *ui.Context) {
	widget.Pane(ui.Column(c.Key("nav")).Width(navNarrow).Shrink(0).FillHeight().Radius(theme.RadiusPanel).Clip()).
		Transition(widget.LayoutMotion).Children(func() {
		ui.Column(c).FillWidth().Padding(theme.S2, 0).AlignItems(ui.Center).Shrink(0).Children(func() {
			if widget.IconButton(c.Key("unfold"), icon.Sidebar, "Expand the sidebar ("+modKey+"B)", false).Clicked() {
				a.state.RailWide = true
			}
		})
		ui.Scroll(c).Grow(1).FillWidth().Children(func() {
			ui.Column(c).FillWidth().Padding(0, 0, theme.S2, 0).Gap(theme.S1).AlignItems(ui.Center).Children(func() {
				for _, e := range a.projectEntries() {
					for _, p := range e.projects {
						a.narrowProject(c, p)
					}
				}
			})
		})
	})
}

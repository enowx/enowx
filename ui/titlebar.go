package ui

import (
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// titleBarHeight is the height of the bar the window controls sit in.
const titleBarHeight = 40

// titleBar draws the window's title bar: the command bar in the middle,
// the credits and the host on the right. It drags the window, keeping
// clear of the window controls.
func (a *App) titleBar(c *ui.Context) {
	a.titleBarWith(c, func() {
		a.goaChip(c)
		a.updateChip(c)
		a.chatChip(c)
		a.memoryChip(c)
		a.creditChip(c)
		a.hostChip(c)
	})
}

// titleBarWith is the title bar with chips on its right.
func (a *App) titleBarWith(c *ui.Context, chips func()) {
	bar := c.TitleBar()
	ui.Row(c).Height(max(bar.Height, titleBarHeight)).Shrink(0).FillWidth().
		Padding(0, bar.Right+10, 0, bar.Left+14).Gap(8).DragWindow().Children(func() {
		// The sides keep what they hold, as wide as they were last
		// frame, the search between them giving way: centered while there
		// is room.
		ui.Row(c).Grow(1).Basis(0).MinWidth(a.titleLeftW).Children(func() {
			inner := ui.Row(c).Shrink(0).Children(func() { a.windowButtons(c) })
			a.titleLeftW = inner.Bounds().W
		})
		a.commandButton(c)
		ui.Row(c).Grow(1).Basis(0).MinWidth(a.titleRightW).Justify(ui.End).Children(func() {
			inner := ui.Row(c).Shrink(0).Gap(6).Children(chips)
			if w := inner.Bounds().W; w != a.titleRightW {
				a.titleRightW = w
				c.After(0) // lay out again with it
			}
		})
	})
}

// commandButton opens the command palette.
func (a *App) commandButton(c *ui.Context) {
	b := ui.ButtonBase(c).Width(360).Shrink(1).MinWidth(160).Height(26).PaddingX(10).Gap(8).Radius(theme.RadiusControl).
		Background(theme.Sunken).Label("Run a command").Cursor(ui.CursorPointer)
	border := theme.BorderStrong
	if b.Hovered() {
		border = theme.AccentLine
	}
	widget.Pressable(b.Border(1, widget.Tint(b, "border", border))).Justify(ui.Center).Children(func() {
		ui.Icon(c, icon.Search).FontSize(12).TextColor(theme.Muted)
		ui.Text(c, "Search sessions, hosts and actions").FontSize(11.5).TextColor(theme.Muted).SingleLine().Shrink(1)
		ui.Text(c, modKey+"K").FontSize(11).TextColor(theme.Idle).SingleLine()
	})
	if b.Clicked() {
		a.openPalette()
	}
}

// hostChip is the current host and its connection; it opens the Hosts
// page.
func (a *App) hostChip(c *ui.Context) {
	col, state := a.connectionState()
	b := titleChip(c, "host", state)
	b.Children(func() {
		widget.Dot(c, col, false, 6)
		ui.Text(c, a.host.Name).FontSize(11.5).SingleLine()
	})
	if b.Clicked() {
		a.togglePage(pageHosts)
	}
}

// titleChip is a small button of the title bar.
func titleChip(c *ui.Context, key, label string) ui.Element {
	b := ui.ButtonBase(c.Key(key)).Height(26).PaddingX(9).Gap(6).Radius(theme.RadiusControl).Shrink(0).
		Background(theme.Sunken).Label(label).Tooltip(label).Cursor(ui.CursorPointer).TextColor(theme.TextSoft)
	border := theme.BorderStrong
	if b.Hovered() {
		border = theme.AccentLine
	}
	return widget.Pressable(b.Border(1, widget.Tint(b, "border", border)))
}

package ui

import (
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/hosts"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// titleBar is the window's top: not a bar but separate pieces of glass over
// the wallpaper. The window controls, the host and project, the pages, the
// search, and the actions. What is between them drags the window.
func (a *App) titleBar(c *ui.Context) {
	a.titleBarWith(c, func() {
		a.updateChip(c)
		a.chatChip(c)
		a.newSessionAction(c)
		a.usageAction(c)
		a.settingsAction(c)
		a.accountAction(c)
	})
}

// titleBarWith is the title bar with actions on its right.
func (a *App) titleBarWith(c *ui.Context, actions func()) {
	bar := c.TitleBar()
	row := ui.Row(c).Height(max(bar.Height, theme.BarHeight)).Shrink(0).FillWidth().Padding(0, bar.Right, 0, bar.Left).DragWindow()
	gap := theme.S2
	if theme.Butted {
		row.Background(theme.Panel)
		gap = 0
	}
	row.Gap(gap).Children(func() {
		ui.Row(c).Height(theme.BarHeight).PaddingX(theme.S3).Shrink(0).Children(func() { a.windowButtons(c) })
		a.contextCapsule(c)
		a.pagesCapsule(c)
		a.commandButton(c)
		ui.Spacer(c)
		capsule(c).Shrink(0).Gap(theme.S1).PaddingX(theme.S1).Children(actions)
	})
}

// capsule is a piece of the top's glass.
func capsule(c *ui.Context) ui.Element {
	return widget.BarPiece(ui.Row(c).Height(theme.BarHeight).AlignItems(ui.Center))
}

// contextMax is how wide the host's and the project's names may be in their
// capsule; longer, they run.
const contextMax = 112

// contextCapsule is the host and project the app is on, a menu of the others.
func (a *App) contextCapsule(c *ui.Context) {
	col, state := a.connectionState()
	b := widget.BarPiece(ui.ButtonBase(c.Key("host-switch")).Height(theme.BarHeight).PaddingX(theme.S4).Gap(theme.S2).
		Cursor(ui.CursorPointer).Shrink(0).Label("Host: " + a.host.Name + ", " + state).Tooltip(a.host.Name + " · " + state))
	b.Children(func() {
		if a.client == nil {
			widget.Dot(c, col, false, 8)
		}
		ui.Row(c).MaxWidth(contextMax).MinWidth(0).Children(func() {
			widget.Running(c, a.host.Name, widget.RunStyle{Weight: 500})
		})
		ui.Icon(c, icon.ChevronsUD).FontSize(11)
		if p, ok := a.currentProject(); ok {
			ui.Row(c).MaxWidth(contextMax).MinWidth(0).Children(func() {
				widget.Running(c, p.Name, widget.RunStyle{Color: theme.TextSoft})
			})
		}
	})
	a.menu.OpenBelow(b, "host-switch", theme.BarHeight)
	a.menu.Show(c, b, "host-switch", func(done func()) { a.hostMenuItems(c, done) })
}

// pagesCapsule is the app's pages.
func (a *App) pagesCapsule(c *ui.Context) {
	capsule(c).Shrink(0).PaddingX(theme.S1).Gap(theme.S1).Children(func() {
		page := func(label string, on, enabled bool, open func()) {
			if pageButton(c, label, on, enabled).Clicked() && enabled && !on {
				open()
			}
		}
		goa := a.goaOn()
		page("Sessions", a.page == "" && !goa, true, func() {
			if goa {
				a.confirmLeaveGOA(nil)
				return
			}
			a.page = ""
		})
		page("Notes", a.page == pageNotes, a.client != nil, func() { a.page = pageNotes })
		page("Canvas", a.page == pageCanvas, a.client != nil, func() { a.page = pageCanvas })
		if a.client != nil {
			a.goaLoad()
			page("GOD of Agents", a.page == "" && goa, true, func() {
				if goa {
					a.page = ""
					return
				}
				a.toggleGOA()
			})
		}
		a.navMore(c)
	})
}

// pageButton is a page's entry in the pages' capsule.
func pageButton(c *ui.Context, label string, on, enabled bool) ui.Element {
	b := ui.ButtonBase(c.Key("page:" + label)).Height(theme.ControlHeight).PaddingX(theme.S3).Gap(theme.S1).Radius(theme.RadiusFull).
		Label(label).Shrink(0)
	bg, fg, weight := theme.Fade(theme.Selected, 0), theme.TextSoft, 400
	switch {
	case on:
		bg, fg, weight = theme.Picked, theme.OnPicked, 500
	case !enabled:
		b.Opacity(0.45)
	case b.Hovered():
		bg, fg = theme.Selected, theme.TextStrong
	}
	if enabled && !on {
		b.Cursor(ui.CursorPointer)
	}
	b.Background(widget.QuickTint(b, "bg", bg)).TextColor(fg).Children(func() {
		ui.Text(c, label).FontSize(theme.TextUI).FontWeight(weight).SingleLine()
	})
	return b
}

// commandButton opens the command palette.
func (a *App) commandButton(c *ui.Context) {
	b := widget.BarField(ui.ButtonBase(c.Key("command")).Width(340).Shrink(1).MinWidth(160).Height(theme.BarHeight).PaddingX(theme.S4).Gap(theme.S2).
		Label("Run a command").Cursor(ui.CursorPointer))
	b.Justify(ui.Start).TextColor(theme.TextSoft).Children(func() {
		ui.Icon(c, icon.Search).FontSize(14)
		ui.Text(c, "Search sessions, hosts and actions").FontSize(theme.TextUI).SingleLine().Shrink(1).MinWidth(0)
		ui.Spacer(c)
		ui.Text(c, modKey+"K").FontSize(theme.TextUI).SingleLine().Shrink(0)
	})
	if b.Clicked() {
		a.openPalette()
	}
}

// newSessionAction starts a session in the current project.
func (a *App) newSessionAction(c *ui.Context) {
	p, ok := a.currentProject()
	b := actionButton(c, "new", "New session", icon.Plus, false).Tooltip("New session (" + modKey + "N)")
	if !ok {
		b.Disabled(true).Opacity(0.45)
	} else if b.Clicked() {
		a.openNewSession(p)
	}
}

// actionButton is an entry of the actions' capsule, with its words or an icon alone.
func actionButton(c *ui.Context, key, label string, svg *ui.SVG, iconOnly bool) ui.Element {
	b := ui.ButtonBase(c.Key(key)).Height(theme.ControlHeight).MinWidth(theme.ControlHeight).PaddingX(theme.S2).Gap(theme.S1).Radius(theme.RadiusFull).
		Center().Label(label).Cursor(ui.CursorPointer).Shrink(0)
	bg := theme.Fade(theme.Selected, 0)
	if b.Hovered() {
		bg = theme.Selected
	}
	b.Background(widget.QuickTint(b, "bg", bg)).Children(func() {
		ui.Icon(c, svg).FontSize(14)
		if !iconOnly {
			ui.Text(c, label).FontSize(theme.TextUI).SingleLine()
		}
	})
	return b
}

// lit marks an action whose page shows.
func lit(b ui.Element, on bool) ui.Element {
	if on {
		b.Background(theme.Selected).TextColor(theme.TextStrong)
	}
	return b
}

// usageAction opens Resource usage; its tooltip says what the status bar used to.
func (a *App) usageAction(c *ui.Context) {
	if a.client == nil {
		return
	}
	tip := "Resource usage"
	if s := a.usageLine(); s != "" {
		tip += " · " + s
	}
	b := lit(actionButton(c, "usage", "Resource usage", icon.Usage, true), a.page == pageUsage).Tooltip(tip)
	if b.Clicked() {
		a.togglePage(pageUsage)
	}
}

// settingsAction opens Settings.
func (a *App) settingsAction(c *ui.Context) {
	if a.client == nil {
		return
	}
	b := lit(actionButton(c, "settings", "Settings", icon.Settings, true), a.page == pageSettings).Tooltip("Settings")
	if b.Clicked() {
		a.togglePage(pageSettings)
	}
}

// hostIsRemote says the app is connected to a host other than this device.
func (a *App) hostIsRemote() bool { return a.client != nil && a.host.ID != hosts.LocalID }

// titleChip is a small button of the actions' capsule, its words its own.
func titleChip(c *ui.Context, key, label string) ui.Element {
	b := ui.ButtonBase(c.Key(key)).Height(theme.ControlHeight).PaddingX(theme.S2).Gap(theme.S1).Radius(theme.RadiusFull).Shrink(0).
		Label(label).Tooltip(label).Cursor(ui.CursorPointer)
	bg := theme.Fade(theme.Selected, 0)
	if b.Hovered() {
		bg = theme.Selected
	}
	return b.Background(widget.QuickTint(b, "bg", bg))
}

// usageLine is the host's use, what the window's foot once said.
func (a *App) usageLine() string {
	line := ""
	if h, ok := a.usage.Host(); ok && h.MemTotal > 0 {
		line = textfmt.Bytes(h.MemUsed) + " / " + textfmt.Bytes(h.MemTotal)
	}
	if s := a.credits.Summary(a.creditsHost()); s != "" {
		if line != "" {
			line += " · "
		}
		line += s
	}
	return line
}

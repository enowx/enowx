package ui

import (
	"image/color"
	"log"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// The app's icon in the menu bar (macOS) or the notification area
// (Windows, Linux). Closing the window keeps the app there, its sessions
// followed, unless Settings says closing quits. A click shows a panel of
// the sessions that need the user, are working or are done, each opening
// the window on it; Linux, whose tray tells no clicks, shows a menu.

// traySessions is how many sessions the panel lists at most.
const traySessions = 8

// setUpTray adds the icon, once the window is open.
func (a *App) setUpTray() {
	ink := color.NRGBA{R: 0xe8, G: 0xd5, B: 0xb5, A: 255} // the accent, on light and dark taskbars
	if runtime.GOOS == "darwin" {
		ink = color.NRGBA{A: 255} // a template, which macOS tints
	}
	t, err := mygo.NewTray(mygo.TrayOptions{ID: "enowx", Icon: icon.LogoPNG(36, ink), IconIsTemplate: runtime.GOOS == "darwin", ToolTip: "enowX"})
	if err != nil {
		log.Print("no tray icon: ", err)
		return
	}
	a.tray = t
	if runtime.GOOS == "linux" {
		a.updateTrayMenu() // AppIndicator shows menus only
	} else {
		t.OnClick(a.toggleTrayPanel)
		t.OnRightClick(func() {
			if a.trayPanel != nil {
				a.trayPanel.Close()
			}
			t.SetMenu(a.trayMenu())
			defer t.SetMenu(nil) // clicks keep opening the panel
			t.PopUpMenu()
		})
	}
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { a.quitting = true })
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) { t.Destroy() })
	// The Dock's icon (macOS) shows the window again.
	mygo.App.OnActivate(func(bool) { a.showWindow() })
	a.win.OnClose(func(e *mygo.CloseEvent) {
		if a.quitting || a.state.CloseQuits || a.tray == nil {
			return
		}
		e.PreventDefault()
		a.win.Hide()
	})
}

// showWindow brings the window back.
func (a *App) showWindow() {
	if a.win == nil {
		return
	}
	a.win.Show()
	a.win.Focus()
}

// updateTray says on the icon what the sessions are at: the number
// needing the user next to it (macOS), all of it in its tooltip. It runs
// as the sessions change.
func (a *App) updateTray() {
	if a.tray == nil {
		return
	}
	waiting, working := 0, 0
	for _, s := range a.sessions {
		switch {
		case s.Exited:
		case s.State == proto.StateWaiting:
			waiting++
		case isWorking(s):
			working++
		}
	}
	title, tip := "", "enowX"
	if waiting > 0 {
		title = strconv.Itoa(waiting)
	}
	if s := summary(a.sessions); s != "" {
		tip += ": " + s
	}
	if working > 0 && waiting > 0 {
		tip += ", " + strconv.Itoa(working) + " working"
	}
	if a.trayPanel != nil {
		a.trayPanel.Update(func() {}) // its list draws again
	}
	if title != a.trayTitle || tip != a.trayTip {
		a.trayTitle, a.trayTip = title, tip
		a.tray.SetTitle(title)
		a.tray.SetToolTip(tip)
		if runtime.GOOS == "linux" {
			a.updateTrayMenu()
		}
	}
}

func (a *App) updateTrayMenu() {
	if a.tray != nil {
		a.tray.SetMenu(a.trayMenu())
	}
}

// trayMenu is the icon's menu: the window, the sessions needing the user
// (Linux, which has no panel), a new session, quitting.
func (a *App) trayMenu() *mygo.Menu {
	items := []*mygo.MenuItem{
		{Label: "Open enowX", Click: func(*mygo.MenuItem, *mygo.Window) { a.showWindow() }},
	}
	if runtime.GOOS == "linux" {
		for _, s := range a.traySessionList() {
			if s.State != proto.StateWaiting {
				continue
			}
			s := s
			items = append(items, &mygo.MenuItem{Label: a.sessionName(s) + ": needs input", Click: func(*mygo.MenuItem, *mygo.Window) { a.openFromTray(s) }})
		}
	}
	items = append(items,
		&mygo.MenuItem{Label: "New session…", Click: func(*mygo.MenuItem, *mygo.Window) { a.newSessionFromTray() }},
		mygo.Separator(),
		&mygo.MenuItem{Label: "Quit enowX", Click: func(*mygo.MenuItem, *mygo.Window) { mygo.App.Quit() }},
	)
	return mygo.NewMenu(items)
}

// toggleTrayPanel opens the panel next to the icon, or closes it.
func (a *App) toggleTrayPanel() {
	if a.trayPanel != nil {
		a.trayPanel.Close()
		return
	}
	a.trayPanel = mygo.NewFlyout(mygo.FlyoutOptions{
		Tray:      a.tray,
		Placement: mygo.PlacementBottom,
		Gap:       4,
		Width:     340,
		Height:    a.trayPanelHeight(),
		Focusable: true,
		Shadow:    true,
		Popover:   true,
		Content:   ui.View(a.trayView),
	})
	a.trayPanel.OnClosed(func() { a.trayPanel = nil })
}

func (a *App) trayPanelHeight() int {
	n := len(a.traySessionList())
	if n == 0 {
		n = 1 // the note that there are none
	}
	return 118 + n*40
}

// traySessionList are the sessions the panel lists: those needing the
// user, then working, then done, most recent first within each.
func (a *App) traySessionList() []proto.SessionInfo {
	rank := func(s proto.SessionInfo) int {
		switch {
		case s.State == proto.StateWaiting:
			return 0
		case isWorking(s):
			return 1
		case s.State == proto.StateDone:
			return 2
		}
		return 3
	}
	var list []proto.SessionInfo
	for _, s := range a.sessions {
		if s.Parent == "" && !s.Exited && rank(s) < 3 {
			list = append(list, s)
		}
	}
	slices.SortStableFunc(list, func(x, y proto.SessionInfo) int {
		if d := rank(x) - rank(y); d != 0 {
			return d
		}
		return y.Created.Compare(x.Created)
	})
	if len(list) > traySessions {
		list = list[:traySessions]
	}
	return list
}

// openFromTray shows the window on session s.
func (a *App) openFromTray(s proto.SessionInfo) {
	if a.trayPanel != nil {
		a.trayPanel.Close()
	}
	a.showWindow()
	a.page = ""
	a.selectSession(s)
}

func (a *App) newSessionFromTray() {
	if a.trayPanel != nil {
		a.trayPanel.Close()
	}
	a.showWindow()
	a.page = ""
	if p, ok := a.currentProject(); ok {
		a.openNewSession(p)
	}
}

// trayView is the panel.
func (a *App) trayView(c *ui.Context) {
	c.SetTheme(a.theme)
	c.Root().Background(ui.Transparent)
	box := ui.Column(c).Fill().TextColor(theme.Text)
	switch {
	case c.Vibrancy() || runtime.GOOS == "windows": // the system rounds and outlines these
		box.Background(theme.Dialog)
	default:
		box.Background(theme.Dialog).Radius(theme.RadiusSmall).Border(1, theme.BorderStrong)
	}
	box.Padding(theme.S3).Gap(theme.S2).Children(func() {
		ui.Row(c).FillWidth().Gap(theme.S2).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
				ui.Text(c, "enowX").FontSize(theme.TextUI).FontWeight(600)
				var parts []string
				for _, p := range []string{a.host.Name, summary(a.sessions)} {
					if p != "" {
						parts = append(parts, p)
					}
				}
				widget.Running(c, strings.Join(parts, " · "), widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
			})
		})
		list := a.traySessionList()
		if len(list) == 0 {
			ui.Text(c, "No agent is working or waiting.").FontSize(theme.TextUI).TextColor(theme.TextSoft).Padding(theme.S2, 0)
		}
		for _, s := range list {
			a.traySessionRow(c.Key(s.ID), s)
		}
		ui.Spacer(c)
		ui.Row(c).FillWidth().Gap(theme.S1).Children(func() {
			if widget.Button(c, widget.Secondary, nil, "Open enowX", "").Clicked() {
				if a.trayPanel != nil {
					a.trayPanel.Close()
				}
				a.showWindow()
			}
			if _, ok := a.currentProject(); ok && widget.Button(c, widget.Secondary, icon.Plus, "New session", "").Clicked() {
				a.newSessionFromTray()
			}
			ui.Spacer(c)
			if widget.GhostButton(c, "Quit").Clicked() {
				mygo.App.Quit()
			}
		})
	})
}

func (a *App) traySessionRow(c *ui.Context, s proto.SessionInfo) {
	row := ui.ButtonBase(c).FillWidth().Height(36).Shrink(0).PaddingX(theme.S2).Gap(theme.S2).Radius(theme.RadiusCard).Justify(ui.Start).
		Label(a.sessionName(s)).Cursor(ui.CursorPointer)
	if row.Hovered() {
		row.Background(theme.Hover)
	}
	row.Children(func() {
		col, what := theme.Done, "done"
		switch {
		case s.State == proto.StateWaiting:
			col, what = theme.Warning, "needs input"
		case isWorking(s):
			col, what = theme.Working, "working"
		}
		widget.Dot(c, col, false, 8)
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			widget.Running(c, a.sessionName(s), widget.RunStyle{})
			widget.Running(c.Key("line"), textfmt.LastPathPart(s.Project)+" · "+what, widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
		})
	})
	if row.Clicked() {
		a.openFromTray(s)
	}
}

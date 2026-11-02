package ui

import "github.com/egoist/mygo"

// setAppMenu is macOS's menu bar with "Quit and Stop All Agents" beside
// Quit, where people look for it.
func setAppMenu(a *App) {
	mygo.App.SetMenu(mygo.NewMenu([]*mygo.MenuItem{
		{Role: mygo.RoleAppMenu, Submenu: []*mygo.MenuItem{
			{Role: mygo.RoleAbout},
			mygo.Separator(),
			{Role: mygo.RoleServices},
			mygo.Separator(),
			{Role: mygo.RoleHide},
			{Role: mygo.RoleHideOthers},
			{Role: mygo.RoleUnhide},
			mygo.Separator(),
			{Label: "Quit and Stop All Agents", Accelerator: "CmdOrCtrl+Alt+Q", Click: func(*mygo.MenuItem, *mygo.Window) { a.quitAndStopAgents() }},
			{Role: mygo.RoleQuit},
		}},
		{Role: mygo.RoleFileMenu, Submenu: fileMenuItems(a)},
		{Role: mygo.RoleEditMenu},
		{Role: mygo.RoleViewMenu},
		{Role: mygo.RoleWindowMenu},
	}))
}

// fileMenuItems has Cmd+W close a tab, not the window: with the tab it was meant
// for gone, a session killed from elsewhere, the default closed the only window,
// and with it the app. Another window, focused, still closes.
func fileMenuItems(a *App) []*mygo.MenuItem {
	return []*mygo.MenuItem{
		{Label: "Close Tab", Accelerator: "CmdOrCtrl+W", Click: func(_ *mygo.MenuItem, w *mygo.Window) {
			if w != nil && w != a.win {
				w.Close()
				return
			}
			a.onMain(a.closeFrontTab)
		}},
		{Role: mygo.RoleClose, Accelerator: "CmdOrCtrl+Shift+W"},
	}
}

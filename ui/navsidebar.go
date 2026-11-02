package ui

import (
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/hosts"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// The sidebar: the sessions, by state or by project, on the Sessions page;
// Notes and Canvas their own lists, in the same place. The other pages have
// no list of sessions. Folded (Cmd+B), the sessions show as icons.

// navWidth is the sidebar's width.
const navWidth = 248

// sessionListShows says the sidebar shows at all.
func (a *App) sessionListShows() bool {
	if a.goaOn() && a.navPage() == "" {
		return false
	}
	if a.navPage() != "" {
		return true
	}
	return a.page == "" && !a.sessionsHidden()
}

// navShown says the sidebar shows unfolded: wide, unless the user folded it
// or the window is too narrow for it beside a page.
func (a *App) navShown() bool {
	return a.state.RailWide && !(a.page != "" && a.winW > 0 && a.winW < 1100)
}

func (a *App) navSidebar(c *ui.Context) {
	widget.Glass(ui.Column(c.Key("nav")).Width(navWidth).Shrink(0).FillHeight().Radius(theme.RadiusPanel).Clip()).
		Transition(widget.LayoutMotion).Children(func() {
		ui.Column(c.Key("nav-list:" + a.navPage())).Grow(1).FillWidth().MinHeight(0).Children(func() {
			switch a.navPage() {
			case pageNotes:
				a.loadCatalogs()
				o := a.notesOptions()
				a.notes.DrawList(c, o)
			case pageCanvas:
				a.canvases.DrawList(c, a.canvasOptions())
			default:
				a.sessionsBody(c)
			}
		})
		a.navRunning(c)
	})
}

// navPage is the list the sidebar shows: Notes and Canvas their lists,
// every other page the sessions.
func (a *App) navPage() string {
	switch {
	case a.page == pageNotes && a.client != nil:
		return pageNotes
	case a.page == pageCanvas && a.client != nil:
		return pageCanvas
	}
	return ""
}

// hostMenuItems are the hosts to switch to: devices, servers, the team's
// shared hosts, then adding or managing them.
func (a *App) hostMenuItems(c *ui.Context, done func()) {
	var devices, servers []hosts.Host
	for _, h := range a.allHosts() {
		if h.IsDevice() {
			devices = append(devices, h)
		} else {
			servers = append(servers, h)
		}
	}
	widget.PopupLabel(c, "Hosts")
	pick := func(h hosts.Host) {
		if widget.MenuItemSub(c.Key("host:"+h.ID), hostIcon(h), h.Name, a.hostFaceOf(h).state, false) {
			if h.ID != a.host.ID {
				a.connect(h)
			}
			done()
		}
	}
	for _, h := range devices {
		pick(h)
	}
	for _, h := range servers {
		pick(h)
	}
	if a.client != nil && len(a.spacesOf().list) > 0 {
		if shared := a.teamHostsOf().hosts; len(shared) > 0 {
			for _, h := range shared {
				if widget.MenuItemSub(c.Key("teamhost:"+h.Device), icon.Server, h.Name, h.Owner+"'s, "+a.spaceName(h.Team), false) {
					a.openTeamHost(h)
					done()
				}
			}
		}
	}
	// The tailnet's devices not added yet, asked for again now and then.
	if time.Since(a.peersAt) > 30*time.Second {
		a.peersAt = time.Now()
		go a.loadPeers()
	}
	for _, p := range a.newPeers() {
		if widget.MenuItemSub(c.Key("peer:"+p.Address()), icon.Tailscale, p.HostName, "On your tailnet", false) {
			a.connectPeer(p)
			done()
		}
	}
	if a.client != nil && len(a.projects) > 0 {
		widget.PopupLabel(c, "Project")
		for _, p := range a.projects {
			if widget.MenuItem(c.Key("project:"+p.Path), icon.Folder, p.Name, false) {
				a.project, a.selected, a.page = p.Path, "", ""
				delete(a.collapsed, p.Path)
				done()
			}
		}
	}
	widget.MenuSeparator(c)
	if widget.MenuItem(c.Key("open-folder"), icon.FolderPlus, "Open a folder…", false) {
		a.openFolder()
		done()
	}
	if widget.MenuItem(c.Key("clone"), icon.Git, "Clone from Git…", false) {
		a.openClone()
		done()
	}
	widget.MenuSeparator(c)
	if a.client != nil && a.host.ID != hosts.LocalID && widget.MenuItem(c.Key("sync"), icon.Sync, "Sync environment…", false) {
		a.openSync()
		done()
	}
	if a.client != nil && a.host.ID != hosts.LocalID && a.project != "" && widget.MenuItem(c.Key("setup"), icon.Sparkle, "Set up this host…", false) {
		a.openSetup()
		done()
	}
	if widget.MenuItem(c.Key("add-vps"), icon.Plus, "Add VPS…", false) {
		a.openAddHost()
		done()
	}
	if widget.MenuItem(c.Key("hosts"), icon.Server, "Manage hosts…", false) {
		a.page = pageHosts
		done()
	}
}

// navMore is the menu of the pages without an entry of their own.
func (a *App) navMore(c *ui.Context) {
	more := ui.ButtonBase(c.Key("nav-more")).Height(theme.ControlHeight).PaddingX(theme.S3).Gap(theme.S1).Radius(theme.RadiusFull).
		Label("More").Tooltip("More pages").Cursor(ui.CursorPointer).Shrink(0)
	bg, fg, weight := theme.Fade(theme.Selected, 0), theme.TextSoft, 400
	switch {
	case a.page == pageVault || a.page == pageTeam || a.page == pageTeamHost:
		bg, fg, weight = theme.Selected, theme.TextStrong, 500
	case more.Hovered():
		bg, fg = theme.Selected, theme.TextStrong
	}
	more.Background(widget.QuickTint(more, "bg", bg)).TextColor(fg).Children(func() {
		ui.Text(c, "More").FontSize(theme.TextUI).FontWeight(weight).SingleLine()
		ui.Icon(c, icon.ChevronDown).FontSize(12)
	})
	a.menu.OpenBelow(more, "nav-more", theme.BarHeight)
	a.menu.Show(c, more, "nav-more", func(done func()) {
		items := []struct {
			svg     *ui.SVG
			label   string
			enabled bool
			open    func()
		}{
			{icon.Users, "Team", a.client != nil, a.openTeamPage},
			{icon.Key, "Credentials", true, a.openVault},
		}
		for _, it := range items {
			if it.enabled && widget.MenuItem(c.Key(it.label), it.svg, it.label, false) {
				it.open()
				done()
			}
		}
	})
}

// navRunning are the sessions at work or waiting, while another page than
// the sessions' shows: an agent keeps in sight from anywhere.
func (a *App) navRunning(c *ui.Context) {
	if a.navPage() == "" {
		return
	}
	var busy []proto.SessionInfo
	for _, s := range a.sessions {
		if !s.Exited && (isWorking(s) || s.State == proto.StateWaiting) {
			busy = append(busy, s)
		}
	}
	if len(busy) == 0 {
		return
	}
	ui.Column(c.Key("nav-running")).FillWidth().Shrink(0).Padding(theme.S2).Gap(0).Children(func() {
		widget.SectionLabel(c, "At work", theme.TextSoft).Padding(theme.S1, theme.S3)
		for i, s := range busy {
			if i == 4 {
				ui.Text(c, "and more in Sessions").FontSize(theme.TextLabel).TextColor(theme.TextSoft).Padding(theme.S1, theme.S3)
				break
			}
			st := statusOf(s)
			row := ui.ButtonBase(c.Key("run:" + s.ID)).FillWidth().Height(theme.ControlHeight).PaddingX(theme.S3).Gap(theme.S2).Radius(theme.RadiusFull).AlignItems(ui.Center).
				Cursor(ui.CursorPointer).Label("Open " + a.sessionName(s))
			if row.Hovered() {
				row.Background(theme.Hover)
			}
			row.Children(func() {
				ui.Row(c).Grow(1).MinWidth(0).Children(func() { widget.Running(c, a.sessionName(s), widget.RunStyle{}) })
				widget.Dot(c, st.color, st.hollow, 8)
			})
			if row.Clicked() {
				a.page = ""
				a.selectSession(s)
			}
		}
	})
}

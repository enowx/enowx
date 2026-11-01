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

// The sidebar, one column where there were three (the hosts, the
// sessions, a page's own list): the host as a menu at its top, the pages
// as tabs, then the list of the page shown (sessions, notes' pages,
// canvases), the sessions at work while another page shows, and the
// account. Folded (Cmd+B), the rail of icons and the Sessions panel come
// back.

// navWidth is the sidebar's width.
const navWidth = 284

// navShown says the sidebar shows unfolded (never in GOD of Agents' mode,
// so Notes and Canvas draw their lists themselves): wide, unless the user folded
// it or the window is too narrow for it beside a page.
func (a *App) navShown() bool {
	return a.state.RailWide && !a.goaOn() && !a.sessionsHidden() && !(a.page != "" && a.winW > 0 && a.winW < 1100)
}

func (a *App) navSidebar(c *ui.Context) {
	widget.Bento(ui.Column(c.Key("nav")).Width(navWidth).Shrink(0).FillHeight(), theme.Panel).
		Transition(widget.LayoutMotion).Children(func() {
		a.navHead(c)
		a.navTabs(c)
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
		ui.Column(c).FillWidth().Padding(4, 8, 8, 8).Children(func() { a.railAccount(c, true) })
	})
}

// navPage is the tab the sidebar shows: Notes and Canvas their lists,
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

// navHead is the sidebar's top: enowX's mark, the host as a menu, and
// folding the sidebar.
func (a *App) navHead(c *ui.Context) {
	ui.Row(c.Key("nav-head")).FillWidth().Height(52).Shrink(0).Padding(0, 8, 0, 10).Gap(6).AlignItems(ui.Center).Children(func() {
		mark := ui.Box(c).Size(30, 30).Shrink(0).Center().Radius(theme.RadiusControl).
			Gradient(theme.AccentHigh, theme.Accent, 180).TextColor(theme.OnAccent).InsetShadow(0, 1, 0, 0, ui.RGBA(255, 255, 255, 0.3))
		mark.Children(func() { ui.Icon(c, icon.Logo).FontSize(24) })
		a.hostSwitcher(c)
		ui.Spacer(c)
		if widget.IconButton(c, icon.Collapse, "Collapse sidebar ("+modKey+"B)", false).Clicked() {
			a.state.RailWide = false
		}
	})
}

// hostSwitcher is the host the app is on, a menu of the others: devices,
// servers, the team's shared hosts, and adding or managing them.
func (a *App) hostSwitcher(c *ui.Context) {
	col, state := a.connectionState()
	b := ui.ButtonBase(c.Key("host-switch")).Height(32).Padding(0, 8).Gap(6).Radius(theme.RadiusControl).AlignItems(ui.Center).
		Cursor(ui.CursorPointer).Label("Host: " + a.host.Name + ", " + state).Tooltip(a.host.Name + " · " + state).Shrink(1).MinWidth(0)
	if b.Hovered() {
		b.Background(theme.Hover)
	}
	b.Children(func() {
		ui.Text(c, a.host.Name).FontSize(13).FontWeight(600).TextColor(theme.TextStrong).SingleLine().Shrink(1)
		widget.Dot(c, col, false, 7)
		ui.Icon(c, icon.ChevronDown).FontSize(11).TextColor(theme.Muted)
	})
	a.menu.OpenBelow(b, "host-switch", 34)
	a.menu.Show(c, b, "host-switch", func(done func()) { a.hostMenuItems(c, done) })
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
	pick := func(h hosts.Host) {
		label := h.Name + " · " + hostDetail(h)
		if widget.MenuItem(c.Key("host:"+h.ID), hostIcon(h), label, false) {
			if h.ID != a.host.ID {
				a.connect(h)
			}
			done()
		}
	}
	for _, h := range devices {
		pick(h)
	}
	if len(servers) > 0 {
		widget.MenuSeparator(c)
		for _, h := range servers {
			pick(h)
		}
	}
	if a.client != nil && len(a.spacesOf().list) > 0 {
		if shared := a.teamHostsOf().hosts; len(shared) > 0 {
			widget.MenuSeparator(c)
			for _, h := range shared {
				if widget.MenuItem(c.Key("teamhost:"+h.Device), icon.Server, h.Name+" · "+h.Owner+"'s, "+a.spaceName(h.Team), false) {
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
	if peers := a.newPeers(); len(peers) > 0 {
		widget.MenuSeparator(c)
		for _, p := range peers {
			if widget.MenuItem(c.Key("peer:"+p.Address()), icon.Tailscale, p.HostName+" · on your tailnet", false) {
				a.connectPeer(p)
				done()
			}
		}
	}
	widget.MenuSeparator(c)
	if a.client != nil && a.host.ID != hosts.LocalID && widget.MenuItem(c.Key("sync"), icon.Sync, "Sync environment…", false) {
		a.openSync()
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

// navTabs are the pages: Sessions, Notes and Canvas, the others in a
// menu.
func (a *App) navTabs(c *ui.Context) {
	ui.Row(c.Key("nav-tabs")).FillWidth().Shrink(0).Padding(0, 10, 8, 10).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Row(c).Grow(1).Height(34).Padding(3).Gap(2).Radius(theme.RadiusControl+1).Background(theme.Sunken).Border(1, theme.BorderStrong).Children(func() {
			tab := func(id, label string, enabled bool) {
				on := a.navPage() == id && (id != "" || a.page == "")
				t := ui.ButtonBase(c.Key("tab:" + id)).Grow(1).Height(26).PaddingX(10).Radius(theme.RadiusControl - 2).Center().Label(label)
				fg := theme.Muted
				switch {
				case on:
					t.Background(theme.Selected).InsetShadow(0, 1, 0, 0, theme.Highlight)
					fg = theme.Accent
				case !enabled:
					fg = theme.Idle
				case t.Hovered():
					fg = theme.Text
					t.Cursor(ui.CursorPointer)
				default:
					t.Cursor(ui.CursorPointer)
				}
				// Text alone: three tabs share the sidebar's width with room
				// around each label.
				t.Children(func() { ui.Text(c, label).FontSize(12).FontWeight(500).TextColor(fg).SingleLine() })
				if enabled && !on && t.Clicked() {
					a.page = id
				}
			}
			tab("", "Sessions", true)
			tab(pageNotes, "Notes", a.client != nil)
			tab(pageCanvas, "Canvas", a.client != nil)
		})
		a.navMore(c)
	})
}

// navMore is the menu of the pages without a tab.
func (a *App) navMore(c *ui.Context) {
	more := widget.IconButton(c.Key("nav-more"), icon.More, "More: credentials, settings, appearance, usage", a.navPage() == "" && a.page != "")
	a.menu.OpenBelow(more, "nav-more", 30)
	a.menu.Show(c, more, "nav-more", func(done func()) {
		remote := a.client != nil && a.host.ID != hosts.LocalID
		items := []struct {
			svg     *ui.SVG
			label   string
			enabled bool
			open    func()
		}{
			{icon.Key, "Credentials", true, a.openVault},
			{icon.Settings, "Settings", a.client != nil, func() { a.page = pageSettings }},
			{icon.Palette, "Appearance", true, func() { a.accentInput = a.state.Accent; a.page = pageAppearance }},
			{icon.Usage, "Resource usage", a.client != nil, func() { a.page = pageUsage }},
			{icon.Credits, "Credits", a.client != nil, func() { a.page = pageCredits }},
			{icon.Sync, "Sync environment", remote, a.openSync},
		}
		for _, it := range items {
			if !it.enabled {
				continue
			}
			if widget.MenuItem(c.Key(it.label), it.svg, it.label, false) {
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
	ui.Column(c.Key("nav-running")).FillWidth().Shrink(0).Padding(8, 8, 4, 8).Gap(2).BorderWidth(1, 0, 0, 0).BorderColor(theme.Border).Children(func() {
		ui.Text(c, "At work").FontSize(11).TextColor(theme.Muted).Padding(0, 6, 2, 6)
		for i, s := range busy {
			if i == 4 {
				ui.Text(c, "and more in Sessions").FontSize(11).TextColor(theme.Idle).Padding(2, 6)
				break
			}
			st := statusOf(s)
			row := ui.ButtonBase(c.Key("run:"+s.ID)).FillWidth().Height(32).Padding(0, 6).Gap(8).Radius(8).AlignItems(ui.Center).
				Cursor(ui.CursorPointer).Label("Open " + a.sessionName(s))
			if row.Hovered() {
				row.Background(theme.Hover)
			}
			row.Children(func() {
				agentTile(c, s.Agent, 22)
				ui.Text(c, a.sessionName(s)).FontSize(12).SingleLine().Grow(1).Shrink(1).MinWidth(0)
				widget.Dot(c, st.color, st.hollow, 6)
			})
			if row.Clicked() {
				a.page = ""
				a.selectSession(s)
			}
		}
	})
}

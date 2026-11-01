package ui

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/hosts"
	"github.com/enowx/enowx/core/vps"
	"github.com/enowx/enowx/transport"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

func (a *App) connectionState() (ui.Color, string) {
	switch {
	case a.connecting != "":
		return theme.Warning, "Connecting…"
	case a.client == nil:
		return theme.Error, "Not connected"
	case a.host.ID == hosts.LocalID:
		return theme.Done, "Local"
	}
	if a.host.RunAs != "" {
		return theme.Done, "SSH · " + a.host.Address + " · as " + a.host.RunAs
	}
	return theme.Done, "SSH · " + a.host.Address
}

// peerColor is a tailnet device's dot: online or not.
func peerColor(online bool) ui.Color {
	if online {
		return theme.Done
	}
	return theme.Idle
}

// hostDetail is a host's address, or what this machine is.
func hostDetail(h hosts.Host) string {
	if h.ID == hosts.LocalID {
		return "this machine"
	}
	return h.Address
}

// connectionView fills the workspace while connecting or after a failure:
// a card with a badge colored by the state, under the usual breadcrumb.
func (a *App) connectionView(c *ui.Context) bool {
	switch {
	case a.connecting != "":
		widget.PageHeader(c, a.host.Name, "Connecting", nil)
		ui.Column(c).Grow(1).FillWidth().Center().Padding(theme.S5).Children(func() {
			var buttons func()
			if link := a.connectLink; link != "" {
				buttons = func() {
					if widget.PrimaryButton(c, icon.External, "Open link", "").Clicked() {
						go func() { _ = mygo.Shell.OpenExternal(link) }()
					}
				}
			}
			widget.StateCard(c, theme.Accent, nil, "Connecting to "+a.host.Name, a.connecting,
				func() { widget.Spinner(c, 18) }, buttons)
		})
		return true
	case a.connErr != "":
		title := "Couldn't connect to " + a.host.Name
		crumb := "Couldn't connect"
		if strings.HasPrefix(a.connErr, "Disconnected from ") {
			title, crumb = "Disconnected from "+a.host.Name, "Not connected"
		}
		widget.PageHeader(c, a.host.Name, crumb, nil)
		ui.Column(c).Grow(1).FillWidth().Center().Padding(theme.S5).Children(func() {
			widget.StateCard(c, theme.Error, icon.Alert, title, a.connErr, nil, func() {
				var refusal *transport.AuthError
				if errors.As(a.connFail, &refusal) && widget.GhostButton(c, "Sign in…").Clicked() {
					a.promptSignIn(a.host, a.connFail)
				}
				if _, saved := a.storedHost(a.host.ID); saved && widget.GhostButton(c, "Edit host…").Clicked() {
					a.openEditHost(a.host)
				}
				if a.host.ID != hosts.LocalID && widget.GhostButton(c, "Back to "+LocalHost().Name).Clicked() {
					a.connect(LocalHost())
				}
				if widget.PrimaryButton(c, nil, "Retry", "").Clicked() {
					a.connect(a.host)
				}
			})
		})
		return true
	}
	return false
}

func hostIcon(h hosts.Host) *ui.SVG {
	if h.IsDevice() {
		return icon.Laptop
	}
	return icon.Server
}

// hostsPage lists the saved servers and, in a card of its own, the tailnet's
// other devices.
func (a *App) hostsPage(c *ui.Context) {
	widget.PageHeader(c, "Hosts", "All devices, saved on this device", func() {
		if a.opts.Hosts != nil {
			b := widget.IconButton(c, icon.More, "Import and export", false)
			a.menu.OpenBelow(b, "hosts-list", theme.ControlHeight)
			a.menu.Show(c, b, "hosts-list", func(done func()) {
				if widget.MenuItem(c, icon.Download, "Import…", false) {
					go a.importHosts()
					done()
				}
				if widget.MenuItem(c, icon.External, "Export…", false) {
					go a.exportHosts()
					done()
				}
			})
			if widget.GhostButton(c, "Create a server…").Tooltip("At Hetzner, DigitalOcean or Vultr, with your own API token").Clicked() {
				a.openCreateServer()
			}
		}
		if widget.PrimaryButton(c, icon.Plus, "Add VPS", "").Clicked() {
			a.openAddHost()
		}
	})
	if !a.hostsPageShown {
		a.hostsPageShown = true
		go a.loadPeers()
	}
	list := a.allHosts()
	widget.FigsBand(c, a.hostsFigures(list))
	widget.FillGrid(c, 3, len(list)+1, hostCardMinHeight, func() {
		for _, h := range list {
			a.hostCard(c.Key(h.ID), h)
		}
		a.tailnetCard(c.Key("tailnet"))
	})
}

// hostCardMinHeight is what a host's card needs: its status, its figures and
// its actions. Under two rows' worth of frame the page scrolls.
const hostCardMinHeight = 228

// hostsFigures sums the hosts up.
func (a *App) hostsFigures(list []hosts.Host) []widget.Fig {
	connected, sessions, failed := 0, 0, 0
	for _, h := range list {
		if a.hostFaceOf(h).failed {
			failed++
		}
		if st, ok := a.statusOfHost(h); ok && st.state == hostOnline {
			connected++
			sessions += sessionCount(st)
		}
	}
	return []widget.Fig{
		{Label: "Hosts", Value: strconv.Itoa(len(list))},
		{Label: "Connected", Value: strconv.Itoa(connected)},
		{Label: "Sessions", Value: strconv.Itoa(sessions)},
		{Label: "Couldn't connect", Value: strconv.Itoa(failed)},
		{Label: "On your tailnet", Value: strconv.Itoa(len(a.newPeers()))},
	}
}

func sessionCount(st hostStatus) int {
	n := 0
	for _, s := range st.sessions {
		if !s.Exited {
			n++
		}
	}
	return n
}

// hostFace is how a host's card reads now.
type hostFace struct {
	color  ui.Color
	state  string
	tip    string
	failed bool
	err    string
}

// hostFaceOf says how host h is: from the connection shown for the current
// host, else from its watch.
func (a *App) hostFaceOf(h hosts.Host) hostFace {
	if h.ID == a.host.ID {
		switch {
		case a.connecting != "":
			return hostFace{color: theme.Warning, state: "Connecting…"}
		case a.client != nil:
			return hostFace{color: theme.Done, state: "Connected"}
		case a.connErr != "" && !strings.HasPrefix(a.connErr, "Disconnected from "):
			return hostFace{color: theme.Error, state: "Couldn't connect", failed: true, err: a.connErr}
		case a.connErr != "":
			return hostFace{color: theme.Idle, state: "Not connected", err: a.connErr}
		}
		return hostFace{color: theme.Idle, state: "Not connected"}
	}
	st, ok := a.statusOfHost(h)
	if !ok {
		return hostFace{color: theme.Idle, state: "Not connected"}
	}
	switch st.state {
	case hostOnline:
		return hostFace{color: hostColor(st), state: "Connected", tip: hostState(st)}
	case hostConnecting:
		return hostFace{color: theme.Warning, state: "Connecting…"}
	case hostNeedsYou:
		return hostFace{color: theme.Warning, state: "Not connected", tip: hostState(st)}
	}
	if st.err != "" {
		return hostFace{color: theme.Error, state: "Couldn't connect", failed: true, err: st.err, tip: hostState(st)}
	}
	return hostFace{color: theme.Idle, state: "Not connected", tip: hostState(st)}
}

// hostLine is where a host is and how the app gets there.
func hostLine(h hosts.Host) string {
	if h.ID == hosts.LocalID {
		return "Local"
	}
	return "SSH · " + h.Address
}

// hostDaemon is the version of the daemon on host h, once known.
func (a *App) hostDaemon(h hosts.Host) string {
	if h.ID == a.host.ID {
		return a.daemonInfo.Version
	}
	if w := a.watches[h.ID]; w != nil {
		return w.info.Version
	}
	return ""
}

// hostFigures are a connected host's figures and how full its memory is; its
// CPU and disk show in the meter's tooltip.
func (a *App) hostFigures(c *ui.Context, h hosts.Host, st hostStatus) {
	u := st.usage
	memory, used := "-", 0.0
	if u.MemTotal > 0 {
		used = float64(u.MemUsed) / float64(u.MemTotal)
		memory = fmt.Sprintf("%.0f%%", used*100)
	}
	version := a.hostDaemon(h)
	if version == "" {
		version = "-"
	}
	widget.CardStats(c, []widget.Stat{
		{Value: strconv.Itoa(sessionCount(st)), Label: "sessions"},
		{Value: memory, Label: "memory"},
		{Value: version, Label: "daemon"},
	})
	ui.Column(c.Key("meter")).FillWidth().Shrink(0).Tooltip(hostUsageTip(u)).Children(func() {
		widget.Meter(c, widget.Segment{Fraction: used, Color: theme.Working})
	})
}

// hostCard is a host of the Hosts page with what can be done to it.
func (a *App) hostCard(c *ui.Context, h hosts.Host) {
	current := h.ID == a.host.ID
	face := a.hostFaceOf(h)
	card := widget.GridCard(c)
	if face.failed {
		card.Background(theme.Fade(theme.Error, 0.13))
	}
	card.Children(func() {
		widget.CardTitle(c, hostIcon(h), h.Name, nil)
		status := ui.Row(c.Key("status")).FillWidth().Shrink(0)
		if face.tip != "" {
			status.Tooltip(face.tip)
		}
		status.Children(func() { widget.StatusLine(c, face.color, false, face.state) })
		if face.err != "" {
			widget.CardNote(c.Key("err"), face.err, 2).Shrink(0)
		} else {
			widget.CardNote(c.Key("line"), hostLine(h), 1).Shrink(0)
		}
		if note := runAsNote(h); note != "" {
			widget.CardNote(c.Key("run-as"), note, 1).Shrink(0)
		}
		if st, ok := a.statusOfHost(h); ok && st.state == hostOnline {
			a.hostFigures(c, h, st)
		}
		ui.Spacer(c)
		widget.CardFoot(c, func() { a.hostActions(c, h, current, face) })
	})
}

// hostActions are a host card's buttons, then what is done now and then behind
// its More.
func (a *App) hostActions(c *ui.Context, h hosts.Host, current bool, face hostFace) {
	if current && h.ID != hosts.LocalID && a.client != nil {
		label := "Set up…"
		if n := a.drift[h.ID]; n > 0 {
			label = fmt.Sprintf("Set up · %d to bring", n)
		}
		if widget.GhostButton(c, label).Tooltip("Bring it to what this device has: toolchains, git, gh, agents").Clicked() {
			a.openSetup()
		}
		if widget.GhostButton(c, "Sync environment").Tooltip("Skills, subagents, commands, MCP servers and settings").Clicked() {
			a.openSync()
		}
	}
	_, saved := a.storedHost(h.ID)
	switch {
	case face.failed:
		if widget.PrimaryButton(c, nil, "Retry", "").Clicked() {
			a.page = ""
			a.connect(h)
		}
		if saved && widget.GhostButton(c, "Edit host…").Clicked() {
			a.openEditHost(h)
		}
	case !current:
		var b ui.Element
		if st, ok := a.statusOfHost(h); ok && st.state == hostOnline {
			b = widget.GhostButton(c, "Connect")
		} else {
			b = widget.PrimaryButton(c, nil, "Connect", "")
		}
		if b.Clicked() {
			a.page = ""
			a.connect(h)
		}
	}
	ui.Spacer(c)
	if saved {
		widget.MoreMenu(c, &a.menu, "host:"+h.ID, func(done func()) { a.hostMenu(c, h, done) })
	}
}

func (a *App) hostMenu(c *ui.Context, h hosts.Host, done func()) {
	if widget.MenuItem(c, icon.Pencil, "Edit…", false) {
		a.openEditHost(h)
		done()
	}
	if (h.RunAs != "" || h.KeepRoot) && widget.MenuItemTip(c, icon.Agent("shell"), "Choose user again", "Ask which user to run as at the next connection", false) {
		h.RunAs, h.KeepRoot = "", false
		a.chooseRunAs = h.ID // the next connection asks, rather than picking
		if err := a.opts.Hosts.Update(h); err != nil {
			a.status = err.Error()
		}
		a.reloadHosts()
		done()
	}
	widget.MenuSeparator(c)
	if p, ok := vps.Find(h.Provider); ok && h.ProviderID != "" && widget.MenuItemTip(c, icon.Trash, "Delete server…", "Delete the server at "+p.Name+": it and its disk are gone", true) {
		a.deletingServer = &h
		done()
	}
	if widget.MenuItem(c, icon.Trash, "Remove from the list", true) {
		a.askRemoveHost(h)
		done()
	}
	if h.ID != hosts.LocalID && widget.MenuItemTip(c, icon.Trash, "Remove & clear…", "Remove it from the list and clear the data synced to it", true) {
		a.clearingHost = &h
		done()
	}
}

// tailnetCard lists the tailnet's devices not saved yet, or why there are none.
func (a *App) tailnetCard(c *ui.Context) {
	widget.GridCard(c).Children(func() {
		widget.CardTitle(c, nil, "On your tailnet", nil)
		if note := a.tailnetNote(); note != "" {
			widget.CardNote(c.Key("note"), note, 0)
		}
		ui.Scroll(c.Key("peers")).Grow(1).FillWidth().MinHeight(0).Children(func() {
			widget.CardLines(c, func() {
				for i, p := range a.newPeers() {
					widget.CardLine(c.Key(p.Address()), i == 0, p.HostName, p.Address()+" · "+p.OS, func() {
						widget.Dot(c, peerColor(p.Online), !p.Online, 8)
						if widget.GhostButton(c, "Connect").Clicked() {
							a.page = ""
							a.connectPeer(p)
						}
					})
				}
			})
		})
	})
}

func (a *App) trustDialog(c *ui.Context) {
	if a.trust == nil || a.connectByApp {
		return
	}
	open := true
	widget.Dialog(c, &open, 520, 14, func() {
		widget.DialogHeader(c, "Trust this server?", "", false, "Hosts", a.host.Name, "Trust server")
		trustFacts(c, a.trust)
		widget.DialogFooter(c, "", func() {
			if widget.GhostButton(c, "Cancel").Clicked() {
				a.answerTrust(false)
			}
			if widget.PrimaryButton(c, nil, "Trust and connect", "").Clicked() {
				a.answerTrust(true)
			}
		})
	})
	if !open {
		a.answerTrust(false)
	}
}

// trustAsk is trustDialog on a connection the user didn't start.
func (a *App) trustAsk(c *ui.Context) *widget.Ask {
	t := a.trust
	if t == nil || !a.connectByApp {
		return nil
	}
	return &widget.Ask{Key: "trust:" + t.addr, Title: "Trust this server?", Body: func() { trustFacts(c, t) }, Buttons: []widget.AskButton{
		{Kind: widget.Secondary, Label: "Cancel", Do: func() { a.answerTrust(false) }},
		{Kind: widget.Primary, Label: "Trust and connect", Do: func() { a.answerTrust(true) }},
	}}
}

func trustFacts(c *ui.Context, t *trustPrompt) {
	ui.Text(c, "This is the first connection to "+t.addr+". Check that its key fingerprint matches the server's before trusting it:").
		FontSize(12.5).TextColor(theme.TextSoft)
	ui.Text(c, t.fingerprint).Font(theme.MonoFont).FontSize(theme.TextLabel).Padding(10).Radius(theme.RadiusControl).
		Background(theme.Sunken).Border(1, theme.Border).FillWidth()
	ui.Text(c, "On the server: for f in /etc/ssh/ssh_host_*_key.pub; do ssh-keygen -lf $f; done").Font(theme.MonoFont).FontSize(theme.TextLabel).TextColor(theme.Muted)
}

// exportHosts saves the host list to a file the user picks, for another
// device; it runs off the main thread, as the dialog waits.
func (a *App) exportHosts() {
	data, err := a.opts.Hosts.Export()
	if err == nil {
		var path string
		path, err = mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: a.win, Title: "Export the host list",
			DefaultPath: "enowx-hosts.json", Message: "The hosts without their passwords, which stay in this device's keychain."})
		if err != nil || path == "" {
			return
		}
		err = os.WriteFile(path, data, 0o600)
		if err == nil {
			a.onMain(func() { a.notify("Host list exported to " + path) })
			return
		}
	}
	a.onMain(func() { a.status = "Couldn't export the host list: " + err.Error() })
}

// importHosts adds the hosts of a list file the user picks; it runs off
// the main thread.
func (a *App) importHosts() {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.win, Title: "Import a host list",
		Filters: []mygo.FileFilter{{Name: "Host list", Extensions: []string{"json"}}}})
	if err != nil || len(paths) == 0 {
		return
	}
	data, err := os.ReadFile(paths[0])
	n := 0
	if err == nil {
		n, err = a.opts.Hosts.Import(data)
	}
	a.onMain(func() {
		a.reloadHosts()
		if err != nil {
			a.status = "Couldn't import the host list: " + err.Error()
			return
		}
		a.notify(textfmt.Plural(n, "A host", fmt.Sprintf("%d hosts", n)) + " added")
	})
}

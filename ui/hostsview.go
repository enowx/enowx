package ui

import (
	"errors"
	"fmt"
	"os"
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

func hostTile(c *ui.Context, h hosts.Host) { widget.Avatar(c, hostIcon(h), 30, theme.TextSoft) }

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
		a.viewHeader(c, func() { widget.Breadcrumb(c, a.host.Name, "Connecting") })
		ui.Column(c).Grow(1).FillWidth().Center().Children(func() {
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
		a.viewHeader(c, func() { widget.Breadcrumb(c, a.host.Name, crumb) })
		ui.Column(c).Grow(1).FillWidth().Center().Children(func() {
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

// hostsPage lists the saved servers and the tailnet's other devices.
func (a *App) hostsPage(c *ui.Context) {
	a.viewHeader(c, func() {
		ui.Row(c).Grow(1).MinWidth(0).Children(func() { widget.Breadcrumb(c, "Hosts", "All devices") })
		if a.opts.Hosts != nil {
			if widget.GhostButton(c, "Import…").Clicked() {
				go a.importHosts()
			}
			if widget.GhostButton(c, "Export…").Clicked() {
				go a.exportHosts()
			}
		}
		if a.opts.Hosts != nil && widget.GhostButton(c, "Create a server…").Tooltip("At Hetzner, DigitalOcean or Vultr, with your own API token").Clicked() {
			a.openCreateServer()
		}
		if widget.PrimaryButton(c, icon.Plus, "Add VPS", "").Clicked() {
			a.openAddHost()
		}
		if widget.IconButton(c, icon.Close, "Back to sessions", false).Clicked() {
			a.page = ""
		}
	})
	if !a.hostsPageShown {
		a.hostsPageShown = true
		go a.loadPeers()
	}
	ui.Scroll(c).Grow(1).FillWidth().Children(func() {
		ui.Column(c).FillWidth().Padding(16).Gap(12).Children(func() {
			widget.SectionLabel(c, "HOSTS", theme.Muted)
			ui.Text(c, "Saved on this device in ~/"+transport.HomeName+"/hosts.json. Removing a host leaves its server and sessions untouched.").
				FontSize(12).TextColor(theme.Muted)
			widget.FitGrid(c, 340, 3).Gap(8).Children(func() {
				for _, h := range a.allHosts() {
					a.hostCard(c.Key(h.ID), h)
				}
			})
			widget.SectionLabel(c, "ON YOUR TAILNET", theme.Muted).Margin(10, 0, 0, 0)
			if note := a.tailnetNote(); note != "" {
				ui.Text(c, note).FontSize(theme.TextLabel).TextColor(theme.Muted)
			}
			widget.FitGrid(c, 340, 3).Gap(8).Children(func() {
				for _, p := range a.newPeers() {
					widget.Card(c.Key(p.Address())).Children(func() {
						ui.Row(c).FillWidth().Gap(10).Children(func() {
							widget.Avatar(c, icon.Tailscale, 30, theme.TextSoft)
							ui.Column(c).Gap(1).Grow(1).MinWidth(0).Children(func() {
								ui.Text(c, p.HostName).SingleLine()
								ui.Text(c, p.OS+" · "+p.Address()).Font(theme.MonoFont).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine()
							})
							widget.Dot(c, peerColor(p.Online), !p.Online, 6)
							if widget.GhostButton(c, "Connect").Clicked() {
								a.page = ""
								a.connectPeer(p)
							}
						})
					})
				}
			})
		})
	})
}

// hostCard is a host of the Hosts page with what can be done to it.
func (a *App) hostCard(c *ui.Context, h hosts.Host) {
	current := h.ID == a.host.ID
	card := widget.Card(c)
	if current {
		card.Border(1, theme.AccentLine)
	}
	card.Children(func() {
		ui.Row(c).FillWidth().Gap(10).Children(func() {
			hostTile(c, h)
			ui.Column(c).Gap(1).Grow(1).MinWidth(0).Children(func() {
				ui.Text(c, h.Name).SingleLine()
				ui.Text(c, hostDetail(h)).Font(theme.MonoFont).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine()
				if note := runAsNote(h); note != "" {
					ui.Text(c, note).FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine()
				}
			})
			if current {
				col, state := a.connectionState()
				ui.Row(c).Gap(6).Shrink(0).Children(func() {
					widget.Dot(c, col, false, 6)
					ui.Text(c, state).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine().MaxWidth(160)
				})
			} else if st, ok := a.statusOfHost(h); ok {
				ui.Row(c).Gap(6).Shrink(0).Children(func() {
					col := hostColor(st)
					widget.Dot(c, col, col == (ui.Color{}), 6)
					ui.Text(c, hostState(st)).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine().MaxWidth(200)
				})
			}
		})
		if st, ok := a.statusOfHost(h); ok && st.state == hostOnline {
			hostUsageRow(c, st)
		} else if ok && st.err != "" && st.state != hostConnecting {
			ui.Text(c, st.err).FontSize(theme.TextLabel).TextColor(theme.Muted).MaxLines(2)
		}
		ui.Row(c).FillWidth().Gap(6).Justify(ui.End).Children(func() {
			// What is done now and then waits behind ⋯; the card keeps
			// what it is for.
			if _, saved := a.storedHost(h.ID); saved {
				widget.MoreMenu(c, &a.menu, "host:"+h.ID, func(done func()) {
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
				})
			}
			ui.Spacer(c)
			if current && h.ID != hosts.LocalID && a.client != nil {
				label := "Set up…"
				if n := a.drift[h.ID]; n > 0 {
					label = fmt.Sprintf("Set up · %d to bring", n)
				}
				if widget.GhostButton(c, label).Tooltip("Bring it to what this device has: toolchains, git, gh, agents").Clicked() {
					a.openSetup()
				}
			}
			if !current && widget.GhostButton(c, "Connect").Clicked() {
				a.page = ""
				a.connect(h)
			}
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

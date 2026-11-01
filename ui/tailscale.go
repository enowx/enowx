package ui

import (
	"context"
	"errors"
	"os/user"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/hosts"
	"github.com/enowx/enowx/core/tailnet"
	"github.com/enowx/enowx/transport"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// loadPeers lists the tailnet's devices; it runs off the main thread.
func (a *App) loadPeers() {
	peers, err := tailnet.Peers(context.Background())
	a.onMain(func() {
		a.peers, a.peersErr, a.peersLoaded = peers, "", true
		switch {
		case errors.Is(err, tailnet.ErrNotInstalled):
			a.peersErr = "Tailscale isn't installed on this device. Install it to pick your servers from your tailnet here."
		case err != nil:
			a.peersErr = "Couldn't read Tailscale: " + strings.TrimPrefix(err.Error(), "tailnet: ")
		}
	})
}

// tailnetNote says why the switcher lists no tailnet device, if it doesn't.
func (a *App) tailnetNote() string {
	switch {
	case !a.peersLoaded:
		return "Looking for Tailscale…"
	case a.peersErr != "":
		return a.peersErr
	case len(a.peers) == 0:
		return "No other devices on your tailnet."
	case len(a.newPeers()) == 0:
		return "Every device on your tailnet is saved above."
	}
	return ""
}

func (a *App) openAddHost() {
	a.addHost = addHostForm{open: true}
	go a.loadPeers()
}

// savedPeer returns the saved host that points at a tailnet device, by its
// IP or its MagicDNS name.
func (a *App) savedPeer(p tailnet.Peer) (hosts.Host, bool) {
	for _, h := range a.hostList {
		t, err := transport.ParseSSHTarget(h.Address)
		if err != nil {
			continue
		}
		if (p.IP != "" && t.Host == p.IP) || (p.DNSName != "" && strings.EqualFold(strings.TrimSuffix(t.Host, "."), p.DNSName)) {
			return h, true
		}
	}
	return hosts.Host{}, false
}

// newPeers are the tailnet's devices not saved as hosts yet.
func (a *App) newPeers() []tailnet.Peer {
	var out []tailnet.Peer
	for _, p := range a.peers {
		if _, saved := a.savedPeer(p); !saved {
			out = append(out, p)
		}
	}
	return out
}

// connectPeer saves a tailnet device as a host, signing in as the tailnet
// user, and connects to it; a device saved already is just opened.
func (a *App) connectPeer(p tailnet.Peer) {
	if h, ok := a.savedPeer(p); ok {
		a.addHost.open = false
		a.connect(a.peerByIP(h, p))
		return
	}
	if a.opts.Hosts == nil {
		return
	}
	// Without a user, ~/.ssh/config's applies, else this machine's.
	addr := p.Address()
	if u := strings.TrimSpace(a.tailUser); u != "" {
		addr = u + "@" + addr
	}
	h, err := a.opts.Hosts.Add(hosts.Host{Name: p.HostName, Address: addr, Kind: peerKind(p)})
	if err != nil {
		a.addHost.err = err.Error()
		return
	}
	a.addHost.open = false
	a.reloadHosts()
	a.connect(h)
}

// tailnetSection lists the tailnet's devices in the Add VPS dialog, each
// one click away from being connected.
func (a *App) tailnetSection(c *ui.Context) {
	ui.Column(c).FillWidth().Gap(8).Children(func() {
		ui.Row(c).FillWidth().Gap(8).Children(func() {
			ui.Icon(c, icon.Tailscale).FontSize(14).TextColor(theme.TextSoft)
			ui.Text(c, "On your tailnet").FontSize(theme.TextLabel).TextColor(theme.TextSoft).Grow(1)
			ui.Text(c, "Sign in as").FontSize(theme.TextLabel).TextColor(theme.Muted)
			widget.Input(c, &a.tailUser, localUser()).Font(theme.MonoFont).Width(120)
		})
		ui.Scroll(c).FillWidth().MaxHeight(260).Children(func() {
			ui.Column(c).FillWidth().Gap(4).Children(func() {
				for _, p := range a.peers {
					a.peerRow(c.Key(p.Address()), p)
				}
			})
		})
	})
}

func (a *App) peerRow(c *ui.Context, p tailnet.Peer) {
	saved, isSaved := a.savedPeer(p)
	ui.Row(c).FillWidth().Gap(10).Padding(8, 10).Radius(theme.RadiusSmall).Background(theme.Raised).Children(func() {
		ui.Box(c).Size(28, 28).Radius(theme.RadiusSmall).Background(theme.Selected).Center().TextColor(theme.TextSoft).Children(func() {
			ui.Icon(c, icon.Server).FontSize(14)
		})
		ui.Column(c).Gap(1).Grow(1).MinWidth(0).Children(func() {
			ui.Row(c).Gap(6).Children(func() {
				if p.Online {
					widget.Dot(c, theme.Done, false, 7)
				} else {
					widget.Dot(c, theme.Idle, true, 7)
				}
				ui.Text(c, p.HostName).SingleLine()
			})
			detail := p.OS + " · " + p.Address()
			if !p.Online {
				detail = "offline · " + detail
			}
			ui.Text(c, detail).Font(theme.MonoFont).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine()
		})
		label := "Connect"
		if isSaved {
			label = "Open"
		}
		var b ui.Element
		if p.Online {
			b = widget.PrimaryButton(c, icon.Server, label, "")
		} else {
			b = widget.GhostButton(c, label)
		}
		if b.Clicked() {
			if isSaved {
				a.addHost.open = false
				a.connect(saved)
			} else {
				a.connectPeer(p)
			}
		}
	})
}

func localUser() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	return "root"
}

// peerByIP moves a host saved by MagicDNS name to the device's Tailscale
// IP, which needs no DNS.
func (a *App) peerByIP(h hosts.Host, p tailnet.Peer) hosts.Host {
	t, err := transport.ParseSSHTarget(h.Address)
	if err != nil || p.IP == "" || t.Host == p.IP || a.opts.Hosts == nil {
		return h
	}
	t.Host = p.IP
	h.Address = t.String()
	if err := a.opts.Hosts.Update(h); err == nil {
		a.reloadHosts()
	}
	return h
}

// peerKind guesses what a tailnet device is: a Mac or a PC is one of the
// user's computers, anything else a server.
func peerKind(p tailnet.Peer) string {
	switch strings.ToLower(p.OS) {
	case "macos", "windows":
		return hosts.KindDevice
	}
	return ""
}

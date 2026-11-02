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
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// loadPeers lists the tailnet's devices; it runs off the main thread.
func (a *App) loadPeers() {
	if devSample() {
		return
	}
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
	ui.Column(c.Key("tailnet")).FillWidth().Shrink(0).Gap(theme.S2).Children(func() {
		ui.Row(c.Key("sign-in-as")).FillWidth().Shrink(0).Gap(theme.S2).Children(func() {
			ui.Spacer(c)
			ui.Text(c, "Sign in as").FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine()
			ui.Column(c).Width(160).Shrink(0).Children(func() { widget.Input(c, &a.tailUser, localUser()).FillWidth() })
		})
		widget.Scroll(c.Key("peers")).FillWidth().MaxHeight(260).Children(func() {
			widget.CardLines(c, func() {
				for i, p := range a.peers {
					a.peerRow(c.Key(p.Address()), p, i == 0)
				}
			})
		})
	})
}

func (a *App) peerRow(c *ui.Context, p tailnet.Peer, first bool) {
	saved, isSaved := a.savedPeer(p)
	detail := p.OS + " · " + p.Address()
	if !p.Online {
		detail = "offline · " + detail
	}
	r := ui.Row(c).FillWidth().Shrink(0).MinHeight(44).Padding(theme.S1, 0).Gap(theme.S2)
	if !first {
		r.BorderWidth(0.5, 0, 0, 0).BorderColor(theme.Border)
	}
	r.Children(func() {
		widget.Dot(c, peerColor(p.Online), !p.Online, 8)
		ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
			widget.Running(c, p.HostName, widget.RunStyle{})
			widget.Running(c.Key("detail"), detail, widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
		})
		label := "Connect"
		if isSaved {
			label = "Open"
		}
		var b ui.Element
		if p.Online {
			b = widget.PrimaryButton(c, nil, label, "")
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

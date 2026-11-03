package ui

import (
	"context"
	"fmt"
	"github.com/enowx/enowx/ui/widget"
	"net"
	"os"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/hosts"
	"github.com/enowx/enowx/core/vps"
)

// createServer creates the server, waits until it is up and its SSH
// answers, then adds it to the hosts and connects.
func (a *App) createServer(f *vpsForm) {
	c, err := a.vpsClient(f)
	if err != nil {
		f.err = err.Error()
		return
	}
	pub, err := os.ReadFile(f.key)
	if err != nil {
		f.err = err.Error()
		return
	}
	if f.remember && !f.fromVault {
		_ = saveToken(f)
	}
	f.busy, f.err = "Creating the server…", ""
	p := vps.CreateParams{Name: strings.TrimSpace(f.name), Region: f.region, Size: f.size, PublicKey: string(pub)}
	provider := f.provider
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		s, err := c.Create(ctx, p)
		if err == nil {
			s, err = vps.WaitReady(ctx, c, s.ID, 3*time.Second, func(s vps.Server) {
				a.onMain(func() { f.busy = "Waiting for it to start (" + s.Status + ")…" })
			})
		}
		if err == nil && s.SSH == "" {
			a.onMain(func() { f.busy = "Waiting for its SSH to answer…" })
			err = waitSSH(ctx, s.IPv4)
		}
		a.onMain(func() {
			if err != nil {
				f.busy, f.err = "", err.Error()
				return
			}
			f.busy = "Adding it to your hosts…"
			address := "root@" + s.IPv4
			if s.SSH != "" {
				address = s.SSH // a gateway's, as the machine's own user
			}
			h, err := a.opts.Hosts.Add(hosts.Host{Name: p.Name, Address: address, Provider: provider, ProviderID: s.ID})
			f.busy = ""
			if err != nil {
				f.err = err.Error()
				return
			}
			f.open = false
			a.reloadHosts()
			a.page = ""
			a.connect(h)
		})
	}()
}

// waitSSH waits until the server's SSH port takes connections.
func waitSSH(ctx context.Context, ip string) error {
	for {
		conn, err := net.DialTimeout("tcp", net.JoinHostPort(ip, sshPort), 5*time.Second)
		if err == nil {
			conn.Close()
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("the server's SSH never answered: %w", err)
		case <-time.After(3 * time.Second):
		}
	}
}

// deleteServerDialog asks before deleting a server at its provider, which
// can't be undone.
func (a *App) deleteServerDialog(c *ui.Context) {
	h := a.deletingServer
	if h == nil {
		return
	}
	p, _ := vps.Find(h.Provider)
	open := true
	msg := "Its disk and everything on it go for good, sessions included, and " + p.Name + " stops billing for it. enowX removes it from your hosts."
	switch widget.AlertDialog(c, &open, "Delete "+h.Name+" at "+p.Name+"?", msg, "Cancel", "Delete the server") {
	case 1:
		a.deleteServer(*h)
		a.deletingServer = nil
	case 0, ui.AlertDismissed:
		a.deletingServer = nil
	}
	if !open {
		a.deletingServer = nil
	}
}

// deleteServer deletes host h's server at its provider, then removes it
// from the hosts. The provider's token comes from the vault or the keychain.
func (a *App) deleteServer(h hosts.Host) {
	go func() {
		tok, err := a.providerToken(h)
		if err == nil {
			var c vps.Client
			if c, err = vps.New(h.Provider, tok, vpsAPI); err == nil {
				ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
				err = c.Delete(ctx, h.ProviderID)
				cancel()
			}
		} else {
			err = fmt.Errorf("%w: delete the server in its console", err)
		}
		a.onMain(func() {
			if err != nil {
				a.status = err.Error()
				return
			}
			_ = a.opts.Hosts.Remove(h.ID)
			a.reloadHosts()
			if a.host.ID == h.ID {
				a.connect(LocalHost())
			}
			a.notify(h.Name + " was deleted.")
		})
	}()
}

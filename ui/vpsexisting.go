package ui

import (
	"context"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/hosts"
	"github.com/enowx/enowx/core/vps"
	"github.com/enowx/enowx/ui/widget"
)

// Create a server's other way, for providers that list the user's servers
// (vps.Lister, InstaBox): connect to one they made already, in its console
// or before, which enowX adds to the hosts like one it created.

// loadServers reads the user's servers at f's provider.
func (a *App) loadServers(f *vpsForm, l vps.Lister) {
	provider := f.provider
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		list, err := l.Servers(ctx)
		a.onMain(func() {
			if f.provider != provider {
				return
			}
			if err != nil {
				f.err = err.Error()
				return
			}
			f.servers, f.listed = list, true
			if len(list) > 0 && f.pick == "" {
				f.pick = list[0].ID
			}
		})
	}()
}

// existingServers lists the user's servers to pick one.
func (a *App) existingServers(c *ui.Context, f *vpsForm) {
	widget.Field(c, "Yours", func() {
		if len(f.servers) == 0 {
			widget.CardNote(c, "You have none yet: make a new one.", 0)
			return
		}
		opts := make([]vps.Option, len(f.servers))
		for i, s := range f.servers {
			detail := s.Status
			if s.Size != "" {
				detail += " · " + s.Size
			}
			if h, ok := a.hostOfServer(f.provider, s.ID); ok {
				detail += " · in your hosts as " + h.Name
			}
			opts[i] = vps.Option{ID: s.ID, Name: s.Name, Detail: detail}
		}
		optionSelect(c, &f.pick, opts, "Machine")
		widget.CardNote(c.Key("wakes"), "A paused or stopped one is woken as enowX connects.", 0)
	})
	if f.key == "" {
		a.vpsKeyField(c, f, "")
		return
	}
	widget.Field(c, "SSH key", func() { widget.CardNote(c.Key("key"), f.key, 0) })
}

// hostOfServer is the host of server id at provider, when it is one.
func (a *App) hostOfServer(provider, id string) (hosts.Host, bool) {
	if a.opts.Hosts == nil {
		return hosts.Host{}, false
	}
	for _, h := range a.opts.Hosts.List() {
		if h.Provider == provider && h.ProviderID == id {
			return h, true
		}
	}
	return hosts.Host{}, false
}

// useServer connects to the server picked: its host when it has one, else
// a new one, where its provider says to sign in.
func (a *App) useServer(f *vpsForm) {
	if h, ok := a.hostOfServer(f.provider, f.pick); ok {
		f.open = false
		a.page = ""
		a.connect(h)
		return
	}
	c, err := a.vpsClient(f)
	if err != nil {
		f.err = err.Error()
		return
	}
	if f.remember && !f.fromVault {
		_ = saveToken(f)
	}
	f.busy, f.err = "Reading where to sign in…", ""
	provider, id := f.provider, f.pick
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		s, err := c.Server(ctx, id)
		a.onMain(func() {
			f.busy = ""
			if err == nil && s.SSH == "" && s.IPv4 == "" {
				err = errNoAddress
			}
			if err != nil {
				f.err = err.Error()
				return
			}
			address := s.SSH
			if address == "" {
				address = "root@" + s.IPv4
			}
			h, err := a.opts.Hosts.Add(hosts.Host{Name: strings.TrimSpace(s.Name), Address: address, Provider: provider, ProviderID: s.ID})
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

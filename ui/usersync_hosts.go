package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/hosts"
	"github.com/enowx/enowx/core/usersync"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/transport"
)

// The device's hosts are shared too, in the synced settings/hosts.json:
// a server added on one computer shows on the others, a server removed
// is removed there too. Passwords stay in each device's keychain.

const hostsFile = "settings/hosts.json"

// sharedHosts is settings/hosts.json: the hosts, and those removed.
type sharedHosts struct {
	Hosts   []hosts.Host `json:"hosts"`
	Removed []string     `json:"removed,omitempty"`
}

// shareHosts merges the hub's shared hosts into this device's list, then
// writes the list back; it runs off the main thread after a sync.
func (a *App) shareHosts(hub *client.Client) {
	store := a.opts.Hosts
	if store == nil {
		return
	}
	side := client.SyncSide{C: hub}
	got, err := side.Read([]string{hostsFile})
	if err != nil {
		return
	}
	var shared sharedHosts
	expect := ""
	if data, ok := got[hostsFile]; ok {
		expect = usersync.Hash(data)
		if json.Unmarshal(data, &shared) != nil {
			return
		}
	}
	removed := map[string]bool{}
	for _, id := range shared.Removed {
		removed[id] = true
	}
	for _, id := range a.removedHosts() {
		removed[id] = true
	}
	changed := false
	for _, h := range shared.Hosts {
		if _, mine := store.Get(h.ID); !mine && !removed[h.ID] && h.ID != hosts.LocalID {
			if _, err := store.Add(h); err == nil {
				changed = true
			}
		}
	}
	for id := range removed {
		if _, mine := store.Get(id); mine {
			if store.Remove(id) == nil {
				changed = true
			}
		}
	}
	out := sharedHosts{Hosts: store.List()}
	for id := range removed {
		out.Removed = append(out.Removed, id)
	}
	slices.Sort(out.Removed)
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return
	}
	if usersync.Hash(data) != expect {
		_, _, _ = side.Write([]proto.SyncChange{{Path: hostsFile, Data: data, Expect: expect}})
	}
	if changed {
		a.onMain(a.reloadHosts)
	}
}

// removedHosts are the hosts removed on this device, to remove on the
// others.
func (a *App) removedHosts() []string {
	dir, err := transport.Dir()
	if err != nil {
		return nil
	}
	var ids []string
	if data, err := os.ReadFile(filepath.Join(dir, "usersync", "removed-hosts.json")); err == nil {
		_ = json.Unmarshal(data, &ids)
	}
	return ids
}

// hostRemoved records that host id was removed here, for the others.
func (a *App) hostRemoved(id string) {
	dir, err := transport.Dir()
	if err != nil {
		return
	}
	ids := a.removedHosts()
	if !slices.Contains(ids, id) {
		ids = append(ids, id)
	}
	data, _ := json.Marshal(ids)
	if os.MkdirAll(filepath.Join(dir, "usersync"), 0o700) == nil {
		_ = os.WriteFile(filepath.Join(dir, "usersync", "removed-hosts.json"), data, 0o600)
	}
	a.syncSoon()
}

// clearHostDialog asks before taking the synced data off a host being
// removed from the list.
func (a *App) clearHostDialog(c *ui.Context) {
	h := a.clearingHost
	if h == nil {
		return
	}
	open := true
	msg := "Its notes, canvases and settings go to its trash (kept 30 days there); this device and the other hosts keep theirs. Its sessions and project files are left alone."
	switch ui.AlertDialog(c, &open, "Remove "+h.Name+" and clear its synced data?", msg, "Cancel", "Remove and clear") {
	case 1:
		a.clearHost(*h)
		a.clearingHost = nil
	case 0, ui.AlertDismissed:
		a.clearingHost = nil
	}
	if !open {
		a.clearingHost = nil
	}
}

// clearHost takes the synced data off host h, then removes it from the
// list. No sync with it runs meanwhile: one would take its emptied folder
// for deletions to make everywhere.
func (a *App) clearHost(h hosts.Host) {
	var cl *client.Client
	if h.ID == a.host.ID {
		cl = a.client
	} else if w := a.watches[h.ID]; w != nil {
		cl = w.cl
	}
	if cl == nil {
		a.status = h.Name + " isn't connected: connect to it to clear its data, or just remove it"
		return
	}
	if a.dsync.leaving == nil {
		a.dsync.leaving = map[string]bool{}
	}
	a.dsync.leaving[h.ID] = true
	var run func()
	run = func() {
		if a.dsync.running {
			time.AfterFunc(200*time.Millisecond, func() { a.onMain(run) })
			return
		}
		go func() {
			m, err := cl.SyncManifest()
			if err == nil {
				err = cl.SyncForget()
			}
			if err == nil {
				if dir, derr := transport.Dir(); derr == nil {
					_ = usersync.ForgetBase(filepath.Join(dir, "usersync"), m.ID)
				}
			}
			a.onMain(func() {
				delete(a.dsync.leaving, h.ID)
				if err != nil {
					a.status = "Couldn't clear " + h.Name + ": " + client.Explain(err)
					return
				}
				a.removeHost(h)
			})
		}()
	}
	run()
}

// removeHost removes host h from the list, here and on the user's other
// devices.
// askRemoveHost removes h from the list once the user agrees.
func (a *App) askRemoveHost(h hosts.Host) {
	a.confirm("Remove “"+h.Name+"” from the list?", "This device stops connecting to it and showing its sessions. Nothing on the host changes: its daemon and sessions go on, and adding it again brings them back.", "Remove", func() { a.removeHost(h) })
}

func (a *App) removeHost(h hosts.Host) {
	if err := a.opts.Hosts.Remove(h.ID); err != nil {
		a.status = err.Error()
	}
	a.forgetPassword(h)
	a.hostRemoved(h.ID)
	a.reloadHosts()
	if h.ID == a.host.ID {
		a.connect(LocalHost())
	}
}

package ui

import (
	"encoding/json"
	"fmt"
	"maps"
	"path/filepath"
	"strings"
	"time"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/hosts"
	"github.com/enowx/enowx/core/usersync"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/transport"
)

// The user's data (notes, groups, canvases, settings…) is kept the same
// on every host (core/usersync): this device's own daemon is the hub, and
// each other host reached, the one shown or one watched, syncs with it
// when connected, every syncEvery, and soon after a change on either.
// The bases of those syncs live in this device's folder.

// syncEvery is how often the hosts reached sync when nothing asks sooner.
const syncEvery = 30 * time.Second

// Sync states of a host.
const (
	syncIdle    = "synced"
	syncSyncing = "syncing"
	syncFailed  = "failed"
	syncOffline = "offline"
	syncOld     = "update needed" // its daemon has no sync yet
)

// dataSync is the syncs' state, the main thread's.
type dataSync struct {
	running bool
	again   bool
	soon    *time.Timer
	hosts   map[string]hostSync // by host ID
	hub     proto.SyncStatus    // this device's, after the last sync
	ticking bool

	leaving   map[string]bool       // hosts being cleared, not synced
	conflicts []proto.SyncConflict  // on this device, after the last sync that changed anything
	trash     []proto.SyncTrashItem // deleted lately, on this device
	listed    bool                  // conflicts and trash have been read
}

// hostSync is how syncing a host goes.
type hostSync struct {
	state string
	err   string
	last  time.Time
	rep   usersync.Report
	// seen is both hosts' files as this sync found them (usersync.Fingerprint), ran
	// whether it had to do anything about them.
	seen string
	ran  bool
}

// syncPeer is a host to sync with the hub, and its connection.
type syncPeer struct {
	host hosts.Host
	cl   *client.Client
}

// syncSoon syncs in a moment, gathering the changes made meanwhile.
func (a *App) syncSoon() {
	if a.dsync.soon != nil {
		a.dsync.soon.Stop()
	}
	a.dsync.soon = time.AfterFunc(1500*time.Millisecond, func() { a.onMain(a.syncData) })
	if !a.dsync.ticking && a.win != nil {
		a.dsync.ticking = true
		go func() {
			for range time.Tick(syncEvery) {
				a.onMain(a.syncData)
			}
		}()
	}
}

// hubClient is the connection to this device's daemon, nil while there's
// none.
func (a *App) hubClient() *client.Client {
	if a.host.ID == hosts.LocalID {
		return a.client
	}
	if w := a.watches[hosts.LocalID]; w != nil {
		return w.cl
	}
	return nil
}

// syncPeers are the other hosts connected now.
func (a *App) syncPeers() []syncPeer {
	var out []syncPeer
	if a.host.ID != hosts.LocalID && a.client != nil && !a.dsync.leaving[a.host.ID] {
		out = append(out, syncPeer{a.host, a.client})
	}
	for id, w := range a.watches {
		if id != hosts.LocalID && w.cl != nil && !a.dsync.leaving[id] {
			out = append(out, syncPeer{w.host, w.cl})
		}
	}
	return out
}

// syncData syncs every host connected with the hub, one after another,
// off the main thread.
func (a *App) syncData() {
	if a.dsync.running {
		a.dsync.again = true
		return
	}
	hub, peers := a.hubClient(), a.syncPeers()
	if hub == nil || len(peers) == 0 {
		return
	}
	if a.dsync.hosts == nil {
		a.dsync.hosts = map[string]hostSync{}
	}
	before := maps.Clone(a.dsync.hosts)
	listed := a.dsync.listed
	for _, p := range peers {
		st := a.dsync.hosts[p.host.ID]
		st.state = syncSyncing
		a.dsync.hosts[p.host.ID] = st
	}
	a.dsync.running = true
	go func() {
		results := map[string]hostSync{}
		// What's in conflict and deleted only changes with the files: read when they did.
		read := !listed
		for _, p := range peers {
			r := syncWith(hub, p, before[p.host.ID])
			results[p.host.ID] = r
			read = read || r.ran
		}
		a.shareHosts(hub)
		status, _ := hub.SyncStatus()
		var conflicts []proto.SyncConflict
		var trash []proto.SyncTrashItem
		var errC, errT error
		if read {
			conflicts, errC = hub.SyncConflicts()
			trash, errT = hub.SyncTrash()
		}
		a.onMain(func() {
			for id, r := range results {
				a.dsync.hosts[id] = r
			}
			a.dsync.hub = status
			if read {
				a.dsync.conflicts, a.dsync.trash = conflicts, trash
				a.dsync.listed = errC == nil && errT == nil
			}
			a.dsync.running = false
			if a.dsync.again {
				a.dsync.again = false
				a.syncData()
			}
		})
	}()
}

// syncWith syncs host p with the hub, and tells both how it went.
func syncWith(hub *client.Client, p syncPeer, before hostSync) hostSync {
	mh, err := hub.SyncManifest()
	if err != nil {
		return hostSync{state: syncFailed, err: "this device: " + client.Explain(err), last: time.Now()}
	}
	mp, err := p.cl.SyncManifest()
	if err != nil {
		if strings.Contains(err.Error(), "unknown method") {
			return hostSync{state: syncOld, err: "its enowX daemon doesn't sync yet: connect to update it"}
		}
		return hostSync{state: syncOffline, err: client.Explain(err)}
	}
	seen := usersync.Fingerprint(mh, mp)
	here := LocalHost().Name
	var rep usersync.Report
	out := hostSync{state: syncIdle, last: time.Now(), seen: seen}
	if before.state != syncIdle || before.seen != seen {
		// Files as they were when the last sync, which went well, looked at them: it has
		// nothing to do but read and write the whole base again.
		dir, err := transport.Dir()
		if err != nil {
			return hostSync{state: syncFailed, err: err.Error()}
		}
		dir = filepath.Join(dir, "usersync")
		base, err := usersync.LoadBase(dir, mp.ID)
		if err != nil {
			return hostSync{state: syncFailed, err: err.Error()}
		}
		rep, err = usersync.Sync(client.SyncSide{C: hub, From: p.host.Name, Name: here}, client.SyncSide{C: p.cl, From: here, Name: p.host.Name}, base)
		out.rep, out.ran = rep, true
		if err == nil {
			err = base.Save(dir)
		}
		if err != nil {
			out.state, out.err = syncFailed, client.Explain(err)
		}
	} else {
		out.rep = before.rep
	}
	report := func(c *client.Client, peer proto.SyncManifest) {
		_ = c.SyncReport(proto.SyncPeerStatus{Peer: peer.ID, Name: peer.Name, Last: out.last.UTC(), Err: out.err,
			Sent: rep.Sent, Received: rep.Received, Merged: rep.Merged, Conflict: rep.Conflicts})
	}
	mp.Name, mh.Name = p.host.Name, here
	report(hub, mp)
	report(p.cl, mh)
	return out
}

// syncEvent syncs soon after a change to the user's data, or a sync asked
// for, on a host's connection; it's called on the client's goroutine.
func (a *App) syncEvent(ev proto.Event) {
	switch ev.Name {
	case proto.EventNotesChanged, proto.EventNoteGroupsChanged, proto.EventSyncRequested, proto.EventProjectsChanged:
	case proto.EventSyncApplied:
		// What a sync wrote doesn't call for another, unless someone else
		// wrote it.
		var ap proto.SyncApplied
		if json.Unmarshal(ev.Data, &ap) != nil || ap.From != "" {
			return
		}
	default:
		return
	}
	a.onMain(a.syncSoon)
}

// syncSummary says how the user's data is synced, in a few words, and
// whether it calls for the user.
func (a *App) syncSummary() (string, bool) {
	if a.dsync.hub.Conflicts > 0 {
		return fmt.Sprintf("%d sync conflict(s) to settle", a.dsync.hub.Conflicts), true
	}
	if a.dsync.running {
		return "Syncing…", false
	}
	var failed []string
	synced := 0
	for id, st := range a.dsync.hosts {
		switch st.state {
		case syncIdle:
			synced++
		case syncFailed:
			failed = append(failed, a.hostName(id))
		}
	}
	switch {
	case len(failed) > 0:
		return "Sync failed with " + strings.Join(failed, ", "), true
	case synced > 0:
		return fmt.Sprintf("Synced with %d host(s)", synced), false
	}
	return "Not synced: no other host connected", false
}

// hostName is host id's name.
func (a *App) hostName(id string) string {
	for _, h := range a.allHosts() {
		if h.ID == id {
			return h.Name
		}
	}
	return id
}

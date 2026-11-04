package ui

import (
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/egoist/mygo"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/hosts"
	"github.com/enowx/enowx/core/projectlinks"
	"github.com/enowx/enowx/core/projectsync"
	"github.com/enowx/enowx/proto"
)

// Links between hosts' copies of a project (core/projectlinks) sync on
// their own here: the hosts' daemons can't reach each other, the app
// reaches both, over the connection shown and the watches kept in the
// background. A link syncs while both its hosts are reachable, at its
// interval or as its files change (its hosts watch the folders), paused
// while either is offline or busy, or this computer on battery when the
// user says.

// Auto sync timing.
const (
	autoSyncTick       = 5 * time.Second
	autoSyncLinksEvery = 30 * time.Second
	// A link set to sync as files change syncs this often too, for the
	// changes a watch missed.
	autoSyncRealtimeFallback = 10 * time.Minute
	// DefaultLinkInterval is a link's interval when the user set none.
	defaultLinkInterval = 5 * time.Minute
	busyCPU             = 85 // percent
)

// onBattery reports whether this computer runs on battery; tests set it.
var onBattery = func() bool { return mygo.Power.IsOnBattery() }

// autoSync is the app's side of the links. It belongs to the main thread.
type autoSync struct {
	started  bool
	links    map[string]proto.ProjectLinkState // by ID, as the hosts reached told
	running  map[string]string                 // links syncing, by ID: what it does
	dirty    map[string]bool                   // links whose files changed, watched
	watched  map[*client.Client]map[string]bool
	read     time.Time                       // when the links were last read
	pushes   map[string]proto.AutopushStatus // by project ID
	notified map[string]int                  // conflicts told, by link
}

// hostConn is a host reached now: its connection and daemon.
type hostConn struct {
	host hosts.Host
	cl   *client.Client
	info proto.DaemonInfo
	st   hostStatus
}

// reachable lists the hosts reached now: the one shown and those watched.
func (a *App) reachable() []hostConn {
	var out []hostConn
	if a.client != nil && a.daemonInfo.ID != "" {
		st, _ := a.statusOfHost(a.host)
		out = append(out, hostConn{host: a.host, cl: a.client, info: a.daemonInfo, st: st})
	}
	for _, w := range a.watches {
		if w.cl != nil && w.info.ID != "" {
			out = append(out, hostConn{host: w.host, cl: w.cl, info: w.info, st: w.status})
		}
	}
	return out
}

// reach is the host whose daemon is id, when reached now.
func (a *App) reach(id string) (hostConn, bool) {
	for _, h := range a.reachable() {
		if h.info.ID == id {
			return h, true
		}
	}
	return hostConn{}, false
}

// startAutoSync looks over the links every autoSyncTick from now on.
func (a *App) startAutoSync() {
	if a.links.started {
		return
	}
	a.links.started = true
	go func() {
		t := time.NewTicker(autoSyncTick)
		defer t.Stop()
		for range t.C {
			a.quietly(func() {
				if len(a.sortedLinks()) == 0 {
					a.autoSyncTick() // reading the links asks for its own frame
					return
				}
				a.onMain(a.autoSyncTick) // what it starts shows
			})
		}
	}()
}

// autoSyncTick reads the links now and then, keeps the folders of those
// syncing as files change watched, and syncs those due.
func (a *App) autoSyncTick() {
	if time.Since(a.links.read) >= autoSyncLinksEvery {
		a.readLinks()
	}
	now := time.Now()
	for _, l := range a.sortedLinks() {
		if !l.Auto || a.links.running[l.ID] != "" {
			continue
		}
		src, ok1 := a.reach(l.Source.Daemon)
		dst, ok2 := a.reach(l.Target.Daemon)
		if !ok1 || !ok2 {
			continue // paused while a host is offline
		}
		if l.Realtime {
			a.watchFolder(src.cl, l.Source.Path)
			a.watchFolder(dst.cl, l.Target.Path)
		}
		if !a.linkDue(l, now) {
			continue
		}
		if l.PauseBusy && (src.st.usage.CPU > busyCPU || dst.st.usage.CPU > busyCPU) {
			continue
		}
		if l.PauseBattery && a.win != nil && onBattery() {
			continue
		}
		a.runLink(l.ProjectLink, src, dst, nil)
	}
}

// linkDue reports whether link l syncs now.
func (a *App) linkDue(l proto.ProjectLinkState, now time.Time) bool {
	last := l.Status.Last
	if l.Realtime {
		return a.links.dirty[l.ID] || now.Sub(last) >= autoSyncRealtimeFallback
	}
	every := defaultLinkInterval
	if l.Interval > 0 {
		every = time.Duration(l.Interval) * time.Second
	}
	return now.Sub(last) >= every
}

func (a *App) sortedLinks() []proto.ProjectLinkState {
	out := make([]proto.ProjectLinkState, 0, len(a.links.links))
	for _, l := range a.links.links {
		out = append(out, l)
	}
	slices.SortFunc(out, func(x, y proto.ProjectLinkState) int { return x.Created.Compare(y.Created) })
	return out
}

// readLinks asks every host reached for its links, the latest status of
// each kept.
func (a *App) readLinks() {
	a.links.read = time.Now()
	for _, h := range a.reachable() {
		go func() {
			list, err := h.cl.ProjectLinks("")
			if err != nil {
				return
			}
			a.onMain(func() { a.mergeLinks(list) })
		}()
	}
}

// mergeLinks takes links a host told, keeping each one's latest status.
func (a *App) mergeLinks(list []proto.ProjectLinkState) {
	if a.links.links == nil {
		a.links.links = map[string]proto.ProjectLinkState{}
	}
	for _, l := range list {
		old, ok := a.links.links[l.ID]
		if ok && old.Status.Last.After(l.Status.Last) {
			l.Status = old.Status
		}
		a.links.links[l.ID] = l
	}
}

// watchFolder has cl's host tell when the folder dir changes.
func (a *App) watchFolder(cl *client.Client, dir string) {
	if a.links.watched == nil {
		a.links.watched = map[*client.Client]map[string]bool{}
	}
	for c := range a.links.watched {
		select {
		case <-c.Done():
			delete(a.links.watched, c)
		default:
		}
	}
	if a.links.watched[cl][dir] {
		return
	}
	if a.links.watched[cl] == nil {
		a.links.watched[cl] = map[string]bool{}
	}
	a.links.watched[cl][dir] = true
	go func() { _ = cl.WatchProject(dir, false) }()
}

// filesChanged marks the links of folder dir on daemon id to sync.
func (a *App) filesChanged(id, dir string) {
	for _, l := range a.links.links {
		if l.Realtime && (l.Source.Daemon == id && l.Source.Path == dir || l.Target.Daemon == id && l.Target.Path == dir) {
			if a.links.dirty == nil {
				a.links.dirty = map[string]bool{}
			}
			a.links.dirty[l.ID] = true
		}
	}
}

// runLink syncs link l now, off the main thread, and tells both hosts how
// it went; done, when not nil, learns the outcome on the main thread.
func (a *App) runLink(l proto.ProjectLink, src, dst hostConn, done func(proto.ProjectLinkState, error)) {
	if a.links.running == nil {
		a.links.running = map[string]string{}
	}
	if a.links.running[l.ID] != "" {
		if done != nil {
			done(proto.ProjectLinkState{}, fmt.Errorf("“%s” is syncing already", l.Name))
		}
		return
	}
	a.links.running[l.ID] = "syncing"
	delete(a.links.dirty, l.ID)
	prev := a.links.links[l.ID].Status
	l.Source.Host, l.Target.Host = src.host.Name, dst.host.Name
	go func() {
		// A copy's folder renamed or moved is found again by the
		// project's ID, and the link follows it.
		moved := false
		for _, end := range []*proto.ProjectEnd{&l.Source, &l.Target} {
			h := src
			if end == &l.Target {
				h = dst
			}
			if p, err := h.cl.FindProject(l.Project, end.Path); err == nil && p.Path != end.Path {
				end.Path, moved = p.Path, true
			}
		}
		if moved {
			_, _ = src.cl.SaveProjectLink(l)
			_, _ = dst.cl.SaveProjectLink(l)
		}
		var base *projectsync.Base
		for _, cl := range []*client.Client{src.cl, dst.cl} {
			if raw, err := cl.ProjectLinkBase(l.ID); err == nil && len(raw) > 0 {
				var b projectsync.Base
				if json.Unmarshal(raw, &b) == nil {
					base = &b
					break
				}
			}
		}
		clients := [2]*client.Client{src.cl, dst.cl}
		res, err := projectlinks.Sync{
			Link: l, Daemons: [2]projectlinks.Daemon{src.cl, dst.cl}, Base: base,
			Carry: func(from projectsync.Side, id string, size int64) error {
				return client.CopyTransfer(clients[from], clients[from.Other()], id, size, throttle(l.Bandwidth))
			},
		}.Do()
		now := time.Now().UTC()
		st := proto.ProjectLinkStatus{Last: now, Sent: res.Sent, Received: res.Received, Conflicts: keepFound(prev.Conflicts, res.Conflicts)}
		entry := proto.ProjectHistoryEntry{Time: now, Project: l.Project, Kind: "sync", Link: l.ID, OK: err == nil, Summary: res.Summary(l)}
		report := proto.ProjectLinkReport{Link: l.ID, Status: st, Entry: entry}
		if err != nil {
			st.Err, entry.Err, entry.Summary = client.Explain(err), client.Explain(err), "failed"
			st.Conflicts = prev.Conflicts
			report.Status, report.Entry = st, entry
		} else if raw, jerr := json.Marshal(res.Base); jerr == nil {
			report.Base = raw
		}
		quiet := err == nil && res.Sent == 0 && res.Received == 0 && len(st.Conflicts) == len(prev.Conflicts)
		if quiet {
			report.Entry = proto.ProjectHistoryEntry{} // an unchanged sync isn't history
		}
		_ = src.cl.ReportProjectLink(report)
		_ = dst.cl.ReportProjectLink(report)
		a.onMain(func() {
			delete(a.links.running, l.ID)
			state := proto.ProjectLinkState{ProjectLink: l, Status: st}
			if old, ok := a.links.links[l.ID]; ok {
				state.ProjectLink = old.ProjectLink // its settings as changed meanwhile
				state.Source.Path, state.Target.Path = l.Source.Path, l.Target.Path
			}
			a.links.links[l.ID] = state
			a.tellConflicts(l, st.Conflicts)
			if done != nil {
				done(state, err)
			}
		})
	}()
}

// keepFound keeps when each conflict still there was first found.
func keepFound(prev, now []proto.ProjectSyncConflict) []proto.ProjectSyncConflict {
	for i := range now {
		for _, p := range prev {
			if p.Path == now[i].Path {
				now[i].Found = p.Found
			}
		}
	}
	return now
}

// tellConflicts tells the user of a link's new conflicts.
func (a *App) tellConflicts(l proto.ProjectLink, conflicts []proto.ProjectSyncConflict) {
	if a.links.notified == nil {
		a.links.notified = map[string]int{}
	}
	n := len(conflicts)
	if n > a.links.notified[l.ID] {
		msg := fmt.Sprintf("“%s”: %d changed on both %s and %s since the last sync, kept as each has it", l.Name, n, l.Source.Host, l.Target.Host)
		a.notify(msg)
		a.queueSound(proto.NotifyInput) // the user's to settle, as an agent's question
		if a.win != nil && !a.state.NotifyOff {
			note := mygo.NewNotification(mygo.NotificationOptions{ID: "link-conflict:" + l.ID, Title: "Sync conflict", Subtitle: l.Name, Body: msg})
			go func() { _ = note.Show() }()
		}
	}
	a.links.notified[l.ID] = n
	a.updateBadge()
}

// linkConflictCount is how many files of project dir on this host are in
// conflict with its copies elsewhere.
func (a *App) linkConflictCount(dir string) int {
	n := 0
	for _, l := range a.links.links {
		for _, e := range []proto.ProjectEnd{l.Source, l.Target} {
			if e.Daemon == a.daemonInfo.ID && e.Path == dir {
				n += len(l.Status.Conflicts)
				break
			}
		}
	}
	return n
}

// allLinkConflicts counts the conflicts of every link known.
func (a *App) allLinkConflicts() int {
	n := 0
	for _, l := range a.links.links {
		n += len(l.Status.Conflicts)
	}
	return n
}

// throttle is a progress callback that holds a copy to kbps KB a second,
// nil for no cap.
func throttle(kbps int) func(int64) {
	if kbps <= 0 {
		return nil
	}
	start := time.Now()
	return func(done int64) {
		want := time.Duration(float64(done) / float64(kbps*1024) * float64(time.Second))
		if wait := want - time.Since(start); wait > 0 {
			time.Sleep(wait)
		}
	}
}

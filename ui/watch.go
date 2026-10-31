package ui

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/hosts"
	"github.com/enowx/enowx/hostconn"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// The hosts the app doesn't show are watched over connections of their
// own, kept in the background: whether each is online, how fast it
// answers, what it uses, and its sessions, so the rail and the session
// list tell what happens there, an agent waiting for input included. A
// watch never asks the user anything: a host that needs a password typed,
// a key trusted or a user picked waits until the user connects to it.

// Watch timing: how often a connected host is asked, and how long an
// unreachable one waits before the next try, at most.
const (
	watchEvery    = 15 * time.Second
	watchRetryMax = 5 * time.Minute
)

// Host states a watch reports.
const (
	hostOnline     = "online"
	hostOffline    = "offline"
	hostConnecting = "connecting"
	hostNeedsYou   = "needs you" // connecting asks something: connect to it
)

// hostStatus is what the app knows of a host it doesn't show.
type hostStatus struct {
	state    string
	err      string
	latency  time.Duration
	usage    proto.HostUsage
	sessions []proto.SessionInfo
	projects []proto.ProjectInfo
	checked  time.Time
}

// running counts sessions working and waiting for input.
func (s hostStatus) running() (working, waiting int) {
	for _, info := range s.sessions {
		switch {
		case info.Exited || info.Queued:
		case info.State == proto.StateWaiting:
			waiting++
		case isWorking(info):
			working++
		}
	}
	return
}

// hostWatch is one watched host: its status, which the main thread owns,
// and the goroutine keeping its connection, which stops with stop.
type hostWatch struct {
	host   hosts.Host
	status hostStatus
	stop   chan struct{}
	// cl is the watch's connection while it is up, info its daemon: links
	// between hosts sync over it (autosync.go).
	cl   *client.Client
	info proto.DaemonInfo
}

// errNeedsYou stops a watch's connecting where the user would be asked.
var errNeedsYou = errors.New("needs you")

// syncWatches watches every host but the one shown, and stops watching
// those gone from the list.
func (a *App) syncWatches() {
	if a.win == nil {
		return
	}
	if a.watches == nil {
		a.watches = map[string]*hostWatch{}
	}
	want := map[string]hosts.Host{}
	for _, h := range a.allHosts() {
		if h.ID != a.host.ID {
			want[h.ID] = h
		}
	}
	for id, w := range a.watches {
		if _, ok := want[id]; !ok || w.host != want[id] {
			close(w.stop)
			delete(a.watches, id)
		}
	}
	for id, h := range want {
		if _, ok := a.watches[id]; !ok {
			w := &hostWatch{host: h, status: hostStatus{state: hostConnecting}, stop: make(chan struct{})}
			a.watches[id] = w
			go a.watch(w)
		}
	}
}

// watch keeps w's connection and asks it how the host is, until w stops.
func (a *App) watch(w *hostWatch) {
	retry := watchEvery
	for {
		cl, err := hostconn.Connect(w.host, a.watchOptions(w))
		if err != nil {
			state := hostOffline
			if errors.Is(err, errNeedsYou) {
				state = hostNeedsYou
			}
			a.onMain(func() { w.status.state, w.status.err = state, client.Explain(err) })
			select {
			case <-w.stop:
				return
			case <-time.After(retry):
			}
			retry = min(retry*2, watchRetryMax)
			continue
		}
		retry = watchEvery
		a.onMain(func() {
			w.cl = cl
			a.syncSoon()
			a.vaultConnected(cl) // the vault answers its requests too
		})
		go func() { _ = cl.ServeApp() }() // what agents there ask across hosts comes here
		a.poll(w, cl)
		a.onMain(func() {
			if w.cl == cl {
				w.cl = nil
			}
		})
		cl.Close()
		select {
		case <-w.stop:
			return
		default:
		}
	}
}

// poll asks cl how its host is every watchEvery, until the connection
// drops or w stops.
func (a *App) poll(w *hostWatch, cl *client.Client) {
	t := time.NewTicker(watchEvery)
	defer t.Stop()
	for {
		start := time.Now()
		info, err := cl.Info()
		latency := time.Since(start)
		if err != nil {
			return
		}
		stats, _ := cl.Stats()
		sessions, err := cl.Sessions()
		if err != nil {
			return
		}
		projects, _ := cl.Projects()
		a.onMain(func() {
			w.status = hostStatus{state: hostOnline, latency: latency, usage: stats.Host, sessions: sessions, projects: projects, checked: time.Now()}
			w.cl, w.info = cl, info
		})
		select {
		case <-w.stop:
			return
		case <-cl.Done():
			return
		case <-t.C:
		}
	}
}

// watchOptions connect a watch without asking anything: a password only
// when it is saved, no new host trusted, no user picked.
func (a *App) watchOptions(w *hostWatch) hostconn.Options {
	return hostconn.Options{
		DaemonCommand: a.opts.DaemonCommand,
		OnEvent:       func(ev proto.Event) { a.watchEvent(w, ev) },
		TrustHost:     func(string, string) bool { return false },
		Password:      a.watchPassword,
		ProviderToken: a.providerToken,
		RunAs: func(hosts.Host, []hostconn.ServerUser) (hosts.Host, error) {
			return hosts.Host{}, errNeedsYou
		},
		Passphrase: func(string) (string, error) { return "", errNeedsYou },
		// A server asking for a browser check (Tailscale SSH) waits for the
		// user: each try would make another link.
		Banner: func(msg string) error {
			if linkIn(msg) != "" {
				return errNeedsYou
			}
			return nil
		},
	}
}

// watchEvent keeps a watched host's sessions current between polls, so an
// agent there asking for input shows at once.
func (a *App) watchEvent(w *hostWatch, ev proto.Event) {
	if ev.Name == proto.EventProjectsChanged {
		var projects []proto.ProjectInfo
		if json.Unmarshal(ev.Data, &projects) == nil {
			a.onMain(func() { w.status.projects = projects })
		}
	}
	a.syncEvent(ev)
	a.notifyEvent(ev, w.host.Name)
	if apply := a.linkEvent(ev, func() (*client.Client, string) { return w.cl, w.info.ID }); apply != nil {
		a.onMain(apply)
		return
	}
	if ev.Name == proto.EventFeaturesRequested {
		var req proto.FeatureRequest
		if json.Unmarshal(ev.Data, &req) == nil {
			a.onMain(func() { a.featureRequested(w.cl, w.host.Name, req) })
		}
		return
	}
	if ev.Name == proto.EventVaultRequest {
		var req proto.VaultRequest
		if json.Unmarshal(ev.Data, &req) == nil {
			a.onMain(func() { a.vaultEvent(w.cl, w.host, req) })
		}
		return
	}
	if ev.Name != proto.EventSessionUpdated {
		return
	}
	var info proto.SessionInfo
	if json.Unmarshal(ev.Data, &info) != nil {
		return
	}
	a.onMain(func() {
		i := slices.IndexFunc(w.status.sessions, func(s proto.SessionInfo) bool { return s.ID == info.ID })
		if i < 0 {
			w.status.sessions = append(w.status.sessions, info)
			return
		}
		old := w.status.sessions[i]
		w.status.sessions[i] = info
		a.goaRelay(w.host.Name, info) // GOA's worker there, maybe
		a.soundFor(old, info, false)
		if old.State != proto.StateWaiting && info.State == proto.StateWaiting {
			a.notify(a.sessionName(info) + " on " + w.host.Name + " needs input")
		}
	})
}

// statusOfHost is what the app knows of host h: from the connection shown
// for the current host, else from its watch.
func (a *App) statusOfHost(h hosts.Host) (hostStatus, bool) {
	if h.ID == a.host.ID {
		if a.client == nil {
			return hostStatus{}, false
		}
		st := hostStatus{state: hostOnline, sessions: a.sessions}
		if u, ok := a.usage.Host(); ok {
			st.usage = u
		}
		return st, true
	}
	w := a.watches[h.ID]
	if w == nil {
		return hostStatus{}, false
	}
	return w.status, true
}

// stopWatches stops every watch, as the app closes.
func (a *App) stopWatches() {
	for id, w := range a.watches {
		close(w.stop)
		delete(a.watches, id)
	}
}

// hostColor is a host's dot: amber while an agent waits for input, lit
// while online, none while offline.
func hostColor(st hostStatus) ui.Color {
	switch st.state {
	case hostOnline:
		if _, waiting := st.running(); waiting > 0 {
			return theme.Warning
		}
		return theme.Done
	case hostNeedsYou:
		return theme.Warning
	}
	return ui.Color{}
}

// hostSummary says how a host is, in a few words.
func hostSummary(st hostStatus) string {
	switch st.state {
	case hostOnline:
		working, waiting := st.running()
		parts := []string{"online"}
		if st.latency > 0 {
			parts = append(parts, fmt.Sprintf("%d ms", max(st.latency.Milliseconds(), 1)))
		}
		if waiting > 0 {
			parts = append(parts, fmt.Sprintf("%d need input", waiting))
		}
		if working > 0 {
			parts = append(parts, fmt.Sprintf("%d working", working))
		}
		return strings.Join(parts, " · ")
	case hostNeedsYou:
		return "connect to sign in"
	case hostConnecting:
		return "checking…"
	}
	return "offline"
}

// hostUsageRow is a host's CPU, memory and disk, each a small meter, and
// its sessions running.
func hostUsageRow(c *ui.Context, st hostStatus) {
	u := st.usage
	ui.Row(c).FillWidth().Gap(14).Children(func() {
		meter := func(label string, fraction float64, value string) {
			ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
				ui.Row(c).FillWidth().Children(func() {
					ui.Text(c, label).FontSize(11).TextColor(theme.Muted)
					ui.Spacer(c)
					ui.Text(c, value).FontSize(11).TextColor(theme.TextSoft).SingleLine()
				})
				widget.Meter(c, widget.Segment{Fraction: fraction, Color: widget.Level(fraction)})
			})
		}
		if u.CPUs > 0 {
			meter("CPU", u.CPU/100, fmt.Sprintf("%.0f%% of %d", u.CPU, u.CPUs))
		}
		if u.MemTotal > 0 {
			meter("Memory", float64(u.MemUsed)/float64(u.MemTotal), textfmt.Bytes(u.MemUsed)+" / "+textfmt.Bytes(u.MemTotal))
		}
		if u.DiskTotal > 0 {
			meter("Disk", float64(u.DiskUsed)/float64(u.DiskTotal), textfmt.Bytes(u.DiskUsed)+" / "+textfmt.Bytes(u.DiskTotal))
		}
	})
	working, waiting := st.running()
	ui.Text(c, fmt.Sprintf("%s · %d working · %d need input", textfmt.Plural(len(st.sessions), "a session", fmt.Sprintf("%d sessions", len(st.sessions))), working, waiting)).
		FontSize(11).TextColor(theme.Muted)
}

// otherHostsSessions lists, under the shown host's projects, the sessions
// running on the hosts watched: picking one connects there and shows it.
func (a *App) otherHostsSessions(c *ui.Context) {
	var shown []hosts.Host
	for _, h := range a.allHosts() {
		st, ok := a.statusOfHost(h)
		if h.ID != a.host.ID && ok && st.state == hostOnline && len(liveSessions(st, a.filter, a)) > 0 {
			shown = append(shown, h)
		}
	}
	if len(shown) == 0 {
		return
	}
	ui.Text(c, "ON OTHER HOSTS").FontSize(10.5).TextColor(theme.Idle).Padding(14, 6, 4, 6)
	for _, h := range shown {
		st, _ := a.statusOfHost(h)
		ui.Column(c.Key("other:" + h.ID)).FillWidth().Gap(1).Children(func() {
			ui.Row(c).FillWidth().Gap(6).Padding(4, 6).Children(func() {
				ui.Icon(c, hostIcon(h)).FontSize(12).TextColor(theme.Muted)
				ui.Text(c, h.Name).FontSize(11.5).TextColor(theme.TextSoft).SingleLine().Shrink(1)
			})
			for _, s := range liveSessions(st, a.filter, a) {
				a.otherSessionRow(c.Key(s.ID), h, s)
			}
		})
	}
}

// liveSessions are a watched host's sessions to list: running, not tabs,
// matching filter.
func liveSessions(st hostStatus, filter string, a *App) []proto.SessionInfo {
	var out []proto.SessionInfo
	for _, s := range st.sessions {
		if s.Exited || s.Parent != "" {
			continue
		}
		if filter != "" && !strings.Contains(strings.ToLower(a.sessionName(s)), strings.ToLower(filter)) {
			continue
		}
		out = append(out, s)
	}
	return out
}

// otherSessionRow is a session of host h in the list: picking it connects
// to h and shows it.
func (a *App) otherSessionRow(c *ui.Context, h hosts.Host, s proto.SessionInfo) {
	st := statusOf(s)
	row := ui.ButtonBase(c).FillWidth().Height(30).PaddingX(10).Gap(8).Radius(theme.RadiusControl).
		Justify(ui.Start).Cursor(ui.CursorPointer).Label(a.sessionName(s) + " on " + h.Name)
	if row.Hovered() {
		row.Background(theme.Hover)
	}
	row.Children(func() {
		widget.Dot(c, st.color, st.hollow, 7)
		ui.Text(c, a.sessionName(s)).FontSize(12).SingleLine().Shrink(1)
		ui.Spacer(c)
		ui.Text(c, st.label).FontSize(10.5).TextColor(st.color).SingleLine()
	})
	if row.Clicked() {
		a.openSession = s.ID
		a.connect(h)
	}
}

// hostState is a host's state and latency, for its card, whose sessions
// show under it.
func hostState(st hostStatus) string {
	if st.state == hostOnline && st.latency > 0 {
		return fmt.Sprintf("online · %d ms", max(st.latency.Milliseconds(), 1))
	}
	if st.state == hostOnline {
		return "online"
	}
	return hostSummary(st)
}

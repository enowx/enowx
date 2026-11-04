package daemon

import (
	"path/filepath"
	"runtime"
	"slices"
	"time"

	"github.com/enowx/enowx/core/procstat"
	"github.com/enowx/enowx/proto"
)

// portsEvery is how often the daemon looks for ports opened or closed,
// while a client is connected (watchPorts). Tests shorten it.
var portsEvery = 5 * time.Second

// portsSupported: where the daemon can tell which ports a process holds.
var portsSupported = runtime.GOOS == "linux" || runtime.GOOS == "darwin"

// ports lists the TCP ports session id's processes listen on.
func (s *Server) ports(id string) (proto.SessionPorts, error) {
	if _, err := s.sessions.Get(id); err != nil {
		return proto.SessionPorts{}, err
	}
	if !portsSupported {
		return proto.SessionPorts{}, nil
	}
	all, err := s.allPorts()
	if err != nil {
		return proto.SessionPorts{}, err
	}
	return proto.SessionPorts{Supported: true, Ports: all[id]}, nil
}

// allPorts lists each session's listening ports, with one look at the
// processes. A tab's ports are its session's: a dev server started in a
// tab previews with the session.
func (s *Server) allPorts() (map[string][]proto.Port, error) {
	parents := map[string]string{}
	for _, sess := range s.sessions.List() {
		if p := sess.Meta().Parent; p != "" {
			parents[sess.ID()] = p
		}
	}
	top := func(id string) string {
		if p, ok := parents[id]; ok {
			return p
		}
		return id
	}
	s.stats() // samples the processes and finds each session's
	st := &s.statsState
	st.mu.Lock()
	owner := map[int]string{}
	var pids []int
	for pid, id := range st.owners {
		if id == "" {
			continue
		}
		for _, p := range st.sampler.Descendants(pid) {
			owner[p] = top(id)
			pids = append(pids, p)
		}
	}
	st.mu.Unlock()
	listeners, err := procstat.Listening(pids)
	if err != nil {
		return nil, err
	}
	out := map[string][]proto.Port{}
	for id, p := range s.displays.ports() {
		out[top(id)] = append(out[top(id)], p)
	}
	for _, l := range listeners {
		name := ""
		if args := procstat.Args(l.PID); len(args) > 0 {
			name = filepath.Base(args[0])
		}
		id := owner[l.PID]
		if slices.ContainsFunc(out[id], func(p proto.Port) bool { return p.Port == l.Port }) {
			continue // the session's display, already listed
		}
		out[id] = append(out[id], proto.Port{Port: l.Port, Addr: l.Addr, PID: l.PID, Process: name})
	}
	return out, nil
}

// portsQuietFactor is how much less often ports are looked at with every session at rest.
const portsQuietFactor = 4

// anySessionAtWork says a session's program is producing output, or was left working.
func (s *Server) anySessionAtWork() bool {
	for _, sess := range s.sessions.List() {
		info := sess.Info()
		if !info.Exited && (info.Active || info.State == proto.StateWorking) {
			return true
		}
	}
	return false
}

// watchPorts tells clients when a session opens or closes a port
// (proto.EventPortsChanged), looking every portsEvery while one is
// connected. Its first look only learns what is open: ports open before
// aren't news. It runs until done closes.
func (s *Server) watchPorts(done <-chan struct{}) {
	if !portsSupported {
		return
	}
	t := time.NewTicker(portsEvery)
	defer t.Stop()
	var last map[string][]proto.Port
	var looked time.Time
	for {
		select {
		case <-done:
			return
		case <-t.C:
		}
		s.mu.Lock()
		watched := len(s.conns) > 0
		s.mu.Unlock()
		hooked := s.hostWants(proto.HookPort)
		if !watched && !hooked {
			continue
		}
		// A look runs lsof on macOS and reads all sockets on Linux. Ports open as programs
		// start, which is when a session is at work; one at rest is looked at less.
		if time.Since(looked) < portsQuietFactor*portsEvery && !s.anySessionAtWork() {
			continue
		}
		looked = time.Now()
		now, err := s.allPorts()
		if err != nil {
			continue
		}
		if last != nil {
			changes := portChanges(last, now)
			for _, ch := range changes {
				s.broadcast(proto.EventPortsChanged, ch)
			}
			if hooked {
				s.portsOpened(changes)
			}
		}
		last = now
	}
}

// portChanges are the sessions whose ports differ between before and
// now, with the ports opened since.
func portChanges(before, now map[string][]proto.Port) []proto.PortsChange {
	var out []proto.PortsChange
	ids := map[string]bool{}
	for id := range before {
		ids[id] = true
	}
	for id := range now {
		ids[id] = true
	}
	for id := range ids {
		was, is := before[id], now[id]
		if slices.Equal(was, is) {
			continue
		}
		ch := proto.PortsChange{Session: id, Ports: is}
		for _, p := range is {
			if !slices.ContainsFunc(was, func(w proto.Port) bool { return w.Port == p.Port }) {
				ch.Opened = append(ch.Opened, p)
			}
		}
		out = append(out, ch)
	}
	slices.SortFunc(out, func(a, b proto.PortsChange) int {
		if a.Session < b.Session {
			return -1
		}
		return 1
	})
	return out
}

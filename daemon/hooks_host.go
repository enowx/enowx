package daemon

import (
	"slices"
	"time"

	"github.com/enowx/enowx/core/automation"
	"github.com/enowx/enowx/proto"
)

// Hooks on the host's events: a session listening on a port, a sync
// bringing changes, a notification sent to the apps, and hooks' own
// schedules.

// scheduleLook is how often hooks' schedules are looked at.
const scheduleLook = 30 * time.Second

// hostWants says some hook of the host's, or of an allowed project's, is
// on for event.
func (s *Server) hostWants(event string) bool {
	if slices.ContainsFunc(s.hooks.store.List(), func(h proto.Hook) bool { return h.On && h.Event == event }) {
		return true
	}
	// Asked on a timer: the folders are all it needs, and their branches would take git.
	for _, dir := range s.projects.Paths() {
		if ph, ok := s.hooks.store.Project(dir); ok && slices.ContainsFunc(ph.Hooks, func(h proto.Hook) bool { return h.On && h.Event == event }) {
			return true
		}
	}
	return false
}

// portsOpened runs the hooks on the ports sessions began listening on.
func (s *Server) portsOpened(changes []proto.PortsChange) {
	for _, ch := range changes {
		sess, err := s.sessions.Get(ch.Session)
		if err != nil {
			continue
		}
		info := sess.Info()
		for _, p := range ch.Opened {
			go s.fireHooks(info, proto.HookEvent{Event: proto.HookPort, Port: p.Port, Via: proto.HookOutside})
		}
	}
}

// synced runs the hooks on a sync's changes.
func (s *Server) synced(paths []string) {
	go s.fireHooks(proto.SessionInfo{}, proto.HookEvent{Event: proto.HookSync, Paths: paths, Via: proto.HookOutside})
}

// notified runs the hooks on a notification sent to the apps.
func (s *Server) notified(p proto.NotifySoundParams) {
	info := proto.SessionInfo{}
	if sess, err := s.sessions.Get(p.Session); err == nil {
		info = sess.Info()
	}
	go s.fireHooks(info, proto.HookEvent{Event: proto.HookNotify, Kind: p.Kind, Text: p.Text, Via: proto.HookOutside})
}

// runSchedules runs scheduled hooks as their time comes, until done
// closes: the host's, and projects' in their folder.
func (s *Server) runSchedules(done <-chan struct{}) {
	since := time.Now()
	t := time.NewTicker(scheduleLook)
	defer t.Stop()
	for {
		select {
		case <-done:
			return
		case now := <-t.C:
			s.scheduleDue(since, now)
			since = now
		}
	}
}

// scheduleDue runs the scheduled hooks whose time came after since, up to
// now.
func (s *Server) scheduleDue(since, now time.Time) {
	due := func(h proto.Hook) bool {
		sc, err := automation.Parse(h.Schedule)
		return err == nil && h.On && h.Event == proto.HookSchedule && !sc.Next(since).After(now)
	}
	run := func(h proto.Hook, dir string) {
		ev := proto.HookEvent{Event: proto.HookSchedule, Dir: dir, Project: dir, Via: proto.HookScheduled, Time: now}
		ev = s.eventOf(proto.SessionInfo{}, ev)
		if !s.hooks.limits.Take(h, ev, now) {
			return
		}
		go func() {
			r, _ := s.hookRunner(proto.SessionInfo{}).Run(h, ev)
			s.logHookRun(r)
		}()
	}
	for _, h := range s.hooks.store.List() {
		if due(h) {
			run(h, "")
		}
	}
	for _, p := range s.projects.List() {
		if ph, ok := s.hooks.store.Project(p.Path); ok {
			for _, h := range ph.Hooks {
				if due(h) {
					run(h, p.Path)
				}
			}
		}
	}
}

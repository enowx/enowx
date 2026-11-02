package daemon

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/goa"
	"github.com/enowx/enowx/core/procstat"
	"github.com/enowx/enowx/core/session"
	"github.com/enowx/enowx/proto"
)

// A task that isn't moving: nothing changes a session's state then, so GOA, told
// only of changes, would wait for ever, and the board go on saying working.

const (
	stallEvery       = 15 * time.Second
	stallCPUIdle     = 1.0              // percent of a core a tree working uses at least
	stallAgain       = 3                // a further line after this many times the limit
	stallCallTimeout = 20 * time.Second // another host's answer, through the app
)

// goaStallFile keeps the limits apart from the synced settings: how long a
// worker may take depends on the host it watches from.
func (s *Server) goaStallFile() string { return filepath.Join(s.dataDir, "goa-stall.json") }

// goaStall are this host's limits, the defaults for any not set.
func (s *Server) goaStall() proto.GOAStall {
	var l proto.GOAStall
	if data, err := os.ReadFile(s.goaStallFile()); err == nil {
		_ = json.Unmarshal(data, &l)
	}
	return goa.StallOr(l)
}

// saveGOAStall keeps l, a limit of 0 going back to its default.
func (s *Server) saveGOAStall(l proto.GOAStall) (proto.GOAStall, error) {
	if err := goa.CheckStall(l); err != nil {
		return proto.GOAStall{}, err
	}
	data, err := json.MarshalIndent(goa.StallOr(l), "", "  ")
	if err != nil {
		return proto.GOAStall{}, err
	}
	if err := writeTeamFile(s.goaStallFile(), data); err != nil {
		return proto.GOAStall{}, fmt.Errorf("goa: stall limits: %w", err)
	}
	select {
	case s.goa.stallSet <- struct{}{}:
	default:
	}
	return s.goaStall(), nil
}

// stallSeconds is a limit as a time.
func stallSeconds(seconds int) time.Duration { return time.Duration(seconds) * time.Second }

// stallLook is what a task's session is like now.
type stallLook struct {
	Info  proto.SessionInfo
	Gone  bool // its host has no such session any more
	Blank bool
	Still time.Duration // since its screen last changed
	CPU   float64
	NoCPU bool          // its host can't say what its processes use
	Age   time.Duration // since the task was handed over
}

// stallCase names how a task isn't moving, and after how long that counts; ""
// when it is moving, as far as can be told. It looks only at tasks the board
// has as working, so a session done with its turn never took this one (a turn
// ending moves the board on). A long build shows no output, but uses the CPU; a
// slow model's reply shows the agent's own timer, on its screen.
func stallCase(l stallLook, lim proto.GOAStall) (string, time.Duration) {
	blank, idle, stuck := stallSeconds(lim.Blank), stallSeconds(lim.Idle), stallSeconds(lim.Stuck)
	quiet := l.NoCPU || l.CPU < stallCPUIdle
	switch {
	case l.Gone:
		if l.Still >= idle {
			return "idle: its session is gone", idle
		}
	case l.Info.Exited && l.Info.Hibernated:
		if l.Still >= idle {
			return "idle: its session is hibernated", idle
		}
	case l.Info.Exited, l.Info.Queued, !quiet:
	case l.Blank:
		if l.Age >= blank {
			return "never started: its screen is blank", blank
		}
	case l.Info.State == proto.StateWorking || l.Info.StateStale:
		if l.Still >= stuck {
			return "stuck: nothing has changed on its screen, and nothing runs", stuck
		}
	case l.Info.State == "" || l.Info.State == proto.StateDone:
		if l.Still >= idle {
			return "idle: its agent isn't working on it", idle
		}
	}
	return "", 0
}

// queuedQuiet is how long a session's screen stays still, with nothing running,
// before a task queued for it goes in although its turn was never seen to end.
const queuedQuiet = 20 * time.Second

// queuedCanGo says a task queued for a session can go in now, its turn's end
// unheard: an interrupt ends a turn with no hook, and a command typed at the
// prompt (/model) says nothing either, so its state stays the working it last
// was, gone stale. Only such a session counts: one with no state may be a plain
// shell, in a command that reads what it is typed.
func queuedCanGo(l stallLook, screen string) bool {
	return l.Info.StateStale && !l.Info.Exited && !l.Info.Queued && !l.Gone &&
		l.Still >= queuedQuiet && (l.NoCPU || l.CPU < stallCPUIdle) && agent.PromptFree(screen)
}

// stallFollow is what was told of a task. Only watchStalls touches it.
type stallFollow struct {
	screen string
	since  time.Time
	told   time.Time // its last line
	fired  bool      // the hook ran for this stall
}

// stallHost is what a host says of its sessions, for one look.
type stallHost struct {
	infos    map[string]proto.SessionInfo
	cpuKnown bool
	cpuOf    func(id string) float64
	screen   func(id string) (string, error)
}

// stallWatch is what watchStalls keeps between looks.
type stallWatch struct {
	follows map[string]*stallFollow // by task
	sampled map[string]bool         // hosts ("" this one) sampled, as CPU use is measured between two
}

// stallTick is how often to look: often enough for the shortest limit, with
// two samples of the CPU apart.
func stallTick(lim proto.GOAStall) time.Duration {
	half := stallSeconds(min(lim.Blank, lim.Idle, lim.Stuck)) / 2
	return min(max(half, 2*minSample), stallEvery)
}

// watchStalls looks at the tasks not settled until done closes; new limits
// take at once.
func (s *Server) watchStalls(done <-chan struct{}) {
	w := &stallWatch{follows: map[string]*stallFollow{}, sampled: map[string]bool{}}
	for {
		select {
		case <-done:
			return
		case <-s.goa.stallSet:
			continue
		case <-time.After(stallTick(s.goaStall())):
		}
		s.stallPass(w)
	}
}

// stallPass looks once at every task said to be running. A host that can't be
// reached says nothing of its tasks, which wait as they are.
func (s *Server) stallPass(w *stallWatch) {
	lim := s.goaStall()
	var open []proto.GOATask
	for _, t := range s.goa.board.Working() {
		if t.Session != "" && (t.Status == proto.GOATaskWorking || t.Status == proto.GOATaskStalled || t.Status == proto.GOATaskQueued) {
			open = append(open, t)
		}
	}
	if len(open) == 0 {
		clear(w.sampled)
		clear(w.follows)
		return
	}
	hosts := map[string]*stallHost{}
	seen := map[string]bool{}
	given := map[string]bool{} // sessions given a queued task in this pass
	for _, t := range open {
		seen[t.ID] = true
		key := t.Host
		if s.thisHost(key) {
			key = ""
		}
		h, looked := hosts[key]
		if !looked {
			h = s.stallHostNow(key)
			if h != nil && !w.sampled[key] {
				// What this sample says of the CPU counts from the next.
				w.sampled[key] = true
				h = nil
			} else if h == nil {
				delete(w.sampled, key)
			}
			hosts[key] = h
		}
		switch {
		case h == nil:
		case t.Status != proto.GOATaskQueued:
			s.stallCheck(w, t, *h, lim)
		case !given[t.Session] && s.queuedCheck(w, t, *h):
			given[t.Session] = true
		}
	}
	for id := range w.follows {
		if !seen[id] {
			delete(w.follows, id)
		}
	}
}

// queuedCheck gives task t's session its oldest queued task if its prompt is
// free, on host h, and says whether it did.
func (s *Server) queuedCheck(w *stallWatch, t proto.GOATask, h stallHost) bool {
	now := time.Now()
	f := w.follows[t.ID]
	if f == nil {
		f = &stallFollow{since: now}
		w.follows[t.ID] = f
	}
	info, there := h.infos[t.Session]
	if !there {
		return false
	}
	text, err := h.screen(t.Session)
	if err != nil {
		return false
	}
	if text != f.screen {
		f.screen, f.since = text, now
	}
	look := stallLook{Info: info, NoCPU: !h.cpuKnown, CPU: h.cpuOf(t.Session), Still: now.Sub(f.since)}
	if !queuedCanGo(look, text) {
		return false
	}
	s.logger().Info("goa: a queued task goes in, its session's prompt being free", "session", t.Session, "task", t.ID)
	s.goaDeliver(info)
	return true
}

// stallCheck tells what's new of task t, on host h.
func (s *Server) stallCheck(w *stallWatch, t proto.GOATask, h stallHost, lim proto.GOAStall) {
	now := time.Now()
	f := w.follows[t.ID]
	first := f == nil
	if first {
		f = &stallFollow{since: now}
		w.follows[t.ID] = f
		if t.Status == proto.GOATaskStalled {
			// From before the daemon restarted: still not moving until seen to,
			// its hook run. What GOA was told may have gone, so it hears again.
			f.since, f.fired = t.Created, true
		}
	}
	info, there := h.infos[t.Session]
	look := stallLook{Info: info, Gone: !there, NoCPU: !h.cpuKnown, Age: now.Sub(t.Created)}
	text := ""
	if there {
		if info.Exited && !info.Hibernated {
			s.goaWorkerChanged(info) // it ended unseen, while the daemon was down
			return
		}
		var err error
		if text, err = h.screen(t.Session); err != nil {
			return
		}
		if first {
			f.screen = text
		} else if text != f.screen {
			f.screen, f.since = text, now
		}
		look.Blank = strings.TrimSpace(text) == ""
		look.CPU = h.cpuOf(t.Session)
	}
	look.Still = now.Sub(f.since)
	why, limit := stallCase(look, lim)
	if why == "" {
		f.told, f.fired = time.Time{}, false
		if t.Status == proto.GOATaskStalled {
			s.stallMark(t.ID, proto.GOATaskStalled, proto.GOATaskWorking)
		}
		return
	}
	if t.Status != proto.GOATaskStalled && !s.stallMark(t.ID, proto.GOATaskWorking, proto.GOATaskStalled) {
		return // settled meanwhile
	}
	if s.goa.board.IsSettled(t.ID) {
		return // t is as this pass began, and a look at another host takes time
	}
	if !f.told.IsZero() && now.Sub(f.told) < stallAgain*limit {
		return
	}
	f.told = now
	line := fmt.Sprintf("%s, for %s", why, look.Still.Round(time.Second))
	if look.Blank {
		line = fmt.Sprintf("%s, for %s", why, look.Age.Round(time.Second))
	}
	if !f.fired {
		f.fired = true
		if !there {
			info = proto.SessionInfo{ID: t.Session, Agent: t.Agent, Dir: t.Dir, Branch: t.Branch, Project: t.Project, Name: t.SessionName}
		}
		go s.fireHooks(info, proto.HookEvent{Event: proto.HookStalled, Text: line})
	}
	if look.NoCPU {
		line += " (its host can't say what its processes use, so it may be a long build)"
	}
	s.goaTellLine(goaLine{text: stallLine(t, line, text), task: t.ID})
}

// stallMark moves task id from one status to another, unless something else
// moved it first, and says whether it did.
func (s *Server) stallMark(id, from, to string) bool {
	moved := false
	if _, err := s.goa.board.Update(id, func(t *proto.GOATask) {
		if t.Status == from {
			t.Status, moved = to, true
		}
	}); err != nil {
		return false
	}
	if moved {
		s.goaChanged()
	}
	return moved
}

// stallLine is what GOA is told of task t, not moving as why says, its screen
// showing text.
func stallLine(t proto.GOATask, why, text string) string {
	where, hostFlag := goaTaskPlace(t)
	shows := "Its screen is blank."
	if end := lastLines(text, 3); end != "" {
		shows = "Its screen ends: " + clip(oneLine(end), 200) + "."
	}
	return fmt.Sprintf("[enowX GOA] Task %s %q (%s) isn't moving: %s. %s Read it (read %s%s), then restart or move the task and tell the user; don't repeat what already failed.",
		t.ID, t.Title, where, why, shows, t.Session, hostFlag)
}

// stallHostNow is what host ("" this one) says of its sessions now, nil when it
// can't be reached.
func (s *Server) stallHostNow(host string) *stallHost {
	if host != "" {
		return s.stallOtherHost(host)
	}
	h := s.stallThisHost()
	return &h
}

// stallCall reads method from another host, through the app as goaCall does and
// with the app's same checks. Not goaCall itself: the daemon is looking, no
// session is acting, and these reads, every few seconds, would soon fill the
// log of what sessions did (200 entries); nor may an app not answering hold up
// the look at every other task for its two minutes.
func (s *Server) stallCall(host, method string, params, out any) error {
	raw, err := json.Marshal(params)
	if err != nil {
		return err
	}
	rep, err := s.askAppReply("host.call", proto.HostCallParams{Host: host, Method: method, Params: raw}, stallCallTimeout)
	if err != nil {
		return err
	}
	return json.Unmarshal(rep.Result, out)
}

// stallOtherHost is what another host says of its sessions now, nil when it
// can't be reached. A daemon or app too old to measure leaves the CPU unknown.
func (s *Server) stallOtherHost(host string) *stallHost {
	var list []proto.SessionInfo
	if err := s.stallCall(host, proto.MethodSessionList, nil, &list); err != nil {
		return nil
	}
	h := &stallHost{infos: map[string]proto.SessionInfo{}}
	for _, info := range list {
		h.infos[info.ID] = info
	}
	var st proto.DaemonStats
	h.cpuKnown = s.stallCall(host, proto.MethodDaemonStats, nil, &st) == nil && st.Supported
	tree := map[string]float64{}
	for _, u := range st.Sessions {
		tree[u.ID] = u.CPU
	}
	h.cpuOf = func(id string) float64 { return tree[id] }
	h.screen = func(id string) (string, error) {
		var sc proto.SessionScreen
		err := s.stallCall(host, proto.MethodSessionScreen, proto.SessionRef{ID: id}, &sc)
		return sc.Text, err
	}
	return h
}

// stallThisHost is what this host says of its sessions now.
func (s *Server) stallThisHost() stallHost {
	st := s.stats()
	h := stallHost{infos: map[string]proto.SessionInfo{}, cpuKnown: st.Supported}
	for _, sess := range s.sessions.List() {
		h.infos[sess.ID()] = sess.Info()
	}
	tree := map[string]float64{}
	for _, u := range st.Sessions {
		tree[u.ID] = u.CPU
	}
	h.cpuOf = func(id string) float64 {
		if tree[id] >= stallCPUIdle {
			return tree[id]
		}
		return tree[id] + s.taggedCPU(id)
	}
	h.screen = func(id string) (string, error) {
		sess, err := s.sessions.Get(id)
		if err != nil {
			return "", err
		}
		return s.maskLent(sess.Terminal().Text()), nil
	}
	return h
}

// taggedCPU is the percent of a core what a session started and left its tree
// uses (tmux, nohup), as of the last measure. Only where a process's environment
// can be read, so not on recent macOS.
func (s *Server) taggedCPU(id string) float64 {
	cpu := 0.0
	for _, pid := range procstat.Tagged(session.EnvSession, id) {
		cpu += s.statsState.sampler.One(pid).CPU
	}
	return cpu
}

// lastLines are text's last n lines with something on them.
func lastLines(text string, n int) string {
	var out []string
	lines := strings.Split(text, "\n")
	for i := len(lines) - 1; i >= 0 && len(out) < n; i-- {
		if t := strings.TrimSpace(lines[i]); t != "" {
			out = append([]string{t}, out...)
		}
	}
	return strings.Join(out, " / ")
}

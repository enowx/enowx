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

// queuedCanGo says a task queued for a session can go in now, though no turn's end
// gave it: its session says it is done, as one does whose turn ended before the task
// was queued; or its turn's end went unheard, as an interrupt's, or a command typed at
// the prompt (/model), which leave the working it last was, gone stale, or as after a
// daemon's restart, which leaves an agent no state. No state and no agent may be a
// plain shell, in a command that reads what it is typed.
func queuedCanGo(l stallLook, screen string) bool {
	if l.Info.Exited || l.Info.Queued || l.Gone || l.Still < queuedQuiet {
		return false
	}
	switch {
	case l.Info.State == proto.StateDone:
		return l.NoCPU || l.CPU < turnOverCPU // an agent at rest isn't at 0
	case l.Info.StateStale:
	case l.Info.State == "":
		if _, isAgent := agent.Find(l.Info.Agent); !isAgent {
			return false
		}
	default:
		return false
	}
	return (l.NoCPU || l.CPU < stallCPUIdle) && agent.PromptFree(screen)
}

const (
	// turnOverStill is how long a screen shows a turn's end before it counts.
	turnOverStill = 20 * time.Second
	// turnOverCPU is the percent of a core a session whose screen shows its turn over
	// may use: an agent at rest isn't at 0 (Claude Code was seen at 1.6, its MCP
	// servers beside it), and what really works shows it on its screen.
	turnOverCPU = 20.0
)

// turnOverOnScreen says a session no hook speaks for (an agent with none, one started
// by hand in a shell tab, or one whose hooks fail: it still says done from its last
// turn, GOA having sent it another) shows its turn has ended. That is a report, not
// "isn't moving". What is only quiet stays a stall.
func turnOverOnScreen(l stallLook, screen string) bool {
	silent := l.Info.State == "" || l.Info.StateStale || l.Info.State == proto.StateDone
	return silent && !l.Info.Exited && l.Still >= turnOverStill && (l.NoCPU || l.CPU < turnOverCPU) && agent.TurnOverShown(screen)
}

// stallFollow is what was told of a task. Only watchStalls touches it.
type stallFollow struct {
	screen string
	since  time.Time
	told   time.Time // its last line
	fired  bool      // the hook ran for this stall
	waking bool      // its queued task's session is being brought back
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
	lost    map[string]time.Time    // hosts not reached, since when
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

// stallPass looks once at every task said to be running, and every session GOA sent
// something to with no task. A host that can't be reached says nothing of its
// sessions, which wait as they are.
func (s *Server) stallPass(w *stallWatch) {
	s.goaFlushIdle()
	lim := s.goaStall()
	var open []proto.GOATask
	for _, t := range s.goa.board.Working() {
		if t.Session != "" && (t.Status == proto.GOATaskWorking || t.Status == proto.GOATaskStalled || t.Status == proto.GOATaskQueued) {
			open = append(open, t)
		}
	}
	told := s.goaToldNow()
	if len(open) == 0 && len(told) == 0 {
		clear(w.sampled)
		clear(w.follows)
		return
	}
	hosts := map[string]*stallHost{}
	seen := map[string]bool{}
	given := map[string]bool{} // sessions given a queued task in this pass
	for _, t := range open {
		seen[t.ID] = true
		key := s.stallKey(t.Host)
		h := s.stallLookAt(w, hosts, key)
		switch {
		case h == nil:
			if !w.sampled[key] { // else it was reached, and sampled for the first time
				s.stallHostLost(w, key, t, lim)
			}
		case t.Status != proto.GOATaskQueued:
			s.stallCheck(w, t, *h, lim)
		case !given[t.Session] && s.queuedCheck(w, t, *h):
			given[t.Session] = true
		}
	}
	for id, was := range told {
		seen[toldFollow(id)] = true
		if h := s.stallLookAt(w, hosts, s.stallKey(was.Host)); h != nil {
			s.toldCheck(w, id, was, *h, lim)
		}
	}
	for id := range w.follows {
		if !seen[id] {
			delete(w.follows, id)
		}
	}
}

// stallKey is host as watchStalls keeps it, "" for this one.
func (s *Server) stallKey(host string) string {
	if s.thisHost(host) {
		return ""
	}
	return host
}

// stallLookAt is what host key says now, asked once a pass: nil when it can't be
// reached, or was sampled for the first time, as CPU use is measured between two.
func (s *Server) stallLookAt(w *stallWatch, hosts map[string]*stallHost, key string) *stallHost {
	if h, looked := hosts[key]; looked {
		return h
	}
	h := s.stallHostNow(key)
	if h != nil && !w.sampled[key] {
		// What this sample says of the CPU counts from the next.
		w.sampled[key] = true
		h = nil
	} else if h == nil {
		delete(w.sampled, key)
	}
	if h != nil || w.sampled[key] {
		delete(w.lost, key)
	}
	hosts[key] = h
	return h
}

// stallHostLost tells of task t once its host hasn't been reached for as long as
// an idle session may be: the app, which reaches it, is closed, or the connection
// dropped. Its session may be fine, and the line says so. A task told of moves to
// stalled, so it is told once.
func (s *Server) stallHostLost(w *stallWatch, key string, t proto.GOATask, lim proto.GOAStall) {
	if w.lost == nil {
		w.lost = map[string]time.Time{}
	}
	since, known := w.lost[key]
	if !known {
		w.lost[key] = time.Now()
		return
	}
	limit := stallSeconds(lim.Idle)
	if limit <= 0 || time.Since(since) < limit {
		return
	}
	if t.Status == proto.GOATaskQueued {
		s.queuedHostLost(w, t)
		return
	}
	if t.Status != proto.GOATaskWorking {
		return
	}
	if s.stallMark(t.ID, proto.GOATaskWorking, proto.GOATaskStalled) {
		s.goaTellLine(goaLine{Task: t.ID, Session: t.Session, Text: fmt.Sprintf("[enowX GOA] Task %s %q on %s: its host can't be reached (the app is closed there, or the connection dropped), so what its session does isn't known. It stays open: look again once the host is back.", t.ID, t.Title, nonEmpty(t.Host, "this host"))})
	}
}

// queuedHostLost says once, on the board and to GOA, that queued task t waits for its
// host. It stays queued, and goes in when the host is back.
func (s *Server) queuedHostLost(w *stallWatch, t proto.GOATask) {
	f := w.follows[t.ID]
	if f == nil {
		f = &stallFollow{since: time.Now()}
		w.follows[t.ID] = f
	}
	if !f.told.IsZero() {
		return
	}
	f.told = time.Now()
	const why = "Its host can't be reached (the app that reaches it is closed, or the connection dropped): it goes in when the host is back."
	_, _ = s.goa.board.Update(t.ID, func(t *proto.GOATask) { t.Why = strings.TrimSpace(t.Why + " " + why) })
	s.goaChanged()
	s.goaTellLine(goaLine{Task: t.ID, Session: t.Session, Text: fmt.Sprintf("[enowX GOA] Task %s %q is still queued. %s", t.ID, t.Title, why)})
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
	switch {
	case !there:
		// Its host answers and has no such session, for longer than a restart takes.
		if now.Sub(f.since) >= queuedQuiet {
			s.goaQueuedFailed(t, "its session is gone")
			return true
		}
		return false
	case info.Exited && info.Hibernated:
		if !f.waking { // once: waking it takes a while, and each pass comes back here
			f.waking = true
			go s.queuedWake(t, info)
		}
		return false
	case info.Exited:
		s.goaDeliver(info) // says it can't go
		return true
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

// queuedWake brings back the session of queued task t, hibernated or ended, and gives
// it its task; GOA hears when it can't. Once at a time for a session: a turn's end, the
// app's word and the stall watch may each come here.
func (s *Server) queuedWake(t proto.GOATask, info proto.SessionInfo) {
	s.goa.mu.Lock()
	if s.goa.waking[info.ID] {
		s.goa.mu.Unlock()
		return
	}
	if s.goa.waking == nil {
		s.goa.waking = map[string]bool{}
	}
	s.goa.waking[info.ID] = true
	s.goa.mu.Unlock()
	defer func() {
		s.goa.mu.Lock()
		delete(s.goa.waking, info.ID)
		s.goa.mu.Unlock()
	}()
	from := ""
	if g, ok := s.goaSession(); ok {
		from = g.ID
	}
	if _, err := s.goaReady(proto.GOADelegateParams{From: from, Host: t.Host}, info); err != nil {
		s.goaQueuedFailed(t, "its session couldn't be brought back: "+err.Error())
		return
	}
	info.Exited, info.Hibernated = false, false
	s.goaDeliver(info)
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
	if there && turnOverOnScreen(look, text) {
		s.goaTurnOverSeen(t, text)
		return
	}
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
	s.goaTellLine(goaLine{Text: stallLine(t, line, text), Task: t.ID, Session: t.Session})
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

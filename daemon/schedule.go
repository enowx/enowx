package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/session"
	"github.com/enowx/enowx/proto"
)

// The scheduler keeps at most DaemonSettings.MaxAgents agents working at
// once: an agent session started past it waits, queued, until one
// finishes its turn. It also interrupts agents past their time budget.
// One goroutine (run) does both, woken by session updates, so starting a
// session never happens inside another's update.

// warmup is how long a started agent counts as working before it says:
// an agent with a first prompt starts on it, and the next one queued
// shouldn't start in the meantime.
const warmup = 10 * time.Second

type scheduler struct {
	wake chan struct{}

	mu          sync.Mutex
	path        string
	settings    proto.DaemonSettings
	queue       []queued
	started     map[string]time.Time // agent sessions started, by ID
	interrupted map[string]bool      // sessions interrupted for their budget
}

// queued is a session waiting for its agent to start.
type queued struct {
	spec    session.Spec
	command []string // as persisted
	typed   string   // prompt typed in once the agent is ready
}

func (sc *scheduler) open(path string) error {
	sc.path, sc.wake = path, make(chan struct{}, 1)
	sc.started, sc.interrupted = map[string]time.Time{}, map[string]bool{}
	sc.settings.HibernateAfter = defaultHibernateAfter
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("daemon: settings: %w", err)
	}
	var st storedSettings
	if err := json.Unmarshal(data, &st); err != nil {
		return err
	}
	if !st.HibernateSet {
		st.HibernateAfter = defaultHibernateAfter // never chosen: the default
	}
	sc.settings = st.DaemonSettings
	return nil
}

// defaultHibernateAfter is how many idle minutes an agent done with its
// turn keeps running until the user picks otherwise.
const defaultHibernateAfter = 30

// storedSettings are the settings as saved: HibernateSet tells "Never"
// (0) chosen from a hibernation window never chosen.
type storedSettings struct {
	proto.DaemonSettings
	HibernateSet bool `json:"hibernateSet,omitempty"`
}

func (sc *scheduler) poke() {
	select {
	case sc.wake <- struct{}{}:
	default:
	}
}

func (sc *scheduler) get() proto.DaemonSettings {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	return sc.settings
}

func (sc *scheduler) set(st proto.DaemonSettings) (proto.DaemonSettings, error) {
	st.MaxAgents = max(st.MaxAgents, 0)
	st.HibernateAfter = min(max(st.HibernateAfter, 0), 24*60)
	data, err := json.MarshalIndent(storedSettings{DaemonSettings: st, HibernateSet: true}, "", "  ")
	if err != nil {
		return proto.DaemonSettings{}, err
	}
	if err := os.WriteFile(sc.path, data, 0o600); err != nil {
		return proto.DaemonSettings{}, fmt.Errorf("daemon: settings: %w", err)
	}
	sc.mu.Lock()
	sc.settings = st
	sc.mu.Unlock()
	sc.poke()
	return st, nil
}

// working reports whether a session's agent is on a task, as clients see.
func working(info proto.SessionInfo) bool {
	if info.Exited || info.Queued {
		return false
	}
	switch info.State {
	case proto.StateWorking:
		return true
	case "":
		return info.Active
	}
	return false
}

// busyAgents counts the agents working, or just started.
func (s *Server) busyAgents() int {
	sc := &s.sched
	now := time.Now()
	n := 0
	for _, sess := range s.sessions.List() {
		info := sess.Info()
		if _, ok := agent.Find(info.Agent); !ok || info.Exited || info.Queued {
			continue
		}
		sc.mu.Lock()
		at := sc.started[info.ID]
		sc.mu.Unlock()
		if working(info) || now.Sub(at) < warmup {
			n++
		}
	}
	return n
}

// mustQueue reports whether an agent session starting now waits: the host
// has a limit it reached, or others wait already.
func (s *Server) mustQueue() bool {
	sc := &s.sched
	sc.mu.Lock()
	limit, waiting := sc.settings.MaxAgents, len(sc.queue)
	sc.mu.Unlock()
	return limit > 0 && (waiting > 0 || s.busyAgents() >= limit)
}

// enqueue shows spec as a queued session until its turn.
func (s *Server) enqueue(spec session.Spec, command []string, typed string) (*session.Session, error) {
	if spec.ID == "" {
		spec.ID = session.NewID()
	}
	held := spec
	held.Command, held.Env = nil, nil
	held.Meta.Queued = true
	limit := s.sched.get().MaxAgents
	held.History = fmt.Appendf(nil, "\x1b[2mQueued: %d %s already working on this host. This one starts when one finishes its turn.\x1b[0m\r\n",
		limit, map[bool]string{true: "agent is", false: "agents are"}[limit == 1])
	sess, err := s.sessions.Create(held)
	if err != nil {
		return nil, err
	}
	s.sched.mu.Lock()
	s.sched.queue = append(s.sched.queue, queued{spec: spec, command: command, typed: typed})
	s.sched.mu.Unlock()
	return sess, nil
}

// unqueue forgets session id's place in the queue, if it has one.
func (s *Server) unqueue(id string) {
	s.sched.mu.Lock()
	s.sched.queue = slices.DeleteFunc(s.sched.queue, func(q queued) bool { return q.spec.ID == id })
	delete(s.sched.started, id)
	delete(s.sched.interrupted, id)
	s.sched.mu.Unlock()
}

// runScheduler starts queued agents as others finish, and interrupts those
// past their budget, until done closes.
func (s *Server) runScheduler(done <-chan struct{}) {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-done:
			return
		case <-s.sched.wake:
		case <-t.C:
		}
		s.startQueued()
		s.enforceBudgets()
		s.hibernate()
	}
}

// startQueued starts queued agents while fewer than the limit work.
func (s *Server) startQueued() {
	sc := &s.sched
	for {
		sc.mu.Lock()
		limit, waiting := sc.settings.MaxAgents, len(sc.queue)
		sc.mu.Unlock()
		if waiting == 0 || limit > 0 && s.busyAgents() >= limit {
			return
		}
		sc.mu.Lock()
		q := sc.queue[0]
		sc.queue = sc.queue[1:]
		sc.mu.Unlock()
		if _, err := s.sessions.Get(q.spec.ID); err != nil {
			continue // deleted while it waited
		}
		sess, err := s.sessions.Replace(q.spec)
		if err != nil {
			s.logger().Warn("could not start a queued session", "id", q.spec.ID, "err", err)
			continue
		}
		s.began(sess, q.command, q.typed)
	}
}

// began records a session whose program started: for a restart, and for
// the scheduler, and types its first prompt once it is ready.
func (s *Server) began(sess *session.Session, command []string, typed string) {
	s.agentModes.forget(sess.ID()) // a new process says its own
	s.persist.update(sess, command)
	_, isAgent := agent.Find(sess.Info().Agent)
	if isAgent {
		s.sched.mu.Lock()
		s.sched.started[sess.ID()] = time.Now()
		s.sched.mu.Unlock()
	}
	trusted := make(chan struct{}) // closed once trust is answered, or not asked
	if isAgent {
		go s.answerTrust(sess, trusted)
	} else {
		close(trusted)
	}
	if typed != "" {
		go func() {
			// A screen that never settles (an animation, an ad) still gets
			// the prompt when the wait is over, rather than losing it.
			deadline := time.Now().Add(promptTimeout)
			for {
				sess.WaitQuiet(promptQuiet, time.Until(deadline))
				if sess.Info().Exited {
					return
				}
				select {
				case <-trusted:
				default:
					if agent.TrustAsked(sess.Terminal().Text()) {
						<-trusted // answered next, then it settles again
						continue
					}
				}
				s.sessions.Input(sess, []byte(typed))
				if strings.HasPrefix(typed, pasteStart) {
					// A program shows a paste before it takes Enter.
					time.Sleep(pasteSettle)
				}
				s.sessions.Input(sess, []byte("\r"))
				return
			}
		}()
	}
}

// input types p's text into its session, as a paste when asked, then
// Enter after the program had a moment to show it.
func (s *Server) input(p proto.SessionInputParams) error {
	sess, err := s.sessions.Get(p.ID)
	if err != nil {
		return err
	}
	if sess.Info().Exited {
		return fmt.Errorf("session %s isn't running, so nothing was sent", p.ID) // wakeThen brings it back
	}
	text := p.Text
	if p.Paste {
		text = pasteStart + text + pasteEnd
	}
	s.sessions.Input(sess, []byte(text))
	if p.Enter {
		if text != "" {
			time.Sleep(pasteSettle)
		}
		s.sessions.Input(sess, []byte("\r"))
	}
	return nil
}

// answerTrust says yes once when a new agent asks to trust its folder, the
// one the user picked for the session (agent.TrustAsked), in its first
// moments; it closes done when it did, or the moments passed.
func (s *Server) answerTrust(sess *session.Session, done chan<- struct{}) {
	defer close(done)
	presses := 0
	for end := time.Now().Add(trustWindow); time.Now().Before(end); time.Sleep(trustPoll) {
		if sess.Info().Exited {
			return
		}
		moves, ok := agent.TrustMoves(sess.Terminal().Text())
		switch {
		case !ok:
		case moves == 0:
			s.sessions.Input(sess, []byte("\r"))
			return
		case presses < maxTrustPresses:
			// One step, then the next look checks it took: a program
			// still starting may drop keys, and Enter on "No" exits.
			key := "\x1b[B" // down
			if moves < 0 {
				key = "\x1b[A"
			}
			s.sessions.Input(sess, []byte(key))
			presses++
		}
	}
}

// How long, and how often, a new agent's screen is watched for a question
// about trusting its folder.
const (
	trustWindow = 30 * time.Second
	trustPoll   = 250 * time.Millisecond
	// maxTrustPresses bounds the arrow keys sent to reach yes, should a
	// list not move as expected.
	maxTrustPresses = 10
)

// enforceBudgets interrupts each agent that just went past its budget.
func (s *Server) enforceBudgets() {
	sc := &s.sched
	for _, sess := range s.sessions.List() {
		info := sess.Info()
		sc.mu.Lock()
		done := sc.interrupted[info.ID]
		if !info.OverBudget {
			delete(sc.interrupted, info.ID)
		}
		sc.mu.Unlock()
		if !info.OverBudget || done || info.Exited {
			continue
		}
		sc.mu.Lock()
		sc.interrupted[info.ID] = true
		sc.mu.Unlock()
		key := "\x03"
		if preset, ok := agent.Find(info.Agent); ok {
			key = preset.InterruptKey()
		}
		s.sessions.Input(sess, []byte(key))
	}
}

// settingsFile is where the daemon keeps the host's settings.
func settingsFile(dir string) string { return filepath.Join(dir, "settings.json") }

// hibernate stops the agents done and idle for longer than the setting
// says, keeping their sessions to continue on their conversation: what
// they hold (memory, a model's session) is freed meanwhile.
func (s *Server) hibernate() {
	s.hibernateWaiting()
	s.sched.mu.Lock()
	after := time.Duration(s.sched.settings.HibernateAfter) * hibernateUnit
	s.sched.mu.Unlock()
	if after <= 0 {
		return
	}
	now := time.Now()
	var ports map[string][]proto.Port // asked once, when a session is due
	for _, sess := range s.sessions.List() {
		meta := sess.Meta()
		preset, ok := agent.Find(meta.Agent)
		if !ok || len(preset.Continue) == 0 || !meta.Relaunch || meta.Parent != "" || meta.NoAutoHibernate || sess.DoneFor(now) < after {
			continue // only what comes back on its own conversation
		}
		// A dev server or an app the agent started would stop with it, and a task under way.
		if meta.Display > 0 || s.goaTaskUnderWay(sess.ID()) != "" {
			continue
		}
		if ports == nil {
			ports, _ = s.allPorts()
			if ports == nil {
				ports = map[string][]proto.Port{}
			}
		}
		if len(ports[sess.ID()]) > 0 {
			continue
		}
		if err := s.hibernateSession(sess, false); err != nil {
			s.logger().Warn("could not hibernate a session", "id", sess.ID(), "err", err)
		}
	}
}

// stopAgents hibernates every agent still running, ready or not, and says
// how many: they come back on their conversation when shown.
func (s *Server) stopAgents() int {
	n := 0
	for _, sess := range s.sessions.List() {
		meta := sess.Meta()
		if _, ok := agent.Find(meta.Agent); !ok || sess.Info().Exited {
			continue
		}
		if err := s.hibernateSession(sess, false); err != nil {
			s.logger().Warn("could not stop an agent", "id", sess.ID(), "err", err)
			continue
		}
		n++
	}
	return n
}

// hibernateUnit is what HibernateAfter counts: minutes (shorter in tests).
var hibernateUnit = time.Minute

// hibernateSession stops sess's program, showing what it showed, marked
// hibernated (byHand: by the user), and stops what it started.
func (s *Server) hibernateSession(sess *session.Session, byHand bool) error {
	info, meta := sess.Info(), sess.Meta()
	meta.Hibernated, meta.HibernatedByHand, meta.HibernatedAt = true, byHand, time.Now().UTC()
	s.procsLook()
	_, err := s.sessions.Replace(session.Spec{
		ID: info.ID, Dir: info.Dir, Created: info.Created, History: sess.History(),
		Command: info.Command, Exited: true, Meta: meta,
	})
	if err != nil {
		return err
	}
	s.wakes.undo(info.ID)
	s.reapSession(info.ID)
	if byHand {
		s.endRegionsOfLead(info) // an idle one, on its own, may be waiting for them
	}
	s.logger().Info("hibernated an idle agent", "id", info.ID, "agent", meta.Agent)
	return nil
}

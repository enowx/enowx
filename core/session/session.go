// Package session keeps terminal sessions alive in the daemon.
//
// Each session runs a program in a PTY and emulates its screen in a
// headless terminal, so clients can attach, detach and reattach at any time
// and always see the current screen and scrollback, cell for cell.
package session

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/plugins/terminal"

	"github.com/enowx/enowx/proto"
)

// ErrNotFound is returned for an unknown session ID.
var ErrNotFound = errors.New("session: not found")

// ErrReplaced is returned when a session changed while being replaced.
var ErrReplaced = errors.New("session: it changed meanwhile")

// Scrollback is how many bytes of scrollback each session keeps.
const Scrollback = 10 << 20

// A session is active while its screen changed within activeFor; the
// screen is checked every activityPeriod. Changes within echoFor of the
// user typing are the echo of their keys, not the program working.
const (
	activeFor      = 1500 * time.Millisecond
	activityPeriod = 150 * time.Millisecond
	echoFor        = 400 * time.Millisecond
	// An idle session no client watches is checked every idleCheck. Streams
	// check at least every streamFresh (daemon/stream.go), so one checked
	// within it is watched.
	idleCheck   = 500 * time.Millisecond
	streamFresh = 500 * time.Millisecond
	// A working agent whose screen stood still for staleWorking stopped
	// without its hooks saying so (interrupted, say): its agents animate
	// while they work.
	staleWorking = 15 * time.Second
)

// Meta describes what a session is for. The daemon fills it from the
// request; the session only reports it.
type Meta struct {
	Name   string // given by the user; empty lets clients pick a label
	Prompt string // first prompt, if given at creation
	// Relaunch says the program comes back after the daemon restarts, as
	// Restart (an agent continuing its conversation; nil: the login shell).
	Relaunch        bool
	Restart         []string
	SkipPermissions bool
	// Conversation is its agent's conversation, by the id the agent
	// resumes it with: it goes on with its own, not the folder's latest.
	Conversation string
	Restored     bool
	Project      string
	Agent        string
	Branch       string
	Worktree     bool
	// Queued: the session waits for the daemon to start its agent
	// (it shows a note and runs nothing meanwhile).
	Queued bool
	// Budget is how long the agent may work, 0 for no limit.
	Budget time.Duration
	// Display is the port of the session's virtual display, 0 for none.
	Display int
	// Parent is the session this one is a tab of (SessionInfo.Parent).
	Parent string
	// DisplayScale is the scale of the display's apps (SessionInfo).
	DisplayScale int
	// Note is what the session is at, as its agent or the user last said
	// (SessionInfo.Note).
	Note string
	// Group is the fan-out the session is part of (SessionInfo.Group).
	Group string
	// Account is the agent's account it runs as (SessionInfo.Account).
	Account string
	// AccountFirst, after a fallback, is "=" and the account the session
	// started its run on (its name, "" its agent's own), to go back to;
	// "" when it is on it.
	AccountFirst string `json:",omitempty"`
	// Config is the agent's config it runs with (SessionInfo.Config).
	Config *proto.SessionConfig `json:",omitempty"`
	// Member is the team's member who started it (SessionInfo.Member).
	Member *proto.SessionMember
	// Private keeps it from the host's teams (SessionInfo.Private).
	Private bool
	// NotesFor is the note whose Notes page agent it is (SessionInfo.NotesFor).
	NotesFor string
	// CanvasFor is the canvas whose Canvas page agent it is
	// (SessionInfo.CanvasFor).
	CanvasFor string
	// Features are its own feature switches (proto.FeatureSetParams).
	Features map[string]bool
	// Creds is the vault's credentials lent to it (SessionInfo.Creds).
	Creds []string
	// Hibernated: the daemon stopped the session's idle agent to free what
	// it held; continuing the session brings it back on its conversation.
	Hibernated bool
	// HibernatedByHand and HibernatedAt say how and when it hibernated.
	HibernatedByHand bool
	HibernatedAt     time.Time
	// NoAutoHibernate keeps the daemon from hibernating it on its own.
	NoAutoHibernate bool
}

// EnvSession names the environment variable holding a session's ID in
// the processes it runs.
const EnvSession = "ENOWX_SESSION"

// Session is one running (or exited) program.
type Session struct {
	id      string
	meta    Meta
	command []string
	dir     string
	created time.Time
	term    *terminal.Terminal
	done    chan struct{}

	mu         sync.Mutex
	title      string
	exited     bool
	code       int
	lastChange time.Time
	screenAt   time.Time // the screen last changed, typing's echo included
	lastInput  time.Time
	streamed   time.Time // when an attached stream last checked the screen
	active     bool
	state      string    // what the agent's hooks last said (proto.State*)
	stateAt    time.Time // when they said it
	shown      string    // the state last reported to OnUpdate
	prompt     string
	typed      []rune // the line being typed, until the first prompt is known
	inEscape   bool
	lines      lineTyper     // every line sent, for OnPrompt
	worked     time.Duration // how long the agent worked, as the watch saw
	tick       time.Time     // the watch's last look, for worked
	overBudget bool
}

// DoneFor is how long the session's agent has been done (its hooks said
// so) with nothing typed or shown since; 0 when it isn't.
func (s *Session) DoneFor(now time.Time) time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.exited || s.state != proto.StateDone || s.active {
		return 0
	}
	since := s.stateAt
	for _, t := range []time.Time{s.lastInput, s.lastChange} {
		if t.After(since) {
			since = t
		}
	}
	return now.Sub(since)
}

// ID returns the session's ID.
func (s *Session) ID() string { return s.id }

// Terminal returns the session's headless terminal.
func (s *Session) Terminal() *terminal.Terminal { return s.term }

// Info describes the session.
func (s *Session) Info() proto.SessionInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	return proto.SessionInfo{
		ID:              s.id,
		Name:            s.meta.Name,
		Note:            s.meta.Note,
		Group:           s.meta.Group,
		Account:         s.meta.Account,
		Config:          cloneConfig(s.meta.Config),
		Member:          s.meta.Member,
		Private:         s.meta.Private,
		NotesFor:        s.meta.NotesFor,
		CanvasFor:       s.meta.CanvasFor,
		Creds:           slices.Clone(s.meta.Creds),
		Hibernated:      s.meta.Hibernated && s.exited,
		HibernatedHow:   hibernatedHow(s.meta, s.exited),
		NoAutoHibernate: s.meta.NoAutoHibernate,
		Project:         s.meta.Project,
		Agent:           s.meta.Agent,
		Branch:          s.meta.Branch,
		Worktree:        s.meta.Worktree,
		Command:         slices.Clone(s.command),
		Dir:             s.dir,
		Title:           s.title,
		Prompt:          s.prompt,
		Created:         s.created,
		Active:          s.active,
		State:           s.stateLocked(time.Now()),
		SkipPermissions: s.meta.SkipPermissions,
		Restored:        s.meta.Restored,
		Exited:          s.exited,
		ExitCode:        s.code,
		Queued:          s.meta.Queued,
		Budget:          int64(s.meta.Budget / time.Second),
		Worked:          int64(s.worked / time.Second),
		OverBudget:      s.overBudget,
		Display:         s.meta.Display,
		Parent:          s.meta.Parent,
		DisplayScale:    s.meta.DisplayScale,
	}
}

// Meta returns what the session is for.
func (s *Session) Meta() Meta {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.meta
	m.Prompt = s.prompt
	return m
}

// History returns the scrollback and screen, to show above a relaunched
// program's output after a restart.
func (s *Session) History() []byte { return s.term.History() }

// ScreenChanged is when the screen last changed, as far as checked; zero
// before it did.
func (s *Session) ScreenChanged() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.screenAt
}

// NoteInput records what the user typed: its echo must not count as
// activity, and its first line becomes the session's prompt when none was
// given. It reports whether the prompt was just set.
//
// Typing answers an agent that waits for the user, and Enter sends its
// next prompt after it finished, so the state the hooks reported no longer
// holds; agents with hooks report the next one themselves.
func (s *Session) NoteInput(p []byte) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastInput = time.Now()
	if s.state == proto.StateWaiting || s.state == proto.StateDone && bytes.ContainsAny(p, "\r\n") {
		s.state = ""
	}
	if s.prompt != "" {
		return false
	}
	for _, r := range string(p) {
		switch {
		case s.inEscape:
			// CSI and SS3 sequences end with a letter or ~; others after one rune.
			if r >= '@' && r <= '~' && r != '[' && r != 'O' {
				s.inEscape = false
			}
		case r == 0x1b:
			s.inEscape = true
		case r == '\r' || r == '\n':
			if line := strings.TrimSpace(string(s.typed)); line != "" {
				s.prompt, s.typed = truncate(line, 200), nil
				return true
			}
			s.typed = nil
		case r == 0x7f || r == 0x08:
			if len(s.typed) > 0 {
				s.typed = s.typed[:len(s.typed)-1]
			}
		case r == 0x15: // Ctrl-U
			s.typed = nil
		case r >= 0x20:
			if len(s.typed) < 400 {
				s.typed = append(s.typed, r)
			}
		}
	}
	return false
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// stateLocked is the agent's state as clients see it: what the hooks said,
// unless it went stale. The caller holds s.mu.
func (s *Session) stateLocked(now time.Time) string {
	if s.exited {
		return ""
	}
	if s.state == proto.StateWorking && now.Sub(s.stateAt) > staleWorking && now.Sub(s.lastChange) > staleWorking {
		return ""
	}
	return s.state
}

// stateChanged reports whether the state clients see changed since it was
// last reported, and marks it reported. The caller holds s.mu.
func (s *Session) stateChanged(now time.Time) bool {
	st := s.stateLocked(now)
	if st == s.shown {
		return false
	}
	s.shown = st
	return true
}

// WaitQuiet waits until the screen has changed at least once and then
// stayed still for quiet, or until timeout. It reports whether the session
// settled; programs show their prompt and then wait, which is when typing
// into them is safe.
func (s *Session) WaitQuiet(quiet, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		last, exited := s.lastChange, s.exited
		s.mu.Unlock()
		if exited {
			return false
		}
		if !last.IsZero() && time.Since(last) >= quiet {
			return true
		}
		select {
		case <-s.done:
			return false
		case <-time.After(100 * time.Millisecond):
		}
	}
	return false
}

// Manager owns all sessions of a daemon.
type Manager struct {
	// OnUpdate is called (from any goroutine, without locks held) when a
	// session is created, changes title, becomes active or idle, or exits.
	// Set it before Create.
	OnUpdate func(proto.SessionInfo)
	// OnPrompt is called, if set, with each line the user sends a
	// session by typing it and Enter: a prompt, to an agent.
	OnPrompt func(id, line string)

	mu       sync.Mutex
	sessions map[string]*Session
	order    []string
}

// NewManager returns an empty manager.
func NewManager() *Manager {
	return &Manager{sessions: map[string]*Session{}}
}

// Spec describes a session to start.
type Spec struct {
	ID         string   // empty for a new session; set when restoring one
	Command    []string // the login shell when empty
	Dir        string
	Env        []string
	Cols, Rows int
	Meta       Meta
	Created    time.Time // zero for now
	// History is shown above the program's output: the screen of the
	// session before the daemon restarted.
	History []byte
	// Exited restores a session whose program had ended: it shows History
	// and runs nothing.
	Exited   bool
	ExitCode int
}

// Create starts a session.
func (m *Manager) Create(spec Spec) (*Session, error) { return m.create(spec, nil) }

// create starts spec, in place of old when it isn't nil.
func (m *Manager) create(spec Spec, old *Session) (*Session, error) {
	if err := terminal.Load(); err != nil {
		return nil, fmt.Errorf("session: load terminal emulator: %w", err)
	}
	s := &Session{
		id:      spec.ID,
		meta:    spec.Meta,
		command: slices.Clone(spec.Command),
		dir:     spec.Dir,
		created: spec.Created,
		done:    make(chan struct{}),
		prompt:  truncate(strings.TrimSpace(spec.Meta.Prompt), 200),
		exited:  spec.Exited,
		code:    spec.ExitCode,
	}
	if s.id == "" {
		s.id = newID()
	}
	if s.created.IsZero() {
		s.created = time.Now()
	}
	opts := terminal.Options{
		Command: marked(s.id, spec.Command),
		Dir:     spec.Dir,
		// EnvSession tells the session's processes apart from the others,
		// for measuring what each session uses.
		Env:        append(slices.Clone(spec.Env), EnvSession+"="+s.id),
		Scrollback: Scrollback,
		// Programs' clipboard reads go to the attached window to answer.
		ClipboardRead: terminal.ClipboardForward,
		OnTitle: func(title string) {
			s.mu.Lock()
			s.title = title
			s.mu.Unlock()
			m.notify(s)
		},
		OnExit: func(code int) {
			s.mu.Lock()
			s.exited, s.code, s.active = true, code, false
			s.mu.Unlock()
			m.notify(s)
		},
	}
	idle := spec.Exited || spec.Meta.Queued
	if idle {
		opts.Conn = newIdleConn() // nothing runs; the history is all it shows
	}
	term, err := terminal.New(opts)
	if err != nil {
		return nil, fmt.Errorf("session: start %v: %w", spec.Command, err)
	}
	s.term = term
	if len(spec.History) > 0 {
		term.Feed(spec.History)
		if !idle {
			term.Feed([]byte("\r\n\x1b[2m── enowX restarted this session ──\x1b[0m\r\n"))
		}
	}
	// Without a size, the program starts at the first client's size (or at
	// 80x24 after half a second), so it doesn't draw once at a size nobody
	// sees and then redraw.
	if spec.Cols > 0 && spec.Rows > 0 {
		term.Resize(spec.Cols, spec.Rows)
	}

	m.mu.Lock()
	if old != nil && m.sessions[s.id] != old {
		// Replaced or removed meanwhile (hibernated and stopped at once,
		// say): old is someone else's to close.
		m.mu.Unlock()
		_ = term.Close()
		return nil, ErrReplaced
	}
	m.sessions[s.id] = s
	if old == nil {
		m.order = append(m.order, s.id)
	}
	m.mu.Unlock()
	if old != nil {
		close(old.done)
		_ = old.term.Close()
	}
	if !idle {
		go m.watchActivity(s)
	}
	m.notify(s)
	return s, nil
}

// watchActivity reports when the session starts or stops producing output.
// While a client is attached, its stream's checks of the screen tell
// (NoteFrame); otherwise a mirror of its own does, less often while idle:
// each check copies the whole screen.
func (m *Manager) watchActivity(s *Session) {
	mirror, err := s.term.NewMirror()
	if err != nil {
		return
	}
	defer mirror.Close()
	t := time.NewTicker(activityPeriod)
	defer t.Stop()
	var checked time.Time // when the mirror last checked the screen
	resync := false       // the mirror missed what streams saw
	for {
		select {
		case <-s.done:
			return
		case <-s.term.Done():
			return
		case <-t.C:
		}
		now := time.Now()
		s.mu.Lock()
		streamed := now.Sub(s.streamed) < streamFresh
		idle := !s.active && now.Sub(s.lastInput) > activeFor
		s.mu.Unlock()
		switch {
		case streamed:
			resync = true
		case idle && now.Sub(checked) < idleCheck:
		default:
			checked = now
			frame, due := mirror.Frame()
			// After streams checked the screen, the first frame brings the
			// mirror up to date rather than showing new output.
			if !resync {
				s.noteScreen(now, len(frame) > 0 || due)
			}
			resync = false
		}
		s.mu.Lock()
		active := !s.exited && !s.lastChange.IsZero() && now.Sub(s.lastChange) < activeFor
		changed := active != s.active
		s.active = active
		changed = s.countWork(now) || changed
		changed = s.stateChanged(now) || changed
		s.mu.Unlock()
		if changed {
			m.notify(s)
		}
	}
}

// NoteFrame records that an attached client's stream checked the screen,
// and whether it changed, so that the activity watch needn't check it too.
func (s *Session) NoteFrame(changed bool) {
	now := time.Now()
	s.mu.Lock()
	s.streamed = now
	s.mu.Unlock()
	s.noteScreen(now, changed)
}

// noteScreen records a check of the screen; changes within echoFor of
// typing are its echo.
func (s *Session) noteScreen(now time.Time, changed bool) {
	s.mu.Lock()
	if changed {
		s.screenAt = now
	}
	if changed && now.Sub(s.lastInput) > echoFor {
		s.lastChange = now
	}
	s.mu.Unlock()
}

// Input types p into the session and records it (see NoteInput).
func (m *Manager) Input(s *Session, p []byte) {
	s.term.Send(p)
	prompted := s.NoteInput(p)
	s.mu.Lock()
	changed := s.stateChanged(time.Now())
	sent := s.lines.feed(p)
	s.mu.Unlock()
	if m.OnPrompt != nil {
		for _, line := range sent {
			m.OnPrompt(s.id, line)
		}
	}
	if prompted || changed {
		m.notify(s)
	}
}

// SetState records the state the agent's hooks report (proto.State*).
func (m *Manager) SetState(s *Session, state string) error {
	switch state {
	case proto.StateWorking, proto.StateWaiting, proto.StateDone:
	default:
		return fmt.Errorf("session: unknown state %q", state)
	}
	now := time.Now()
	s.mu.Lock()
	s.state, s.stateAt = state, now
	changed := s.stateChanged(now)
	s.mu.Unlock()
	if changed {
		m.notify(s)
	}
	return nil
}

// Get returns the session with the given ID.
func (m *Manager) Get(id string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok {
		return nil, ErrNotFound
	}
	return s, nil
}

// List returns all sessions in creation order.
func (m *Manager) List() []*Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*Session, 0, len(m.order))
	for _, id := range m.order {
		out = append(out, m.sessions[id])
	}
	return out
}

// Kill hangs up the session's program and forgets the session.
func (m *Manager) Kill(id string) error {
	m.mu.Lock()
	s, ok := m.sessions[id]
	if ok {
		delete(m.sessions, id)
		m.order = slices.DeleteFunc(m.order, func(o string) bool { return o == id })
	}
	m.mu.Unlock()
	if !ok {
		return ErrNotFound
	}
	close(s.done)
	return s.term.Close()
}

// CloseAll hangs up every session.
func (m *Manager) CloseAll() {
	for _, s := range m.List() {
		_ = m.Kill(s.id)
	}
}

// notify reports a change, except for killed sessions: their program's
// exit after Kill must not announce them again.
func (m *Manager) notify(s *Session) {
	m.mu.Lock()
	live := m.sessions[s.id] == s // not killed, nor replaced
	m.mu.Unlock()
	if m.OnUpdate != nil && s.term != nil && live {
		m.OnUpdate(s.Info())
	}
}

func newID() string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func hibernatedHow(m Meta, exited bool) *proto.HibernateState {
	if !m.Hibernated || !exited || m.HibernatedAt.IsZero() {
		return nil
	}
	return &proto.HibernateState{Manual: m.HibernatedByHand, Since: m.HibernatedAt}
}

// cloneConfig is a copy of c, which Info's callers may keep.
func cloneConfig(c *proto.SessionConfig) *proto.SessionConfig {
	if c == nil {
		return nil
	}
	x := *c
	return &x
}

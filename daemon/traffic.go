package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/display"
	"github.com/enowx/enowx/core/intercept"
	"github.com/enowx/enowx/core/session"
	"github.com/enowx/enowx/proto"
)

// The host's interceptor (docs/plans/traffic.md) starts the first time
// something asks for it, keeps its CA and the targets attached in
// <daemon dir>/intercept, and records in memory.

// trafficFlush is how often changed exchanges are told to clients: a
// busy stream changes many times a second.
const trafficFlush = 150 * time.Millisecond

// traffic is the daemon's interceptor. Its fields belong to mu; the proxy
// and store are safe for use by several goroutines themselves.
type traffic struct {
	once       sync.Once
	err        error
	proxy      *intercept.Proxy
	trust      intercept.Trust
	dir        string
	mu         sync.Mutex
	targets    map[string]*trafficTarget // by source id
	revs       map[string]net.Listener   // reverse proxies, by session id and port
	revSources map[string]string         // their sources
	held       map[string]string         // sessions waiting on a breakpoint → their state before
	rules      map[string]*intercept.Rules
	filters    map[string][]string // saved filters, by project
	pending    map[int64]proto.TrafficSummary
	timer      *time.Timer
	live       bool                  // the proxy runs
	capture    *proto.TrafficCapture // to tell, gathered
	capTimer   *time.Timer
}

// trafficTarget is a target attached, as kept across daemon restarts.
type trafficTarget struct {
	proto.TrafficTarget
	Src   intercept.Source `json:"src"`
	Port  int              `json:"port"`
	Agent bool             `json:"agent,omitempty"`
}

func (s *Server) trafficDir() string { return filepath.Join(filepath.Dir(s.WorktreeRoot), "intercept") }

// trafficOn starts the interceptor once, and returns it.
func (s *Server) trafficOn() (*traffic, error) {
	t := &s.traf
	t.once.Do(func() { t.err = s.startTraffic(t) })
	return t, t.err
}

func (s *Server) startTraffic(t *traffic) error {
	t.dir = s.trafficDir()
	ca, err := intercept.LoadCA(t.dir)
	if err != nil {
		return err
	}
	if t.trust, err = intercept.WriteTrust(t.dir, ca); err != nil {
		return err
	}
	store := intercept.NewStore()
	// Large bodies go to files, cleared with the daemon's capture.
	store.Dir = filepath.Join(t.dir, "bodies")
	os.RemoveAll(store.Dir)
	if err := os.MkdirAll(store.Dir, 0o700); err != nil {
		store.Dir = ""
	}
	store.OnChange = func(sum proto.TrafficSummary) {
		s.queueTraffic(sum)
		s.meterExchange(sum) // an agent's tokens (tokens_layers.go)
	}
	t.targets, t.rules, t.pending = map[string]*trafficTarget{}, map[string]*intercept.Rules{}, map[int64]proto.TrafficSummary{}
	t.proxy = &intercept.Proxy{
		CA: ca, Store: store,
		RulesFor: s.trafficRulesFor,
		OnPaused: func(list []proto.TrafficPaused) {
			s.markHeld(list)
			s.broadcast(proto.EventTrafficPaused, list)
		},
		OnCapture: s.captureChanged,
	}
	t.proxy.SetCapturePrefs(s.loadCapturePrefs())
	t.mu.Lock()
	t.live = true
	t.mu.Unlock()
	if err := s.applyTrafficSettings(t.proxy, s.loadTrafficSettings()); err != nil {
		s.logger().Warn("traffic settings", "err", err)
	}
	port := 0
	if b, err := os.ReadFile(filepath.Join(t.dir, "port")); err == nil {
		port, _ = strconv.Atoi(strings.TrimSpace(string(b)))
	}
	if err := t.proxy.Listen("127.0.0.1:" + strconv.Itoa(port)); err != nil {
		if err = t.proxy.Listen("127.0.0.1:0"); err != nil {
			return err
		}
	}
	os.WriteFile(filepath.Join(t.dir, "port"), []byte(strconv.Itoa(t.proxy.Port())), 0o600)
	// Targets attached before come back on their ports, so the programs
	// pointed at them keep working.
	var kept []*trafficTarget
	if b, err := os.ReadFile(filepath.Join(t.dir, "targets.json")); err == nil {
		json.Unmarshal(b, &kept)
	}
	for _, tg := range kept {
		t.proxy.RestoreSource(tg.Src)
		if p, err := t.proxy.SourcePort(tg.Src, tg.Port); err == nil {
			tg.Port = p
			tg.Proxy = "http://127.0.0.1:" + strconv.Itoa(p)
			t.targets[tg.Src.ID] = tg
		}
	}
	s.saveTargets()
	return nil
}

// stopTraffic closes the interceptor with the daemon.
func (s *Server) stopTraffic() {
	if t := &s.traf; t.proxy != nil {
		t.proxy.Close()
	}
}

// queueTraffic tells clients of a changed exchange soon, once per flush.
func (s *Server) queueTraffic(sum proto.TrafficSummary) {
	t := &s.traf
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pending[sum.ID] = sum
	if t.timer == nil {
		t.timer = time.AfterFunc(trafficFlush, s.flushTraffic)
	}
}

func (s *Server) flushTraffic() {
	t := &s.traf
	t.mu.Lock()
	list := make([]proto.TrafficSummary, 0, len(t.pending))
	for _, sum := range t.pending {
		list = append(list, sum)
	}
	t.pending, t.timer = map[int64]proto.TrafficSummary{}, nil
	t.mu.Unlock()
	slices.SortFunc(list, func(a, b proto.TrafficSummary) int { return int(a.ID - b.ID) })
	for _, sum := range list {
		s.broadcast(proto.EventTrafficExchange, sum)
	}
}

func (s *Server) saveTargets() {
	t := &s.traf
	t.mu.Lock()
	list := make([]*trafficTarget, 0, len(t.targets))
	for _, tg := range t.targets {
		list = append(list, tg)
	}
	t.mu.Unlock()
	slices.SortFunc(list, func(a, b *trafficTarget) int { return a.Attached.Compare(b.Attached) })
	b, _ := json.MarshalIndent(list, "", "  ")
	writeFileAtomic(filepath.Join(t.dir, "targets.json"), b)
}

// trafficStatus says how the interceptor is.
func (s *Server) trafficStatus() proto.TrafficStatus {
	t, err := s.trafficOn()
	if err != nil {
		return proto.TrafficStatus{Error: err.Error()}
	}
	n, bytes := t.proxy.Store.Stats()
	st := proto.TrafficStatus{Port: t.proxy.Port(), Fingerprint: t.proxy.CurrentCA().Fingerprint(), Exchanges: n, Bytes: bytes, Display: needsDisplay()}
	for _, b := range intercept.Browsers() {
		st.Browsers = append(st.Browsers, proto.TrafficProgram{ID: b.ID, Name: b.Name, Path: b.Path})
	}
	return st
}

func (s *Server) trafficCA() (proto.TrafficCA, error) {
	t, err := s.trafficOn()
	if err != nil {
		return proto.TrafficCA{}, err
	}
	ca := t.proxy.CurrentCA()
	return proto.TrafficCA{PEM: string(ca.PEM), Fingerprint: ca.Fingerprint(), SPKI: ca.SPKIHash(), File: t.trust.CA, Bundle: t.trust.Bundle}, nil
}

// trafficRulesFor is the rules for src's traffic: its project's, or its
// session's project's, else the host's own.
func (s *Server) trafficRulesFor(src intercept.Source) *intercept.Rules {
	project := src.Project
	if project == "" && src.Session != "" {
		if sess, err := s.sessions.Get(src.Session); err == nil {
			project = sess.Meta().Project
		}
	}
	rs, _ := s.loadTrafficRules(project)
	return rs
}

// rulesFile is where a project keeps its rules, or the host its own.
func (s *Server) rulesFile(project string) string {
	if project == "" {
		return filepath.Join(s.trafficDir(), "rules.json")
	}
	return filepath.Join(project, ".enowx", "traffic.json")
}

func (s *Server) loadTrafficRules(project string) (*intercept.Rules, error) {
	t := &s.traf
	t.mu.Lock()
	rs, ok := t.rules[project]
	t.mu.Unlock()
	if ok {
		return rs, nil
	}
	var file proto.TrafficRules
	if b, err := os.ReadFile(s.rulesFile(project)); err == nil {
		if err := json.Unmarshal(b, &file); err != nil {
			return nil, fmt.Errorf("traffic: %s: %w", s.rulesFile(project), err)
		}
	}
	rs = intercept.CompileRules(file.Rules, projectFiles(project))
	t.mu.Lock()
	t.rules[project] = rs
	if t.filters == nil {
		t.filters = map[string][]string{}
	}
	t.filters[project] = file.Filters
	t.mu.Unlock()
	return rs, nil
}

// projectFiles reads a project's files for mocks' bodies, never outside
// it.
func projectFiles(project string) func(string) ([]byte, error) {
	return func(name string) ([]byte, error) {
		if project == "" {
			return nil, errors.New("only a project's rules can answer from its files")
		}
		target, err := insideOf(project, name)
		if err != nil {
			return nil, err
		}
		return os.ReadFile(target)
	}
}

func (s *Server) trafficRules(p proto.TrafficRulesParams) (proto.TrafficRules, error) {
	if _, err := s.trafficOn(); err != nil {
		return proto.TrafficRules{}, err
	}
	rs, err := s.loadTrafficRules(p.Project)
	if err != nil {
		return proto.TrafficRules{}, err
	}
	t := &s.traf
	t.mu.Lock()
	filters := t.filters[p.Project]
	t.mu.Unlock()
	return proto.TrafficRules{Project: p.Project, Rules: nonNil(rs.List()), Filters: filters}, nil
}

// setTrafficRules saves a project's rules and filters, keeping the rules'
// hits.
func (s *Server) setTrafficRules(p proto.TrafficRules) (proto.TrafficRules, error) {
	if _, err := s.trafficOn(); err != nil {
		return proto.TrafficRules{}, err
	}
	old, _ := s.loadTrafficRules(p.Project)
	hits := old.Hits()
	for i := range p.Rules {
		r := &p.Rules[i]
		if r.ID == "" {
			r.ID = "rule-" + strconv.FormatInt(time.Now().UnixNano(), 36) + strconv.Itoa(i)
		}
		r.Hits, r.Problem = hits[r.ID], ""
	}
	saved := proto.TrafficRules{Rules: p.Rules, Filters: p.Filters}
	for i := range saved.Rules {
		saved.Rules[i].Hits = 0
	}
	b, err := json.MarshalIndent(saved, "", "  ")
	if err != nil {
		return proto.TrafficRules{}, err
	}
	file := s.rulesFile(p.Project)
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return proto.TrafficRules{}, err
	}
	if err := writeFileAtomic(file, append(b, '\n')); err != nil {
		return proto.TrafficRules{}, err
	}
	for i := range p.Rules {
		p.Rules[i].Hits = hits[p.Rules[i].ID]
	}
	rs := intercept.CompileRules(p.Rules, projectFiles(p.Project))
	t := &s.traf
	t.mu.Lock()
	t.rules[p.Project] = rs
	if t.filters == nil {
		t.filters = map[string][]string{}
	}
	t.filters[p.Project] = p.Filters
	t.mu.Unlock()
	return proto.TrafficRules{Project: p.Project, Rules: nonNil(rs.List()), Filters: p.Filters}, nil
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// addTarget registers a source for a target and opens its port.
func (s *Server) addTarget(kind, label, session string, agentToo bool) (*trafficTarget, error) {
	t, err := s.trafficOn()
	if err != nil {
		return nil, err
	}
	project := ""
	if session != "" {
		sess, err := s.sessions.Get(session)
		if err != nil {
			return nil, err
		}
		project = sess.Meta().Project
		if label == "" {
			label = sess.Meta().Name
		}
	}
	src := t.proxy.AddSource(kind, label, session, project)
	port, err := t.proxy.SourcePort(src, 0)
	if err != nil {
		t.proxy.RemoveSource(src.ID)
		return nil, err
	}
	tg := &trafficTarget{Src: src, Port: port, Agent: agentToo, TrafficTarget: proto.TrafficTarget{
		Source: src.ID, Kind: kind, Label: label, Session: session,
		Proxy: "http://127.0.0.1:" + strconv.Itoa(port), Attached: time.Now(),
	}}
	t.mu.Lock()
	t.targets[src.ID] = tg
	t.mu.Unlock()
	s.saveTargets()
	return tg, nil
}

// trafficEnvOf is the environment of a target's programs.
func (s *Server) trafficEnvOf(tg *trafficTarget) []string {
	env := intercept.Env(tg.Proxy, s.traf.trust)
	return append(env, "ENOWX_TRAFFIC_SOURCE="+tg.Src.ID)
}

// trafficEnvFor is what a session's environment gets from a session
// target: its tabs (parent is theirs), and its agent too when asked
// (agentOf is the session whose agent starts).
func (s *Server) trafficEnvFor(parent, agentOf string) []string {
	t := &s.traf
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, tg := range t.targets {
		if tg.Kind != proto.TargetSession || tg.Src.Quiet {
			continue
		}
		if (parent != "" && tg.Session == parent) || (agentOf != "" && tg.Session == agentOf && tg.Agent) {
			env := intercept.Env(tg.Proxy, t.trust)
			return append(env, "ENOWX_TRAFFIC_SOURCE="+tg.Src.ID)
		}
	}
	return nil
}

func (s *Server) trafficTargets() []proto.TrafficTarget {
	t := &s.traf
	t.mu.Lock()
	defer t.mu.Unlock()
	out := []proto.TrafficTarget{}
	for _, tg := range t.targets {
		if !tg.Src.Quiet {
			out = append(out, tg.TrafficTarget)
		}
	}
	slices.SortFunc(out, func(a, b proto.TrafficTarget) int { return a.Attached.Compare(b.Attached) })
	return out
}

func (s *Server) targetsChanged() {
	s.saveTargets()
	s.broadcast(proto.EventTrafficAttached, s.trafficTargets())
}

// attach attaches the interceptor to a target.
func (s *Server) attach(p proto.TrafficAttachParams) (proto.TrafficTarget, error) {
	switch p.Kind {
	case proto.TargetSession:
		if p.Session == "" {
			return proto.TrafficTarget{}, errors.New("traffic: which session?")
		}
		tg, err := s.addTarget(p.Kind, p.Label, p.Session, p.Agent)
		if err != nil {
			return proto.TrafficTarget{}, err
		}
		tg.Note = "New tabs of this session and what the Run menu starts are caught; tabs open now need This terminal."
		if p.Agent {
			tg.Note += " The agent's own traffic is caught once it starts again."
		}
		s.targetsChanged()
		return tg.TrafficTarget, nil
	case proto.TargetTerminal:
		return s.attachTerminal(p)
	case proto.TargetShell:
		return s.attachShell(p)
	case proto.TargetBrowser, proto.TargetElectron:
		return s.attachProgram(p)
	case proto.TargetManual, proto.TargetWebView, proto.TargetAndroid:
		tg, err := s.addTarget(p.Kind, p.Label, p.Session, false)
		if err != nil {
			return proto.TrafficTarget{}, err
		}
		// A device's own settings are made by its host's daemon
		// (android.proxy), which the app asks, wherever the device is.
		tg.Device = p.Device
		s.targetsChanged()
		return tg.TrafficTarget, nil
	case proto.TargetDocker:
		return s.attachDocker(p)
	}
	return proto.TrafficTarget{}, fmt.Errorf("traffic: %q can't be attached", p.Kind)
}

// attachTerminal opens a new tab of a session whose shell sends its
// traffic through the interceptor.
func (s *Server) attachTerminal(p proto.TrafficAttachParams) (proto.TrafficTarget, error) {
	if p.Session == "" {
		return proto.TrafficTarget{}, errors.New("traffic: a terminal opens in a session")
	}
	tg, err := s.addTarget(proto.TargetTerminal, "Terminal", p.Session, false)
	if err != nil {
		return proto.TrafficTarget{}, err
	}
	tab, err := s.createSession(proto.SessionCreateParams{Parent: p.Session, Name: "Traffic", Agent: agent.Shell, Env: s.trafficEnvOf(tg)})
	if err != nil {
		s.detach(tg.Source)
		return proto.TrafficTarget{}, err
	}
	tg.Tab = tab.ID()
	s.targetsChanged()
	return tg.TrafficTarget, nil
}

// attachShell types the environment into an existing tab's shell, so
// what it runs next is caught.
func (s *Server) attachShell(p proto.TrafficAttachParams) (proto.TrafficTarget, error) {
	tab, err := s.sessions.Get(p.Tab)
	if err != nil {
		return proto.TrafficTarget{}, err
	}
	if !isShell(tab) {
		return proto.TrafficTarget{}, errors.New("traffic: that tab runs a program, not a shell: open a new terminal instead")
	}
	session := tab.Meta().Parent
	if session == "" {
		session = tab.ID()
	}
	tg, err := s.addTarget(proto.TargetShell, tab.Meta().Name, session, false)
	if err != nil {
		return proto.TrafficTarget{}, err
	}
	tg.Tab = tab.ID()
	line := " " + strings.ReplaceAll(strings.TrimSpace(intercept.ShellExports(s.trafficEnvOf(tg))), "\n", "; ") + "\r"
	s.sessions.Input(tab, []byte(line))
	s.targetsChanged()
	return tg.TrafficTarget, nil
}

// isShell reports whether a session's program is a shell waiting for
// commands.
func isShell(sess *session.Session) bool {
	info := sess.Info()
	if info.Exited {
		return false
	}
	return info.Agent == agent.Shell || info.Agent == ""
}

// attachProgram starts a browser or an Electron app on a fresh profile,
// in a tab of the session (on its display, on a server without a screen).
func (s *Server) attachProgram(p proto.TrafficAttachParams) (proto.TrafficTarget, error) {
	if p.Session == "" {
		return proto.TrafficTarget{}, errors.New("traffic: a browser opens from a session")
	}
	t, err := s.trafficOn()
	if err != nil {
		return proto.TrafficTarget{}, err
	}
	var prog intercept.Program
	if p.Kind == proto.TargetBrowser {
		for _, b := range intercept.Browsers() {
			if b.ID == p.Program || b.Path == p.Program {
				prog = b
			}
		}
		if prog.Path == "" {
			return proto.TrafficTarget{}, fmt.Errorf("traffic: no browser %q on this host", p.Program)
		}
	} else {
		exe := intercept.AppExecutable(p.Program)
		if _, err := os.Stat(exe); err != nil {
			return proto.TrafficTarget{}, fmt.Errorf("traffic: %s: %w", p.Program, err)
		}
		prog = intercept.Program{ID: "electron", Name: strings.TrimSuffix(filepath.Base(p.Program), ".app"), Path: exe}
	}
	tg, err := s.addTarget(p.Kind, prog.Name, p.Session, false)
	if err != nil {
		return proto.TrafficTarget{}, err
	}
	tg.Program = prog.Path
	profile := filepath.Join(t.dir, "profiles", tg.Source)
	var args []string
	if prog.Firefox {
		a, note, err := intercept.FirefoxProfile(profile, tg.Port, t.trust.CA, "")
		if err != nil {
			s.detach(tg.Source)
			return proto.TrafficTarget{}, err
		}
		args, tg.Note = a, note
	} else {
		args = intercept.ChromiumArgs(profile, tg.Port, t.proxy.CurrentCA().SPKIHash(), "")
	}
	tab, err := s.createSession(proto.SessionCreateParams{
		Parent: p.Session, Name: prog.Name, Agent: agent.Custom, Command: append([]string{prog.Path}, args...),
		Env: s.trafficEnvOf(tg), Display: needsDisplay(),
	})
	if err != nil {
		s.detach(tg.Source)
		return proto.TrafficTarget{}, err
	}
	tg.Tab = tab.ID()
	s.targetsChanged()
	return tg.TrafficTarget, nil
}

// needsDisplay reports whether windows started here need a virtual
// display: a Linux host without a screen.
func needsDisplay() bool {
	if runtime.GOOS != "linux" || os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != "" {
		return false
	}
	ok, _ := display.Available()
	return ok
}

// detach stops catching a target. Its port stays, passing traffic on
// unrecorded, so programs pointed at it keep working; a tab opened for it
// stays open.
func (s *Server) detach(source string) error {
	t, err := s.trafficOn()
	if err != nil {
		return err
	}
	t.mu.Lock()
	tg, ok := t.targets[source]
	if ok {
		tg.Src.Quiet = true
	}
	t.mu.Unlock()
	if !ok {
		return errors.New("traffic: that target isn't attached")
	}
	t.proxy.SetQuiet(source, true)
	s.closeReverses(source)
	s.targetsChanged()
	return nil
}

// trafficEnv is the environment for a shell, as a session's own source
// (made the first time) or source.
func (s *Server) trafficEnv(p proto.TrafficEnvParams) (proto.TrafficEnv, error) {
	t, err := s.trafficOn()
	if err != nil {
		return proto.TrafficEnv{}, err
	}
	var tg *trafficTarget
	t.mu.Lock()
	if p.Source != "" {
		tg = t.targets[p.Source]
	}
	t.mu.Unlock()
	if tg == nil {
		if p.Source != "" {
			return proto.TrafficEnv{}, errors.New("traffic: no such source")
		}
		if tg, err = s.addTarget(proto.TargetShell, "Shell", p.Session, false); err != nil {
			return proto.TrafficEnv{}, err
		}
		s.targetsChanged()
	} else if tg.Src.Quiet {
		t.mu.Lock()
		tg.Src.Quiet = false
		t.mu.Unlock()
		t.proxy.SetQuiet(tg.Source, false)
		s.targetsChanged()
	}
	env := s.trafficEnvOf(tg)
	return proto.TrafficEnv{Source: tg.Source, Env: env, Shell: intercept.ShellExports(env), Unset: intercept.ShellUnset()}, nil
}

// markHeld shows a session whose app waits at a breakpoint as waiting for
// the user, and gives it its state back once none does, unless something
// else changed it meanwhile.
func (s *Server) markHeld(list []proto.TrafficPaused) {
	now := map[string]bool{}
	for _, p := range list {
		if p.Session != "" {
			now[p.Session] = true
		}
	}
	t := &s.traf
	t.mu.Lock()
	if t.held == nil {
		t.held = map[string]string{}
	}
	var hold, release []string
	before := map[string]string{}
	for id := range now {
		if _, ok := t.held[id]; !ok {
			hold = append(hold, id)
		}
	}
	for id, st := range t.held {
		if !now[id] {
			release = append(release, id)
			before[id] = st
			delete(t.held, id)
		}
	}
	t.mu.Unlock()
	for _, id := range hold {
		sess, err := s.sessions.Get(id)
		if err != nil {
			continue
		}
		t.mu.Lock()
		t.held[id] = sess.Info().State
		t.mu.Unlock()
		s.sessions.SetState(sess, proto.StateWaiting)
	}
	for _, id := range release {
		sess, err := s.sessions.Get(id)
		if err != nil || sess.Info().State != proto.StateWaiting || before[id] == "" {
			continue
		}
		s.sessions.SetState(sess, before[id])
	}
}

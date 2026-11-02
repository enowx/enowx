package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/hostcontrol"
	"github.com/enowx/enowx/core/procstat"
	"github.com/enowx/enowx/core/session"
	"github.com/enowx/enowx/proto"
)

// Hibernating sessions by hand (proto.MethodSessionHibernate): the same
// as the daemon does with idle agents, asked for. A session that would
// lose work is planned first, for the user to confirm, or waits for its
// agent to be done.

// hibernations are the sessions waiting for their agent to be done to
// hibernate.
type hibernations struct {
	mu      sync.Mutex
	waiting map[string]bool
}

// hibernatePrefs are the user's hibernation preferences, synced between
// hosts (settings/hibernate.json): whether new sessions keep from
// hibernating on their own, and the sessions kept so.
type hibernatePrefs struct {
	NeverByDefault bool            `json:"neverByDefault,omitempty"`
	Sessions       map[string]bool `json:"sessions,omitempty"` // never on their own
}

func (s *Server) hibernatePrefsFile() string {
	return filepath.Join(s.dataDir, "settings", "hibernate.json")
}

func (s *Server) hibernatePrefs() hibernatePrefs {
	var p hibernatePrefs
	if data, err := os.ReadFile(s.hibernatePrefsFile()); err == nil {
		_ = json.Unmarshal(data, &p)
	}
	return p
}

func (s *Server) saveHibernatePrefs(p hibernatePrefs) error {
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	file := s.hibernatePrefsFile()
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		return err
	}
	tmp := file + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, file)
}

// setAutoHibernate keeps a session from hibernating on its own, or sets
// the default for new sessions.
func (s *Server) setAutoHibernate(p proto.SessionAutoHibernateParams) error {
	prefs := s.hibernatePrefs()
	if p.Default {
		prefs.NeverByDefault = p.Never
		return s.saveHibernatePrefs(prefs)
	}
	sess, err := s.sessions.Get(p.ID)
	if err != nil {
		return err
	}
	s.sessions.SetNoAutoHibernate(sess, p.Never)
	s.persist.update(sess, nil)
	if prefs.Sessions == nil {
		prefs.Sessions = map[string]bool{}
	}
	if p.Never {
		prefs.Sessions[p.ID] = true
	} else {
		delete(prefs.Sessions, p.ID)
	}
	return s.saveHibernatePrefs(prefs)
}

// hibernateSessions hibernates what p asks, as far as it can without
// losing work unless asked to. An agent asking has the user say yes first,
// unless bypass is the permission mode it runs without asking in
// (sessionBypasses).
func (s *Server) hibernateSessions(p proto.SessionHibernateParams, bypass string) (proto.SessionHibernateResult, error) {
	out := proto.SessionHibernateResult{Plans: []proto.HibernatePlan{}}
	if _, err := s.sessions.Get(p.From); p.From != "" && err != nil {
		p.From = "" // not an agent here: the user's own terminal
	}
	ports, _ := s.allPorts()
	var targets []*session.Session
	if p.Idle {
		for _, sess := range s.sessions.List() {
			info := sess.Info()
			if !info.Exited && info.Parent == "" && info.State != proto.StateWorking && info.NotesFor == "" && info.ID != p.From {
				targets = append(targets, sess)
			}
		}
	}
	for _, id := range p.IDs {
		sess, err := s.sessions.Get(id)
		if err != nil {
			return out, err
		}
		if !slices.Contains(targets, sess) {
			targets = append(targets, sess)
		}
	}
	for _, sess := range targets {
		plan := s.hibernatePlan(sess, ports)
		if sess.ID() == p.From && p.From != "" {
			plan.Blocked = "an agent can't hibernate its own session"
		}
		if p.Idle && plan.Blocked == "" && plan.Busy() {
			continue // all idle: only those with nothing to lose
		}
		if p.Idle && plan.Blocked != "" && !slices.Contains(p.IDs, sess.ID()) {
			continue
		}
		out.Plans = append(out.Plans, plan)
	}
	if p.DryRun {
		return out, nil
	}
	if bypass == proto.ControlAlways && slices.ContainsFunc(out.Plans, func(pl proto.HibernatePlan) bool {
		return pl.Blocked == "" && s.stoppingLoses(p.From, pl.ID, ports)
	}) {
		bypass = "" // always allowed is for what loses nothing
	}
	if p.From != "" && slices.ContainsFunc(out.Plans, func(pl proto.HibernatePlan) bool { return pl.Blocked == "" }) {
		if bypass != "" {
			s.control.add(proto.ControlLogEntry{Time: time.Now(), From: p.From, Method: proto.MethodSessionHibernate,
				Level: hostcontrol.LevelOf(proto.MethodSessionHibernate).String(), Bypass: bypass})
		} else {
			// An agent asks: the user says.
			raw, err := s.askApp("session.hibernate", p, 5*time.Minute)
			if err != nil {
				return out, fmt.Errorf("hibernate: %w", err)
			}
			var yes bool
			if json.Unmarshal(raw, &yes) != nil || !yes {
				return out, errors.New("hibernate: the user said no")
			}
		}
	}
	for _, plan := range out.Plans {
		if plan.Blocked != "" {
			continue
		}
		if plan.Busy() && !p.Force {
			if p.WhenDone && plan.Working {
				s.hibernates.mu.Lock()
				if s.hibernates.waiting == nil {
					s.hibernates.waiting = map[string]bool{}
				}
				s.hibernates.waiting[plan.ID] = true
				s.hibernates.mu.Unlock()
				out.Waiting = append(out.Waiting, plan.ID)
			}
			continue
		}
		sess, err := s.sessions.Get(plan.ID)
		if err != nil {
			continue
		}
		if err := s.hibernateSession(sess, true); err != nil {
			return out, err
		}
		out.Hibernated = append(out.Hibernated, plan.ID)
	}
	return out, nil
}

// hibernateWaiting hibernates the sessions waiting for their agent, once
// it is done.
func (s *Server) hibernateWaiting() {
	s.hibernates.mu.Lock()
	var ids []string
	for id := range s.hibernates.waiting {
		ids = append(ids, id)
	}
	s.hibernates.mu.Unlock()
	for _, id := range ids {
		sess, err := s.sessions.Get(id)
		if err != nil || sess.Info().Exited {
			s.forgetWaiting(id)
			continue
		}
		if st := sess.Info().State; st == proto.StateWorking {
			continue
		}
		s.forgetWaiting(id)
		if err := s.hibernateSession(sess, true); err != nil {
			s.logger().Warn("could not hibernate a session", "id", id, "err", err)
		}
	}
}

func (s *Server) forgetWaiting(id string) {
	s.hibernates.mu.Lock()
	delete(s.hibernates.waiting, id)
	s.hibernates.mu.Unlock()
}

// hibernatePlan is what hibernating sess would stop and free.
func (s *Server) hibernatePlan(sess *session.Session, ports map[string][]proto.Port) proto.HibernatePlan {
	info, meta := sess.Info(), sess.Meta()
	plan := proto.HibernatePlan{ID: info.ID, Name: info.Name}
	if plan.Name == "" {
		plan.Name = info.Title
	}
	preset, isAgent := agent.Find(meta.Agent)
	switch {
	case info.Exited && meta.Hibernated:
		plan.Blocked = "hibernated already"
	case info.Exited:
		plan.Blocked = "it isn't running"
	case meta.Parent != "":
		plan.Blocked = "a tab: hibernate its session"
	case meta.Queued:
		plan.Blocked = "queued: it hasn't started"
	case !isAgent || len(preset.Continue) == 0 || !meta.Relaunch:
		plan.Blocked = "it couldn't come back on its conversation (a shell, or an agent enowX can't continue)"
	}
	if plan.Blocked != "" {
		return plan
	}
	plan.Working = info.State == proto.StateWorking || info.State == "" && info.Active
	plan.Display = meta.Display > 0
	for _, port := range ports[info.ID] {
		plan.Ports = append(plan.Ports, port.Port)
	}
	own := s.sessionProgram(info.ID) // samples now
	st := &s.statsState
	for _, u := range s.stats().Sessions {
		if u.ID == info.ID {
			plan.RSS, plan.CPU = u.RSS, u.CPU
		}
	}
	// What it started: under its program, and what left its tree.
	seen := map[int]bool{os.Getpid(): true}
	var pids []int
	left := map[int]bool{}
	for pid := range own {
		for _, d := range st.sampler.Descendants(pid) {
			if !own[d] && !seen[d] {
				seen[d] = true
				pids = append(pids, d)
			}
		}
	}
	for _, pid := range procstat.Tagged(session.EnvSession, info.ID) {
		if !own[pid] && !seen[pid] {
			seen[pid], left[pid] = true, true
			pids = append(pids, pid)
		}
	}
	slices.Sort(pids)
	for _, pid := range pids {
		name := "process"
		if args := procstat.Args(pid); len(args) > 0 {
			name = filepath.Base(args[0])
		}
		plan.Procs = append(plan.Procs, proto.HibernateProc{PID: pid, Name: name, RSS: st.sampler.One(pid).RSS, Left: left[pid]})
	}
	return plan
}

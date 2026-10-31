package daemon

import (
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/hooks"
	"github.com/enowx/enowx/core/session"
	"github.com/enowx/enowx/proto"
)

// The user's hooks run here: on events an agent's own hooks hand over
// (session.hook), on those the daemon sees from outside (hooks_watch.go,
// hooks_git.go, hooks_host.go). Where hooks are kept, and agents'
// proposals, are in hooks_scope.go. See docs/hooks.md.

// hookLogMax is how many runs the log keeps.
const hookLogMax = 200

// hookEngine runs the host's, its projects' and its sessions' hooks and
// keeps their log, the project hook files waiting for the user and the
// changes agents proposed. Safe from any goroutine.
type hookEngine struct {
	store   *hooks.Store
	limits  hooks.Limiter
	propose string // the file proposals are kept in

	mu        sync.Mutex
	log       []proto.HookRun // the latest last
	pending   map[string]proto.ProjectHooks
	watch     map[string]*sessionWatch // by session ID (hooks_watch.go)
	sessions  map[string][]proto.Hook  // sessions' own hooks
	proposals []proto.HookProposal
	asleep    map[string]bool // sessions hibernated, to tell their waking
}

func newHookEngine(dir string) *hookEngine {
	e := &hookEngine{store: hooks.Open(dir), propose: dir + string(os.PathSeparator) + "hooks-proposed.json",
		pending: map[string]proto.ProjectHooks{}, watch: map[string]*sessionWatch{}, sessions: map[string][]proto.Hook{}, asleep: map[string]bool{}}
	e.loadProposals()
	return e
}

// waits says the agent (or the app) waits on event ev's hooks, so they
// can refuse it, add context or change its command.
func waits(ev proto.HookEvent) bool {
	switch ev.Via {
	case proto.HookInAgent:
		return ev.Event == proto.HookToolBefore || ev.Event == proto.HookPromptStart || ev.Event == proto.HookSessionStart || ev.Event == proto.HookGitBefore
	case proto.HookInApp:
		return ev.Event == proto.HookGitBefore
	}
	return false
}

// eventOf is event e of session info, filled in with the session's agent
// and folders and the host, what the agent sent masked of the vault's
// secrets.
func (s *Server) eventOf(info proto.SessionInfo, e proto.HookEvent) proto.HookEvent {
	if info.ID != "" {
		e.Session, e.Agent, e.Dir, e.Branch = info.ID, agent.Base(info.Agent), info.Dir, info.Branch
		e.Project = info.Project
		if e.Project == "" {
			e.Project = info.Dir
		}
	}
	e.Host = s.userdata.Name
	e.Prompt, e.Command, e.Text = s.maskLent(e.Prompt), s.maskLent(e.Command), s.maskLent(e.Text)
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	return e
}

// hooksFor are the hooks that may run for session info: the host's, its
// project's own once the user allowed their file (a file not allowed yet
// waits in the pending list), then the session's own.
func (s *Server) hooksFor(info proto.SessionInfo) []proto.Hook {
	list := s.hooks.store.List()
	if info.Dir != "" {
		ph, allowed := s.hooks.store.Project(info.Dir)
		s.hooks.mu.Lock()
		if len(ph.Hooks) > 0 && !allowed {
			if _, known := s.hooks.pending[info.Dir]; !known {
				defer s.broadcast(proto.EventHooksChanged, nil)
			}
			s.hooks.pending[info.Dir] = ph
		} else {
			delete(s.hooks.pending, info.Dir)
		}
		s.hooks.mu.Unlock()
		if allowed {
			list = append(list, ph.Hooks...)
		}
	}
	if info.ID != "" {
		s.hooks.mu.Lock()
		list = append(list, s.hooks.sessions[info.ID]...)
		s.hooks.mu.Unlock()
	}
	return list
}

// hookRunner is what session info's hooks run with: its ID and enowX's
// program, the daemon's environment without the vault's secrets, and the
// apps to show notifications.
func (s *Server) hookRunner(info proto.SessionInfo) hooks.Runner {
	secrets := s.lentSecrets()
	env := []string{}
	for _, kv := range os.Environ() {
		_, v, _ := strings.Cut(kv, "=")
		if v == "" || !slices.Contains(secrets, v) {
			env = append(env, kv)
		}
	}
	if info.ID != "" {
		env = append(env, session.EnvSession+"="+info.ID)
	}
	if s.HookBin != "" {
		env = append(env, agent.EnvBin+"="+s.HookBin)
	}
	return hooks.Runner{Env: env, Clean: true, Bin: s.HookBin, Notify: func(kind, text string) {
		s.broadcast(proto.EventNotifySound, proto.NotifySoundParams{Kind: kind, Session: info.ID, Text: text})
	}, Judge: func(ev proto.HookEvent) (proto.HookDecision, string) { return s.judgeHook(info, ev) }}
}

// fireHooks runs the hooks matching event ev of session info, within
// their limits, and says what the agent is to do: the first refusal stops
// the rest; contexts add up; a changed command is what the next hooks
// see. Hooks of events nothing waits on, and those set to, run in the
// background.
func (s *Server) fireHooks(info proto.SessionInfo, ev proto.HookEvent) proto.HookDecision {
	ev = s.eventOf(info, ev)
	matched := hooks.For(s.hooksFor(info), ev)
	var d proto.HookDecision
	if len(matched) == 0 {
		return d
	}
	r := s.hookRunner(info)
	wait := waits(ev)
	var contexts []string
	approved := false // by the judge: the user isn't asked
	for _, h := range matched {
		if !s.hooks.limits.Take(h, ev, time.Now()) {
			continue
		}
		if !wait || h.Background {
			go func() {
				run, _ := r.Run(h, ev)
				run.Blocked = false // nothing waits on it to refuse
				s.logHookRun(run)
			}()
			continue
		}
		run, hd := r.Run(h, ev)
		s.logHookRun(run)
		if hd.Block {
			return proto.HookDecision{Block: true, Reason: hd.Reason}
		}
		if hd.Context != "" {
			contexts = append(contexts, hd.Context)
		}
		approved = approved || hd.Approve
		if hd.Command != "" && hd.Command != ev.Command {
			ev.Command, d.Command = hd.Command, hd.Command
		}
	}
	d.Context = strings.Join(contexts, "\n\n")
	if d.Command != "" {
		d = s.rewritten(info, d)
	}
	d.Approve = d.Approve || approved && !d.Block
	return d
}

// rewritten is decision d, its command changed, as session info's agent
// can take it: Claude Code asks the user about the new command unless the
// session skips permissions; Codex takes one only as approved, so a
// session that asks gets it as a refusal naming the command to run.
func (s *Server) rewritten(info proto.SessionInfo, d proto.HookDecision) proto.HookDecision {
	d.Approve = info.SkipPermissions
	if agent.Base(info.Agent) == "codex" && !info.SkipPermissions {
		return proto.HookDecision{Block: true, Reason: "A hook changes this command. Run this instead: " + d.Command, Context: d.Context}
	}
	return d
}

// logHookRun keeps run in the log, the oldest dropped past hookLogMax.
func (s *Server) logHookRun(run proto.HookRun) {
	s.hooks.mu.Lock()
	s.hooks.log = append(s.hooks.log, run)
	if len(s.hooks.log) > hookLogMax {
		s.hooks.log = slices.Clone(s.hooks.log[len(s.hooks.log)-hookLogMax:])
	}
	s.hooks.mu.Unlock()
}

// hookLog is the runs kept, the latest first.
func (s *Server) hookLog() []proto.HookRun {
	s.hooks.mu.Lock()
	defer s.hooks.mu.Unlock()
	out := slices.Clone(s.hooks.log)
	slices.Reverse(out)
	return out
}

// testHook runs hook t.Hook on its sample event now, as an agent would
// wait on it, and logs it.
func (s *Server) testHook(t proto.HookTest) (proto.HookRun, error) {
	if err := hooks.Check(t.Hook); err != nil {
		return proto.HookRun{}, err
	}
	ev := t.Event
	if ev.Event == "" {
		ev.Event = t.Hook.Event
	}
	ev = s.eventOf(proto.SessionInfo{}, ev)
	run, _ := s.hookRunner(proto.SessionInfo{ID: ev.Session}).Run(t.Hook, ev)
	s.logHookRun(run)
	return run, nil
}

package daemon

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/git"
	"github.com/enowx/enowx/proto"
)

// What the daemon sees of a session from outside, for agents whose own
// hooks don't say it: its program starting and ending, a prompt typed, a
// turn ending as its screen goes quiet; and, for every agent, its
// starting to work, the files a turn changed, an error, its hibernating
// and waking, and what the user answered to a permission.

// turnQuiet is how long the screen of an agent without hooks stays quiet
// before its turn counts as done.
const turnQuiet = 4 * time.Second

// sessionWatch is what the daemon keeps of a session for its hooks.
// hookEngine.mu guards it.
type sessionWatch struct {
	started, ended bool
	working        bool
	quiet          *time.Timer
	dirty          map[string]fileStamp // the folder's changed files when its turn began; nil when no hook asks
	asked          string               // the tool the agent asked permission for, unanswered
}

type fileStamp struct {
	mod  int64
	size int64
}

func (e *hookEngine) watchOf(id string) *sessionWatch {
	w := e.watch[id]
	if w == nil {
		w = &sessionWatch{}
		e.watch[id] = w
	}
	return w
}

// wants says some hook that may run for session info is on for event.
func (s *Server) wants(info proto.SessionInfo, event string) bool {
	return slices.ContainsFunc(s.hooksFor(info), func(h proto.Hook) bool { return h.On && h.Event == event })
}

// userSession says info is a session of the user's: not the app's own
// agents (Notes, Canvas), nor a tab.
func userSession(info proto.SessionInfo) bool {
	return info.NotesFor == "" && info.CanvasFor == "" && info.Parent == ""
}

// watchSession follows session info's updates for the events its agent's
// hooks don't hand over.
func (s *Server) watchSession(info proto.SessionInfo) {
	if !userSession(info) {
		return
	}
	support := agent.HookSupport(info.Agent)
	outside := func(event string) bool { return support[event] == proto.HookOutside }
	var fire []proto.HookEvent
	s.hooks.mu.Lock()
	w := s.hooks.watchOf(info.ID)
	if !w.started && !info.Exited {
		w.started = true
		if s.hooks.asleep[info.ID] {
			delete(s.hooks.asleep, info.ID)
			fire = append(fire, proto.HookEvent{Event: proto.HookWake, Via: proto.HookOutside})
		}
		if outside(proto.HookSessionStart) {
			fire = append(fire, proto.HookEvent{Event: proto.HookSessionStart, Via: proto.HookOutside})
		}
	}
	end := info.Exited && !w.ended
	if end {
		w.ended = true
		if w.quiet != nil {
			w.quiet.Stop()
		}
		delete(s.hooks.watch, info.ID)
		switch {
		case info.Hibernated:
			s.hooks.asleep[info.ID] = true
			fire = append(fire, proto.HookEvent{Event: proto.HookHibernate, Via: proto.HookOutside})
		default:
			if info.ExitCode != 0 {
				fire = append(fire, proto.HookEvent{Event: proto.HookError, Exit: info.ExitCode, Via: proto.HookOutside})
			}
			if outside(proto.HookSessionEnd) {
				fire = append(fire, proto.HookEvent{Event: proto.HookSessionEnd, Exit: info.ExitCode, Via: proto.HookOutside})
			}
		}
	}
	var began, quiet bool
	if !info.Exited {
		working := info.State == proto.StateWorking || info.State == "" && info.Active
		began, quiet = working && !w.working, !working && w.working
		w.working = working
		if began {
			fire = append(fire, proto.HookEvent{Event: proto.HookWorking, Via: proto.HookOutside})
			if w.quiet != nil {
				w.quiet.Stop()
			}
		}
		if quiet && outside(proto.HookTurnDone) {
			id := info.ID
			w.quiet = time.AfterFunc(turnQuiet, func() { s.quietTurn(id) })
		}
	}
	s.hooks.mu.Unlock()

	if len(fire) > 0 {
		go func() {
			for _, ev := range fire {
				s.fireHooks(info, ev)
			}
		}()
	}
	if began && outside(proto.HookTurnDone) {
		go s.turnStarted(info)
	}
	if end && !info.Hibernated {
		s.forgetSession(info.ID)
		if outside(proto.HookSessionEnd) {
			go s.ragOffer(info)
		}
	}
}

// quietTurn ends the turn of session id's agent, its screen quiet since.
func (s *Server) quietTurn(id string) {
	sess, err := s.sessions.Get(id)
	if err != nil {
		return
	}
	info := sess.Info()
	s.hooks.mu.Lock()
	w := s.hooks.watch[id]
	still := w != nil && !w.working
	s.hooks.mu.Unlock()
	if !still || info.Exited {
		return
	}
	s.fireHooks(info, proto.HookEvent{Event: proto.HookTurnDone, Via: proto.HookOutside})
	s.turnEnded(info)
	s.ragSoon(sessionProject(info))
	go s.judgeState(info) // does it need the user? (when the use is on)
}

// promptTyped is a prompt the user typed into session id, for agents whose
// hooks don't hand prompts over: seen as it goes, nothing to refuse.
func (s *Server) promptTyped(id, text string) {
	sess, err := s.sessions.Get(id)
	if err != nil {
		return
	}
	info := sess.Info()
	if !userSession(info) || agent.HookSupport(info.Agent)[proto.HookPromptStart] != proto.HookOutside {
		return
	}
	go s.fireHooks(info, proto.HookEvent{Event: proto.HookPromptStart, Prompt: text, Via: proto.HookOutside})
}

// permissionAsked notes the tool session info's agent asked to run; the
// next tool run of it says the user allowed it, the end of the turn or a
// new prompt first that they didn't.
func (s *Server) permissionAsked(info proto.SessionInfo, tool string) {
	s.hooks.mu.Lock()
	s.hooks.watchOf(info.ID).asked = tool
	s.hooks.mu.Unlock()
}

// permissionAnswer fires the answer to the permission session info's
// agent asked, if one waits, after event ev of its own.
func (s *Server) permissionAnswer(info proto.SessionInfo, ev proto.HookEvent) {
	s.hooks.mu.Lock()
	w := s.hooks.watch[info.ID]
	asked := ""
	if w != nil {
		asked = w.asked
	}
	var allowed, answered bool
	switch {
	case asked == "":
	case ev.Event == proto.HookToolAfter && (ev.Tool == asked || asked == "*"):
		allowed, answered = true, true
	case ev.Event == proto.HookTurnDone || ev.Event == proto.HookPromptStart:
		answered = true
	}
	if answered {
		w.asked = ""
	}
	s.hooks.mu.Unlock()
	if answered {
		go s.fireHooks(info, proto.HookEvent{Event: proto.HookAnswered, Tool: asked, Allowed: allowed, Via: proto.HookInAgent})
	}
}

// turnStarted notes the files already changed in session info's folder,
// when a hook asks for those a turn changes.
func (s *Server) turnStarted(info proto.SessionInfo) {
	if !s.wants(info, proto.HookFileChanged) {
		return
	}
	snap := changedFiles(info.Dir)
	s.hooks.mu.Lock()
	s.hooks.watchOf(info.ID).dirty = snap
	s.hooks.mu.Unlock()
}

// turnEnded runs the hooks on the files session info's turn changed:
// those changed now that weren't, or changed again, since it began.
func (s *Server) turnEnded(info proto.SessionInfo) {
	s.hooks.mu.Lock()
	var before map[string]fileStamp
	if w := s.hooks.watch[info.ID]; w != nil {
		before, w.dirty = w.dirty, nil
	}
	s.hooks.mu.Unlock()
	if before == nil {
		return
	}
	now := changedFiles(info.Dir)
	var paths []string
	for p, st := range now {
		if old, ok := before[p]; !ok || old != st {
			paths = append(paths, p)
		}
	}
	for p := range before {
		if _, ok := now[p]; !ok {
			paths = append(paths, p) // committed or put back
		}
	}
	if len(paths) == 0 {
		return
	}
	slices.Sort(paths)
	s.fireHooks(info, proto.HookEvent{Event: proto.HookFileChanged, Paths: paths, Via: proto.HookOutside})
}

// changedFiles are the files git sees changed in folder dir, with their
// size and time; nil outside a repository.
func changedFiles(dir string) map[string]fileStamp {
	out, err := git.Run(dir, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return nil
	}
	files := map[string]fileStamp{}
	entries := strings.Split(out, "\x00")
	for i := 0; i < len(entries); i++ {
		e := entries[i]
		if len(e) < 4 {
			continue
		}
		p := e[3:]
		if e[0] == 'R' || e[0] == 'C' {
			i++ // the name it had follows
		}
		var st fileStamp
		if fi, err := os.Stat(filepath.Join(dir, p)); err == nil {
			st = fileStamp{mod: fi.ModTime().UnixNano(), size: fi.Size()}
		}
		files[p] = st
	}
	return files
}

// sessionProject is session info's project folder, else its own.
func sessionProject(info proto.SessionInfo) string {
	if info.Project != "" {
		return info.Project
	}
	return info.Dir
}

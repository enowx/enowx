package goa

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/proto"
)

// Want is what a task with no session named needs of one.
type Want struct {
	Project string
	Agent   string // "" any agent
	Config  string // "" any; "official" none; else a config's ID or name
}

// PickSession is the session of list a task like want goes to, ok false
// when the project has none, the one case that calls for a new one. A session
// qualifies when it is an agent's of its own in the project's folder (no
// worktree, whose branch is another task's; no shell tab, where a prompt could
// land in the shell), the user's, that runs or can come back. Busy or asking it
// still qualifies, and takes the task after its turn: the user wants to talk
// to the agent that knows the project. Of those: one free before one busy
// before a hibernated one before one that ended; then the newest.
func PickSession(list []proto.SessionInfo, want Want, busy func(id string) bool) (proto.SessionInfo, bool) {
	var found []proto.SessionInfo
	for _, s := range list {
		if qualifies(s, want) {
			found = append(found, s)
		}
	}
	if len(found) == 0 {
		return proto.SessionInfo{}, false
	}
	slices.SortStableFunc(found, func(a, b proto.SessionInfo) int {
		if d := pickRank(a, busy) - pickRank(b, busy); d != 0 {
			return d
		}
		if c := b.Created.Compare(a.Created); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
	return found[0], true
}

func qualifies(s proto.SessionInfo, want Want) bool {
	if s.GOA || s.Parent != "" || s.NotesFor != "" || s.CanvasFor != "" || s.Group != "" || s.Member != nil || s.Worktree || s.Queued {
		return false
	}
	if s.Project == "" || filepath.Clean(s.Project) != filepath.Clean(want.Project) {
		return false
	}
	if want.Agent != "" && s.Agent != want.Agent && agent.Base(s.Agent) != agent.Base(want.Agent) {
		return false
	}
	preset, ok := agent.Find(s.Agent)
	if !ok || s.Agent == agent.Shell || s.Agent == agent.Custom {
		return false
	}
	switch want.Config {
	case "":
	case "official":
		if s.Config != nil {
			return false
		}
	default:
		if s.Config == nil || s.Config.ID != want.Config && !strings.EqualFold(s.Config.Name, want.Config) {
			return false
		}
	}
	return !s.Exited || len(preset.Continue) > 0 // it can come back
}

func pickRank(s proto.SessionInfo, busy func(string) bool) int {
	switch {
	case !s.Exited && !Busy(s) && !busy(s.ID):
		return 0
	case !s.Exited:
		return 1
	case s.Hibernated:
		return 2
	}
	return 3
}

// ComesBack says session s, once its agent stopped, can come back on its conversation:
// not a plain shell, nor a program enowX can't continue.
func ComesBack(s proto.SessionInfo) bool {
	preset, ok := agent.Find(s.Agent)
	return ok && len(preset.Continue) > 0
}

// Busy says a running session is in a turn or asking: a prompt would land
// on top of it, and waits for its turn's end.
func Busy(s proto.SessionInfo) bool {
	return !s.Exited && (s.State == proto.StateWorking || s.State == proto.StateWaiting)
}

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
	Skip    bool   // the task asked to skip permission prompts
	Config  string // "" any; "official" none; else a config's ID or name
}

// PickSession is the session of list a task like want goes to, ok false
// when a new one is called for. A session qualifies when it is an agent's
// of its own in the project's folder (no worktree, whose branch is another
// task's), the user's, that can take a prompt now or come back to, not
// working, not asking the user, and with no open task (busy). Of those: a
// running one before a hibernated one before one that ended; then the
// newest.
func PickSession(list []proto.SessionInfo, want Want, busy func(id string) bool) (proto.SessionInfo, bool) {
	var found []proto.SessionInfo
	for _, s := range list {
		if qualifies(s, want) && !busy(s.ID) {
			found = append(found, s)
		}
	}
	if len(found) == 0 {
		return proto.SessionInfo{}, false
	}
	slices.SortStableFunc(found, func(a, b proto.SessionInfo) int {
		if d := pickRank(a) - pickRank(b); d != 0 {
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
	if s.Project == "" || filepath.Clean(s.Project) != filepath.Clean(want.Project) || s.SkipPermissions != want.Skip {
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
	if s.Exited {
		return len(preset.Continue) > 0 // it can come back
	}
	switch s.State {
	case proto.StateWorking, proto.StateWaiting:
		return false
	case "":
		// No hook said so: fine for an agent whose hooks would have, not
		// for one that may be working unseen.
		return agent.HookSupport(s.Agent)[proto.HookToolBefore] == proto.HookInAgent
	}
	return true
}

func pickRank(s proto.SessionInfo) int {
	switch {
	case !s.Exited:
		return 0
	case s.Hibernated:
		return 1
	}
	return 2
}

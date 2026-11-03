package ui

import (
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// sessionStatus is what the UI shows for a session's state.
type sessionStatus struct {
	label  string
	color  ui.Color
	hollow bool // drawn as a ring: nothing is happening
}

func statusOf(s proto.SessionInfo) sessionStatus {
	switch {
	case s.Hibernated:
		return sessionStatus{label: hibernatedLabel(s), color: theme.Idle, hollow: true}
	case s.Exited && s.Restored && s.ExitCode == -1:
		return sessionStatus{label: "stopped", color: theme.Idle}
	case s.Exited && s.ExitCode == 0:
		return sessionStatus{label: "done", color: theme.Done}
	case s.Exited:
		return sessionStatus{label: "exited " + strconv.Itoa(s.ExitCode), color: theme.Error}
	case s.Queued:
		return sessionStatus{label: "queued", color: theme.Idle, hollow: true}
	case s.State == proto.StateWaiting:
		return sessionStatus{label: "needs input", color: theme.Warning}
	case isWorking(s):
		return sessionStatus{label: "working", color: theme.Working}
	case s.State == proto.StateDone:
		return sessionStatus{label: "done", color: theme.Done, hollow: true}
	}
	return sessionStatus{label: "idle", color: theme.Idle, hollow: true}
}

// isWorking reports whether s's program is on a task: as its agent's hooks
// say, else as its screen shows.
func isWorking(s proto.SessionInfo) bool {
	if s.Exited {
		return false
	}
	switch s.State {
	case proto.StateWorking:
		return true
	case proto.StateWaiting, proto.StateDone:
		return false
	}
	return s.Active
}

// sessionName labels a session: the name the user gave it, else what the
// program says it is doing (its terminal title), else its first prompt,
// else its branch or agent.
func (a *App) sessionName(s proto.SessionInfo) string {
	if s.Name != "" {
		return widget.LongLine(s.Name)
	}
	if t := cleanTitle(s.Title); t != "" && !a.genericTitle(s, t) {
		return widget.LongLine(t)
	}
	if s.Prompt != "" {
		return textfmt.FirstLine(s.Prompt, 60)
	}
	if s.Worktree && s.Branch != "" {
		return s.Branch
	}
	if s.Agent == "" && len(s.Command) > 0 {
		return filepath.Base(s.Command[0])
	}
	return a.agentName(s.Agent)
}

// cleanTitle drops the spinners and marks agents put before their title.
func cleanTitle(t string) string {
	return strings.TrimSpace(strings.TrimLeftFunc(t, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}))
}

// genericTitle reports titles that say nothing about the task: the agent's
// own name, or a shell's "user@host: dir".
func (a *App) genericTitle(s proto.SessionInfo, t string) bool {
	lower := strings.ToLower(t)
	ag := a.agentInfo(s.Agent)
	if lower == strings.ToLower(ag.Name) || lower == strings.ToLower(ag.Command) || lower == s.Agent {
		return true
	}
	for _, generic := range []string{"claude code", "claude", "codex", "openai codex", "opencode", "gemini", "gemini cli", "kiro", "kiro cli", "kiro-cli"} {
		if lower == generic {
			return true
		}
	}
	if at, colon := strings.Index(t, "@"), strings.Index(t, ":"); at > 0 && colon > at {
		return true
	}
	return strings.HasPrefix(t, "~") || strings.HasPrefix(t, "/")
}

func (a *App) agentName(id string) string {
	for _, ag := range a.agents {
		if ag.ID == id {
			return ag.Name
		}
	}
	if id == "" {
		return "Shell"
	}
	return id
}

// projectSessions returns the sessions of a project, oldest first; tabs
// show inside theirs.
func (a *App) projectSessions(path string) []proto.SessionInfo {
	var out []proto.SessionInfo
	for _, s := range a.sessions {
		if s.Parent == "" && (s.Project == path || (s.Project == "" && s.Dir == path)) {
			out = append(out, s)
		}
	}
	return out
}

func (a *App) currentProject() (proto.ProjectInfo, bool) {
	for _, p := range a.projects {
		if p.Path == a.project {
			return p, true
		}
	}
	return proto.ProjectInfo{}, false
}

// canContinue reports whether ended session s can run its program again
// in place (session.continue): agents and shells, not custom commands.
func canContinue(s proto.SessionInfo) bool {
	return s.Exited && s.Agent != "" && s.Agent != agent.Custom
}

// continueSession runs ended session s's program again in it.
func (a *App) continueSession(s proto.SessionInfo) {
	cl := a.client
	if cl == nil {
		return
	}
	go func() {
		info, err := cl.ContinueSession(s.ID)
		a.onMain(func() {
			if err != nil {
				a.status = client.Explain(err)
				return
			}
			a.upsert(info)
		})
	}()
}

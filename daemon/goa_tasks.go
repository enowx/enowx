package daemon

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/decide"
	"github.com/enowx/enowx/core/goa"
	"github.com/enowx/enowx/proto"
)

// GOA's tasks: handed to a session on any host with a model and effort
// the judge picked (or the user), followed to their end.

// switchPause lets an agent take a /model or /effort before the next line.
const switchPause = 1500 * time.Millisecond

// goaResultMax bounds a task's result kept on the board.
const goaResultMax = 1200

// goaDelegate hands p's task to its session, routed, and puts it on the
// board.
func (s *Server) goaDelegate(p proto.GOADelegateParams) (proto.GOATask, error) {
	if strings.TrimSpace(p.Prompt) == "" {
		return proto.GOATask{}, errors.New("goa: delegate what? (a prompt)")
	}
	t := proto.GOATask{Title: p.Title, Prompt: p.Prompt, Host: p.Host, Session: p.Session, Agent: p.Agent,
		Model: p.Model, Effort: p.Effort, Config: p.Config}
	p.SkipPermissions = p.SkipPermissions || s.goaSkipsUnheard(p.Agent)
	var target proto.SessionInfo
	if p.Session != "" {
		info, err := s.goaSessionOn(p.From, p.Host, p.Session)
		if err != nil {
			return t, err
		}
		if info.GOA {
			return proto.GOATask{}, errors.New("goa: a task goes to another session, not GOD of Agents' own: start a new one")
		}
		target, t.Session, t.Agent = info, info.ID, info.Agent
	} else if p.Project != "" && p.Branch == "" {
		// A session the project has before a new one; a new branch is a
		// new one's.
		info, ok, err := s.goaPick(p)
		if err != nil {
			return t, err
		}
		if ok {
			target, t.Session, t.Agent, t.Reused = info, info.ID, info.Agent, true
		}
	}
	if t.Agent == "" {
		t.Agent = "claude"
	}
	var switches []string
	switch {
	case p.Model != "" || p.Effort != "" || p.Config != "":
		t.Override, t.Why = true, "chosen by hand"
		switches = decide.Route{Model: p.Model, Effort: p.Effort}.Switch(t.Agent)
	case p.NoRoute:
		t.Why = "the agent's own model and effort, as asked"
	default:
		project := ""
		if s.thisHost(p.Host) {
			project = p.Project
		}
		rc, err := s.routeFor(proto.DecideRouteParams{Agent: t.Agent, Prompt: p.Prompt, Project: project})
		switch {
		case err != nil:
			t.Why = "not routed (" + err.Error() + "): the agent's own model"
		case rc.Error != "":
			t.Why = "the judge didn't answer (" + rc.Error + "): the agent's own model"
		case rc.Config != "" && t.Session != "":
			// Another provider's model: a running session stays on its own.
			t.Difficulty, t.Why = rc.Difficulty, rc.Why+" (the session keeps its own config and model)"
		default:
			t.Model, t.Effort, t.Difficulty, t.Why, switches = rc.Model, rc.Effort, rc.Difficulty, rc.Why, rc.Switch
			if rc.Config != "" && t.Config == "" {
				t.Config = rc.Config
			}
		}
	}
	if t.Session != "" {
		// An existing session: back and ready first, or nothing reaches
		// it; its model switched, then the prompt.
		woken, err := s.goaReady(p, target)
		if err != nil {
			return t, err
		}
		t.Woken = woken
		for _, line := range switches {
			if err := s.goaCall(p.From, p.Host, proto.MethodSessionInput, proto.SessionInputParams{ID: t.Session, Text: line, Enter: true}, nil); err != nil {
				return t, err
			}
			time.Sleep(switchPause)
		}
		if err := s.goaCall(p.From, p.Host, proto.MethodSessionInput, proto.SessionInputParams{ID: t.Session, Text: p.Prompt, Paste: true, Enter: true}, nil); err != nil {
			return t, err
		}
	} else {
		info, err := s.goaSpawn(p, t)
		if err != nil {
			return t, err
		}
		t.Session, target = info.ID, info
	}
	t.Project = nonEmpty(p.Project, target.Project)
	t.SessionName, t.Dir, t.Branch = target.Name, target.Dir, target.Branch
	if target.Model != "" {
		t.Ran = []string{target.Model + " · " + nonEmpty(target.Effort, "unknown")}
	}
	t, err := s.goa.board.Add(t)
	if err != nil {
		return t, err
	}
	s.goaRememberDelegation(t)
	s.goaChanged()
	return t, nil
}

// goaSpawn starts a new session for task t on p's host, as GOA's worker.
func (s *Server) goaSpawn(p proto.GOADelegateParams, t proto.GOATask) (proto.SessionInfo, error) {
	c := proto.SessionCreateParams{Agent: t.Agent, Project: p.Project, Prompt: p.Prompt, Model: t.Model, Effort: t.Effort,
		Name: nonEmpty(p.Title, goa.TitleOf(p.Prompt)), BranchMode: proto.BranchNone, SkipPermissions: p.SkipPermissions,
		GOAWorker: nonEmpty(p.From, s.goaID()), Cols: 120, Rows: 40}
	if p.Branch != "" {
		c.BranchMode, c.Branch = proto.BranchNew, p.Branch
	}
	if p.Project != "" {
		if err := s.goaCall(p.From, p.Host, proto.MethodProjectAdd, proto.ProjectRef{Path: p.Project}, nil); err != nil {
			return proto.SessionInfo{}, err
		}
	}
	switch {
	case t.Config != "" && !strings.EqualFold(t.Config, "official"):
		var list []proto.Account
		if err := s.goaCall(p.From, p.Host, proto.MethodAccountList, nil, &list); err != nil {
			return proto.SessionInfo{}, err
		}
		i := slices.IndexFunc(list, func(a proto.Account) bool {
			return a.Member == "" && (a.ID == t.Config || a.Agent == agent.Base(t.Agent) && strings.EqualFold(a.Name, t.Config))
		})
		if i < 0 {
			return proto.SessionInfo{}, fmt.Errorf("goa: %s has no config %s there (configs lists them)", t.Agent, t.Config)
		}
		c.Account = list[i].ID
	case t.Config == "":
		c.AccountAuto = true
	}
	var info proto.SessionInfo
	err := s.goaCall(p.From, p.Host, proto.MethodSessionCreate, c, &info)
	return info, err
}

// goaSessionOn is session id (or the start of one, or its name) on host.
func (s *Server) goaSessionOn(from, host, id string) (proto.SessionInfo, error) {
	var list []proto.SessionInfo
	if err := s.goaCall(from, host, proto.MethodSessionList, nil, &list); err != nil {
		return proto.SessionInfo{}, err
	}
	var found []proto.SessionInfo
	for _, info := range list {
		switch {
		case info.ID == id:
			return info, nil
		case strings.HasPrefix(info.ID, id) || strings.EqualFold(info.Name, id):
			found = append(found, info)
		}
	}
	if len(found) != 1 {
		return proto.SessionInfo{}, fmt.Errorf("goa: no one session %q there (sessions lists them)", id)
	}
	return found[0], nil
}

// goaWorkerChanged puts worker info's change on the board, and tells GOA
// what it needs to act on: a task done (its result), waiting (what it
// asks, and whose it is to answer), failed or stopped.
func (s *Server) goaWorkerChanged(info proto.SessionInfo) {
	status := goa.WorkerStatus(info)
	if status == "" {
		return
	}
	changed := s.goa.board.Of(info.ID, func(t *proto.GOATask) bool {
		if t.Status == status {
			return false
		}
		t.Status = status
		if status != proto.GOATaskWaiting {
			t.Asks, t.Urgent = "", ""
		}
		return true
	})
	for _, t := range changed {
		where := "session " + t.Session
		if t.Host != "" {
			where += " on " + t.Host
		}
		hostFlag := ""
		if t.Host != "" {
			hostFlag = " --host " + t.Host
		}
		head := fmt.Sprintf("[enowX GOA] Task %s %q (%s)", t.ID, t.Title, where)
		switch status {
		case proto.GOATaskTurnEnded:
			said := s.goaResult(t)
			t, _ = s.goa.board.Update(t.ID, func(t *proto.GOATask) { t.Said = said })
			s.goaTell(head + " finished a turn, which may be mid-work: " + clip(oneLine(said), 400) + " Check it (chat " + t.Session + hostFlag + "), then settle it: goa task " + t.ID + " --status done|failed --result \"…\".")
		case proto.GOATaskWaiting:
			ask := s.goaAskOf(t)
			t, _ = s.goa.board.Update(t.ID, func(t *proto.GOATask) { t.Asks, t.Urgent = askLine(ask), ask.Urgent })
			if ask.Urgent != "" {
				s.goaTell(head + " waits on the user (" + ask.Urgent + "): " + t.Asks + ". It is theirs to answer: don't.")
			} else {
				s.goaTell(head + " asks: " + nonEmpty(t.Asks, "something its screen shows (read "+t.Session+hostFlag+")") + ". Answer it: answer " + t.Session + hostFlag + " allow|deny|N.")
			}
		case proto.GOATaskFailed:
			s.goaRememberOutcome(t)
			s.goaTell(head + " failed: its agent ended with an error. Read it (read " + t.Session + hostFlag + ").")
		case proto.GOATaskStopped:
			s.goaRememberOutcome(t)
			s.goaTell(head + " stopped: its agent ended.")
		}
	}
	if len(changed) > 0 {
		s.goaChanged()
	}
}

// goaResult is what task t's session said last, or its screen's end.
func (s *Server) goaResult(t proto.GOATask) string {
	from := ""
	if g, ok := s.goaSession(); ok {
		from = g.ID
	}
	var tr proto.Transcript
	if err := s.goaCall(from, t.Host, proto.MethodSessionTranscript, proto.TranscriptParams{ID: t.Session, Tail: 20}, &tr); err == nil {
		for i := len(tr.Entries) - 1; i >= 0; i-- {
			if e := tr.Entries[i]; e.Kind == proto.TranscriptAssistant && strings.TrimSpace(e.Text) != "" {
				return clip(strings.TrimSpace(e.Text), goaResultMax)
			}
		}
	}
	var sc proto.SessionScreen
	if err := s.goaCall(from, t.Host, proto.MethodSessionScreen, proto.SessionRef{ID: t.Session}, &sc); err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimRight(sc.Text, " \n"), "\n")
	if len(lines) > 12 {
		lines = lines[len(lines)-12:]
	}
	return clip(strings.TrimSpace(strings.Join(lines, "\n")), goaResultMax)
}

// goaAskOf is what task t's session waits on.
func (s *Server) goaAskOf(t proto.GOATask) proto.SessionAsk {
	from := ""
	if g, ok := s.goaSession(); ok {
		from = g.ID
	}
	var ask proto.SessionAsk
	_ = s.goaCall(from, t.Host, proto.MethodSessionAsk, proto.SessionRef{ID: t.Session}, &ask)
	return ask
}

// askLine is what ask asks, in a line.
func askLine(ask proto.SessionAsk) string {
	if ask.Deadline != "" {
		ask.Prompt += " (it gives " + ask.Deadline + ": read the screen and answer soon)"
	}
	switch {
	case ask.Prompt != "" && ask.Tool == "" && len(ask.Questions) == 0:
		return clip(oneLine(ask.Prompt), 300) + ". Answer with its keys, as its screen shows (send), or answer if it offers them"
	case len(ask.Questions) > 0:
		return ask.Questions[0].Question
	case ask.Tool != "":
		return clip(oneLine(strings.TrimSpace(ask.Tool+" "+ask.Detail)), 300)
	}
	return ask.Reason
}

// goaSetTask is GOA settling a task: its status and result.
func (s *Server) goaSetTask(p proto.GOATaskParams) (proto.GOATask, error) {
	switch p.Status {
	case "", proto.GOATaskWorking, proto.GOATaskWaiting, proto.GOATaskDone, proto.GOATaskFailed, proto.GOATaskStopped:
	default:
		return proto.GOATask{}, fmt.Errorf("goa: a task is working, waiting, done, failed or stopped, not %q", p.Status)
	}
	settles := false
	t, err := s.goa.board.Update(p.ID, func(t *proto.GOATask) {
		if p.Status != "" {
			settles = goa.Settled(p.Status) && !goa.Settled(t.Status)
			t.Status = p.Status
		}
		if p.Result != "" {
			t.Result = clip(p.Result, goaResultMax)
		} else if settles && t.Result == "" {
			t.Result = t.Said // what it last said is all that says how it ended
		}
	})
	if err == nil {
		if settles {
			s.goaRememberOutcome(t)
		}
		s.goaChanged()
	}
	return t, err
}

// goaOverride is the user's model and effort for task p.ID: switched in
// its session where its agent can, kept on the board.
func (s *Server) goaOverride(p proto.GOAOverrideParams) (proto.GOATask, error) {
	var t proto.GOATask
	for _, x := range s.goa.board.List() {
		if x.ID == p.ID {
			t = x
		}
	}
	if t.ID == "" {
		return t, fmt.Errorf("goa: no task %q on the board", p.ID)
	}
	lines := decide.Route{Model: p.Model, Effort: p.Effort}.Switch(t.Agent)
	if len(lines) == 0 {
		return t, fmt.Errorf("goa: %s's model and effort can't be switched while it runs", t.Agent)
	}
	if !goa.Settled(t.Status) && t.Session != "" {
		for _, line := range lines {
			if err := s.goaCall("", t.Host, proto.MethodSessionInput, proto.SessionInputParams{ID: t.Session, Text: line, Enter: true}, nil); err != nil {
				return t, err
			}
			time.Sleep(switchPause)
		}
	}
	t, err := s.goa.board.Update(t.ID, func(t *proto.GOATask) {
		if p.Model != "" {
			t.Model = p.Model
		}
		if p.Effort != "" {
			t.Effort = p.Effort
		}
		t.Override, t.Why = true, "chosen by you"
	})
	if err == nil {
		s.goaChanged()
		s.goaTell(fmt.Sprintf("[enowX GOA] The user switched task %s %q to model %s, effort %s.", t.ID, t.Title, nonEmpty(t.Model, "its own"), nonEmpty(t.Effort, "its own")))
	}
	return t, err
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

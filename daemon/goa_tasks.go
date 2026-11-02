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
	"github.com/enowx/enowx/core/noteblocks"
	"github.com/enowx/enowx/proto"
)

// GOA's tasks: handed to a session on any host with a model and effort
// the judge picked (or the user), followed to their end.

// switchPause lets an agent take a /model or /effort before the next line.
const switchPause = 1500 * time.Millisecond

// goaResultMax bounds a task's result kept on the board and a reply in a line to
// GOA, which so needn't read the session's screen; chat ID has the rest.
const goaResultMax = 2000

// goaDelegate hands p's task to its session, routed, and puts it on the
// board.
func (s *Server) goaDelegate(p proto.GOADelegateParams) (proto.GOATask, error) {
	if strings.TrimSpace(p.Prompt) == "" {
		return proto.GOATask{}, errors.New("goa: delegate what? (a prompt)")
	}
	p.Prompt += goaToDoLines(p) + goaReportAsk
	t := proto.GOATask{Title: p.Title, Prompt: p.Prompt, Host: p.Host, Session: p.Session, Agent: p.Agent,
		Model: p.Model, Effort: p.Effort, Config: p.Config, Note: p.Note, Items: p.Items}
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
		s.goaSkipOn(p, target)
		// Busy: it takes the task at its turn's end, in order (goaDeliver).
		if !t.Woken && (goa.Busy(target) || s.goaOpenTask(t.Session)) {
			t.Status = proto.GOATaskQueued
			t.Why = strings.TrimSpace(t.Why + " Queued: its session is busy, and gets it when its turn ends.")
		}
		if t.Status == proto.GOATaskQueued {
			return s.goaAddTask(p, t, target)
		}
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
		why := "New session: the project has none to take it."
		if p.Branch != "" {
			why = "New session: asked for, on branch " + p.Branch + "."
		} else if p.Project == "" {
			why = "New session: no project named."
		}
		t.Why = strings.TrimSpace(t.Why + " " + why)
	}
	return s.goaAddTask(p, t, target)
}

// goaAddTask keeps where task t goes and puts it on the board.
func (s *Server) goaAddTask(p proto.GOADelegateParams, t proto.GOATask, target proto.SessionInfo) (proto.GOATask, error) {
	t.Project = nonEmpty(p.Project, target.Project)
	t.SessionName, t.Dir, t.Branch = target.Name, target.Dir, target.Branch
	if target.Model != "" {
		t.Ran = []string{target.Model + " · " + nonEmpty(target.Effort, "unknown")}
	}
	t.ReportAt = target.ReportAt // what its session reported so far is another task's
	t, err := s.goa.board.Add(t)
	if err != nil {
		return t, err
	}
	s.goaRememberDelegation(t)
	s.goaChanged()
	return t, nil
}

// goaOpenTask says a session has a task that hasn't been answered yet.
func (s *Server) goaOpenTask(id string) bool {
	return slices.ContainsFunc(s.goa.board.List(), func(t proto.GOATask) bool {
		return t.Session == id && (t.Status == proto.GOATaskWorking || t.Status == proto.GOATaskWaiting || t.Status == proto.GOATaskQueued)
	})
}

// goaSkipOn switches a session started asking to skipping prompts, when the
// task asks that and its agent can change mode as it runs. One that can't goes
// on as it is: its prompts reach GOA.
func (s *Server) goaSkipOn(p proto.GOADelegateParams, info proto.SessionInfo) {
	if !p.SkipPermissions || info.SkipPermissions {
		return
	}
	set := agent.SettingsOf(info.Agent)
	if _, ok := agent.FindChoice(set.Modes, "bypassPermissions"); ok && set.ModeKey != "" {
		_ = s.goaCall(p.From, p.Host, proto.MethodSessionMode, proto.SessionModeParams{ID: info.ID, Mode: "bypassPermissions"}, nil)
	}
}

// goaDeliver gives a session that ended its turn its oldest queued task, or,
// when it ended for good, says those tasks can't go.
func (s *Server) goaDeliver(info proto.SessionInfo) {
	var next *proto.GOATask
	tasks := s.goa.board.List()
	for i := range tasks {
		if t := &tasks[i]; t.Session == info.ID && t.Status == proto.GOATaskQueued && (next == nil || t.Created.Before(next.Created)) {
			next = t
		}
	}
	if next == nil {
		return
	}
	if info.Exited {
		t, _ := s.goa.board.Update(next.ID, func(t *proto.GOATask) {
			t.Status, t.Result = proto.GOATaskFailed, "its session ended before it could take this"
		})
		s.goaChanged()
		s.goaTell(fmt.Sprintf("[enowX GOA] Task %s %q failed: its session %s ended before it could take it. Delegate it again.", t.ID, t.Title, t.Session))
		s.goaDeliver(info)
		return
	}
	from := ""
	if g, ok := s.goaSession(); ok {
		from = g.ID
	}
	// Claimed first: a turn's end and the stall watch may both come to give it.
	claimed := false
	_, _ = s.goa.board.Update(next.ID, func(t *proto.GOATask) {
		if t.Status == proto.GOATaskQueued {
			t.Status, claimed = proto.GOATaskWorking, true
			t.ReportAt = info.ReportAt // what it reported while this waited was the task before's
		}
	})
	if !claimed {
		return
	}
	err := s.goaCall(from, next.Host, proto.MethodSessionInput, proto.SessionInputParams{ID: next.Session, Text: next.Prompt, Paste: true, Enter: true}, nil)
	if err != nil {
		_, _ = s.goa.board.Update(next.ID, func(t *proto.GOATask) {
			t.Status, t.Result = proto.GOATaskFailed, "it couldn't be delivered: "+err.Error()
		})
	}
	s.goaChanged()
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

// goaTurnLeast is how soon after a task is given to another host's session a turn's end
// can't be its own. One that really was goes unheard only until the screen shows it.
const goaTurnLeast = 3 * time.Second

// goaTurnStale says a turn's end told of session info is from before task t: here, the
// session is no longer done, a task having been typed in since; on another host, which
// can't be asked as simply, it came just after the task was given.
func (s *Server) goaTurnStale(info proto.SessionInfo, t proto.GOATask) bool {
	if sess, err := s.sessions.Get(info.ID); err == nil {
		return sess.Info().State != proto.StateDone
	}
	return time.Since(t.Updated) < goaTurnLeast
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
		if t.Status == status || t.Status == proto.GOATaskQueued {
			return false
		}
		if t.Status == proto.GOATaskStalled && status == proto.GOATaskWorking {
			return false // working is what it said while stalled: watchStalls sees it move
		}
		if status == proto.GOATaskTurnEnded && s.goaTurnStale(info, *t) {
			return false // the turn before this task's: this change was on its way as it was given
		}
		t.Status = status
		if status != proto.GOATaskWaiting {
			t.Asks, t.Urgent = "", ""
		}
		return true
	})
	for _, t := range changed {
		where, hostFlag := goaTaskPlace(t)
		head := fmt.Sprintf("[enowX GOA] Task %s %q (%s)", t.ID, t.Title, where)
		switch status {
		case proto.GOATaskTurnEnded:
			said := s.goaResult(t)
			t, _ = s.goa.board.Update(t.ID, func(t *proto.GOATask) { t.Said = said })
			s.goaTell(head + " finished a turn, which may be mid-work: " + clip(oneLine(said), goaResultMax) + " Check it (chat " + t.Session + hostFlag + "), then settle it: goa task " + t.ID + " --status done|failed --result \"…\".")
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
			t = s.goaNoteEnded(t)
			s.goaTell(head + fmt.Sprintf(" failed: its agent ended with exit code %d%s. ", info.ExitCode, goaReportWord(t)) + goaLastShown(t) + "Read it (read " + t.Session + hostFlag + ").")
		case proto.GOATaskStopped:
			s.goaRememberOutcome(t)
			t = s.goaNoteEnded(t)
			s.goaTell(head + fmt.Sprintf(" stopped: its agent ended with exit code %d%s. ", info.ExitCode, goaReportWord(t)) + goaLastShown(t))
		}
	}
	if len(changed) > 0 {
		s.goaChanged()
	}
}

// goaNoteEnded keeps what task t's session last showed, unless it reported, and
// that it ended with no report if so.
func (s *Server) goaNoteEnded(t proto.GOATask) proto.GOATask {
	said := s.goaResult(t)
	out, err := s.goa.board.Update(t.ID, func(x *proto.GOATask) {
		if x.Outcome == "" {
			x.Said, x.NoReport = said, true
		}
	})
	if err != nil {
		return t
	}
	return out
}

func goaReportWord(t proto.GOATask) string {
	if t.NoReport {
		return " and no report"
	}
	return ", having reported " + t.Outcome
}

func goaLastShown(t proto.GOATask) string {
	if t.Said == "" {
		return ""
	}
	return "Its last words: " + clip(oneLine(t.Said), goaResultMax) + " "
}

// goaLimitHit tells GOA a session it follows stopped on a limit its agent showed:
// nothing else would, the session only sitting there.
func (s *Server) goaLimitHit(info proto.SessionInfo, line string) {
	s.goa.mu.Lock()
	_, told := s.goa.told[info.ID]
	s.goa.mu.Unlock()
	if told || s.goa.board.Has(info.ID) {
		s.goaTell(fmt.Sprintf("[enowX GOA] Session %s stopped on a limit: %s", info.ID, clip(oneLine(line), goaResultMax)))
	}
}

// goaReportAsk is what every task asks of its session, whatever comes of it.
const goaReportAsk = "\n\nEnd every turn with a report, whatever came of it, nothing to do and refusing included: \"$ENOWX_BIN\" goa report --outcome done|partly|nothing|blocked|refused|failed \"what happened, in a line or two\". It goes to GOD of Agents, who gave you this."

// goaReport is session p.Session's report of its turn, kept on that session, on
// its own host: goaReportSeen takes it to GOA from there.
func (s *Server) goaReport(p proto.GOAReportParams) error {
	if !slices.Contains(proto.GOAOutcomes, p.Outcome) {
		return fmt.Errorf("goa: a report's outcome is one of %s", strings.Join(proto.GOAOutcomes, ", "))
	}
	sess, err := s.sessions.Get(p.Session)
	if err != nil {
		return fmt.Errorf("goa: report: %w", err)
	}
	s.sessions.SetReport(sess, p.Outcome, clip(strings.TrimSpace(p.Text), goaResultMax))
	return nil
}

// goaReportSeen puts a report info carries that wasn't seen yet on its task, and
// tells GOA in its session's words.
func (s *Server) goaReportSeen(info proto.SessionInfo) {
	if info.ReportAt == 0 {
		return
	}
	changed := s.goa.board.Of(info.ID, func(t *proto.GOATask) bool {
		// A report belongs to the task under way when it was sent: not to one only
		// queued, nor to one given after it.
		if t.Status == proto.GOATaskQueued || t.ReportAt >= info.ReportAt {
			return false
		}
		t.Outcome, t.ReportAt, t.Said = info.ReportOutcome, info.ReportAt, info.Report
		return true
	})
	for _, t := range changed {
		where, hostFlag := goaTaskPlace(t)
		s.goaTell(fmt.Sprintf("[enowX GOA] Task %s %q (%s) reports %s: %s Check it (chat %s%s), then settle it: goa task %s --status done|failed --result \"…\".", t.ID, t.Title, where, t.Outcome, clip(oneLine(t.Said), goaResultMax), t.Session, hostFlag, t.ID))
	}
	if len(changed) > 0 {
		s.goaChanged()
	}
}

// goaTurnOverSeen tells of task t, whose session's screen shows its turn ended
// with no hook saying so, as a hook's turn end would.
func (s *Server) goaTurnOverSeen(t proto.GOATask, screen string) {
	said := s.goaResult(t)
	moved := false
	t, err := s.goa.board.Update(t.ID, func(x *proto.GOATask) {
		if x.Status == proto.GOATaskWorking || x.Status == proto.GOATaskStalled {
			x.Status, x.Said, moved = proto.GOATaskTurnEnded, said, true
		}
	})
	if err != nil || !moved {
		return
	}
	s.goaChanged()
	where, hostFlag := goaTaskPlace(t)
	how := "finished a turn"
	if agent.InterruptShown(screen) {
		how = "was interrupted"
	}
	s.goaTell(fmt.Sprintf("[enowX GOA] Task %s %q (%s) %s, as its screen shows, which may be mid-work: %s Check it (chat %s%s), then settle it: goa task %s --status done|failed --result \"…\".", t.ID, t.Title, where, how, clip(oneLine(said), goaResultMax), t.Session, hostFlag, t.ID))
}

// goaTaskPlace is where task t's session is, in GOA's words, and the flag its
// commands need to reach it.
func goaTaskPlace(t proto.GOATask) (where, hostFlag string) {
	where = "session " + t.Session
	if t.Host != "" {
		where += " on " + t.Host
		hostFlag = " --host " + t.Host
	}
	return where, hostFlag
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

// goaToDoLines tells a worker the to-dos its task finishes, and to tick them.
func goaToDoLines(p proto.GOADelegateParams) string {
	if p.Note == "" || len(p.Items) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\nThis task finishes these to-dos of note " + p.Note + ". Tick each in the same turn as the commit that finishes it (\"$ENOWX_BIN\" notes check " + p.Note + " --item TEXT):")
	for _, item := range p.Items {
		b.WriteString("\n- " + item)
	}
	return b.String()
}

// goaOpenItems are task t's to-dos its note doesn't have ticked, all of them
// when the note can't be read: what can't be seen to be ticked isn't.
func (s *Server) goaOpenItems(t proto.GOATask) []string {
	if t.Note == "" || len(t.Items) == 0 {
		return nil
	}
	n, err := s.notesFor(t.Note).Get(t.Note)
	if err != nil {
		return t.Items
	}
	return noteblocks.Open(n.Body, t.Items)
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
		if p.Status == proto.GOATaskDone {
			t.Open = s.goaOpenItems(*t)
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

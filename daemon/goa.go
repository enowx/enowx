package daemon

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/goa"
	"github.com/enowx/enowx/core/session"
	"github.com/enowx/enowx/proto"
)

// GOD of Agents on this host (docs/goa.md): its session runs in a folder
// of its own with every feature, held to enowX's tools as far as its agent
// allows (its hooks refuse the rest here; goa_watch.go flags what an agent
// without either does). Its board follows its workers on every host: this
// host's through their updates, others' through the app (goa.worker); what
// changes is typed into GOA once it is idle, so it never polls.

// goaState is what the daemon keeps of GOA. The board is safe on its own;
// mu guards the rest.
type goaState struct {
	board *goa.Board

	mu     sync.Mutex
	tell   []goaLine         // lines for GOA, typed once it is idle, kept in tellFile
	urgent map[string]string // worker session → why the user answers what it asks
	// pending is the tool call of each worker in urgent the user hasn't been
	// told of, as its agent hasn't asked yet.
	pending map[string]proto.HookEvent
	flags   []proto.GOAFlag
	watch   map[string]*goaWatch // watched GOA sessions, by ID (goa_watch.go)
	// rulesGiven is the hash of the rules each GOA session was last given
	// (goa_rules.go).
	rulesGiven map[string]string
	// stallSet tells watchStalls the limits changed.
	stallSet chan struct{}

	tellFile string
	// tellSave has one write of tellFile at a time, the latest last. It is taken
	// before mu, which no file is written under.
	tellSave sync.Mutex
}

func (s *Server) newGOA(dir string) {
	s.goa = goaState{board: goa.OpenBoard(filepath.Join(dir, "goa-board.json")),
		urgent: map[string]string{}, pending: map[string]proto.HookEvent{}, watch: map[string]*goaWatch{}, stallSet: make(chan struct{}, 1),
		tellFile: filepath.Join(dir, "goa-tell.json")}
	s.loadGOATell()
}

// goaDir is GOA's own folder, outside every project.
func (s *Server) goaDir() string { return filepath.Join(filepath.Dir(s.WorktreeRoot), "goa") }

// goaSession is GOA's session on this host: the one running, else the
// latest.
func (s *Server) goaSession() (proto.SessionInfo, bool) {
	var found proto.SessionInfo
	ok := false
	for _, sess := range s.sessions.List() {
		info := sess.Info()
		if !info.GOA {
			continue
		}
		if !ok || !info.Exited && found.Exited || info.Exited == found.Exited && info.Created.After(found.Created) {
			found, ok = info, true
		}
	}
	return found, ok
}

// goaCreate makes p and meta GOA's: its folder, no project or branch,
// every feature, its permissions asked (enowX's tools need none). One GOA
// runs on a host at a time.
func (s *Server) goaCreate(p *proto.SessionCreateParams, meta *session.Meta) (string, error) {
	if g, ok := s.goaSession(); ok && !g.Exited {
		return "", fmt.Errorf("goa: GOD of Agents already runs on this host: session %s", g.ID)
	}
	if _, ok := agent.Find(p.Agent); !ok {
		return "", fmt.Errorf("goa: %q isn't an agent GOD of Agents can run on", p.Agent)
	}
	dir := s.goaDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("goa: %w", err)
	}
	p.Project, p.Parent, p.BranchMode, p.SkipPermissions, p.GOAWorker = "", "", proto.BranchNone, false, ""
	p.Features = agent.AllFeatures()
	meta.Project, meta.GOA, meta.GOAWorker, meta.Features = "", true, "", p.Features
	if meta.Name == "" {
		meta.Name = "GOD of Agents"
	}
	return dir, nil
}

// goaChat is chat as GOA's, for a GOA session (meta): its guide first.
func (s *Server) goaChat(chat chatFor, meta session.Meta) chatFor {
	if meta.GOA {
		chat.goa = true
		chat.guide = agent.GOAGuide + goaRulesGuide(s.goaRules().Rules) + chat.guide
	}
	return chat
}

// goaEnv is what a GOA session (meta) gets in its environment.
func goaEnv(meta session.Meta) []string {
	if !meta.GOA {
		return nil
	}
	return agent.GOAEnv(agent.Base(meta.Agent))
}

// goaHook is GOA's say on event ev of session info, before the user's
// hooks: GOA's tools other than enowX's and those for files are refused
// (stop), what only reads allowed; a worker's
// call is judged (goa_workers.go), its decision merged after the hooks.
func (s *Server) goaHook(info proto.SessionInfo, ev proto.HookEvent) (d proto.HookDecision, stop bool) {
	if ev.Event != proto.HookToolBefore {
		return d, false
	}
	if info.GOA {
		if ok, why := goa.Allowed(ev.Tool, ev.Command, s.HookBin); !ok {
			s.logger().Info("goa: a tool refused", "session", info.ID, "tool", ev.Tool)
			return proto.HookDecision{Block: true, Reason: why}, true
		}
		// Reading needs no asking, unless of a secret; an edit stays the
		// user's, as its agent asks.
		d.Approve = goa.ReadOnly(ev.Tool, ev.Command) && goa.Urgent(ev.Tool, ev.Command, ev.Paths) == ""
		return d, false
	}
	if info.GOAWorker != "" || s.goa.board.Has(info.ID) {
		return s.goaWorkerTool(info, ev), false
	}
	return d, false
}

// mergeWorker is the hooks' decision d with a worker's judged one w: a
// refusal of either stands; the judge's yes lets it run unasked.
func mergeWorker(d, w proto.HookDecision) proto.HookDecision {
	switch {
	case d.Block:
		return d
	case w.Block:
		return w
	}
	d.Approve = d.Approve || w.Approve
	return d
}

// goaBoard is the board as the app shows it.
func (s *Server) goaBoard() proto.GOABoard {
	b := proto.GOABoard{Tasks: s.goa.board.List()}
	// Tasks from before they kept their session's place: as it is, if it is.
	for i, t := range b.Tasks {
		if t.Dir == "" && t.Host == "" && t.Session != "" {
			if sess, err := s.sessions.Get(t.Session); err == nil {
				info := sess.Info()
				b.Tasks[i].Dir, b.Tasks[i].Branch, b.Tasks[i].SessionName = info.Dir, info.Branch, info.Name
				b.Tasks[i].Project = nonEmpty(t.Project, info.Project)
			}
		}
	}
	if g, ok := s.goaSession(); ok {
		b.Session, b.Agent = g.ID, agent.Base(g.Agent)
		b.Level, _ = goa.LevelOf(b.Agent)
	}
	return b
}

// goaChanged tells the apps the board changed.
func (s *Server) goaChanged() { s.broadcast(proto.EventGOABoard, s.goaBoard()) }

// goaSeen follows session info for GOA: GOA idle gets what waits for it;
// a worker's change goes on the board.
func (s *Server) goaSeen(info proto.SessionInfo) {
	if info.GOA {
		if !info.Exited && info.State != proto.StateWorking {
			go s.goaFlush(info.ID)
		}
		s.goaWatchSeen(info)
		return
	}
	if info.State == proto.StateDone || info.Exited {
		s.goaClearUrgent(info.ID) // its turn is over (its next tool clears it too)
	}
	if s.goa.board.Has(info.ID) {
		s.goaRanOn(info)
		go func() {
			s.goaReportSeen(info)
			s.goaWorkerChanged(info)
			if info.State == proto.StateDone || info.Exited && !info.Hibernated {
				s.goaDeliver(info)
			}
		}()
	}
}

// goaRanOn records the model a task's session says it runs on, each change, on
// the tasks not settled.
func (s *Server) goaRanOn(info proto.SessionInfo) {
	if info.Model == "" {
		return
	}
	ran := info.Model + " · " + nonEmpty(info.Effort, "unknown")
	changed := s.goa.board.Of(info.ID, func(t *proto.GOATask) bool {
		if goa.Settled(t.Status) || len(t.Ran) > 0 && t.Ran[len(t.Ran)-1] == ran {
			return false
		}
		t.Ran = append(t.Ran, ran)
		return true
	})
	if len(changed) > 0 {
		s.goaChanged()
	}
}

// goaTell has line typed into GOA once it is idle.
func (s *Server) goaTell(line string) { s.goaTellLine(goaLine{Text: line}) }

// goaLine is a line waiting for GOA. One about a task's progress (Task set)
// says nothing once the task is settled, as it may be by the time GOA is idle;
// a report has no Task, and always goes.
type goaLine struct {
	Text string `json:"text"`
	Task string `json:"task,omitempty"`
}

// goaTellMax bounds what waits for a GOA that never comes back, the oldest going.
const goaTellMax = 200

// goaTellLine tells GOA line, now if it is idle, else once it is: while it is
// busy, hibernated, stopped or the daemon restarts, the line waits in a file. Only
// with no GOA on this host at all is there no one to tell.
func (s *Server) goaTellLine(line goaLine) {
	g, ok := s.goaSession()
	if !ok {
		return
	}
	s.goa.mu.Lock()
	s.goa.tell = append(s.goa.tell, line)
	if over := len(s.goa.tell) - goaTellMax; over > 0 {
		s.goa.tell = slices.Delete(s.goa.tell, 0, over)
		s.logger().Warn("goa: too much waits for GOA, the oldest goes", "lines", over)
	}
	s.goa.mu.Unlock()
	s.saveGOATell()
	if !g.Exited && g.State != proto.StateWorking {
		s.goaFlush(g.ID)
	}
}

// saveGOATell keeps what waits for GOA, for it to outlast the daemon.
func (s *Server) saveGOATell() {
	s.goa.tellSave.Lock()
	defer s.goa.tellSave.Unlock()
	s.goa.mu.Lock()
	lines := slices.Clone(s.goa.tell)
	s.goa.mu.Unlock()
	data, err := json.Marshal(lines)
	if err == nil {
		err = os.WriteFile(s.goa.tellFile+".tmp", data, 0o600)
	}
	if err == nil {
		err = os.Rename(s.goa.tellFile+".tmp", s.goa.tellFile)
	}
	if err != nil {
		s.logger().Warn("goa: keep what waits for GOA", "err", err)
	}
}

// loadGOATell reads what waited for GOA when the daemon last stopped.
func (s *Server) loadGOATell() {
	data, err := os.ReadFile(s.goa.tellFile)
	if err != nil {
		return
	}
	var lines []goaLine
	if err := json.Unmarshal(data, &lines); err != nil {
		s.logger().Warn("goa: read what waits for GOA", "err", err)
		return
	}
	s.goa.mu.Lock()
	s.goa.tell = lines
	s.goa.mu.Unlock()
}

// goaFlushIdle has an idle GOA get what still waits for it: what a failed typing
// put back, or what waited over a restart with no change of its session to come.
func (s *Server) goaFlushIdle() {
	s.goa.mu.Lock()
	waiting := len(s.goa.tell) > 0
	s.goa.mu.Unlock()
	if !waiting {
		return
	}
	if g, ok := s.goaSession(); ok && !g.Exited && g.State != proto.StateWorking {
		s.goaFlush(g.ID)
	}
}

// goaLinesToSay takes what waits for GOA, less what its task's end made moot.
func (s *Server) goaLinesToSay() []goaLine {
	s.goa.mu.Lock()
	lines := s.goa.tell
	s.goa.tell = nil
	s.goa.mu.Unlock()
	var out []goaLine
	for _, l := range lines {
		if l.Task == "" || !s.goa.board.IsSettled(l.Task) {
			out = append(out, l)
		}
	}
	return out
}

// goaFlush types what waits for GOA (session id) as one message. What couldn't
// be typed goes back, ahead of what came since. The file changes only after: a
// daemon stopped between the two says a line twice rather than never.
func (s *Server) goaFlush(id string) {
	lines := s.goaLinesToSay()
	if len(lines) == 0 {
		s.saveGOATell() // moot lines may have gone
		return
	}
	texts := make([]string, len(lines))
	for i, l := range lines {
		texts[i] = l.Text
	}
	err := s.input(proto.SessionInputParams{ID: id, Text: strings.Join(texts, "\n"), Paste: true, Enter: true})
	if err != nil {
		s.logger().Warn("goa: could not tell GOA", "session", id, "err", err)
		s.goa.mu.Lock()
		s.goa.tell = append(lines, s.goa.tell...)
		s.goa.mu.Unlock()
	}
	s.saveGOATell()
}

// goaCall calls method on host ("" this one) as GOA (from) would: through
// host.call, so control's checks, the user's yes and its log apply.
func (s *Server) goaCall(from, host, method string, params, out any) error {
	raw, err := json.Marshal(params)
	if err != nil {
		return err
	}
	res, err := (&conn{srv: s}).hostCall(proto.HostCallParams{Host: host, Method: method, Params: raw, From: from})
	if err != nil || out == nil {
		return err
	}
	data, err := json.Marshal(res)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

// goaAgents are the agents installed here and how far each holds GOA.
func goaAgents() []proto.GOAAgent {
	var out []proto.GOAAgent
	for _, a := range agent.List() {
		if a.ID == agent.Shell || a.ID == agent.Custom {
			continue
		}
		level, why := goa.LevelOf(a.ID)
		out = append(out, proto.GOAAgent{Agent: a.ID, Name: a.Name, Level: level, Why: why, Installed: a.Installed})
	}
	// The strictest first, as the user would pick.
	rank := map[string]int{proto.GOAEnforced: 0, proto.GOALimited: 1, proto.GOAWatched: 2}
	slices.SortStableFunc(out, func(x, y proto.GOAAgent) int { return rank[x.Level] - rank[y.Level] })
	return out
}

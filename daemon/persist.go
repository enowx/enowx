package daemon

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/session"
	"github.com/enowx/enowx/core/transcript"
	"github.com/enowx/enowx/proto"
)

// Sessions outlive the daemon: their records live in sessions.json and the
// screens they showed in history/<id>.vt. A restarted daemon brings each
// one back, relaunching agents on their last conversation.
const (
	historySaveEvery = 20 * time.Second
	historyMaxBytes  = 2 << 20
)

// record is a session as saved on disk.
type record struct {
	ID       string               `json:"id"`
	Created  time.Time            `json:"created"`
	Dir      string               `json:"dir"`
	Command  []string             `json:"command,omitempty"`
	Relaunch bool                 `json:"relaunch,omitempty"`
	Restart  []string             `json:"restart,omitempty"`      // nil with Relaunch: the login shell
	Convo    string               `json:"conversation,omitempty"` // Meta.Conversation
	Exited   bool                 `json:"exited,omitempty"`
	Note     string               `json:"note,omitempty"`
	ExitCode int                  `json:"exitCode,omitempty"`
	Name     string               `json:"name,omitempty"`
	Prompt   string               `json:"prompt,omitempty"`
	Project  string               `json:"project,omitempty"`
	Agent    string               `json:"agent,omitempty"`
	Branch   string               `json:"branch,omitempty"`
	Worktree bool                 `json:"worktree,omitempty"`
	Skip     bool                 `json:"skipPermissions,omitempty"`
	Budget   int64                `json:"budget,omitempty"` // seconds
	Display  bool                 `json:"display,omitempty"`
	Parent   string               `json:"parent,omitempty"`
	Group    string               `json:"group,omitempty"`
	Account  string               `json:"account,omitempty"`
	Config   *proto.SessionConfig `json:"config,omitempty"`
	// Member is the team's member who started it (visitors.go).
	Member   *proto.SessionMember `json:"member,omitempty"`
	Private  bool                 `json:"private,omitempty"`
	NotesFor string               `json:"notesFor,omitempty"`
	Canvas   string               `json:"canvasFor,omitempty"`
	Features map[string]bool      `json:"features,omitempty"`
	Creds    []string             `json:"creds,omitempty"`
	// Hibernated sessions stay so: by hand or not, since when.
	Hibernated       bool      `json:"hibernated,omitempty"`
	HibernatedByHand bool      `json:"hibernatedByHand,omitempty"`
	HibernatedAt     time.Time `json:"hibernatedAt,omitzero"`
	NoAutoHibernate  bool      `json:"noAutoHibernate,omitempty"`
}

// persistence saves the daemon's sessions. Its methods are safe from any
// goroutine; writes are serialized by mu.
type persistence struct {
	dir string

	mu      sync.Mutex
	records map[string]record
	order   []string
	closing bool // the daemon is stopping: exits are not the user's doing
}

func openPersistence(dir string) (*persistence, error) {
	p := &persistence{dir: dir, records: map[string]record{}}
	data, err := os.ReadFile(p.file())
	if errors.Is(err, os.ErrNotExist) {
		return p, nil
	}
	if err != nil {
		return nil, fmt.Errorf("daemon: sessions: %w", err)
	}
	var list []record
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("daemon: read %s: %w", p.file(), err)
	}
	for _, r := range list {
		p.records[r.ID] = r
		p.order = append(p.order, r.ID)
	}
	return p, nil
}

func (p *persistence) file() string { return filepath.Join(p.dir, "sessions.json") }

func (p *persistence) historyFile(id string) string {
	return filepath.Join(p.dir, "history", id+".vt")
}

// saved returns the records in creation order.
func (p *persistence) saved() []record {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]record, 0, len(p.order))
	for _, id := range p.order {
		out = append(out, p.records[id])
	}
	return out
}

// update records the session's current state when it changed in a way
// that matters after a restart.
func (p *persistence) update(s *session.Session, command []string) {
	info, meta := s.Info(), s.Meta()
	if meta.Queued {
		return // nothing runs yet: a restart forgets it
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closing {
		return
	}
	old, known := p.records[info.ID]
	r := record{
		ID: info.ID, Created: info.Created, Dir: info.Dir, Command: info.Command, Relaunch: meta.Relaunch, Restart: meta.Restart,
		Convo: meta.Conversation, Exited: info.Exited, ExitCode: info.ExitCode, Name: meta.Name, Prompt: info.Prompt,
		Project: meta.Project, Agent: meta.Agent, Branch: meta.Branch, Worktree: meta.Worktree, Skip: meta.SkipPermissions,
		Budget: int64(meta.Budget / time.Second), Display: meta.Display > 0, Parent: meta.Parent, Note: meta.Note,
		Group: meta.Group, Account: meta.Account, Config: meta.Config, Member: meta.Member, Private: meta.Private, NotesFor: meta.NotesFor, Canvas: meta.CanvasFor, Features: meta.Features, Creds: meta.Creds,
		Hibernated: meta.Hibernated && info.Exited, HibernatedByHand: meta.HibernatedByHand, HibernatedAt: meta.HibernatedAt,
		NoAutoHibernate: meta.NoAutoHibernate,
	}
	if command != nil {
		r.Command = command
	} else if known {
		r.Command = old.Command
	}
	if known && equalRecords(old, r) {
		return
	}
	p.records[r.ID] = r
	if !known {
		p.order = append(p.order, r.ID)
	}
	p.saveLocked()
}

func (p *persistence) remove(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.records[id]; !ok {
		return
	}
	delete(p.records, id)
	for i, o := range p.order {
		if o == id {
			p.order = append(p.order[:i], p.order[i+1:]...)
			break
		}
	}
	p.saveLocked()
	_ = os.Remove(p.historyFile(id))
}

// saveHistory writes what the session shows, keeping its last part.
func (p *persistence) saveHistory(s *session.Session) {
	h := s.History()
	if len(h) > historyMaxBytes {
		h = h[len(h)-historyMaxBytes:]
		if i := bytes.IndexByte(h, '\n'); i >= 0 {
			h = h[i+1:] // start on a whole row
		}
	}
	path := p.historyFile(s.ID())
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	_ = writeFileAtomic(path, h)
}

func (p *persistence) loadHistory(id string) []byte {
	h, _ := os.ReadFile(p.historyFile(id))
	return h
}

// stopping marks the daemon as stopping: the sessions it hangs up next are
// relaunched at the next start, not recorded as exited.
func (p *persistence) stopping() {
	p.mu.Lock()
	p.closing = true
	p.mu.Unlock()
}

func (p *persistence) saveLocked() {
	list := make([]record, 0, len(p.order))
	for _, id := range p.order {
		list = append(list, p.records[id])
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return
	}
	if err := os.MkdirAll(p.dir, 0o700); err != nil {
		return
	}
	_ = writeFileAtomic(p.file(), data)
}

func writeFileAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func equalRecords(a, b record) bool {
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	return string(ja) == string(jb)
}

// restore brings back the saved sessions: running ones relaunched (agents
// continue their last conversation), the others as they ended.
func (s *Server) restore() {
	for _, r := range s.persist.saved() {
		spec := session.Spec{
			ID: r.ID, Dir: r.Dir, Created: r.Created, History: s.persist.loadHistory(r.ID), Env: s.hostEnvFor(r.Member),
			Meta: session.Meta{
				Name: r.Name, Prompt: r.Prompt, Project: r.Project, Agent: r.Agent, Branch: r.Branch,
				Worktree: r.Worktree, SkipPermissions: r.Skip, Conversation: r.Convo, Relaunch: r.Relaunch, Restart: r.Restart, Restored: true,
				Budget: time.Duration(r.Budget) * time.Second, Parent: r.Parent, Note: r.Note, Group: r.Group, Account: r.Account, Config: r.Config, Member: r.Member, Private: r.Private, NotesFor: r.NotesFor, CanvasFor: r.Canvas, Features: r.Features, Creds: r.Creds,
				Hibernated: r.Hibernated, HibernatedByHand: r.HibernatedByHand, HibernatedAt: r.HibernatedAt, NoAutoHibernate: r.NoAutoHibernate,
			},
		}
		_, dirErr := os.Stat(r.Dir)
		if r.Exited || !r.Relaunch || dirErr != nil {
			s.exitsLogged.Store(r.ID, true) // ended with an earlier daemon
			spec.Exited, spec.ExitCode, spec.Command = true, r.ExitCode, r.Command
			if !r.Exited {
				spec.ExitCode = -1 // stopped with the daemon, not relaunched
			}
		} else if run, err := s.configRunFor(r.ID, spec.Meta); err != nil {
			// Its config's key can't be read yet (no app to lend it): it
			// waits, stopped, to be continued, rather than start as another.
			s.logger().Warn("could not restore a session's config", "id", r.ID, "err", err)
			spec.Exited, spec.ExitCode, spec.Command = true, -1, r.Command
		} else {
			spec.Meta.Config = run.sessionInfo()
			spec.Env = append(spec.Env, run.environ()...)
			spec.Command = r.Restart
			if r.Member == nil {
				spec.Env = append(spec.Env, s.vaultEnvFor(r.ID, r.Project, r.Creds)...)
			}
			if env, err := s.agentEnv(spec.Meta); err == nil {
				spec.Env = append(spec.Env, env...)
			} else {
				s.logger().Warn("could not restore a session's account", "id", r.ID, "err", err)
			}
			if preset, ok := agent.Find(r.Agent); ok && len(r.Restart) > 0 && len(preset.Continue) > 0 {
				// The command saved has the hooks and guide of the daemon
				// that saved it.
				chat := s.chatOf(r.NotesFor, r.Canvas, s.guideFor(spec.Meta))
				chat.member = r.Member != nil
				chat.store = s.agentStore(r.Agent, r.ID, true)
				chat.config = run
				l := agentLaunch(preset, r.Skip, "", true, chat)
				l.Resume = r.Convo // its own, not the folder's latest
				spec.Command = thenShell(preset.Argv(l))
				spec.Meta.Restart = spec.Command
			}
			if r.Display {
				// A new display: the old one went with the daemon.
				if disp, err := s.displays.start(r.ID); err == nil {
					spec.Env = append(spec.Env, disp.Env()...)
					spec.Meta.Display, spec.Meta.DisplayScale = disp.Port, disp.Scale
				} else {
					s.logger().Warn("could not start a session's display", "id", r.ID, "err", err)
				}
			}
		}
		if r.Member != nil {
			spec.Env = memberEnviron(spec.Env)
		}
		sess, err := s.sessions.Create(spec)
		if err != nil {
			s.logger().Warn("could not restore a session", "id", r.ID, "err", err)
			continue
		}
		s.persist.update(sess, r.Command)
	}
}

// saveHistories saves what every session shows; it runs until done closes.
func (s *Server) saveHistories(done <-chan struct{}) {
	t := time.NewTicker(historySaveEvery)
	defer t.Stop()
	// What each session showed when last saved: one that showed nothing
	// new since isn't written again, which takes copying its whole history.
	saved := map[*session.Session]time.Time{}
	for {
		select {
		case <-done:
			return
		case <-t.C:
		}
		list := s.sessions.List()
		for _, sess := range list {
			at := sess.ScreenChanged()
			if last, ok := saved[sess]; ok && !at.After(last) {
				continue
			}
			s.persist.saveHistory(sess)
			saved[sess] = at
			// The conversation it is on now: /clear or /resume moved it to
			// another, which a restart then goes on with.
			if convo := s.sessionConversation(sess); convo != "" {
				s.sessions.SetConversation(sess, convo)
			}
		}
		if len(saved) > len(list) {
			for sess := range saved {
				if !slices.Contains(list, sess) {
					delete(saved, sess)
				}
			}
		}
	}
}

// continueSession starts session id's program again in the session, its
// screen kept: an agent picks up its last conversation, a shell starts
// over. It is for sessions whose program ended, by itself or killed.
func (s *Server) continueSession(id string) (*session.Session, error) {
	return s.resumeSession(id, "")
}

// resumeSession is continueSession on conversation convo of its agent's,
// where the agent can name one: its last otherwise.
func (s *Server) resumeSession(id, convo string) (*session.Session, error) {
	sess, err := s.sessions.Get(id)
	if err != nil {
		return nil, err
	}
	info, meta := sess.Info(), sess.Meta()
	if !info.Exited {
		return nil, errors.New("the session is still running")
	}
	if !meta.Relaunch {
		return nil, errors.New("this session's command can't be continued: start it again in a new session")
	}
	command := meta.Restart
	run, err := s.configRunFor(id, meta) // its config, as it starts again
	if err != nil {
		return nil, err
	}
	meta.Config = run.sessionInfo()
	if preset, ok := agent.Find(meta.Agent); ok && len(preset.Continue) > 0 {
		// The one asked for, else the one it was on: the folder's latest
		// may be another session's.
		if convo == "" {
			convo = s.sessionConversation(sess)
		}
		if convo == "" {
			convo = meta.Conversation
		}
		chat := s.chatOf(meta.NotesFor, meta.CanvasFor, s.guideFor(meta))
		chat.member = meta.Member != nil
		chat.store = s.agentStore(meta.Agent, id, true)
		chat.config = run
		l := agentLaunch(preset, meta.SkipPermissions, "", true, chat)
		l.Resume = convo
		command = thenShell(preset.Argv(l))
		if len(preset.Resume) > 0 {
			meta.Conversation = convo
		}
	}
	meta.Restart, meta.Restored, meta.Hibernated, meta.HibernatedByHand, meta.HibernatedAt = command, false, false, false, time.Time{}
	env, err := s.agentEnv(meta)
	if err != nil {
		return nil, err
	}
	env = append(env, run.environ()...)
	spec := session.Spec{ID: id, Dir: info.Dir, Created: info.Created, History: sess.History(),
		Env: append(append(s.hostEnvFor(meta.Member), env...), s.trafficEnvFor(meta.Parent, id)...), Command: command, Meta: meta}
	if meta.Member == nil {
		spec.Env = append(spec.Env, s.vaultEnvFor(id, meta.Project, meta.Creds)...)
	}
	if meta.Display > 0 {
		// Its display went with its program, or stays: one either way.
		s.displays.stop(id)
		meta.Display, meta.DisplayScale = 0, 0
		if disp, err := s.displays.start(id); err == nil {
			spec.Env = append(spec.Env, disp.Env()...)
			meta.Display, meta.DisplayScale = disp.Port, disp.Scale
		}
		spec.Meta = meta
	}
	if meta.Member != nil {
		spec.Env = memberEnviron(spec.Env)
	}
	next, err := s.sessions.Replace(spec)
	if err != nil {
		return nil, err
	}
	s.began(next, command, "")
	return next, nil
}

// sessionConversation is the conversation sess's agent is on, by the id
// it resumes it with: "" when unknown, or when the agent takes none.
func (s *Server) sessionConversation(sess *session.Session) string {
	meta := sess.Meta()
	if preset, ok := agent.Find(meta.Agent); !ok || len(preset.Resume) == 0 || !transcript.Supports(meta.Agent) {
		return ""
	}
	return transcript.ConversationID(meta.Agent, s.transcriptPath(sess.ID()))
}

// agentEnv is what meta's agent runs with when started again: its
// preset's variables and its account's home; a team's member's, their
// identity and none of the host's user's model API.
func (s *Server) agentEnv(meta session.Meta) ([]string, error) {
	preset, ok := agent.Find(meta.Agent)
	if !ok {
		return nil, nil
	}
	acc, err := s.accounts.Env(agent.Base(meta.Agent), meta.Account)
	if err != nil {
		return nil, err
	}
	if meta.Member != nil {
		return append(append(preset.Environ(meta.SkipPermissions), acc...), memberEnv(meta.Member, s.accounts.Home(meta.Account))...), nil
	}
	return append(append(append(append(preset.Environ(meta.SkipPermissions), apiEnv(meta.Agent)...), acc...), notesEnv(meta.NotesFor != "")...), canvasEnv(meta.CanvasFor != "")...), nil
}

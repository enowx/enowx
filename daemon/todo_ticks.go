package daemon

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/enowx/enowx/core/git"
	"github.com/enowx/enowx/core/noteblocks"
	"github.com/enowx/enowx/proto"
)

// To-dos ticked by agents, and the reminder to tick them: a turn that made a
// commit, in a project with a note's to-dos open, is asked whether one is done.

// todoState is what's kept of them. mu guards the ticks' file.
type todoState struct {
	mu    sync.Mutex
	heads sync.Map // session → the commit it was last seen at
}

func init() {
	handlers[proto.MethodNoteTick] = with(func(c *conn, p proto.NoteTick) (any, error) {
		return ok, c.srv.recordTick(p)
	})
	handlers[proto.MethodNoteTicks] = with(func(c *conn, p proto.NoteRef) (any, error) {
		return c.srv.ticksOf(p.ID), nil
	})
}

func (s *Server) ticksFile() string { return filepath.Join(s.dataDir, "todo-ticks.jsonl") }

// recordTick keeps who ticked a to-do: the session and the commit it is at.
func (s *Server) recordTick(p proto.NoteTick) error {
	p.Time = time.Now()
	p.Commit = ""
	if sess, err := s.sessions.Get(p.Session); err == nil {
		if head, err := git.Run(sess.Info().Dir, "rev-parse", "--short", "HEAD"); err == nil {
			p.Commit = strings.TrimSpace(head)
		}
	}
	line, err := json.Marshal(p)
	if err != nil {
		return err
	}
	s.todos.mu.Lock()
	defer s.todos.mu.Unlock()
	f, err := os.OpenFile(s.ticksFile(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(line, '\n'))
	return err
}

// ticksOf are note id's to-dos ticked by agents, the oldest first.
func (s *Server) ticksOf(id string) []proto.NoteTick {
	out := []proto.NoteTick{}
	s.todos.mu.Lock()
	defer s.todos.mu.Unlock()
	f, err := os.Open(s.ticksFile())
	if err != nil {
		return out
	}
	defer f.Close()
	lines := bufio.NewScanner(f)
	for lines.Scan() {
		var t proto.NoteTick
		if json.Unmarshal(lines.Bytes(), &t) == nil && t.Note == id {
			out = append(out, t)
		}
	}
	return out
}

// todoHead remembers the commit session info is at as a turn starts.
func (s *Server) todoHead(info proto.SessionInfo) {
	if head, err := git.Run(info.Dir, "rev-parse", "HEAD"); err == nil {
		s.todos.heads.Store(info.ID, strings.TrimSpace(head))
	}
}

// remindToDos asks session info, whose turn just ended, about the to-dos of its
// project's notes when the turn made a commit. Once per commit, so the answer's
// own turn doesn't ask again. What has no hooks never gets here: it has the rule
// in its guide alone (docs/notes.md).
func (s *Server) remindToDos(info proto.SessionInfo) {
	if !userSession(info) || info.GOA || info.Exited {
		return
	}
	head, err := git.Run(info.Dir, "rev-parse", "HEAD")
	if err != nil {
		return
	}
	head = strings.TrimSpace(head)
	was, known := s.todos.heads.Swap(info.ID, head)
	if !known || was == head {
		return
	}
	notes := s.notesWithOpenToDos(sessionProject(info))
	if len(notes) == 0 {
		return
	}
	text := "You made a commit this turn, and this project's notes have to-dos open (" + strings.Join(notes, ", ") + "). If it finished one, tick it now: \"$ENOWX_BIN\" notes check NOTE --item TEXT. If none, say so in a line."
	if err := s.input(proto.SessionInputParams{ID: info.ID, Text: text, Paste: true, Enter: true}); err != nil {
		s.logger().Warn("to-dos: remind", "session", info.ID, "err", err)
	}
}

// notesWithOpenToDos are the titles of the notes attached to project with to-dos
// open.
func (s *Server) notesWithOpenToDos(project string) []string {
	if project == "" {
		return nil
	}
	store := s.notesFor("")
	list, err := store.List()
	if err != nil {
		return nil
	}
	var out []string
	for _, n := range list {
		attached := false
		for _, p := range n.Projects {
			attached = attached || p.Path == project
		}
		if !attached {
			continue
		}
		full, err := store.Get(n.ID)
		if err != nil {
			continue
		}
		if done, total := noteblocks.Progress(full.Body); done < total {
			out = append(out, n.Title)
		}
	}
	return out
}

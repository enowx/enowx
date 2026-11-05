package daemon

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/enowx/enowx/core/git"
	"github.com/enowx/enowx/core/notes"
	"github.com/enowx/enowx/core/projectdocs"
	"github.com/enowx/enowx/core/rag"
	"github.com/enowx/enowx/proto"
)

// RAG on this host: its projects indexed by core/rag, only
// those the user turned on and agreed to send to the embedder; again
// after a turn or a commit, once no session in the project works; their
// attached notes and memories with them. Agents search their own
// session's project. See docs/rag.md.

// ragQuiet is how long after a change a project is indexed again, its
// sessions done.
const ragQuiet = 15 * time.Second

// ragContextMax bounds the context a prompt gets from the index.
const ragContextMax = 6000

// ragContextHits is how many parts a prompt gets: the four most like it.
const ragContextHits = 4

// ragWordsTimeout bounds the word search a failed embedder falls back to:
// it is local, so it costs the prompt next to nothing.
const ragWordsTimeout = 2 * time.Second

// ragContextTimeout bounds a prompt's search: past it the prompt goes on
// without the index rather than wait (a search measured at p90 300 ms).
const ragContextTimeout = 3 * time.Second

// ragState is what the daemon keeps of indexes. mu guards it.
type ragState struct {
	mu      sync.Mutex
	status  map[string]proto.RAGStatus // by project folder
	running map[string]bool
	soon    map[string]*time.Timer
	failed  map[string]string // by project folder: why its last prompt search failed
}

// newRAG makes the host's RAG, keys asked of the vault.
func (s *Server) newRAG(dir string) {
	s.ragSettings = rag.OpenSettings(dir)
	s.rag = rag.NewEngine(dir, s.ragSettings)
	s.rag.KeyOf = func(cred string) (string, error) {
		lend, err := s.vaultAsk(proto.VaultAsk{Action: proto.VaultEnv, Name: cred, Command: "enowX RAG: the embedder's key"})
		if err != nil {
			return "", err
		}
		for _, v := range lend.Env {
			if v != "" {
				return v, nil
			}
		}
		if lend.Secret != "" {
			return lend.Secret, nil
		}
		return "", fmt.Errorf("rag: the credential %s has no value", cred)
	}
	s.rag.OnEmbed = func(project string, tokens int64) { s.ragTokens(project, tokens) }
	s.ragRun = ragState{status: map[string]proto.RAGStatus{}, running: map[string]bool{}, soon: map[string]*time.Timer{}, failed: map[string]string{}}
}

// ragSetupDir is ragDir for looking at and indexing a project, not writing to
// its memories: GOD of Agents sets any project's RAG up with the user.
func (s *Server) ragSetupDir(ref proto.RAGRef) (string, error) {
	if ref.Session != "" && ref.Project != "" {
		if sess, err := s.sessions.Get(ref.Session); err == nil && sess.Meta().GOA {
			return ref.Project, nil
		}
	}
	return s.ragDir(ref)
}

// ragDir is the project folder ref names: its own, or its session's
// project (a session may only reach its own).
func (s *Server) ragDir(ref proto.RAGRef) (string, error) {
	if ref.Session != "" {
		sess, err := s.sessions.Get(ref.Session)
		if err != nil {
			return "", err
		}
		meta, info := sess.Meta(), sess.Info()
		dir := meta.Project
		if dir == "" {
			dir = info.Dir
		}
		if ref.Project != "" && ref.Project != dir {
			return "", errors.New("rag: a session searches its own project only")
		}
		return dir, nil
	}
	if ref.Project == "" {
		return "", errors.New("rag: which project?")
	}
	return ref.Project, nil
}

// ragStatus is project dir's index as it is.
func (s *Server) ragStatus(dir string) proto.RAGStatus {
	id := rag.ProjectID(dir)
	st := s.ragSettings.Get()
	s.ragRun.mu.Lock()
	out, ok := s.ragRun.status[dir]
	running, failed := s.ragRun.running[dir], s.ragRun.failed[dir]
	s.ragRun.mu.Unlock()
	if !ok {
		out, _ = s.rag.Status(context.Background(), dir)
	}
	out.ContextError = failed
	out.Project, out.ID, out.Settings, out.Indexing = dir, id, st.Projects[id], running
	out.Ready = st.Embedder == proto.RAGVoyage || st.Embedder == proto.RAGOpenAI || st.Embedder == proto.RAGTEI
	out.Embedder = s.rag.ProjectHost(dir)
	return out
}

// setRAGProject keeps project dir's settings; turned off, its index goes.
func (s *Server) setRAGProject(p proto.RAGProjectParams) (proto.RAGStatus, error) {
	id := rag.ProjectID(p.Project)
	was := s.ragSettings.Project(id)
	if err := s.ragSettings.SetProject(id, p.Settings); err != nil {
		return proto.RAGStatus{}, err
	}
	if was.On && !p.Settings.On {
		_ = s.rag.Drop(context.Background(), p.Project)
		s.ragRun.mu.Lock()
		delete(s.ragRun.status, p.Project)
		s.ragRun.mu.Unlock()
	}
	return s.ragStatus(p.Project), nil
}

// ragIndex indexes project dir in the background, when it is on and the
// user agreed to its embedder.
func (s *Server) ragIndex(dir string) (proto.RAGStatus, error) {
	id := rag.ProjectID(dir)
	p := s.ragSettings.Project(id)
	switch host := s.rag.ProjectHost(dir); {
	case !p.On:
		return s.ragStatus(dir), errors.New("rag: this project isn't indexed: turn it on first")
	case p.Consent != host:
		return s.ragStatus(dir), errors.New("rag: agree first to send this project's text to " + host)
	}
	s.ragRun.mu.Lock()
	if s.ragRun.running[dir] {
		s.ragRun.mu.Unlock()
		return s.ragStatus(dir), nil
	}
	s.ragRun.running[dir] = true
	s.ragRun.mu.Unlock()
	go func() {
		s.broadcast(proto.EventRAGProgress, s.ragStatus(dir))
		res, err := s.rag.Index(context.Background(), dir, p, s.ragExtra(dir, id))
		res.Indexed, res.Phase = time.Now(), "done"
		if err != nil {
			res.Error = err.Error()
		}
		s.ragRun.mu.Lock()
		s.ragRun.status[dir] = res
		delete(s.ragRun.running, dir)
		s.ragRun.mu.Unlock()
		s.broadcast(proto.EventRAGProgress, s.ragStatus(dir))
	}()
	return s.ragStatus(dir), nil
}

// ragExtra are project dir's memories and attached notes, as documents.
func (s *Server) ragExtra(dir, id string) []rag.Doc {
	var out []rag.Doc
	for _, m := range s.ragSettings.Memories(id) {
		out = append(out, rag.Doc{ID: "memory:" + m.ID, Text: m.Text, Source: proto.RAGMemo})
	}
	remote, _ := git.Run(dir, "config", "--get", "remote.origin.url")
	host := s.daemonID
	list, _ := s.notes.List()
	for _, n := range list {
		attached := false
		for _, np := range n.Projects {
			attached = attached || notes.IsProject(np, host, proto.CatalogProject{Path: dir, Remote: strings.TrimSpace(remote)}) || np.Path == dir
		}
		if !attached {
			continue
		}
		note, err := s.notes.Get(n.ID)
		if err != nil {
			continue
		}
		out = append(out, rag.Doc{ID: "note:" + n.ID, Title: n.Title, Text: projectdocs.IndexText(note.Body), Source: proto.RAGNotes})
	}
	return out
}

// ragSoon indexes project dir again a while after a change, once no
// session in it works.
func (s *Server) ragSoon(dir string) {
	if dir == "" || !s.ragSettings.Project(rag.ProjectID(dir)).On {
		return
	}
	s.ragRun.mu.Lock()
	defer s.ragRun.mu.Unlock()
	if t := s.ragRun.soon[dir]; t != nil {
		t.Stop()
	}
	s.ragRun.soon[dir] = time.AfterFunc(ragQuiet, func() {
		for _, sess := range s.sessions.List() {
			info := sess.Info()
			if (info.Project == dir || info.Dir == dir) && !info.Exited && (info.State == proto.StateWorking || info.State == "" && info.Active) {
				s.ragSoon(dir) // a session writes there still
				return
			}
		}
		_, _ = s.ragIndex(dir)
	})
}

// ragContext is what a prompt in project dir gets from its index: every
// prompt of a session whose project is indexed gets the four parts most
// like it, up to ragContextMax characters. Nothing skips it but consent:
// a project not turned on, or not agreed to for this embedder, sends
// nothing. When the embedder fails the search falls back to the index's
// words, its parts marked so, and the failure is kept for the RAG settings.
func (s *Server) ragContext(dir, prompt string) string {
	if strings.TrimSpace(prompt) == "" {
		return ""
	}
	p := s.ragSettings.Project(rag.ProjectID(dir))
	if !p.On || p.Consent != s.rag.ProjectHost(dir) {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), ragContextTimeout)
	defer cancel()
	st, err := s.rag.Status(ctx, dir)
	if err != nil {
		s.ragContextFailed(dir, err) // no embedder, or its key: nothing searched
		return ""
	}
	if st.Chunks == 0 {
		return "" // no index: nothing to find, and nothing to send
	}
	hits, err := s.rag.Search(ctx, dir, prompt, ragContextHits, nil)
	if err != nil {
		s.ragContextFailed(dir, err)
		// The index's words alone, in this host's own store: no network, so
		// the prompt waits no longer than this.
		wctx, wcancel := context.WithTimeout(context.Background(), ragWordsTimeout)
		defer wcancel()
		hits, _ = s.rag.Words(wctx, dir, prompt, ragContextHits, nil)
	} else {
		s.ragContextFailed(dir, nil)
	}
	if len(hits) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Parts of this project that may bear on the prompt (enowX RAG; read the files before relying on them):\n")
	for _, h := range hits {
		part := hitPart(h)
		if b.Len()+len(part) > ragContextMax {
			break
		}
		b.WriteString(part)
	}
	return b.String()
}

// ragContextFailed keeps err as the reason project dir's last prompt search
// failed (nil: it worked), for the RAG settings and the log.
func (s *Server) ragContextFailed(dir string, err error) {
	s.ragRun.mu.Lock()
	if err == nil {
		delete(s.ragRun.failed, dir)
	} else {
		s.ragRun.failed[dir] = err.Error()
	}
	s.ragRun.mu.Unlock()
	if err != nil {
		s.logger().Warn("rag: a prompt's search failed", "project", dir, "err", err)
	}
}

// hitPart is a search result as a prompt's context shows it: where, then
// its text.
func hitPart(h proto.RAGHit) string {
	where := h.File
	if h.Start > 0 {
		where += fmt.Sprintf(":%d-%d", h.Start, h.End)
	}
	if h.Symbol != "" {
		where += " (" + h.Symbol + ")"
	}
	if where == "" {
		where = h.Source
	}
	if h.Words {
		where += " (found by words: the embedder failed)"
	}
	return "\n" + where + "\n" + h.Snippet + "\n"
}

// ragOffer offers to keep what session info's agent said last as a
// memory, when its project asks for it.
func (s *Server) ragOffer(info proto.SessionInfo) {
	dir := info.Project
	if dir == "" {
		dir = info.Dir
	}
	if !s.ragSettings.Project(rag.ProjectID(dir)).Offer {
		return
	}
	tr, err := s.transcript(proto.TranscriptParams{ID: info.ID, Tail: 20})
	if err != nil {
		return
	}
	last := ""
	for _, e := range tr.Entries {
		if e.Kind == proto.TranscriptAssistant && strings.TrimSpace(e.Text) != "" {
			last = strings.TrimSpace(e.Text)
		}
	}
	if last == "" {
		return
	}
	last, ok := s.memoryOf(info, last) // the decision it holds, when the judge is asked
	if !ok {
		return
	}
	if len(last) > 1200 {
		last = last[:1200] + "…"
	}
	s.broadcast(proto.EventRAGOffer, proto.RAGRememberParams{RAGRef: proto.RAGRef{Project: dir, Session: info.ID}, Text: last})
}

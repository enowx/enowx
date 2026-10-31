package daemon

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/enowx/enowx/core/rag"
	"github.com/enowx/enowx/proto"
)

// GOA's own RAG collection is its folder's (rag.ProjectID of goaDir),
// apart from every project's: its tasks, delegations, outcomes, decisions
// and the user's preferences, kept as memories. It is searched by meaning
// once the user turns RAG on for it, else by words, nothing sent. GOA may
// also search any project's collection, read-only: the one exception to a
// session searching its own project only (ragDir).

// goaRAGKinds are what GOA keeps.
var goaRAGKinds = []string{proto.GOAKindTask, proto.GOAKindDelegation, proto.GOAKindOutcome, proto.GOAKindDecision, proto.GOAKindPreference}

// ragRead is ragDir for reading: a GOA session may name any project of
// this host.
func (s *Server) ragRead(ref proto.RAGRef) (string, error) {
	if ref.Session != "" && ref.Project != "" {
		if sess, err := s.sessions.Get(ref.Session); err == nil && sess.Info().GOA {
			return s.projectOf(ref.Project)
		}
	}
	return s.ragDir(ref)
}

// goaSelf is GOA's session id, checked.
func (s *Server) goaSelf(id string) (proto.SessionInfo, error) {
	sess, err := s.sessions.Get(id)
	if err != nil {
		return proto.SessionInfo{}, err
	}
	info := sess.Info()
	if !info.GOA {
		return info, errors.New("goa: only GOD of Agents keeps this collection (rag remember keeps your project's)")
	}
	return info, nil
}

// goaRemember keeps p.Text, of its kind, in GOA's collection.
func (s *Server) goaRemember(p proto.GOARememberParams) (proto.RAGMemory, error) {
	info, err := s.goaSelf(p.Session)
	if err != nil {
		return proto.RAGMemory{}, err
	}
	kind := p.Kind
	if kind == "" {
		kind = proto.GOAKindDecision
	}
	if !slices.Contains(goaRAGKinds, kind) {
		return proto.RAGMemory{}, fmt.Errorf("goa: a memory is a %s, not %q", strings.Join(goaRAGKinds, ", "), kind)
	}
	text := strings.TrimSpace(p.Text)
	if text == "" {
		return proto.RAGMemory{}, errors.New("goa: remember what?")
	}
	dir := s.goaDir()
	m, err := s.ragSettings.Remember(rag.ProjectID(dir), "["+kind+"] "+s.maskLent(text), info.Agent)
	if err == nil {
		s.ragSoon(dir)
	}
	return m, err
}

// goaRecall searches GOA's collection, then the projects p names.
func (s *Server) goaRecall(p proto.GOARecallParams) ([]proto.RAGHit, error) {
	if _, err := s.goaSelf(p.Session); err != nil {
		return nil, err
	}
	k := p.K
	if k <= 0 {
		k = 6
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	dir := s.goaDir()
	var hits []proto.RAGHit
	if s.ragSettings.Project(rag.ProjectID(dir)).On {
		hits, _ = s.rag.Search(ctx, dir, p.Query, k, nil)
	}
	if len(hits) == 0 {
		hits = wordMatch(s.ragSettings.Memories(rag.ProjectID(dir)), p.Query, k)
	}
	for _, pr := range p.Projects {
		var found []proto.RAGHit
		var err error
		if s.thisHost(pr.Host) {
			var project string
			if project, err = s.projectOf(pr.Path); err == nil {
				found, err = s.rag.Search(ctx, project, p.Query, k, nil)
			}
		} else {
			err = s.goaCall(p.Session, pr.Host, proto.MethodRAGSearch, proto.RAGSearchParams{RAGRef: proto.RAGRef{Project: pr.Path}, Query: p.Query, K: k}, &found)
		}
		if err != nil {
			return hits, fmt.Errorf("goa: search %s: %w", pr.Path, err)
		}
		for i := range found {
			found[i].Source = strings.Trim(pr.Host+":"+pr.Path, ":") + " " + found[i].Source
		}
		hits = append(hits, found...)
	}
	if hits == nil {
		hits = []proto.RAGHit{}
	}
	return hits, nil
}

// wordMatch are the k memories sharing the most words with query, for a
// collection not indexed by meaning.
func wordMatch(list []proto.RAGMemory, query string, k int) []proto.RAGHit {
	var words []string
	for _, w := range strings.Fields(strings.ToLower(query)) {
		if w = strings.Trim(w, `.,;:!?"'()`); len([]rune(w)) > 2 {
			words = append(words, w)
		}
	}
	var hits []proto.RAGHit
	for _, m := range list {
		text := strings.ToLower(m.Text)
		n := 0
		for _, w := range words {
			if strings.Contains(text, w) {
				n++
			}
		}
		if n > 0 || len(words) == 0 {
			hits = append(hits, proto.RAGHit{Snippet: m.Text, Score: float64(n) / float64(max(len(words), 1)), Source: proto.RAGMemo, Memory: m.ID})
		}
	}
	slices.SortStableFunc(hits, func(a, b proto.RAGHit) int {
		switch {
		case a.Score > b.Score:
			return -1
		case a.Score < b.Score:
			return 1
		}
		return 0
	})
	if len(hits) > k {
		hits = hits[:k]
	}
	return hits
}

// Package rag finds a project by meaning:
// the host's projects indexed in SQLite in the daemon's folder (or the
// user's PostgreSQL with pgvector), with the embedder the user
// picked, its key from the vault; searched, and keeping memories, for
// agents and the user. Settings and memories are synced; indexes stay on
// each host, rebuilt where needed. See docs/rag.md.
package rag

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/enowx/enowx/core/envsync"
	"github.com/enowx/enowx/core/projectlinks"
	"github.com/enowx/enowx/proto"
)

// Settings keeps the RAG settings (settings/rag.json) and the projects'
// memories (settings/rag-memories/), synced. Safe from any goroutine.
type Settings struct {
	dir string
	mu  sync.Mutex
}

// OpenSettings is the settings kept in dir, a daemon's folder.
func OpenSettings(dir string) *Settings { return &Settings{dir: dir} }

func (s *Settings) file() string { return filepath.Join(s.dir, "settings", "rag.json") }

// Get is the settings.
func (s *Settings) Get() proto.RAGSettings {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out proto.RAGSettings
	if data, err := os.ReadFile(s.file()); err == nil {
		_ = json.Unmarshal(data, &out)
	}
	if out.Store != proto.RAGPostgres {
		out.Store = proto.RAGBuiltin // older settings' "server" too
	}
	if out.Embedder == proto.RAGFromConfig {
		out.Embedder = "" // no longer offered: picked again
	}
	return out
}

// Set keeps the settings.
func (s *Settings) Set(st proto.RAGSettings) error {
	switch st.Embedder {
	case "", proto.RAGVoyage, proto.RAGOpenAI, proto.RAGTEI:
	default:
		return fmt.Errorf("rag: no embedder %q", st.Embedder)
	}
	switch st.Store {
	case "", proto.RAGBuiltin, proto.RAGPostgres:
	default:
		return fmt.Errorf("rag: no store %q", st.Store)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.file()), 0o700); err != nil {
		return err
	}
	return envsync.WriteFileAtomic(s.file(), data, 0o600)
}

// Project is project id's settings.
func (s *Settings) Project(id string) proto.RAGProject { return s.Get().Projects[id] }

// SetProject keeps project id's settings.
func (s *Settings) SetProject(id string, p proto.RAGProject) error {
	st := s.Get()
	if st.Projects == nil {
		st.Projects = map[string]proto.RAGProject{}
	}
	st.Projects[id] = p
	return s.Set(st)
}

// ProjectID is the ID the project in dir is indexed by: its ID of
// project links (the same for its copies on every host), else one made
// from its folder.
func ProjectID(dir string) string {
	if id := projectlinks.ReadID(dir); id != "" {
		return id
	}
	sum := sha256.Sum256([]byte(filepath.Clean(dir)))
	return "dir-" + hex.EncodeToString(sum[:8])
}

func (s *Settings) memoryFile(id string) string {
	return filepath.Join(s.dir, "settings", "rag-memories", id+".json")
}

// Memories are project id's memories, the oldest first.
func (s *Settings) Memories(id string) []proto.RAGMemory {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []proto.RAGMemory
	if data, err := os.ReadFile(s.memoryFile(id)); err == nil {
		_ = json.Unmarshal(data, &out)
	}
	return out
}

func (s *Settings) saveMemories(id string, list []proto.RAGMemory) error {
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.memoryFile(id)), 0o700); err != nil {
		return err
	}
	return envsync.WriteFileAtomic(s.memoryFile(id), data, 0o600)
}

// Remember keeps text as a memory of project id, by by.
func (s *Settings) Remember(id, text, by string) (proto.RAGMemory, error) {
	var b [6]byte
	_, _ = rand.Read(b[:])
	m := proto.RAGMemory{ID: hex.EncodeToString(b[:]), Text: text, By: by, Time: time.Now().UTC()}
	list := append(s.Memories(id), m)
	s.mu.Lock()
	defer s.mu.Unlock()
	return m, s.saveMemories(id, list)
}

// Change changes memory mid of project id to text, or removes it when
// text is "".
func (s *Settings) Change(id, mid, text string) error {
	list := s.Memories(id)
	i := slices.IndexFunc(list, func(m proto.RAGMemory) bool { return m.ID == mid })
	if i < 0 {
		return fmt.Errorf("rag: no memory %s", mid)
	}
	if text == "" {
		list = slices.Delete(list, i, i+1)
	} else {
		list[i].Text = text
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveMemories(id, list)
}

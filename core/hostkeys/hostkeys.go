// Package hostkeys keeps keys the user typed in the app for one host's own
// use (a judge's, RAG's embedder's, a database URL with its password):
// in a file of that host alone, readable by its user only, never synced to
// other hosts and never sent back to an app (it learns only whether one is
// set). The vault stays the way to share a key across hosts.
package hostkeys

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/enowx/enowx/core/envsync"
)

// Store is the keys of one file. Safe from any goroutine.
type Store struct {
	file string
	mu   sync.Mutex
	keys map[string]string
}

// Open is the store in file (e.g. a daemon's folder/decide-keys.json).
func Open(file string) *Store {
	s := &Store{file: file, keys: map[string]string{}}
	if data, err := os.ReadFile(file); err == nil {
		_ = json.Unmarshal(data, &s.keys)
	}
	return s
}

// Get is the key named name, "" for none.
func (s *Store) Get(name string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.keys[name]
}

// Has says whether a key named name is set.
func (s *Store) Has(name string) bool { return s.Get(name) != "" }

// Set keeps value as the key named name ("" removes it).
func (s *Store) Set(name, value string) error {
	s.mu.Lock()
	if value == "" {
		delete(s.keys, name)
	} else {
		s.keys[name] = value
	}
	data, _ := json.MarshalIndent(s.keys, "", "  ")
	s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.file), 0o700); err != nil {
		return err
	}
	if err := envsync.WriteFileAtomic(s.file, data, 0o600); err != nil {
		return fmt.Errorf("hostkeys: %w", err)
	}
	return nil
}

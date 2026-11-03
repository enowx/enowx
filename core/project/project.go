// Package project keeps the folders a daemon's user works in. Projects are
// stored on the host that runs the daemon, since their paths are its paths.
package project

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/enowx/enowx/core/git"
	"github.com/enowx/enowx/proto"
)

// Store is the list of projects, saved as JSON in one file.
type Store struct {
	path string

	mu     sync.Mutex
	paths  []string          // in the user's order
	groups map[string]string // a project's group, by path; none for most
	listed map[string]listed // what List last learned of a folder, by path
}

// Open loads the store at path, which may not exist yet.
func Open(path string) (*Store, error) {
	s := &Store{path: path}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("project: %w", err)
	}
	if err := json.Unmarshal(data, &s.paths); err != nil {
		return nil, fmt.Errorf("project: read %s: %w", path, err)
	}
	if err := s.loadGroups(); err != nil {
		return nil, err
	}
	return s, nil
}

// List describes every project, in the user's order (the order they were
// added, until they move one). Folders that no longer exist are kept, so a
// project on an unmounted disk comes back.
func (s *Store) List() []proto.ProjectInfo {
	s.mu.Lock()
	paths := slices.Clone(s.paths)
	groups := maps.Clone(s.groups)
	s.mu.Unlock()
	out := make([]proto.ProjectInfo, 0, len(paths))
	for _, p := range paths {
		info := s.describe(p)
		info.Group = groups[p]
		out = append(out, info)
	}
	return out
}

// Paths is the projects' folders in the user's order, for a caller that needs
// nothing else of them: it runs no git.
func (s *Store) Paths() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.paths)
}

// Add registers dir (a git repository registers its root) and returns it.
func (s *Store) Add(dir string) (proto.ProjectInfo, error) {
	abs, err := filepath.Abs(expandHome(dir))
	if err != nil {
		return proto.ProjectInfo{}, err
	}
	st, err := os.Stat(abs)
	if err != nil {
		return proto.ProjectInfo{}, fmt.Errorf("project: %w", err)
	}
	if !st.IsDir() {
		return proto.ProjectInfo{}, fmt.Errorf("project: %s is not a folder", abs)
	}
	if top, err := git.TopLevel(abs); err == nil {
		abs = top
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !slices.Contains(s.paths, abs) {
		s.paths = append(s.paths, abs)
		if err := s.save(); err != nil {
			s.paths = s.paths[:len(s.paths)-1]
			return proto.ProjectInfo{}, err
		}
	}
	return Inspect(abs).ProjectInfo, nil
}

// Remove forgets a project. Its folder is left untouched.
func (s *Store) Remove(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := slices.Index(s.paths, path)
	if i < 0 {
		return fmt.Errorf("project: %s is not a project", path)
	}
	s.paths = slices.Delete(s.paths, i, i+1)
	delete(s.listed, path)
	if _, ok := s.groups[path]; ok {
		delete(s.groups, path)
		if err := s.saveGroups(); err != nil {
			return err
		}
	}
	return s.save()
}

// save writes the list atomically; s.mu is held.
func (s *Store) save() error {
	data, err := json.MarshalIndent(s.paths, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	return writeAtomic(s.path, data)
}

func writeAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Inspect describes the folder at path: whether it is a git repository,
// its current branch and its local branches.
func Inspect(path string) proto.ProjectDetail {
	d := proto.ProjectDetail{ProjectInfo: inspectInfo(path)}
	if !d.IsGit {
		return d
	}
	d.Branches, _ = git.Branches(path)
	if cfg, err := LoadConfig(path); err == nil {
		d.Setup, d.Copy = cfg.Worktree.Setup, cfg.Worktree.Copy
	}
	return d
}

func expandHome(p string) string {
	if p == "~" || len(p) > 1 && p[:2] == "~/" {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[1:])
		}
	}
	return p
}

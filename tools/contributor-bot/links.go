package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// links are who linked what: a Discord ID and a GitHub ID, and nothing else, no token,
// name or email. One GitHub account links to one Discord account.
type links struct {
	mu   sync.Mutex
	path string
	byD  map[string]int64 // Discord ID → GitHub ID
}

// errTaken says a GitHub account is linked to another member.
var errTaken = errors.New("that GitHub account is linked to another member")

func openLinks(path string) (*links, error) {
	l := &links{path: path, byD: map[string]int64{}}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return l, nil
	}
	if err != nil {
		return nil, fmt.Errorf("links: %w", err)
	}
	if err := json.Unmarshal(data, &l.byD); err != nil {
		return nil, fmt.Errorf("links: %s: %w", path, err)
	}
	return l, nil
}

// saveLocked writes the links whole or not at all. The caller holds l.mu.
func (l *links) saveLocked() error {
	data, err := json.Marshal(l.byD)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(l.path), 0o700); err != nil {
		return err
	}
	tmp := l.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, l.path)
}

// link links discord to github. A member linking another account replaces their own.
func (l *links) link(discord string, github int64) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	for d, g := range l.byD {
		if g == github && d != discord {
			return errTaken
		}
	}
	was, had := l.byD[discord]
	l.byD[discord] = github
	if err := l.saveLocked(); err != nil {
		if had {
			l.byD[discord] = was
		} else {
			delete(l.byD, discord)
		}
		return err
	}
	return nil
}

// unlink removes discord's link, and says whether there was one.
func (l *links) unlink(discord string) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	was, had := l.byD[discord]
	if !had {
		return false, nil
	}
	delete(l.byD, discord)
	if err := l.saveLocked(); err != nil {
		l.byD[discord] = was
		return false, err
	}
	return true, nil
}

// discordOf is the member who linked github.
func (l *links) discordOf(github int64) (string, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for d, g := range l.byD {
		if g == github {
			return d, true
		}
	}
	return "", false
}

// has says discord has a link.
func (l *links) has(discord string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	_, ok := l.byD[discord]
	return ok
}

package accounts

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

// Share only known settings and conversation files. An exclusion list would
// share newly introduced databases, transient SQLite sidecars or daemon state.
var codexShared = []string{
	"config.toml", "AGENTS.md", "skills", "prompts", "rules", "plugins",
	"history.jsonl", "session_index.jsonl",
	"sessions", "archived_sessions", "thread-writer-locks",
}

var codexCanonicalDirs = []string{"sessions", "archived_sessions", "thread-writer-locks"}

// Account starts in this daemon serialize link preparation. Concurrent Windows
// symlink creation can fail with transient privilege or identity errors.
var codexShareGate = make(chan struct{}, 1)

// shareCodex leaves runtime files private, including absent journals that
// SQLite may recreate. Legacy runtime links require an offline review: even
// unlinking a live socket or journal can disrupt a running agent.
func shareCodex(src, home string) error {
	codexShareGate <- struct{}{}
	defer func() { <-codexShareGate }()

	root, err := os.Lstat(home)
	if err != nil {
		return fmt.Errorf("accounts: inspect Codex home: %w", err)
	}
	if root.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("accounts: Codex account home %s is a link; use a private directory", home)
	}
	entries, err := os.ReadDir(home)
	if err != nil {
		return fmt.Errorf("accounts: inspect Codex home: %w", err)
	}
	for _, e := range entries {
		if !slices.Contains(codexShared, e.Name()) && e.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("accounts: Codex home has a private runtime link %s; stop users of this home and review it offline before starting (no files changed)", filepath.Join(home, e.Name()))
		}
	}
	// Rollouts and their writer locks must have one namespace, including
	// when the default agent has not created those directories yet.
	for _, name := range codexCanonicalDirs {
		target, link := filepath.Join(src, name), filepath.Join(home, name)
		if err := os.MkdirAll(target, 0o700); err != nil {
			return fmt.Errorf("accounts: prepare canonical Codex %s: %w", name, err)
		}
		if _, err := os.Lstat(link); errors.Is(err, os.ErrNotExist) {
			if err := os.Symlink(target, link); err != nil {
				// Windows may report a privilege error when a concurrent start
				// wins creation. Only accept an existing, validated directory.
				if _, statErr := os.Lstat(link); statErr != nil {
					return fmt.Errorf("accounts: link canonical Codex %s: %w", name, err)
				}
			}
		} else if err != nil {
			return fmt.Errorf("accounts: inspect Codex %s: %w", name, err)
		}
		// A simultaneous start may have made the link. Validate its identity
		// instead of replacing it, preserving any writer lock already held.
		a, err := os.Stat(link)
		if err != nil {
			return fmt.Errorf("accounts: inspect Codex %s: %w", name, err)
		}
		b, err := os.Stat(target)
		if err != nil || !os.SameFile(a, b) {
			return fmt.Errorf("accounts: Codex %s must use %s; review existing data and writer locks offline", link, target)
		}
	}
	for _, name := range codexShared {
		if slices.Contains(codexCanonicalDirs, name) {
			continue
		}
		target, link := filepath.Join(src, name), filepath.Join(home, name)
		if _, err := os.Lstat(link); err == nil {
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("accounts: inspect Codex %s: %w", name, err)
		}
		if _, err := os.Lstat(target); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return fmt.Errorf("accounts: inspect shared Codex %s: %w", name, err)
		}
		// Optional settings retain the other agents' best-effort sharing.
		_ = os.Symlink(target, link)
	}
	return nil
}

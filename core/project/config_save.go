package project

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// SaveConfig writes c's worktree settings into the project at dir's
// .enowx.toml, keeping the file's other tables and keys; its comments
// don't survive. With nothing to say and no other settings, the file goes.
func SaveConfig(dir string, c Config) error {
	path := filepath.Join(dir, ConfigFile)
	all := map[string]any{}
	if _, err := toml.DecodeFile(path, &all); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("project: %s: %w", ConfigFile, err)
	}
	wt, _ := all["worktree"].(map[string]any)
	if wt == nil {
		wt = map[string]any{}
	}
	var copies []string
	for _, p := range c.Worktree.Copy {
		if p = strings.TrimSpace(p); p != "" {
			copies = append(copies, p)
		}
	}
	setWorktreeKey(wt, "copy", copies, len(copies) > 0)
	setup := strings.TrimSpace(c.Worktree.Setup)
	setWorktreeKey(wt, "setup", setup, setup != "")
	if len(wt) > 0 {
		all["worktree"] = wt
	} else {
		delete(all, "worktree")
	}
	if len(all) == 0 {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("project: %s: %w", ConfigFile, err)
		}
		return nil
	}
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(all); err != nil {
		return fmt.Errorf("project: %s: %w", ConfigFile, err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("project: %s: %w", ConfigFile, err)
	}
	return nil
}

func setWorktreeKey(wt map[string]any, key string, v any, keep bool) {
	if keep {
		wt[key] = v
	} else {
		delete(wt, key)
	}
}

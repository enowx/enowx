package project

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// ConfigFile is a project's own settings for enowX, at its root, to
// commit with it: what new worktrees get.
const ConfigFile = ".enowx.toml"

// Config is a project's .enowx.toml.
type Config struct {
	// Guide is how enowX gives the guide to agents with no other way.
	Guide struct {
		// Block false keeps its block out of the project's AGENTS.md or CLAUDE.md.
		Block *bool `toml:"block,omitempty"`
	} `toml:"guide,omitempty"`
	Worktree struct {
		// Copy are files or folders git ignores (.env, say) that new
		// worktrees get from the project's folder, as paths or globs.
		Copy []string `toml:"copy"`
		// Setup is a shell script that readies a new worktree (npm
		// install, say), run in a tab of the session when the user asks.
		Setup string `toml:"setup"`
	} `toml:"worktree"`
}

// LoadConfig reads the project at dir's .enowx.toml; a project without one
// has the zero Config.
func LoadConfig(dir string) (Config, error) {
	var c Config
	_, err := toml.DecodeFile(filepath.Join(dir, ConfigFile), &c)
	if errors.Is(err, fs.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("project: %s: %w", ConfigFile, err)
	}
	c.Worktree.Setup = strings.TrimSpace(c.Worktree.Setup)
	return c, nil
}

// CopyInto copies what c says new worktrees get from the project at from
// into the worktree at to, leaving what the worktree has. Paths can't
// leave the project; it returns what it copied, relative.
func (c Config) CopyInto(from, to string) ([]string, error) {
	var copied []string
	for _, pattern := range c.Worktree.Copy {
		pattern = filepath.Clean(filepath.FromSlash(strings.TrimSpace(pattern)))
		if pattern == "." || filepath.IsAbs(pattern) || strings.HasPrefix(pattern, "..") {
			continue
		}
		matches, err := filepath.Glob(filepath.Join(from, pattern))
		if err != nil {
			return copied, fmt.Errorf("project: copy %s: %w", pattern, err)
		}
		for _, src := range matches {
			rel, err := filepath.Rel(from, src)
			if err != nil || strings.HasPrefix(rel, "..") {
				continue
			}
			if err := copyTree(src, filepath.Join(to, rel)); err != nil {
				return copied, err
			}
			copied = append(copied, filepath.ToSlash(rel))
		}
	}
	return copied, nil
}

// copyTree copies src (a file or folder) to dst, keeping what dst has.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !d.Type().IsRegular() {
			return nil // links and devices stay behind
		}
		if _, err := os.Lstat(target); err == nil {
			return nil
		}
		return copyFile(p, target)
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// GuideBlockOff says the project turned off the guide's block.
func (c Config) GuideBlockOff() bool { return c.Guide.Block != nil && !*c.Guide.Block }

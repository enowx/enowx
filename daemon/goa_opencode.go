package daemon

import (
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/enowx/enowx/core/agent"
)

// goaOpenCodeNewest is the newest major version of OpenCode tested to hold GOA.
const goaOpenCodeNewest = 2

// goaOpenCodeMajor is preset's version, at least 2 for its own preset of version 2.
func goaOpenCodeMajor(preset agent.Preset) int {
	major := agent.OpenCodeMajor(preset.Program())
	if preset.ID == "opencode2" && major < 2 {
		return 2
	}
	return major
}

// goaOpenCodeReady refuses to start GOA on an OpenCode not tested to hold it, or with a
// link in its folder leading out of it: from version 2, what holds its edits goes by the
// path it is asked for, not where that leads, and without a shell it can't make a link.
func goaOpenCodeReady(preset agent.Preset, dir string) error {
	major := goaOpenCodeMajor(preset)
	if major == 0 {
		return fmt.Errorf("goa: %s doesn't say its version: is it installed?", preset.Name)
	}
	if major > goaOpenCodeNewest {
		return fmt.Errorf("goa: %s %d isn't tested yet to hold GOD of Agents to delegating", preset.Name, major)
	}
	if link := linkOutOf(dir); link != "" {
		return fmt.Errorf("goa: %s in GOD of Agents' folder is a link leading out of it, a way to edit outside it: remove it to start", link)
	}
	return nil
}

// linkOutOf is the first symbolic link under dir leading outside it, "" when there's none.
func linkOutOf(dir string) string {
	root, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return ""
	}
	found := ""
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.Type()&os.ModeSymlink == 0 {
			return nil
		}
		to, err := filepath.EvalSymlinks(path)
		if err != nil {
			if target, rerr := os.Readlink(path); rerr == nil {
				to = target // dangling: a write there goes where it says
				if !filepath.IsAbs(to) {
					to = filepath.Join(filepath.Dir(path), to)
				}
			}
		}
		if rel, err := filepath.Rel(root, to); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

// goaOpenCodeEnv is GOA's config on OpenCode id. Failing, it gets none, and with no
// enowX tools does nothing; the log says why.
func goaOpenCodeEnv(id, dataDir string) []string {
	preset, _ := agent.Find(id)
	env, err := agent.GOAOpenCodeEnv(goaOpenCodeMajor(preset), programPath(), dataDir)
	if err != nil {
		slog.Warn("could not give GOD of Agents its config", "agent", id, "err", err)
		return nil
	}
	return env
}

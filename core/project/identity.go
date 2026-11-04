package project

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/enowx/enowx/core/git"
)

// Canonical is folder dir as the disk spells it: absolute, symlinks followed,
// and each name in the case its folder lists it. A name in another case only
// resolves on a filesystem that ignores case (macOS and Windows, by default),
// so a folder really named otherwise stays apart.
func Canonical(dir string) string {
	if dir == "" {
		return ""
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = filepath.Clean(dir)
	}
	p := diskCase(abs)
	if real, err := filepath.EvalSymlinks(p); err == nil {
		p = diskCase(real)
	}
	return p
}

// diskCase is absolute path p with each name as its parent lists it. A name
// listed exactly stays, whatever else is there; one found in another case only
// when it is the only such.
func diskCase(p string) string {
	vol := filepath.VolumeName(p)
	out := strings.ToUpper(vol) + string(filepath.Separator)
	names := strings.Split(strings.TrimPrefix(p[len(vol):], string(filepath.Separator)), string(filepath.Separator))
	for i, name := range names {
		if name == "" {
			continue
		}
		listed, err := os.ReadDir(out)
		if err != nil {
			return filepath.Join(append([]string{out}, names[i:]...)...)
		}
		found, others := "", 0
		for _, e := range listed {
			if e.Name() == name {
				found, others = name, 0
				break
			}
			if strings.EqualFold(e.Name(), name) {
				found = e.Name()
				others++
			}
		}
		if found == "" || others > 1 {
			found = name
		}
		out = filepath.Join(out, found)
	}
	return out
}

// Identity is the folder the project holding dir is known by, whichever way
// it is reached: its canonical spelling, and for a Git repository its main
// folder, from a subfolder or a linked worktree alike. inRepo says it is in
// one: a folder outside any has no way to tell where its project starts.
func Identity(dir string) (path string, inRepo bool) {
	p := Canonical(dir)
	if p == "" {
		return "", false
	}
	out, err := git.Run(p, "rev-parse", "--git-common-dir")
	if err != nil || out == "" {
		return p, false
	}
	common := out
	if !filepath.IsAbs(common) {
		common = filepath.Join(p, common)
	}
	common = Canonical(common)
	if filepath.Base(common) == ".git" {
		return filepath.Dir(common), true
	}
	// A submodule's top is its own; a bare repository's worktrees have only
	// the repository in common.
	gitDir, err := git.Run(p, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return common, true
	}
	if Canonical(gitDir) == common {
		if top, err := git.Run(p, "rev-parse", "--show-toplevel"); err == nil && top != "" {
			return Canonical(top), true
		}
	}
	return common, true
}

// Worktrees are the folders of the repository holding dir, the main one too,
// canonically; none outside a repository.
func Worktrees(dir string) []string {
	out, err := git.Run(dir, "worktree", "list", "--porcelain")
	if err != nil {
		return nil
	}
	var list []string
	for _, line := range strings.Split(out, "\n") {
		if path, ok := strings.CutPrefix(line, "worktree "); ok {
			list = append(list, Canonical(strings.TrimSpace(path)))
		}
	}
	return list
}

package tools

import (
	"bufio"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ignoreRule is one line of a .gitignore.
type ignoreRule struct {
	pattern *globPattern
	negate  bool
	dirOnly bool
}

// ignoreSet is the rules of one .gitignore, for the paths under its folder.
type ignoreSet struct {
	dir   string // with slashes, ending in one
	rules []ignoreRule
}

// parseIgnore reads the rules of a file in dir; none when it doesn't exist.
func parseIgnore(file, dir string) *ignoreSet {
	f, err := os.Open(file)
	if err != nil {
		return nil
	}
	defer f.Close()
	set := &ignoreSet{dir: strings.TrimSuffix(filepath.ToSlash(dir), "/") + "/"}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), " \t\r")
		if line == "" || line[0] == '#' {
			continue
		}
		rule := ignoreRule{}
		if line[0] == '!' {
			rule.negate = true
			line = line[1:]
		}
		if strings.HasSuffix(line, "/") {
			rule.dirOnly = true
			line = strings.TrimRight(line, "/")
		}
		// A slash at the start or inside ties the pattern to this folder.
		anchored := strings.Contains(line, "/")
		line = strings.TrimPrefix(line, "/")
		if line == "" {
			continue
		}
		pattern, err := compileGlob(line, !anchored)
		if err != nil {
			continue // git skips what it can't read too
		}
		rule.pattern = pattern
		set.rules = append(set.rules, rule)
	}
	if len(set.rules) == 0 {
		return nil
	}
	return set
}

// ignored says the path abs is ignored, the deepest file and its last rule deciding.
func ignored(sets []*ignoreSet, abs string, isDir bool) bool {
	abs = filepath.ToSlash(abs)
	for i := len(sets) - 1; i >= 0; i-- {
		set := sets[i]
		rel, ok := strings.CutPrefix(abs, set.dir)
		if !ok {
			continue
		}
		for j := len(set.rules) - 1; j >= 0; j-- {
			rule := set.rules[j]
			if rule.dirOnly && !isDir {
				continue
			}
			if rule.pattern.match(rel) {
				return !rule.negate
			}
		}
	}
	return false
}

// repoIgnores are the rules from above root that hold in it: those of the repository
// root lies in, its top folder's first, for a search that starts in a subfolder.
func repoIgnores(root string) []*ignoreSet {
	var chain []string
	top := ""
	for dir := root; ; dir = filepath.Dir(dir) {
		if _, err := os.Lstat(filepath.Join(dir, ".git")); err == nil {
			top = dir
			break
		}
		if dir == filepath.Dir(dir) {
			return nil
		}
		chain = append(chain, filepath.Dir(dir))
	}
	var sets []*ignoreSet
	if set := parseIgnore(filepath.Join(top, ".git", "info", "exclude"), top); set != nil {
		sets = append(sets, set)
	}
	for i := len(chain) - 1; i >= 0; i-- {
		if set := parseIgnore(filepath.Join(chain[i], ".gitignore"), chain[i]); set != nil {
			sets = append(sets, set)
		}
	}
	return sets
}

// walkFiles calls fn for what's under root, in name order: all but folders, links
// unfollowed, never inside .git, nor what .gitignore rules out when honorIgnore. rel has
// slashes. It ends when fn returns false, and with ctx's error when ctx ends.
func walkFiles(ctx context.Context, root string, honorIgnore bool, fn func(abs, rel string, d fs.DirEntry) bool) error {
	w := &walker{ctx: ctx, honorIgnore: honorIgnore, fn: fn}
	var sets []*ignoreSet
	if honorIgnore {
		sets = repoIgnores(root)
	}
	w.walk(root, "", sets)
	return ctx.Err()
}

type walker struct {
	ctx         context.Context
	honorIgnore bool
	fn          func(abs, rel string, d fs.DirEntry) bool
	stopped     bool
}

func (w *walker) walk(dir, rel string, sets []*ignoreSet) {
	if w.stopped || w.ctx.Err() != nil {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return // a folder that can't be read has nothing to find
	}
	if w.honorIgnore {
		if set := parseIgnore(filepath.Join(dir, ".gitignore"), dir); set != nil {
			sets = append(sets[:len(sets):len(sets)], set)
		}
	}
	for _, entry := range entries {
		if w.stopped || w.ctx.Err() != nil {
			return
		}
		name := entry.Name()
		if name == ".git" {
			continue
		}
		abs, childRel := filepath.Join(dir, name), name
		if rel != "" {
			childRel = rel + "/" + name
		}
		if w.honorIgnore && ignored(sets, abs, entry.IsDir()) {
			continue
		}
		if entry.IsDir() {
			w.walk(abs, childRel, sets)
			continue
		}
		if !w.fn(abs, childRel, entry) {
			w.stopped = true
		}
	}
}

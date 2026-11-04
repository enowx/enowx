package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const globMost = 100

const globSchema = `{"type":"object","properties":{"pattern":{"type":"string","description":"The pattern: * and ? within a name, ** across folders, {a,b} for either. Without a slash it matches at any depth"},"path":{"type":"string","description":"The folder to search (default the working folder)"}},"required":["pattern"],"additionalProperties":false}`

type globTool struct{}

func (globTool) Name() string { return "Glob" }
func (globTool) Description() string {
	return "Find files by name pattern, most recently changed first, up to 100. Leaves out .git."
}
func (globTool) Schema() json.RawMessage { return json.RawMessage(globSchema) }
func (globTool) ReadOnly() bool          { return true }
func (globTool) Edits() bool             { return false }

type globInput struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path"`
}

func (globTool) Detail(input json.RawMessage) string {
	in, err := decode[globInput](input)
	if err != nil {
		return "Glob"
	}
	if in.Path != "" {
		return oneLine(in.Pattern+" in "+in.Path, 200)
	}
	return oneLine(in.Pattern, 200)
}

func (globTool) ReadPaths(input json.RawMessage, dir string) []string {
	in, err := decode[globInput](input)
	if err != nil {
		return nil
	}
	root, _, _ := globRoot(ResolvePath(dir, cmpOr(in.Path, ".")), in.Pattern)
	return []string{root}
}

func (globTool) Run(ctx context.Context, input json.RawMessage, env Env) Result {
	in, err := decode[globInput](input)
	if err != nil {
		return failf("%v", err)
	}
	if in.Pattern == "" {
		return failf("pattern is required")
	}
	base := ResolvePath(env.Dir, cmpOr(in.Path, "."))
	root, rest, anyDepth := globRoot(base, in.Pattern)
	pattern, err := compileGlob(rest, anyDepth)
	if err != nil {
		return failf("%v", err)
	}
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		return failf("%s isn't a folder", shownPath(env.Dir, root))
	}
	type found struct {
		path string
		when time.Time
	}
	var files []found
	err = walkFiles(ctx, root, false, func(abs, rel string, d fs.DirEntry) bool {
		if pattern.match(rel) {
			when := time.Time{}
			if info, err := d.Info(); err == nil {
				when = info.ModTime()
			}
			files = append(files, found{abs, when})
		}
		return true
	})
	if err != nil {
		return errorResult(ctx, err)
	}
	if len(files) == 0 {
		return Result{Text: "No files found"}
	}
	slices.SortFunc(files, func(a, b found) int {
		if c := b.when.Compare(a.when); c != 0 {
			return c
		}
		return strings.Compare(a.path, b.path)
	})
	var out strings.Builder
	for _, f := range files[:min(len(files), globMost)] {
		out.WriteString(shownPath(env.Dir, f.path))
		out.WriteByte('\n')
	}
	if len(files) > globMost {
		fmt.Fprintf(&out, "[%d of %d files shown: narrow the pattern]\n", globMost, len(files))
	}
	return Result{Text: out.String()}
}

// globRoot splits a pattern from the folder it starts in: its leading segments without
// wildcards, so "../x/*.go" and an absolute pattern search where they say. Only a
// pattern with no slash at all matches at any depth.
func globRoot(base, pattern string) (root, rest string, anyDepth bool) {
	slashed := filepath.ToSlash(pattern)
	if !strings.Contains(slashed, "/") {
		return base, pattern, true
	}
	segments := strings.Split(slashed, "/")
	i := 0
	for i < len(segments)-1 && !strings.ContainsAny(segments[i], "*?[{") {
		i++
	}
	leading := filepath.FromSlash(strings.Join(segments[:i], "/"))
	switch {
	case filepath.IsAbs(pattern) && i == 1:
		root = filepath.FromSlash(segments[0] + "/") // "/" or "C:/"
	case filepath.IsAbs(pattern):
		root = filepath.Clean(leading)
	default:
		root = filepath.Join(base, leading)
	}
	return root, strings.Join(segments[i:], "/"), false
}

func cmpOr(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

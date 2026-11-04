package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestGlobMatching(t *testing.T) {
	cases := []struct {
		pattern, rel string
		want         bool
	}{
		{"*.go", "a.go", true},
		{"*.go", "deep/er/a.go", true}, // no slash: any depth
		{"**/*.go", "a.go", true},
		{"**/*.go", "x/y/a.go", true},
		{"src/**/*.ts", "src/a.ts", true},
		{"src/**/*.ts", "src/x/y/a.ts", true},
		{"src/**/*.ts", "lib/a.ts", false},
		{"src/*.ts", "src/x/a.ts", false},
		{"*.{ts,tsx}", "x/a.tsx", true},
		{"*.{ts,tsx}", "x/a.js", false},
		{"{a,b}/*.go", "b/x.go", true},
		{"{a,{b,c}}/x", "c/x", true},
		{"./*.go", "a.go", true},
		{"./*.go", "x/a.go", false},
		{"a?c", "abc", true},
		{"[ab].txt", "b.txt", true},
	}
	for _, c := range cases {
		g, err := newGlob(c.pattern)
		if err != nil {
			t.Errorf("%s: %v", c.pattern, err)
			continue
		}
		if got := g.match(c.rel); got != c.want {
			t.Errorf("%s on %s: got %v", c.pattern, c.rel, got)
		}
	}
	if _, err := newGlob("[a"); err == nil {
		t.Error("a bad pattern compiled")
	}
	if _, err := newGlob(strings.Repeat("{a,b}", 20)); err == nil {
		t.Error("a pattern of a million alternatives compiled")
	}
}

func TestGlobSortsByTimeLeavesGitOutAndCaps(t *testing.T) {
	dir := t.TempDir()
	base := time.Now().Add(-time.Hour)
	for i, name := range []string{"old.go", "sub/mid.go", "sub/new.go"} {
		path := filepath.Join(dir, filepath.FromSlash(name))
		writeFile(t, path, "x")
		when := base.Add(time.Duration(i) * time.Minute)
		if err := os.Chtimes(path, when, when); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, filepath.Join(dir, ".git", "x.go"), "x")
	writeFile(t, filepath.Join(dir, "a.txt"), "x")

	r := call(t, globTool{}, dir, map[string]any{"pattern": "**/*.go"})
	if want := "sub/new.go\nsub/mid.go\nold.go\n"; r.Failed || r.Text != want {
		t.Fatalf("got %q", r.Text)
	}
	if r = call(t, globTool{}, dir, map[string]any{"pattern": "*.go", "path": "sub"}); r.Text != "sub/new.go\nsub/mid.go\n" {
		t.Fatalf("with path: %q", r.Text)
	}
	if r = call(t, globTool{}, dir, map[string]any{"pattern": "sub/*.go"}); r.Text != "sub/new.go\nsub/mid.go\n" {
		t.Fatalf("prefixed: %q", r.Text)
	}
	if r = call(t, globTool{}, dir, map[string]any{"pattern": filepath.ToSlash(dir) + "/sub/n*.go"}); r.Text != "sub/new.go\n" {
		t.Fatalf("absolute: %q", r.Text)
	}
	if r = call(t, globTool{}, dir, map[string]any{"pattern": "*.nope"}); r.Failed || !strings.Contains(r.Text, "No files") {
		t.Fatalf("none: %+v", r)
	}
	if r = call(t, globTool{}, dir, map[string]any{"pattern": "*", "path": "gone"}); !r.Failed {
		t.Fatalf("missing folder: %+v", r)
	}

	many := t.TempDir()
	for i := range globMost + 5 {
		writeFile(t, filepath.Join(many, fmt.Sprintf("f%d.txt", i)), "")
	}
	r = call(t, globTool{}, many, map[string]any{"pattern": "*.txt"})
	if lines := strings.Count(r.Text, "\n"); lines != globMost+1 || !strings.Contains(r.Text, "100 of 105") {
		t.Fatalf("%d lines, %q", lines, r.Text[len(r.Text)-60:])
	}
}

func TestGrepModes(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.go"), "package a\nfunc Hello() {}\n\n// hello again\n")
	writeFile(t, filepath.Join(dir, "sub", "b.txt"), "Hello there\r\nno\r\n")
	writeFile(t, filepath.Join(dir, "none.txt"), "nothing\n")

	r := call(t, grepTool{}, dir, map[string]any{"pattern": "Hello"})
	if r.Failed || r.Text != "a.go\nsub/b.txt\n" {
		t.Fatalf("files: %q", r.Text)
	}
	r = call(t, grepTool{}, dir, map[string]any{"pattern": "hello", "-i": true, "output_mode": "count"})
	if r.Text != "a.go:2\nsub/b.txt:1\n" {
		t.Fatalf("count: %q", r.Text)
	}
	r = call(t, grepTool{}, dir, map[string]any{"pattern": "Hello", "output_mode": "content", "glob": "*.go"})
	if r.Text != "a.go:2:func Hello() {}\n" {
		t.Fatalf("content with glob: %q", r.Text)
	}
	r = call(t, grepTool{}, dir, map[string]any{"pattern": "there$", "output_mode": "content", "path": "sub/b.txt", "-n": false})
	if r.Text != "sub/b.txt:Hello there\n" {
		t.Fatalf("a line ending in \\r: %q", r.Text)
	}
	r = call(t, grepTool{}, dir, map[string]any{"pattern": "nope"})
	if r.Failed || !strings.Contains(r.Text, "No matches") {
		t.Fatalf("no match: %+v", r)
	}
	if r = call(t, grepTool{}, dir, map[string]any{"pattern": "("}); !r.Failed {
		t.Fatalf("bad regexp: %+v", r)
	}
	if r = call(t, grepTool{}, dir, map[string]any{"pattern": "x", "path": "gone"}); !r.Failed {
		t.Fatalf("missing path: %+v", r)
	}
}

func TestGrepContext(t *testing.T) {
	dir := t.TempDir()
	var b strings.Builder
	for i := 1; i <= 12; i++ {
		text := "plain"
		if i == 3 || i == 5 || i == 11 {
			text = "HIT"
		}
		fmt.Fprintf(&b, "%s %d\n", text, i)
	}
	writeFile(t, filepath.Join(dir, "a.txt"), b.String())
	r := call(t, grepTool{}, dir, map[string]any{"pattern": "HIT", "output_mode": "content", "-C": 1})
	want := "a.txt-2-plain 2\na.txt:3:HIT 3\na.txt-4-plain 4\na.txt:5:HIT 5\na.txt-6-plain 6\n--\na.txt-10-plain 10\na.txt:11:HIT 11\na.txt-12-plain 12\n"
	if r.Text != want {
		t.Fatalf("got\n%s", r.Text)
	}
	r = call(t, grepTool{}, dir, map[string]any{"pattern": "HIT 5", "output_mode": "content", "-B": 1, "-A": 0})
	if r.Text != "a.txt-4-plain 4\na.txt:5:HIT 5\n" {
		t.Fatalf("before only: %q", r.Text)
	}
}

func TestGrepFollowsGitignoreAndSkipsBinaries(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, ".git", "config"), "needle\n")
	writeFile(t, filepath.Join(dir, ".gitignore"), "# comment\n*.log\nbuild/\n/rooted.txt\ndocs/gen/\n!keep.log\n")
	writeFile(t, filepath.Join(dir, "sub", ".gitignore"), "local.txt\n!again.tmp\n")
	for _, name := range []string{"a.log", "keep.log", "build/out.txt", "rooted.txt", "sub/rooted.txt", "sub/local.txt", "local.txt", "docs/gen/x.txt", "docs/y.txt", "sub/build/z.txt"} {
		writeFile(t, filepath.Join(dir, filepath.FromSlash(name)), "needle\n")
	}
	writeFile(t, filepath.Join(dir, "bin.dat"), "needle\x00\n")

	r := call(t, grepTool{}, dir, map[string]any{"pattern": "needle"})
	want := "docs/y.txt\nkeep.log\nlocal.txt\nsub/rooted.txt\n"
	if r.Text != want {
		t.Fatalf("got\n%s", r.Text)
	}
	// From a subfolder the repository's rules above it still hold.
	writeFile(t, filepath.Join(dir, "docs", "x.log"), "needle\n")
	r = call(t, grepTool{}, dir, map[string]any{"pattern": "needle", "path": "docs"})
	if r.Text != "docs/y.txt\n" {
		t.Fatalf("from a subfolder: %q", r.Text)
	}
	// A link isn't followed out of the folder, nor read as a file.
	if runtime.GOOS != "windows" {
		outside := filepath.Join(t.TempDir(), "secret.txt")
		writeFile(t, outside, "needle\n")
		if err := os.Symlink(outside, filepath.Join(dir, "docs", "link.txt")); err != nil {
			t.Fatal(err)
		}
		if r = call(t, grepTool{}, dir, map[string]any{"pattern": "needle", "path": "docs"}); r.Text != "docs/y.txt\n" {
			t.Fatalf("through a link: %q", r.Text)
		}
	}
	// A file named outright is searched.
	r = call(t, grepTool{}, dir, map[string]any{"pattern": "needle", "path": "a.log"})
	if r.Text != "a.log\n" {
		t.Fatalf("named outright: %q", r.Text)
	}
}

func TestGrepCaps(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.txt"), strings.Repeat("hit\n", 1000))
	r := call(t, grepTool{}, dir, map[string]any{"pattern": "hit", "output_mode": "content"})
	if lines := strings.Count(r.Text, "\n"); lines != grepDefaultLimit+1 || !strings.Contains(r.Text, "capped") {
		t.Fatalf("%d lines", lines)
	}
	r = call(t, grepTool{}, dir, map[string]any{"pattern": "hit", "output_mode": "content", "head_limit": 3})
	if lines := strings.Count(r.Text, "\n"); lines != 4 {
		t.Fatalf("head_limit: %d lines", lines)
	}
	writeFile(t, filepath.Join(dir, "long.txt"), strings.Repeat("long", 1<<16)+"\n")
	r = call(t, grepTool{}, dir, map[string]any{"pattern": "long", "output_mode": "content", "path": "long.txt"})
	if len(r.Text) > 2*grepLineMost {
		t.Fatalf("a long line takes %d bytes", len(r.Text))
	}
	writeFile(t, filepath.Join(dir, "huge.txt"), strings.Repeat("h", grepFileMost+1))
	r = call(t, grepTool{}, dir, map[string]any{"pattern": "h+$", "path": "huge.txt"})
	if !strings.Contains(r.Text, "not searched") {
		t.Fatalf("huge file: %q", r.Text)
	}
}

func TestGrepAndGlobStopWhenCancelled(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.txt"), "x\n")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, c := range []struct {
		tool  Tool
		input string
	}{{grepTool{}, `{"pattern":"x"}`}, {globTool{}, `{"pattern":"*"}`}} {
		r := c.tool.Run(ctx, []byte(c.input), Env{Dir: dir})
		if !r.Failed || r.Text != "interrupted" {
			t.Errorf("%s: %+v", c.tool.Name(), r)
		}
	}
}

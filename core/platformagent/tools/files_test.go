package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// call runs t with input given as a map, in dir.
func call(t *testing.T, tool Tool, dir string, input map[string]any) Result {
	t.Helper()
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	return tool.Run(context.Background(), raw, Env{Dir: dir, Environ: os.Environ()})
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func fileText(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestReadNumbersLinesAndPages(t *testing.T) {
	dir := t.TempDir()
	var b strings.Builder
	for i := 1; i <= 2500; i++ {
		fmt.Fprintf(&b, "line %d\r\n", i)
	}
	writeFile(t, filepath.Join(dir, "a.txt"), b.String())

	r := call(t, readTool{}, dir, map[string]any{"file_path": "a.txt"})
	if r.Failed || !strings.HasPrefix(r.Text, "     1\tline 1\n") || !strings.Contains(r.Text, "continue with offset 2001") {
		t.Fatalf("first page: failed=%v, start %q, end %q", r.Failed, r.Text[:20], r.Text[len(r.Text)-60:])
	}
	r = call(t, readTool{}, dir, map[string]any{"file_path": "a.txt", "offset": 2499, "limit": 5})
	if want := "  2499\tline 2499\n  2500\tline 2500\n"; r.Failed || r.Text != want {
		t.Fatalf("last lines: %q", r.Text)
	}
	if r = call(t, readTool{}, dir, map[string]any{"file_path": "a.txt", "offset": 9000}); !r.Failed {
		t.Fatalf("offset past the end: %q", r.Text)
	}
}

func TestReadGuardsSize(t *testing.T) {
	dir := t.TempDir()
	// One line far over the cap, and enough lines to pass the text's cap.
	writeFile(t, filepath.Join(dir, "long.txt"), strings.Repeat("x", 1<<20)+"\nafter\n")
	r := call(t, readTool{}, dir, map[string]any{"file_path": "long.txt"})
	if !strings.Contains(r.Text, "line clipped") || !strings.Contains(r.Text, "2\tafter") || len(r.Text) > 3*readLineMost {
		t.Fatalf("long line: %d bytes", len(r.Text))
	}
	writeFile(t, filepath.Join(dir, "wide.txt"), strings.Repeat(strings.Repeat("y", 999)+"\n", 1500))
	r = call(t, readTool{}, dir, map[string]any{"file_path": "wide.txt"})
	if len(r.Text) > readTextMost+2*readLineMost || !strings.Contains(r.Text, "continue with offset") {
		t.Fatalf("wide file: %d bytes", len(r.Text))
	}
}

func TestReadKinds(t *testing.T) {
	dir := t.TempDir()
	png := "\x89PNG\r\n\x1a\nrest of it"
	writeFile(t, filepath.Join(dir, "a.png"), png)
	r := call(t, readTool{}, dir, map[string]any{"file_path": "a.png"})
	if r.Failed || len(r.Images) != 1 || r.Images[0].MediaType != "image/png" || r.Images[0].Base64 != base64.StdEncoding.EncodeToString([]byte(png)) {
		t.Fatalf("png: %+v", r)
	}
	webp := "RIFF\x00\x00\x00\x00WEBPVP8 "
	writeFile(t, filepath.Join(dir, "b.webp"), webp)
	if r = call(t, readTool{}, dir, map[string]any{"file_path": "b.webp"}); len(r.Images) != 1 || r.Images[0].MediaType != "image/webp" {
		t.Fatalf("webp: %+v", r)
	}
	writeFile(t, filepath.Join(dir, "big.png"), "\x89PNG\r\n\x1a\n"+strings.Repeat("x", readImageMost))
	if r = call(t, readTool{}, dir, map[string]any{"file_path": "big.png"}); !r.Failed || len(r.Images) != 0 {
		t.Fatalf("oversized picture: failed=%v images=%d", r.Failed, len(r.Images))
	}
	writeFile(t, filepath.Join(dir, "doc.pdf"), "%PDF-1.7\n\x00\x01")
	if r = call(t, readTool{}, dir, map[string]any{"file_path": "doc.pdf"}); r.Failed || !strings.Contains(r.Text, "PDF") {
		t.Fatalf("pdf: %+v", r)
	}
	writeFile(t, filepath.Join(dir, "junk.bin"), "ab\x00cd\x01\x02")
	if r = call(t, readTool{}, dir, map[string]any{"file_path": "junk.bin"}); !r.Failed || !strings.Contains(r.Text, "binary") {
		t.Fatalf("binary: %+v", r)
	}
	if r = call(t, readTool{}, dir, map[string]any{"file_path": "."}); !r.Failed || !strings.Contains(r.Text, "folder") {
		t.Fatalf("folder: %+v", r)
	}
	if r = call(t, readTool{}, dir, map[string]any{"file_path": "gone"}); !r.Failed {
		t.Fatalf("missing: %+v", r)
	}
}

func TestWriteCreatesFoldersKeepsModeAndLeavesNoTemp(t *testing.T) {
	dir := t.TempDir()
	r := call(t, writeTool{}, dir, map[string]any{"file_path": "deep/er/a.sh", "content": "one\ntwo\n"})
	path := filepath.Join(dir, "deep", "er", "a.sh")
	if r.Failed || fileText(t, path) != "one\ntwo\n" || r.Added == 0 || r.Removed != 0 || r.New != "one\ntwo\n" {
		t.Fatalf("create: %+v", r)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	r = call(t, writeTool{}, dir, map[string]any{"file_path": path, "content": "one\nTWO\n"})
	if r.Failed || r.Old != "one\ntwo\n" || r.Added != 1 || r.Removed != 1 {
		t.Fatalf("replace: %+v", r)
	}
	if info, _ := os.Stat(path); runtime.GOOS != "windows" && info.Mode().Perm() != 0o750 {
		t.Fatalf("mode became %v", info.Mode())
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 1 {
		t.Fatalf("left beside the file: %v, %v", entries, err)
	}
}

// A reader never sees half a file: it is the old content or the new, whole.
func TestWriteIsAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	oldText, newText := strings.Repeat("a", 1<<20), strings.Repeat("b", 1<<20)
	writeFile(t, path, oldText)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 20; i++ {
			content := oldText
			if i%2 == 0 {
				content = newText
			}
			call(t, writeTool{}, dir, map[string]any{"file_path": "a.txt", "content": content})
		}
	}()
	for running := true; running; {
		select {
		case <-done:
			running = false
		default:
			if got := fileText(t, path); got != oldText && got != newText {
				t.Fatalf("read a file of %d bytes, neither version", len(got))
			}
		}
	}
}

func TestWriteThroughLinkClipsDiffAndRefusesFolder(t *testing.T) {
	dir := t.TempDir()
	if r := call(t, writeTool{}, dir, map[string]any{"file_path": ".", "content": "x"}); !r.Failed {
		t.Fatal("wrote over a folder")
	}
	big := strings.Repeat("é", diffClip)
	r := call(t, writeTool{}, dir, map[string]any{"file_path": "big.txt", "content": big})
	if len(r.New) > diffClip+8 || !strings.HasSuffix(r.New, "…") || strings.Contains(r.Text, "é") {
		t.Fatalf("big write: new %d bytes, text %q", len(r.New), r.Text)
	}
	if d := (writeTool{}).Detail(json.RawMessage(`{"file_path":"a.txt","content":"secret\nstuff"}`)); d != "a.txt" {
		t.Fatalf("detail: %q", d)
	}
	if runtime.GOOS == "windows" {
		return
	}
	target := filepath.Join(dir, "real.txt")
	writeFile(t, target, "before")
	if err := os.Symlink(target, filepath.Join(dir, "link.txt")); err != nil {
		t.Fatal(err)
	}
	if r := call(t, writeTool{}, dir, map[string]any{"file_path": "link.txt", "content": "after"}); r.Failed {
		t.Fatal(r.Text)
	}
	if info, _ := os.Lstat(filepath.Join(dir, "link.txt")); info.Mode()&os.ModeSymlink == 0 || fileText(t, target) != "after" {
		t.Fatalf("the link was replaced, or its file not written: %v", info.Mode())
	}
}

func TestEditReplaces(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.go")
	writeFile(t, path, "alpha\nbeta\ngamma\n")
	r := call(t, editTool{}, dir, map[string]any{"file_path": "a.go", "old_string": "beta\n", "new_string": "BETA\nmore\n"})
	if r.Failed || fileText(t, path) != "alpha\nBETA\nmore\ngamma\n" || r.Old != "beta\n" || r.New != "BETA\nmore\n" || r.Added != 2 || r.Removed != 1 {
		t.Fatalf("edit: %+v, file %q", r, fileText(t, path))
	}
}

func TestEditNotUnique(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	writeFile(t, path, "x\ny\nx\nz\nx\n")
	r := call(t, editTool{}, dir, map[string]any{"file_path": "a.txt", "old_string": "x", "new_string": "w"})
	if !r.Failed || !strings.Contains(r.Text, "3 times") || !strings.Contains(r.Text, "1, 3, 5") || fileText(t, path) != "x\ny\nx\nz\nx\n" {
		t.Fatalf("not unique: %+v", r)
	}
	r = call(t, editTool{}, dir, map[string]any{"file_path": "a.txt", "old_string": "x", "new_string": "w", "replace_all": true})
	if r.Failed || fileText(t, path) != "w\ny\nw\nz\nw\n" || !strings.Contains(r.Text, "3") {
		t.Fatalf("replace_all: %+v", r)
	}
}

func TestEditKeepsLineEndings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	writeFile(t, path, "one\r\ntwo\r\nthree\r\n")
	r := call(t, editTool{}, dir, map[string]any{"file_path": "a.txt", "old_string": "one\ntwo", "new_string": "ONE\nTWO\nmore"})
	if want := "ONE\r\nTWO\r\nmore\r\nthree\r\n"; r.Failed || fileText(t, path) != want {
		t.Fatalf("multi-line in CRLF: %+v, file %q", r, fileText(t, path))
	}
	// One line replaced by two: the old text has no ending to go by.
	r = call(t, editTool{}, dir, map[string]any{"file_path": "a.txt", "old_string": "three", "new_string": "3\n4"})
	if want := "ONE\r\nTWO\r\nmore\r\n3\r\n4\r\n"; r.Failed || fileText(t, path) != want {
		t.Fatalf("one line in CRLF: %+v, file %q", r, fileText(t, path))
	}
	// A model that copied \r\n into a file of \n.
	lf := filepath.Join(dir, "lf.txt")
	writeFile(t, lf, "a\nb\n")
	r = call(t, editTool{}, dir, map[string]any{"file_path": "lf.txt", "old_string": "a\r\nb", "new_string": "c\r\nd"})
	if r.Failed || fileText(t, lf) != "c\nd\n" {
		t.Fatalf("CRLF in LF: %+v, file %q", r, fileText(t, lf))
	}
}

func TestEditSaysWhatDidntMatch(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.go"), "func a() {\n\treturn 1\n}\n")
	cases := []struct{ old, says string }{
		{"nothing like it", "first line"},
		{"func a() {\n  return 1\n}", "whitespace"},
		{"func a() {\n\treturn 2\n}", `"return 1"`},
	}
	for _, c := range cases {
		r := call(t, editTool{}, dir, map[string]any{"file_path": "a.go", "old_string": c.old, "new_string": "x"})
		if !r.Failed || !strings.Contains(r.Text, c.says) {
			t.Errorf("old %q: failed=%v, %q", c.old, r.Failed, r.Text)
		}
	}
	for _, in := range []map[string]any{
		{"file_path": "a.go", "old_string": "", "new_string": "x"},
		{"file_path": "a.go", "old_string": "x", "new_string": "x"},
		{"file_path": "gone.go", "old_string": "x", "new_string": "y"},
		{"file_path": "a.go", "old_string": "x"},
	} {
		if r := call(t, editTool{}, dir, in); !r.Failed {
			t.Errorf("%v went through: %q", in, r.Text)
		}
	}
}

func TestEditKeepsMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no such modes")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "run.sh")
	writeFile(t, path, "a\n")
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatal(err)
	}
	call(t, editTool{}, dir, map[string]any{"file_path": "run.sh", "old_string": "a", "new_string": "b"})
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o755 {
		t.Fatalf("mode became %v", info.Mode())
	}
}

func TestCleanEnviron(t *testing.T) {
	in := []string{"PATH=/bin", "ANTHROPIC_API_KEY=fake", "ANTHROPIC_AUTH_TOKEN=fake", "CLAUDE_CODE_OAUTH_TOKEN=fake", "ANTHROPIC_FUTURE_SECRET=fake",
		"ENOWX_BIN=/opt/enowx", "ANTHROPIC_BASE_URL=http://proxy", "CLAUDE_CODE_MAX_OUTPUT_TOKENS=1", "MY_API_KEY=mine"}
	want := []string{"PATH=/bin", "ENOWX_BIN=/opt/enowx", "ANTHROPIC_BASE_URL=http://proxy", "CLAUDE_CODE_MAX_OUTPUT_TOKENS=1", "MY_API_KEY=mine"}
	got := CleanEnviron(in)
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("got %v", got)
	}
	folded := CleanEnviron([]string{"anthropic_api_key=fake"})
	if runtime.GOOS == "windows" && len(folded) != 0 {
		t.Fatal("kept a key differing only in case, on Windows")
	}
}

// Every tool's schema is what the API takes, and stays the same bytes: it's in the
// prompt's cached prefix.
func TestSchemas(t *testing.T) {
	for _, tool := range allTools() {
		a, b := tool.Schema(), tool.Schema()
		if string(a) != string(b) {
			t.Errorf("%s: schema changes", tool.Name())
		}
		var s struct {
			Type       string                     `json:"type"`
			Properties map[string]json.RawMessage `json:"properties"`
			Required   []string                   `json:"required"`
			Extra      *bool                      `json:"additionalProperties"`
		}
		if err := json.Unmarshal(a, &s); err != nil {
			t.Errorf("%s: %v", tool.Name(), err)
			continue
		}
		if s.Type != "object" || s.Extra == nil || *s.Extra || len(s.Required) == 0 {
			t.Errorf("%s: schema %s", tool.Name(), a)
		}
		for _, name := range s.Required {
			if _, ok := s.Properties[name]; !ok {
				t.Errorf("%s: %s required but not a property", tool.Name(), name)
			}
		}
		if tool.Description() == "" {
			t.Errorf("%s: no description", tool.Name())
		}
	}
}

func allTools() []Tool { return Builtin() }

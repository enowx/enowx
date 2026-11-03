package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// Each agent with a file form gets the same text from its file, by its flag, and the
// rest of its arguments stay where they were.
func TestMoveToFiles(t *testing.T) {
	long := strings.Repeat("the guide, with \"quotes\" and $vars\n", 500)
	mcp := `{"mcpServers":{"enowx":{"command":"/bin/enowx","args":["mcp"]}}}`
	settings := `{"hooks":{"Stop":[]}}`
	cases := []struct {
		id   string
		in   []string
		want []string // file holding what, in order of the flag naming it
		text []string
	}{
		{"claude", []string{"--settings", settings, "--mcp-config", mcp, "--append-system-prompt", long, "--x"},
			[]string{"--settings", "@settings.json", "--mcp-config", "@mcp.json", "--append-system-prompt-file", "@system-prompt.md", "--x"}, []string{settings, mcp, long}},
		{"openclaude", []string{"--append-system-prompt", long}, []string{"--append-system-prompt-file", "@system-prompt.md"}, []string{long}},
		{"droid", []string{"--append-system-prompt", long}, []string{"--append-system-prompt-file", "@system-prompt.md"}, []string{long}},
		{"autohand", []string{"--append-sys-prompt", long}, []string{"--append-sys-prompt", "@system-prompt.md"}, []string{long}},
		{"continue", []string{"--rule", long}, []string{"--rule", "@system-prompt.md"}, []string{long}},
		{"kimi", []string{"--mcp-config", mcp}, []string{"--mcp-config-file", "@mcp.json"}, []string{mcp}},
	}
	for _, c := range cases {
		dir := t.TempDir()
		got, err := MoveToFiles(c.id, dir, c.in)
		if err != nil {
			t.Fatal(err)
		}
		var want, text []string
		text = c.text
		n := 0
		for _, w := range c.want {
			if name, ok := strings.CutPrefix(w, "@"); ok {
				w = filepath.Join(dir, name)
				data, err := os.ReadFile(w)
				if err != nil || string(data) != text[n] {
					t.Errorf("%s: %s holds %d bytes, %v", c.id, name, len(data), err)
				}
				n++
			}
			want = append(want, w)
		}
		if !slices.Equal(got, want) {
			t.Errorf("%s: %q", c.id, got)
		}
	}

	// Copilot's is one argument, and a file's path starts with @.
	dir := t.TempDir()
	got, err := MoveToFiles("copilot", dir, []string{"--additional-mcp-config=" + mcp})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "mcp.json")
	if data, _ := os.ReadFile(path); string(data) != mcp || len(got) != 1 || got[0] != "--additional-mcp-config=@"+path {
		t.Errorf("copilot: %q", got)
	}

	// What is a path already, and agents with no file form, stay as they are.
	in := []string{"--settings", "/some/file.json", "--mcp-config", "/other.json"}
	if got, _ := MoveToFiles("claude", t.TempDir(), in); !slices.Equal(got, in) {
		t.Errorf("paths: %q", got)
	}
	in = []string{"--append-system-prompt", long}
	for _, id := range []string{"qoder", "qwen-code", "codebuddy", "omp", "prime-agent"} {
		if got, _ := MoveToFiles(id, t.TempDir(), in); !slices.Equal(got, in) {
			t.Errorf("%s moved its guide", id)
		}
	}
	// No folder: inline, as ever.
	if got, _ := MoveToFiles("claude", "", in); !slices.Equal(got, in) {
		t.Error("no folder, yet moved")
	}
}

// A session's files are the user's alone, and a new start replaces them.
func TestSessionFilesArePrivateAndFresh(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no mode bits: the files take the data folder's access")
	}
	dir := filepath.Join(t.TempDir(), "s1")
	for _, text := range []string{"a long first guide", "second"} {
		if _, err := MoveToFiles("claude", dir, []string{"--append-system-prompt", text}); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, "system-prompt.md")
		data, err := os.ReadFile(path)
		if err != nil || string(data) != text {
			t.Fatalf("%q, %v", data, err)
		}
		fi, _ := os.Stat(path)
		di, _ := os.Stat(dir)
		if fi.Mode().Perm() != 0o600 || di.Mode().Perm() != 0o700 {
			t.Errorf("file %v, folder %v", fi.Mode().Perm(), di.Mode().Perm())
		}
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("left in the folder: %v", entries)
	}
}

// Agents with no file form keep their whole guide in a file named by a short line,
// the rest of their arguments as they were; others are left alone.
func TestShortenGuide(t *testing.T) {
	long := strings.Repeat("the guide \"quoted\"\n", 2000)
	for id, flag := range inlineGuides {
		dir := t.TempDir()
		got, did, err := ShortenGuide(id, dir, []string{"--before", flag, long, "--after"})
		if err != nil || !did {
			t.Fatalf("%s: %v %v", id, did, err)
		}
		path := filepath.Join(dir, "system-prompt.md")
		if data, _ := os.ReadFile(path); string(data) != long {
			t.Errorf("%s: the file holds %d bytes", id, len(data))
		}
		if !strings.Contains(got[2], path) || len(got[2]) > 300 || got[0] != "--before" || got[3] != "--after" {
			t.Errorf("%s: %q", id, got)
		}
	}
	dir := t.TempDir()
	got, did, err := ShortenGuide("codex", dir, []string{"-c", "x=1", "-c", codexGuide + marshal(long)})
	if err != nil || !did {
		t.Fatalf("codex: %v %v", did, err)
	}
	var text string
	if err := json.Unmarshal([]byte(strings.TrimPrefix(got[3], codexGuide)), &text); err != nil || !strings.Contains(text, filepath.Join(dir, "system-prompt.md")) {
		t.Errorf("codex: %q, %v", got[3], err)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "system-prompt.md")); string(data) != long {
		t.Error("codex: the file isn't the guide")
	}
	if _, did, _ := ShortenGuide("claude", t.TempDir(), []string{"--append-system-prompt", long}); did {
		t.Error("Claude Code has a file form")
	}
}

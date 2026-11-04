package goa

import (
	"os"
	"path/filepath"
	"testing"
)

// GOA edits in its own folder alone, whichever way the path leads, and not its agent's
// settings there, which would give it back what it lacks at its next start.
func TestEditInside(t *testing.T) {
	root := t.TempDir()
	dir, project := filepath.Join(root, "goa"), filepath.Join(root, "project")
	for _, d := range []string{dir, project} {
		if err := os.Mkdir(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(project, filepath.Join(dir, "link")); err != nil {
		t.Skip(err)
	}
	for _, c := range []struct {
		tool  string
		paths []string
		want  bool
	}{
		{"Write", []string{"plan.md"}, true},
		{"Edit", []string{filepath.Join(dir, "notes", "a.md")}, true},
		{"MultiEdit", []string{"a.md", filepath.Join(project, "b")}, false},
		{"Write", []string{filepath.Join(project, "main.go")}, false},
		{"Write", []string{"../project/main.go"}, false},
		{"Write", []string{"link/main.go"}, false},
		{"Write", []string{".claude/settings.json"}, false},
		{"Write", []string{".codex/config.toml"}, false},
		{"Write", []string{".opencode/plugins/x.js"}, false},
		{"Write", []string{"opencode.json"}, false},
		{"Write", []string{"."}, false},
		{"Write", nil, false},
		{"apply_patch", nil, true}, // Codex's sandbox holds it
		{"apply_patch", []string{filepath.Join(project, "x")}, false},
		{"Read", []string{filepath.Join(project, "x")}, true},
	} {
		if ok, why := EditInside(c.tool, c.paths, dir); ok != c.want {
			t.Errorf("%s %v: %v (%s)", c.tool, c.paths, ok, why)
		}
	}
}

// Only agents run for real as GOA are offered.
func TestOnlyProvenAgentsAreOffered(t *testing.T) {
	for _, id := range []string{"claude", "claude-agent-teams", "codex", "opencode", "opencode2"} {
		if why := NotProven(id); why != "" {
			t.Errorf("%s: %s", id, why)
		}
	}
	for _, id := range []string{"qwen-code", "goose", "kilo", "gemini", "openclaude", "aider"} {
		if NotProven(id) == "" {
			t.Errorf("%s offered", id)
		}
	}
}

package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// libraryHome is a home holding what the library shows.
func libraryHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	write := func(rel, text string) {
		t.Helper()
		p := filepath.Join(home, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(".claude/skills/concise-context/SKILL.md", "# concise-context\n\nAnswer first.\n")
	write(".claude/skills/handoff/SKILL.md", "# handoff\n\nWrite a handoff.\n")
	write(".claude/commands/summary.md", "Summarize the branch.\n")
	write(".claude.json", `{"mcpServers":{"coolify":{"command":"npx","args":["-y","coolify-mcp"],"env":{"COOLIFY_TOKEN":"made-up"}}}}`)
	write(".claude/settings.json", `{"permissions":{"allow":["Bash(go test:*)"],"ask":["Bash(git push:*)"],"deny":["Bash(rm -rf:*)"]},`+
		`"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"./check-command.sh","timeout":30}]}]},`+
		`"enabledPlugins":{"typesafe@typesafe-ai":true}}`)
	write("plugs/typesafe/.claude-plugin/plugin.json", `{"description":"Typed judgments."}`)
	write(".claude/plugins/installed_plugins.json", `{"plugins":{"typesafe@typesafe-ai":[{"scope":"user","installPath":"`+home+`/plugs/typesafe","version":"0.5.7"}]}}`)
	write(".claude/plugins/known_marketplaces.json", `{"typesafe-ai":{"source":{"source":"github","repo":"typesafe-ai/plugins"}}}`)
	return home
}

// libraryWalk goes through the library as the user would, each state checked
// to stay inside its frames, its picture kept with SHOTS set.
type libraryWalk struct {
	h    *notesHarness
	t    *testing.T
	size string
	n    int
}

func (w *libraryWalk) shows(state string, texts ...string) {
	w.t.Helper()
	w.h.settle(state, func() bool {
		for _, s := range texts {
			if !w.h.tt.HasText(s) {
				return false
			}
		}
		return true
	})
	for range 6 {
		w.h.tt.Advance(100 * time.Millisecond)
		w.h.tt.Frame()
	}
	w.n++
	if os.Getenv("SHOTS") != "" {
		shot(w.t, w.h.tt, filepath.Join("library", fmt.Sprintf("%s-%02d-%s.png", w.size, w.n, state)))
	}
	if out := outOfParents(w.h.tt); len(out) > 0 {
		w.t.Errorf("%s, %s: %s", w.size, state, strings.Join(out, "\n"))
	}
}

func (w *libraryWalk) click(label string) {
	w.t.Helper()
	w.h.settle("the button "+label, func() bool { return w.h.tt.HasText(label) })
	if err := w.h.tt.Click(label); err != nil {
		w.t.Fatalf("click %s: %v; shows %q", label, err, w.h.tt.Texts())
	}
	w.h.tt.Frame()
}

// settled waits for what's on disk to be so.
func (w *libraryWalk) settled(what string, done func() bool) {
	w.t.Helper()
	w.h.settle(what, done)
}

func fileHas(path, text string) bool {
	data, err := os.ReadFile(path)
	return err == nil && strings.Contains(string(data), text)
}

func TestLibraryWalk(t *testing.T) {
	for _, size := range [][2]int{{1200, 720}, {1920, 1080}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			home := libraryHome(t)
			t.Setenv("HOME", home)
			h := newNotesHarness(t)
			h.app.page, h.app.settingsSection = pageSettings, sectionLibrary
			h.tt = ui.NewTester(func(c *ui.Context) { h.app.view(c) }, size[0], size[1])
			w := &libraryWalk{h: h, t: t, size: fmt.Sprintf("%dx%d", size[0], size[1])}

			// The first skill shows at once, its text on the code ground.
			w.shows("skills", "Claude Code · 2", "Answer first.", "Under the home folder of this Mac", "Edit", "Open in editor")

			// A filter leaves the rest out, and a filter matching nothing says so.
			w.click("handoff")
			w.shows("second-skill", "Write a handoff.")

			// Editing writes the file and says so.
			w.click("Edit")
			w.shows("editor", "Cancel", "Save")
			h.tt.Type(" More.")
			w.click("Save")
			w.settled("the file saved", func() bool { return fileHas(filepath.Join(home, ".claude/skills/handoff/SKILL.md"), "More.") })
			w.shows("saved", "Saved handoff")

			// The copy menu goes to the agents that can keep it.
			w.click("Copy to another agent")
			w.shows("copy-menu", "To OpenCode")
			if err := h.tt.Click("To OpenCode"); err != nil {
				t.Fatal(err)
			}
			w.shows("copied", "Copied handoff to OpenCode", "OpenCode · 1")

			// Deleting asks first; Cancel keeps it.
			w.click("Claude Code · 2")
			w.shows("folded", "Claude Code · 2")
			w.click("Claude Code · 2")
			w.click("handoff")
			w.click("Delete handoff")
			w.shows("delete-confirm", "Delete handoff?", "A deleted file comes back only from a backup.")
			w.click("Cancel")
			if _, err := os.Stat(filepath.Join(home, ".claude/skills/handoff/SKILL.md")); err != nil {
				t.Fatalf("Cancel deleted it: %v", err)
			}
			w.click("Delete handoff")
			h.settle("the confirm", func() bool { return h.tt.HasText("Delete handoff?") })
			w.click("Delete")
			w.settled("the skill gone", func() bool {
				_, err := os.Stat(filepath.Join(home, ".claude/skills/handoff/SKILL.md"))
				return os.IsNotExist(err)
			})
			w.shows("deleted", "Deleted handoff", "Claude Code · 1")

			// An MCP server: what it runs, its secrets, the test.
			w.click("MCP servers")
			w.shows("mcp", "Holds env COOLIFY_TOKEN. Sync sends them only when asked.", "Test on this Mac", "Runs it where its agent would and lists its tools.", "Command")

			// Claude Code's settings.json: its lists, edited and saved.
			w.click("Agent settings")
			w.shows("permissions", "Permissions", "Starts in", "Bash(go test:*)", "run without asking")
			w.click("Accept edits")
			w.shows("dirty", "Discard")
			w.click("Save")
			w.settled("the mode saved", func() bool { return fileHas(filepath.Join(home, ".claude/settings.json"), "acceptEdits") })
			w.shows("saved-settings", "Saved settings.json")
			// The rail's Hooks comes first by that name: this is the editor's tab, between two others.
			h.settle("the editor read again", func() bool { return h.tt.HasText("Permissions") })
			perm, ok1 := h.tt.Find("Permissions")
			file, ok2 := h.tt.Find("File")
			if !ok1 || !ok2 {
				t.Fatalf("the editor's tabs: %q", h.tt.Texts())
			}
			h.tt.ClickAt((perm.X+perm.W+file.X)/2, perm.Y+perm.H/2)
			w.shows("hooks", "PreToolUse · Bash · 30 s", "./check-command.sh", "Add a hook", "Add the hook")
			w.click("File")
			w.shows("file", "Edit", "acceptEdits")

			// Plugins, as cards, their marketplaces a tab away.
			w.click("Plugins")
			w.shows("plugins", "typesafe", "Typed judgments.", "Plugins (1)", "Marketplaces (1)")
			// Its menu, in the library alone: the first More of the whole window is the top bar's.
			whole := h.tt
			h.tt = ui.NewTester(func(c *ui.Context) { h.app.settingsPage(c) }, size[0], size[1])
			h.settle("the plugins alone", func() bool { return h.tt.HasText("typesafe") })
			if err := h.tt.Click("More"); err != nil {
				t.Fatalf("%v: %q", err, h.tt.Texts())
			}
			w.shows("plugin-menu", "Update", "Uninstall")
			h.tt.Press(1, 1)
			h.tt.Release(1, 1)
			h.tt.Frame()
			h.tt = whole
			w.settled("the menu closed", func() bool { return !h.tt.HasText("Uninstall") })
			w.click("Marketplaces (1)")
			w.shows("marketplaces", "typesafe-ai", "typesafe-ai/plugins")

			// The dialogs, whichever kind shows.
			w.click("Add an MCP server")
			w.shows("add-mcp", "Add an MCP server on this Mac", "Runs a command", "Command")
			w.click("Serves at a URL")
			w.shows("add-mcp-url", "URL")
			w.click("Cancel")
			w.click("Add a skill")
			w.shows("add-skill", "Add a skill on this Mac", "The folder is picked next, in the system dialog.")
			w.click("A git repository")
			w.shows("add-skill-repo", "Repository", "Look for skills")
			w.click("Cancel")
		})
	}
}

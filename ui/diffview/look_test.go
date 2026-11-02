package diffview

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
)

// lookShot keeps what tt shows as name under $SHOTS, to compare with the design.
func lookShot(t *testing.T, tt *ui.Tester, name string) {
	t.Helper()
	dir := os.Getenv("SHOTS")
	if dir == "" {
		return
	}
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	_ = png.Encode(f, tt.Image())
}

// lookView draws the list in a panel's width beside a file's changes.
func lookView(v *View, o Options, path string) *ui.Tester {
	return ui.NewTester(func(c *ui.Context) {
		c.SetTheme(theme.Window())
		ui.Row(c).Width(1100).Height(640).Background(theme.Frame).TextColor(theme.Text).Children(func() {
			ui.Column(c).Width(700).FillHeight().Children(func() {
				if path != "" {
					v.DrawFile(c, o, path)
				}
			})
			ui.Column(c).Width(320).FillHeight().Children(func() { v.Draw(c, o) })
		})
	}, 1100, 640)
}

// The Git tool's states and dialogs, each saying what it must.
func TestGitToolStates(t *testing.T) {
	o := Options{Notify: func(string) {}, OpenFile: func(string) {}, OpenDiff: func(string) {}, Send: func(string) {}}
	wants := func(tt *ui.Tester, name string, want ...string) {
		t.Helper()
		tt.Frame()
		tt.Frame()
		for _, w := range want {
			if !tt.HasText(w) {
				t.Errorf("%s: no %q in %q", name, w, tt.Texts())
			}
		}
		lookShot(t, tt, "git-"+name)
	}
	v := loaded()
	tt := lookView(v, o, "a.go")
	wants(tt, "list", "2 files changed", "main", "Commit…", "Unified", "Side by side", "Open the file")

	if err := tt.Click("Compare with main"); err != nil {
		t.Fatal(err)
	}
	wants(tt, "compare", "Since the last commit", "Against main")
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if err := tt.Click("More"); err != nil {
		t.Fatal(err)
	}
	wants(tt, "more", "1 ahead, 0 behind main · not pushed", "2 uncommitted", "Refresh", "Discard the worktree…")
	tt.Key(0, ui.KeyEscape)
	tt.Frame()

	if err := tt.Click("Commit…"); err != nil {
		t.Fatal(err)
	}
	wants(tt, "commit", "Commit changes", "Commit message")
	v.commit = nil
	v.pr = &prForm{open: true}
	wants(tt, "pr", "Open a pull request", "Open pull request")
	v.pr = nil
	v.merging = true
	wants(tt, "merge", "Merge feat/x into main?", "Merges the 1 commit of feat/x into main in the project folder. A merge that conflicts is undone.")
	v.merging = false
	v.discard = &discardForm{open: true, deleteBranch: true}
	wants(tt, "discard", "Discard the worktree of feat/x?", "Also delete the branch feat/x")
	v.discard = nil

	v.busy, v.err = "Committing", "the remote hung up: a failure long enough to have to wrap inside the panel"
	wants(tt, "busy", "Committing…")
	v.busy, v.err = "", ""

	if err := tt.Click("Side by side"); err != nil {
		t.Fatal(err)
	}
	wants(tt, "split", "var x = 1", "var x = 2")
	if err := tt.Click("Unified"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("var x = 2"); err != nil {
		t.Fatal(err)
	}
	wants(tt, "draft", "Add comment", "Cancel")
	tt.Type("Use a constant")
	tt.Frame()
	if err := tt.Click("Add comment"); err != nil {
		t.Fatal(err)
	}
	wants(tt, "comment", "Use a constant", "1 review comment", "Send to agent", "Discard")

	conflicts := o
	conflicts.Conflicts = []ConflictNote{{File: "a.go", Session: "s2", With: "search", Lines: true}, {File: "docs/a-rather-long-path/that-goes-on-and-on/and-on.md", Session: "s3", With: "docs"}}
	wants(lookView(v, conflicts, "logo.png"), "conflicts-binary", "2 files also changed in another session", "same lines", "Binary file")

	empty := New()
	empty.loaded = true
	empty.status = proto.ReviewStatus{Branch: "main"}
	wants(lookView(empty, o, ""), "empty", "Nothing changed since the last commit.")
	wants(lookView(New(), o, ""), "reading", "Reading the changes…")
	long := New()
	long.loaded = true
	long.status = proto.ReviewStatus{Branch: "feat/x", Base: "a-base-branch-with-a-rather-long-name", Uncommitted: 1, Remote: "o", Ahead: 1, HasGH: true, Upstream: "o/x", Unpushed: 1}
	long.setDiff(proto.Diff{Files: []proto.DiffFile{{Path: "docs/session-tab-strip-and-center-viewers-behaviour-notes.md", OldPath: "docs/old-name-of-the-notes.md", Status: proto.FileRenamed, Added: 12044, Removed: 3211}}})
	wants(lookView(long, o, "docs/session-tab-strip-and-center-viewers-behaviour-notes.md"), "long", "1 file changed", "Commit…", "Push", "R")
}

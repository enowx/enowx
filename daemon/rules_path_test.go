package daemon

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/enowx/enowx/proto"
)

// plainFolder is a project folder with no remote, its name in mixed case.
func plainFolder(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "MyProject")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func rulesAt(t *testing.T, srv *Server, folder string) string {
	t.Helper()
	r, err := srv.projectRules(folder)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(r.Rules)
}

// However a project with no remote is reached, it has the one set of rules:
// the folder spelled in another case, through a link, from a subfolder and from
// a worktree; and saving through any of them changes that set.
func TestProjectRulesFollowTheFolder(t *testing.T) {
	srv := startServer(t)
	main := plainFolder(t)
	sub := filepath.Join(main, "src", "deep")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "init"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", main}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
	}
	wt := filepath.Join(filepath.Dir(main), "wt")
	if out, err := exec.Command("git", "-C", main, "worktree", "add", "-q", "-b", "side", wt).CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	link := filepath.Join(filepath.Dir(main), "link")
	if err := os.Symlink(main, link); err != nil {
		t.Skip(err)
	}
	if _, err := srv.saveProjectRules(proto.ProjectRules{Path: main, Rules: "Use tabs."}); err != nil {
		t.Fatal(err)
	}
	lower := filepath.Join(filepath.Dir(main), "myproject")
	for _, at := range []string{main, lower, link, sub, wt} {
		if got := rulesAt(t, srv, at); got != "Use tabs." {
			t.Fatalf("at %s: %q", at, got)
		}
	}
	if _, err := srv.saveProjectRules(proto.ProjectRules{Path: wt, Rules: "Use spaces."}); err != nil {
		t.Fatal(err)
	}
	for _, at := range []string{main, lower, link, sub, wt} {
		if got := rulesAt(t, srv, at); got != "Use spaces." {
			t.Fatalf("after a save from a worktree, at %s: %q", at, got)
		}
	}
	// What each saved over is the one history, wherever it's asked from.
	for _, at := range []string{main, wt, lower} {
		if kept := srv.rulesHistory(proto.RulesDraftParams{For: proto.RulesForProject, Path: at}); len(kept) != 1 || !strings.Contains(kept[0].Rules, "Use tabs.") {
			t.Fatalf("history at %s: %+v", at, kept)
		}
	}
}

// A subfolder of a folder in no repository reads the folder's rules, and saving
// from it changes those; home's rules don't reach all that's in it.
func TestProjectRulesOfAFolderReachItsSubfolders(t *testing.T) {
	srv := startServer(t)
	top := plainFolder(t)
	sub := filepath.Join(top, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.saveProjectRules(proto.ProjectRules{Path: top, Rules: "Keep it small."}); err != nil {
		t.Fatal(err)
	}
	if got := rulesAt(t, srv, sub); got != "Keep it small." {
		t.Fatalf("a subfolder: %q", got)
	}
	if _, err := srv.saveProjectRules(proto.ProjectRules{Path: sub, Rules: "Keep it smaller."}); err != nil {
		t.Fatal(err)
	}
	if got := rulesAt(t, srv, top); got != "Keep it smaller." {
		t.Fatalf("saved from a subfolder: %q", got)
	}

	home := plainFolder(t)
	t.Setenv("HOME", home)
	inHome := filepath.Join(home, "other")
	if err := os.Mkdir(inHome, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.saveProjectRules(proto.ProjectRules{Path: home, Rules: "For home."}); err != nil {
		t.Fatal(err)
	}
	if got := rulesAt(t, srv, inHome); got != "" {
		t.Fatalf("home's rules reached a folder in it: %q", got)
	}
}

// A folder's repository of its own keeps it from inheriting the rules above it.
func TestARepositoryInheritsNoFolderRules(t *testing.T) {
	srv := startServer(t)
	top := plainFolder(t)
	inner := filepath.Join(top, "repo")
	if err := os.Mkdir(inner, 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", inner, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if _, err := srv.saveProjectRules(proto.ProjectRules{Path: top, Rules: "Outside."}); err != nil {
		t.Fatal(err)
	}
	if got := rulesAt(t, srv, inner); got != "" {
		t.Fatalf("%q", got)
	}
}

// Rules an earlier enowX saved are still found: by the path as it was spelled,
// by a worktree's, by a project's listed spelling, and by the folder when the
// project has since got a remote; saving moves them, so no old copy comes back.
func TestExistingRulesFilesAreStillFound(t *testing.T) {
	srv := startServer(t)
	saved := func(key, rules string) string {
		t.Helper()
		file := srv.ownRulesFileOf(key)
		if err := writeTeamFile(file, []byte(rules+"\n")); err != nil {
			t.Fatal(err)
		}
		return file
	}

	spelled := plainFolder(t)
	lower := filepath.Join(filepath.Dir(spelled), "myproject")
	old := saved("path:"+lower, "Old spelling.")
	if got := rulesAt(t, srv, lower); got != "Old spelling." {
		t.Fatalf("the spelling it was saved by: %q", got)
	}
	if _, err := srv.saveProjectRules(proto.ProjectRules{Path: lower, Rules: "New."}); err != nil {
		t.Fatal(err)
	}
	if exists(old) {
		t.Fatal("the old copy stayed, to come back")
	}
	if got := rulesAt(t, srv, spelled); got != "New." {
		t.Fatalf("moved: %q", got)
	}
	if _, err := srv.saveProjectRules(proto.ProjectRules{Path: spelled, Clear: true}); err != nil {
		t.Fatal(err)
	}
	if got := rulesAt(t, srv, lower); got != "" {
		t.Fatalf("cleared: %q", got)
	}

	repo := gitRepo(t)
	wt := filepath.Join(t.TempDir(), "wt")
	if out, err := exec.Command("git", "-C", repo, "worktree", "add", "-q", "-b", "side", wt).CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	wtReal, err := filepath.EvalSymlinks(wt)
	if err != nil {
		t.Fatal(err)
	}
	saved("path:"+wtReal, "Saved from the worktree.")
	if got := rulesAt(t, srv, repo); got != "Saved from the worktree." {
		t.Fatalf("a worktree's, from the main folder: %q", got)
	}

	withRemote := gitRepo(t)
	saved("path:"+withRemote, "Before the remote.")
	if out, err := exec.Command("git", "-C", withRemote, "remote", "add", "origin", "https://github.com/enowx/later.git").CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if got := rulesAt(t, srv, withRemote); got != "Before the remote." {
		t.Fatalf("a project that got a remote: %q", got)
	}
}

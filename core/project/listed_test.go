package project

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// countGit puts a git first on PATH that counts its runs, and gives how many so far.
func countGit(t *testing.T) func() int {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("counts git with a shell script")
	}
	real, err := exec.LookPath("git")
	if err != nil {
		t.Skip("no git here")
	}
	bin := t.TempDir()
	log := filepath.Join(bin, "runs")
	script := fmt.Sprintf("#!/bin/sh\necho run >> %q\nexec %q \"$@\"\n", log, real)
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return func() int {
		data, err := os.ReadFile(log)
		if err != nil {
			return 0
		}
		return strings.Count(string(data), "\n")
	}
}

// inDir runs git in dir, with the identity a commit needs.
func inDir(t *testing.T, dir string, args ...string) {
	t.Helper()
	all := append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"}, args...)
	if out, err := exec.Command("git", all...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// listOf lists store s, whose only project is at index 0, n times, and says what the
// last one said of it and how many times git ran for them.
func listOf(t *testing.T, s *Store, runs func() int, n int) (isGit bool, branch string, ran int) {
	t.Helper()
	before := runs()
	for range n {
		list := s.List()
		if len(list) != 1 {
			t.Fatalf("list: %+v", list)
		}
		isGit, branch = list[0].IsGit, list[0].Branch
	}
	return isGit, branch, runs() - before
}

func openStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "projects.json"))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// A list on every tick asks git once, then again only when the answer can have changed.
func TestListAsksGitOnlyWhenItCanHaveChanged(t *testing.T) {
	runs := countGit(t)
	dir := t.TempDir()
	inDir(t, dir, "init", "-q", "-b", "main")
	inDir(t, dir, "commit", "-q", "--allow-empty", "-m", "first")
	s := openStore(t)
	if _, err := s.Add(dir); err != nil {
		t.Fatal(err)
	}

	isGit, branch, ran := listOf(t, s, runs, 10)
	if !isGit || branch != "main" {
		t.Fatalf("listed: git %v, branch %q", isGit, branch)
	}
	if ran != 2 {
		t.Fatalf("10 lists ran git %d times, want 2: its root and its branch, once", ran)
	}

	inDir(t, dir, "commit", "-q", "--allow-empty", "-m", "second")
	if _, _, ran := listOf(t, s, runs, 5); ran != 0 {
		t.Fatalf("a commit, which moves no branch's name, ran git %d times", ran)
	}

	inDir(t, dir, "checkout", "-q", "-b", "feature")
	_, branch, ran = listOf(t, s, runs, 5)
	if branch != "feature" {
		t.Fatalf("after a checkout the branch is %q", branch)
	}
	if ran != 2 {
		t.Fatalf("a checkout ran git %d times over 5 lists, want 2", ran)
	}
}

// A folder that becomes a repository shows at once, and one that stops being one.
func TestListSeesARepositoryComeAndGo(t *testing.T) {
	runs := countGit(t)
	dir := t.TempDir()
	s := openStore(t)
	if _, err := s.Add(dir); err != nil {
		t.Fatal(err)
	}
	if isGit, _, _ := listOf(t, s, runs, 2); isGit {
		t.Fatal("a plain folder listed as a repository")
	}
	if _, _, ran := listOf(t, s, runs, 3); ran != 0 {
		t.Fatalf("a plain folder ran git %d times", ran)
	}
	inDir(t, dir, "init", "-q", "-b", "main")
	if isGit, branch, _ := listOf(t, s, runs, 1); !isGit || branch != "main" {
		t.Fatalf("after git init: git %v, branch %q", isGit, branch)
	}
	if err := os.RemoveAll(filepath.Join(dir, ".git")); err != nil {
		t.Fatal(err)
	}
	if isGit, _, _ := listOf(t, s, runs, 1); isGit {
		t.Fatal("still a repository with its .git gone")
	}
}

// A worktree's HEAD lives in the repository it belongs to: a checkout there shows too.
func TestListFollowsAWorktreesBranch(t *testing.T) {
	runs := countGit(t)
	repo := t.TempDir()
	inDir(t, repo, "init", "-q", "-b", "main")
	inDir(t, repo, "commit", "-q", "--allow-empty", "-m", "first")
	tree := filepath.Join(t.TempDir(), "tree")
	inDir(t, repo, "worktree", "add", "-q", "-b", "side", tree)
	s := openStore(t)
	if _, err := s.Add(tree); err != nil {
		t.Fatal(err)
	}
	if _, branch, ran := listOf(t, s, runs, 4); branch != "side" || ran != 2 {
		t.Fatalf("branch %q, git ran %d times", branch, ran)
	}
	inDir(t, tree, "checkout", "-q", "-b", "side-two")
	if _, branch, _ := listOf(t, s, runs, 1); branch != "side-two" {
		t.Fatalf("after a checkout in the worktree the branch is %q", branch)
	}
}

// What no stat shows still comes in, after a while.
func TestListAsksAgainAfterAWhile(t *testing.T) {
	runs := countGit(t)
	was := listRefresh
	listRefresh = 0
	t.Cleanup(func() { listRefresh = was })
	dir := t.TempDir()
	inDir(t, dir, "init", "-q", "-b", "main")
	s := openStore(t)
	if _, err := s.Add(dir); err != nil {
		t.Fatal(err)
	}
	if _, _, ran := listOf(t, s, runs, 3); ran != 6 {
		t.Fatalf("with nothing to trust, 3 lists ran git %d times, want 6", ran)
	}
	listRefresh = time.Hour
	s.List()
	if _, _, ran := listOf(t, s, runs, 3); ran != 0 {
		t.Fatalf("within the time, 3 lists ran git %d times", ran)
	}
}

// Paths asks git nothing, and follows the list.
func TestPathsRunNoGit(t *testing.T) {
	runs := countGit(t)
	dir := t.TempDir()
	inDir(t, dir, "init", "-q", "-b", "main")
	s := openStore(t)
	info, err := s.Add(dir)
	if err != nil {
		t.Fatal(err)
	}
	before := runs()
	for range 10 {
		if got := s.Paths(); len(got) != 1 || got[0] != info.Path {
			t.Fatalf("paths: %v, want %s", got, info.Path)
		}
	}
	if ran := runs() - before; ran != 0 {
		t.Fatalf("Paths ran git %d times", ran)
	}
	if err := s.Remove(info.Path); err != nil {
		t.Fatal(err)
	}
	if got := s.Paths(); len(got) != 0 {
		t.Fatalf("paths after removing it: %v", got)
	}
}

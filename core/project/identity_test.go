package project

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func repoAt(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "init"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

func realTemp(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

// A folder reached in another case (what macOS allows) or through a link is
// the same folder; one really named in that case stays its own.
func TestCanonicalSpelling(t *testing.T) {
	root := realTemp(t)
	real := filepath.Join(root, "MyProject")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := Canonical(filepath.Join(root, "myproject")); got != real {
		t.Fatalf("lowercase: %s, not %s", got, real)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if got := Canonical(link); got != real {
		t.Fatalf("through a link: %s", got)
	}
	other := filepath.Join(root, "myproject")
	if err := os.Mkdir(other, 0o755); err != nil {
		if os.IsExist(err) {
			t.Skip("this filesystem ignores case")
		}
		t.Fatal(err)
	}
	if got := Canonical(other); got != other {
		t.Fatalf("a folder of its own: %s", got)
	}
	if got := Canonical(filepath.Join(root, "gone", "Deep")); got != filepath.Join(root, "gone", "Deep") {
		t.Fatalf("a folder not there: %s", got)
	}
}

// A repository's subfolders and worktrees are its project.
func TestIdentityOfARepository(t *testing.T) {
	root := realTemp(t)
	main := filepath.Join(root, "Main")
	sub := filepath.Join(main, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	repoAt(t, main)
	wt := filepath.Join(root, "wt")
	if out, err := exec.Command("git", "-C", main, "worktree", "add", "-q", "-b", "side", wt).CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for _, dir := range []string{main, sub, wt, filepath.Join(root, "main", "A")} {
		if got, in := Identity(dir); got != main || !in {
			t.Fatalf("%s: %s, %v", dir, got, in)
		}
	}
	list := Worktrees(wt)
	if len(list) != 2 || list[0] != main || list[1] != wt {
		t.Fatalf("worktrees: %v", list)
	}

	plain := filepath.Join(root, "plain")
	if err := os.Mkdir(plain, 0o755); err != nil {
		t.Fatal(err)
	}
	if got, in := Identity(plain); got != plain || in {
		t.Fatalf("outside a repository: %s, %v", got, in)
	}
}

// A bare repository's worktrees share the repository, having no main folder.
func TestIdentityOfABareRepository(t *testing.T) {
	root := realTemp(t)
	seed := filepath.Join(root, "seed")
	if err := os.Mkdir(seed, 0o755); err != nil {
		t.Fatal(err)
	}
	repoAt(t, seed)
	bare := filepath.Join(root, "repo.git")
	if out, err := exec.Command("git", "clone", "-q", "--bare", seed, bare).CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	wt := filepath.Join(root, "wt")
	if out, err := exec.Command("git", "-C", bare, "worktree", "add", "-q", "-b", "side", wt).CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if got, in := Identity(wt); got != bare || !in {
		t.Fatalf("%s, %v", got, in)
	}
}

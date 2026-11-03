package files

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/enowx/enowx/proto"
)

// A save replaces the file whole, with its mode; a file changed meanwhile isn't written over
// unless asked; a read-only one never is.
func TestWrite(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a.sh")
	if err := os.WriteFile(path, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	first, err := ReadWhole(root, "a.sh", false, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	done, err := Write(root, proto.FileWrite{Path: "a.sh", Text: "new", Unless: first.Modified})
	if err != nil || done.Changed {
		t.Fatalf("%+v %v", done, err)
	}
	if b, _ := os.ReadFile(path); string(b) != "new" {
		t.Fatalf("wrote %q", b)
	}
	if st, _ := os.Stat(path); runtime.GOOS != "windows" && st.Mode().Perm() != 0o755 {
		t.Fatalf("mode %v", st.Mode())
	}
	stale, err := Write(root, proto.FileWrite{Path: "a.sh", Text: "mine", Unless: first.Modified.Add(-time.Hour)})
	if err != nil || !stale.Changed {
		t.Fatalf("over a change: %+v %v", stale, err)
	}
	if b, _ := os.ReadFile(path); string(b) != "new" {
		t.Fatalf("wrote over a change: %q", b)
	}
	if _, err := Write(root, proto.FileWrite{Path: "a.sh", Text: "mine", Unless: first.Modified.Add(-time.Hour), Force: true}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != "mine" {
		t.Fatalf("forced: %q", b)
	}
	if err := os.Chmod(path, 0o444); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if _, err := Write(root, proto.FileWrite{Path: "a.sh", Text: "x", Force: true}); err == nil {
			t.Fatal("wrote a read-only file")
		}
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 1 {
		t.Fatalf("left behind: %v", entries)
	}
}

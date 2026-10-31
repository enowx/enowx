package hostkeys

import (
	"os"
	"path/filepath"
	"testing"
)

// A key is kept readable by the host's user alone, read back after a
// restart, and removed by an empty value.
func TestKeys(t *testing.T) {
	file := filepath.Join(t.TempDir(), "keys.json")
	s := Open(file)
	if err := s.Set("judge", "sk-1"); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(file); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v %v", fi.Mode(), err)
	}
	if again := Open(file); again.Get("judge") != "sk-1" || !again.Has("judge") {
		t.Fatal("not kept")
	}
	s.Set("judge", "")
	if Open(file).Has("judge") {
		t.Fatal("not removed")
	}
}

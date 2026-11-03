package canvas

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/enowx/enowx/proto"
)

// The originals stay in the backup after a conversion, whole, and a backup that can't be
// made says so, for the conversion not to run.
func TestBackupBeforeConversion(t *testing.T) {
	dir := t.TempDir()
	st := Open(filepath.Join(dir, "canvas"))
	id := "0123456789ab"
	version := filepath.Join(dir, "canvas", id, "history", "1.json")
	if err := writeJSON(filepath.Join(dir, "canvas", id, "canvas.json"), proto.Canvas{ID: id}); err != nil {
		t.Fatal(err)
	}
	whole := proto.Canvas{ID: id, Frames: []proto.CanvasFrame{{ID: "f", HTML: "<p>one</p>"}}}
	if err := writeJSON(version, whole); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(version)
	if err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "backup", id)
	todo, err := st.BackupHistory(id, dest)
	if err != nil || !todo {
		t.Fatalf("backup: %v %v", todo, err)
	}
	if _, err := st.ConvertHistory(id); err != nil {
		t.Fatal(err)
	}
	now, _ := os.ReadFile(version)
	if string(now) == string(original) {
		t.Fatal("not converted")
	}
	kept, err := os.ReadFile(filepath.Join(dest, "history", "1.json"))
	if err != nil || string(kept) != string(original) {
		t.Fatalf("the backup changed with it: %v", err)
	}
	if todo, err := st.BackupHistory(id, dest); err != nil || todo {
		t.Fatalf("converted, it has nothing to back up: %v %v", todo, err)
	}

	other := "ba9876543210"
	if err := writeJSON(filepath.Join(dir, "canvas", other, "canvas.json"), proto.Canvas{ID: other}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(dir, "canvas", other, "history", "1.json"), whole); err != nil {
		t.Fatal(err)
	}
	blocked := filepath.Join(dir, "a-file")
	if err := os.WriteFile(blocked, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if todo, err := st.BackupHistory(other, filepath.Join(blocked, other)); err == nil || todo {
		t.Fatalf("a backup that can't be made: %v %v", todo, err)
	}
}

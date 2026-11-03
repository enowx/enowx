package ui

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A picture is there after a restart under its key alone, and the folder stays
// bounded, the longest unused going.
func TestCanvasThumbsKeep(t *testing.T) {
	dir := t.TempDir()
	if readCanvasThumb(dir, "c:f1:a") != nil {
		t.Fatal("a picture from nowhere")
	}
	keepCanvasThumb(dir, "c:f1:a", []byte("one"))
	if got := readCanvasThumb(dir, "c:f1:a"); !bytes.Equal(got, []byte("one")) {
		t.Fatalf("%q", got)
	}
	if readCanvasThumb(dir, "c:f1:b") != nil {
		t.Fatal("the design changed, and its old picture came back")
	}
	keepCanvasThumb(dir, "old", []byte("x"))
	old := canvasThumbFile(dir, "old")
	then := time.Now().Add(-time.Hour)
	if err := os.Chtimes(old, then, then); err != nil {
		t.Fatal(err)
	}
	pruneCanvasThumbs(dir, 1)
	if _, err := os.Stat(old); err == nil {
		t.Fatal("the longest unused stayed")
	}
	left, _ := filepath.Glob(filepath.Join(dir, "*.png"))
	if len(left) != 1 {
		t.Fatalf("left %v", left)
	}
}

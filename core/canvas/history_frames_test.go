package canvas

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/enowx/enowx/proto"
)

// A version comes back whole, a frame unchanged across versions is kept once, and an
// older whole version converts to the same thing.
func TestVersionsShareFrames(t *testing.T) {
	s := Open(t.TempDir())
	c := proto.Canvas{Frames: []proto.CanvasFrame{{ID: "a", HTML: "<p>one</p>"}, {ID: "b", HTML: "<p>two</p>"}}}
	c.ID = "0123456789ab"
	for n, html := range map[int]string{1: "<p>one</p>", 2: "<p>changed</p>"} {
		c.Frames[0].HTML = html
		if err := s.writeVersion(c, n); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.readVersion(c.ID, 1)
	if err != nil || len(got.Frames) != 2 || got.Frames[0].HTML != "<p>one</p>" || got.Frames[1].HTML != "<p>two</p>" {
		t.Fatalf("%+v %v", got, err)
	}
	blobs, _ := os.ReadDir(s.framesDir(c.ID))
	if len(blobs) != 3 {
		t.Fatalf("%d frames kept, want 3", len(blobs))
	}
	// As a build before this kept it.
	if err := writeJSON(s.versionFile(c.ID, 3), c); err != nil {
		t.Fatal(err)
	}
	before, _ := s.readVersion(c.ID, 3)
	if _, err := s.ConvertHistory(c.ID); err != nil {
		t.Fatal(err)
	}
	after, err := s.readVersion(c.ID, 3)
	if err != nil || len(after.Frames) != len(before.Frames) || after.Frames[0].HTML != before.Frames[0].HTML {
		t.Fatalf("%+v %v", after, err)
	}
	var v storedVersion
	if err := readJSON(s.versionFile(c.ID, 3), &v); err != nil || v.FrameRefs == nil || len(v.Canvas.Frames) != 0 {
		t.Fatalf("not converted: %+v %v", v, err)
	}
	if _, err := os.Stat(filepath.Join(s.dir, c.ID, "history", historyFormat)); err != nil {
		t.Fatal(err)
	}
	s.pruneFrames(c.ID, []proto.CanvasVersion{{N: 1}})
	if blobs, _ = os.ReadDir(s.framesDir(c.ID)); len(blobs) != 2 {
		t.Fatalf("%d frames left, want version 1's 2", len(blobs))
	}
}

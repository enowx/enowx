package canvas

import (
	"testing"

	"github.com/enowx/enowx/proto"
)

// The frames walk a design's time exactly: a loop's end isn't its first frame twice,
// what ends includes its end, and a range keeps to itself.
func TestVideoFramesAt(t *testing.T) {
	ends, err := VideoFramesAt(1000, false, 0, 0, 10)
	if err != nil || len(ends) != 11 || ends[0] != 0 || ends[10] != 1000 {
		t.Fatalf("what ends: %v %v", ends, err)
	}
	loops, err := VideoFramesAt(1000, true, 0, 0, 10)
	if err != nil || len(loops) != 10 || loops[9] != 900 {
		t.Fatalf("a loop: %v %v", loops, err)
	}
	part, err := VideoFramesAt(1000, true, 200, 400, 10)
	if err != nil || part[0] != 200 || part[len(part)-1] != 400 {
		t.Fatalf("a range: %v %v", part, err)
	}
	for _, c := range []struct {
		d, from, to float64
		fps         int
	}{{0, 0, 0, 30}, {1000, 900, 100, 30}, {1000, 0, 0, 0}, {MostVideoMs + 1, 0, 0, 30}} {
		if _, err := VideoFramesAt(c.d, false, c.from, c.to, c.fps); err == nil {
			t.Errorf("no error for %+v", c)
		}
	}
	again, _ := VideoFramesAt(1000, false, 0, 0, 10)
	for i := range ends {
		if again[i] != ends[i] {
			t.Fatal("not the same twice")
		}
	}
}

// A video's size is even, its shape kept.
func TestVideoSize(t *testing.T) {
	for _, c := range []struct{ w, h, out, wantW, wantH int }{
		{1440, 900, 0, 1440, 900}, {1441, 901, 0, 1440, 900}, {1440, 900, 2880, 2880, 1800}, {1440, 900, 1001, 1000, 626},
	} {
		if w, h := VideoSize(c.w, c.h, c.out); w != c.wantW || h != c.wantH {
			t.Errorf("%+v: %d×%d", c, w, h)
		}
	}
}

// A design that draws by its own clock is said to.
func TestVideoWarning(t *testing.T) {
	if VideoWarning(proto.CanvasFrame{HTML: `renderer.setAnimationLoop(draw)`}) == "" {
		t.Error("no warning")
	}
	if VideoWarning(proto.CanvasFrame{HTML: `enowx.motion.add(draw)`}) != "" {
		t.Error("a warning for what is on the clock")
	}
}

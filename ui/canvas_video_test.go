package ui

import (
	"bytes"
	"errors"
	"slices"
	"testing"
	"time"
)

// The encoder that gives the same file each time comes first; the systems' after it.
func TestPickEncoder(t *testing.T) {
	list := " V....D h264_videotoolbox    VideoToolbox\n V....D libx264              x264\n"
	if got := pickEncoder(list); got != "libx264" {
		t.Errorf("got %q", got)
	}
	if got := pickEncoder(" V....D h264_videotoolbox VideoToolbox\n"); got != "h264_videotoolbox" {
		t.Errorf("got %q", got)
	}
	if got := pickEncoder(" V....D mpeg4 MPEG-4\n"); got != "" {
		t.Errorf("got %q", got)
	}
}

// What plays everywhere: yuv420p, no sound, its index at the front.
func TestVideoArgs(t *testing.T) {
	args := videoArgs("libx264", 30, 1440, 900, "out.part")
	for _, want := range []string{"yuv420p", "-an", "+faststart", "scale=1440:900:flags=lanczos", "out.part"} {
		if !slices.Contains(args, want) {
			t.Errorf("no %s in %v", want, args)
		}
	}
}

func TestTimeLeft(t *testing.T) {
	if got := timeLeft(50, 100, 10*time.Second); got != "10s left" {
		t.Errorf("%q", got)
	}
	if got := timeLeft(0, 100, time.Second); got != "" {
		t.Errorf("%q", got)
	}
}

// The walk gives each frame in order, and a cancel stops it where it is.
func TestVideoWalk(t *testing.T) {
	frame := func(at float64) ([]byte, error) { return []byte{byte(at)}, nil }
	var out bytes.Buffer
	if err := videoWalk([]float64{1, 2, 3}, func() bool { return false }, frame, &out, func(int, int) {}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out.Bytes(), []byte{1, 2, 3}) {
		t.Fatalf("frames: %v", out.Bytes())
	}
	out.Reset()
	calls := 0
	err := videoWalk([]float64{1, 2, 3}, func() bool { calls++; return calls > 1 }, frame, &out, func(int, int) {})
	if !errors.Is(err, errVideoCancelled) || out.Len() != 1 {
		t.Fatalf("cancelled: %v, %d frames", err, out.Len())
	}
}

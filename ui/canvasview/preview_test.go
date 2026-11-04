package canvasview

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
)

func previewPNG(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 8, 8))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func previewFrames(n int) []proto.CanvasFrame {
	var out []proto.CanvasFrame
	for i := range n {
		out = append(out, proto.CanvasFrame{ID: fmt.Sprintf("f%d", i), Name: "Home", Width: 400, Height: 300, HTML: "<main><h1>Raw words of the page</h1></main>"})
	}
	return out
}

func previewTester(v *View, o Options, frames []proto.CanvasFrame) *ui.Tester {
	return ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Children(func() {
			for _, f := range frames {
				v.designPreview(c.Key(f.ID), o, f, 40)
			}
		})
	}, 400, 800)
}

// Until its picture comes a preview says so in its frame, never the page's words;
// a few are asked for at a time, and the rest wait their turn.
func TestPreviewLoadsQuietlyAFewAtATime(t *testing.T) {
	frames := previewFrames(6)
	v := &View{canvas: &proto.Canvas{CanvasInfo: proto.CanvasInfo{ID: "c1"}, Frames: frames}}
	var later []func([]byte, error)
	o := Options{Render: func(id, frame, format string, done func([]byte, error)) { later = append(later, done) }}
	tt := previewTester(v, o, frames)
	tt.Frame()
	tt.Frame()
	if len(later) > thumbsAtOnce {
		t.Fatalf("%d asked at once", len(later))
	}
	for _, text := range tt.Texts() {
		if strings.Contains(text, "Raw words") {
			t.Fatalf("the page's words show: %q", tt.Texts())
		}
	}
	if !slices.Contains(tt.Texts(), "Loading preview") {
		t.Fatalf("no loading state: %q", tt.Texts())
	}
}

// A picture kept shows with no render, and one made is kept.
func TestPreviewCache(t *testing.T) {
	frames := previewFrames(1)
	v := &View{canvas: &proto.Canvas{CanvasInfo: proto.CanvasInfo{ID: "c1"}, Frames: frames}}
	data := previewPNG(t)
	renders, kept := 0, 0
	o := Options{
		Render: func(id, frame, format string, done func([]byte, error)) {
			renders++
			done(data, nil)
		},
		PictureCached: func(key string, done func([]byte)) { done(data) },
		PictureKeep:   func(key string, data []byte) { kept++ },
	}
	tt := previewTester(v, o, frames)
	tt.Frame()
	tt.Frame()
	if renders != 0 || kept != 0 {
		t.Fatalf("rendered %d, kept %d, with a picture kept", renders, kept)
	}
	if v.thumbs["f0"] == nil || v.thumbs["f0"].img == nil {
		t.Fatal("what was kept doesn't show")
	}

	v = &View{canvas: v.canvas}
	o.PictureCached = func(key string, done func([]byte)) { done(nil) }
	tt = previewTester(v, o, frames)
	tt.Frame()
	tt.Frame()
	if renders != 1 || kept != 1 {
		t.Fatalf("with none kept: rendered %d, kept %d", renders, kept)
	}
}

// A render that failed says so on the card.
func TestPreviewSaysItFailed(t *testing.T) {
	frames := previewFrames(1)
	v := &View{canvas: &proto.Canvas{CanvasInfo: proto.CanvasInfo{ID: "c1"}, Frames: frames}}
	o := Options{Render: func(id, frame, format string, done func([]byte, error)) { done(nil, errors.New("no web view")) }}
	tt := previewTester(v, o, frames)
	for range 3 {
		tt.Frame()
	}
	if !slices.Contains(tt.Texts(), "Preview failed") || v.thumbFailure(frames[0]) != "no web view" {
		t.Fatalf("%q", tt.Texts())
	}
}

// Pictures in memory stay under their bound, the longest out of sight going first, and
// never one that may be on screen.
func TestTrimThumbsBoundsPicturesHeld(t *testing.T) {
	now := time.Now()
	const each = 16 << 20
	v := &View{thumbs: map[string]*thumb{}}
	bmp := ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 1, 1)))
	for i := range 20 {
		v.thumbs[fmt.Sprint(i)] = &thumb{img: bmp, key: "k", asked: "k", size: each, drawn: now.Add(-time.Duration(20-i) * time.Minute)}
	}
	v.thumbs["shown"] = &thumb{img: bmp, key: "k", size: each, drawn: now}
	v.trimThumbs(now)
	total, oldestKept := 0, 0
	for name, th := range v.thumbs {
		if th.img != nil {
			total += th.size
			if name != "shown" {
				var i int
				fmt.Sscan(name, &i)
				if oldestKept == 0 || i < oldestKept {
					oldestKept = i
				}
			}
		}
	}
	if total > thumbsKeep || v.thumbs["shown"].img == nil {
		t.Fatalf("held %d, shown kept %v", total, v.thumbs["shown"].img != nil)
	}
	if oldestKept < 17 || v.thumbs["0"].asked != "" {
		t.Fatalf("kept from %d, the oldest asked %q", oldestKept, v.thumbs["0"].asked)
	}
}

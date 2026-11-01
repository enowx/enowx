package canvas

import (
	"regexp"
	"strings"
	"testing"

	"github.com/enowx/enowx/proto"
)

// The shared page carries a library in the designs that use it alone, with
// controls under those, and no CDN address; a plain canvas's stays without.
func TestSharedPageMotion(t *testing.T) {
	c := proto.Canvas{Frames: []proto.CanvasFrame{frameOf(`<p>plain</p>`), frameOf(animeTag + threeBare), frameOf(`<p>plain too</p>`)}}
	page := Standalone(c, nil)
	if regexp.MustCompile(cdnAddress).MatchString(page) {
		t.Fatal("a CDN address is left")
	}
	if n := strings.Count(page, threeLibrary.dataURL()); n != 1 {
		t.Fatalf("three is in the page %d times", n)
	}
	if n := strings.Count(page, `class="enowx-motion"`); n != 1 {
		t.Fatalf("%d motion bars for one moving design", n)
	}
	if !strings.Contains(page, "enowx:'motion'") {
		t.Fatal("no controls' script")
	}
	if grown := len(page) - len(threeLibrary.dataURL()); grown > 80_000 {
		t.Fatalf("the page takes %d bytes besides three", grown)
	}
	plain := Standalone(proto.Canvas{Frames: []proto.CanvasFrame{frameOf(`<p>plain</p>`)}}, nil)
	if strings.Contains(plain, "enowx-motion") || strings.Contains(plain, "data:text/javascript") {
		t.Fatal("a plain canvas got motion's controls")
	}
}

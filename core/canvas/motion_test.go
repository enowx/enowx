package canvas

import (
	"strings"
	"testing"
)

// Only a design that uses a library or motion pays for them, and a plain one's
// document stays as it was.
func TestPlainDesignsStayPlain(t *testing.T) {
	for _, html := range []string{`<h1>Hello</h1><style>a{transition:color .2s}a:hover{color:red}</style>`, `<!doctype html><html><head><title>x</title></head><body>y</body></html>`} {
		f := frameOf(html)
		doc := FrameDocument(f, nil)
		if UsesMotion(html) || strings.Contains(doc, "enowx") || strings.Contains(doc, "data:text/javascript") {
			t.Errorf("%s got the runtime or a library", html)
		}
		if HoldMotion(doc) != doc {
			t.Error("holding a plain design changed it")
		}
	}
	css := FrameDocument(frameOf(`<style>@keyframes a{to{opacity:0}}</style>`), nil)
	if !strings.Contains(css, "__enowxMotionHold") || strings.Contains(css, "data:text/javascript") {
		t.Error("CSS motion takes the runtime alone")
	}
	anime := FrameDocument(frameOf(animeTag), nil)
	if !strings.Contains(anime, animeLibrary.dataURL()) || strings.Contains(anime, threeLibrary.dataURL()) {
		t.Error("anime brought three")
	}
	if !strings.Contains(HoldMotion(anime), "window.__enowxMotionHold=1") {
		t.Error("a moving design wasn't held")
	}
}

// The runtime comes before anything of the design's, and the prototype's runtime
// doesn't take window.enowx from it.
func TestMotionRuntimeOrder(t *testing.T) {
	doc := FrameDocument(frameOf(`<!doctype html><html><head><script>x()</script></head><body data-set="a=b"><style>@keyframes a{}</style></body></html>`), nil)
	state, motion, own := strings.Index(doc, "window.enowx={state"), strings.Index(doc, "E.motion = {"), strings.Index(doc, "<script>x()")
	if state < 0 || motion < state || own < motion {
		t.Fatalf("state %d, motion %d, the design's %d", state, motion, own)
	}
}

package canvas

import (
	_ "embed"
	"strings"
)

// A design's motion plays by one clock in its page (motion.js), so whoever shows
// it (Interact mode, the shared page, a render) can play, pause and seek it.

//go:embed motion.js
var motionRuntimeJS string

// motionRuntime goes in the head of a page with motion, after the prototype's
// runtime and before any script of the design's.
func motionRuntime() string { return "<script>" + motionRuntimeJS + "</script>" }

// UsesMotion says whether a design's HTML has motion for the runtime to run, so
// that a plain design's document stays as it was written. A hover's transition
// isn't: it plays when asked, and takes no clock.
func UsesMotion(html string) bool {
	if usesLibrary(html) {
		return true
	}
	for _, mark := range []string{"enowx.motion", "@keyframes", "anime.timeline", "anime({"} {
		if strings.Contains(html, mark) {
			return true
		}
	}
	return false
}

// HoldMotion has doc's motion show as it stands rather than play, as on the
// board; doc without motion stays as it is.
func HoldMotion(doc string) string {
	if !strings.Contains(doc, "__enowxMotionHold") {
		return doc
	}
	return WithHead(doc, "<script>window.__enowxMotionHold=1</script>")
}

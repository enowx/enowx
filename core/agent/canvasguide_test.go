package agent

import (
	"regexp"
	"strings"
	"testing"
)

// Every library the canvas guide loads from a CDN names an exact version,
// so a mockup made today looks the same tomorrow.
func TestCanvasGuidePinsLibraries(t *testing.T) {
	urls := regexp.MustCompile(`https://(cdn\.jsdelivr\.net|cdnjs\.cloudflare\.com|cdn\.tailwindcss\.com|unpkg\.com)[^\s"'<>)]*`).FindAllString(CanvasGuide+CanvasMotionGuide, -1)
	if len(urls) < 8 {
		t.Fatalf("only %d CDN links: %q", len(urls), urls)
	}
	pinned := regexp.MustCompile(`(@|/)\d+\.\d+\.\d+(/|$)`)
	for _, u := range urls {
		if !pinned.MatchString(u) || strings.Contains(u, "latest") {
			t.Errorf("%s names no exact version", u)
		}
	}
	for _, want := range []string{"canvas grid ID", "canvas icon lucide:rocket", "Offline", "prefers-reduced-motion"} {
		if !strings.Contains(CanvasGuide, want) {
			t.Errorf("the guide doesn't say %q", want)
		}
	}
}

// The guide teaches motion by the addresses that work offline, on the clock, and
// not by what can't be scrubbed. The bundled addresses are core/canvas's.
func TestCanvasGuideTeachesMotion(t *testing.T) {
	for _, want := range []string{"enowx.motion.add", "canvas frames ID", "canvas guide motion", "unslop", "three.module.js"} {
		if !strings.Contains(CanvasGuide, want) {
			t.Errorf("the guide doesn't say %q", want)
		}
	}
	if strings.Contains(CanvasGuide, "r.setAnimationLoop(") || strings.Contains(CanvasMotionGuide, "r.setAnimationLoop(") {
		t.Error("an example renders off the clock")
	}
	for _, want := range []string{"Timing", "Easing", "Choreography", "cheap", "prefers-reduced-motion", "canvas frames ID"} {
		if !strings.Contains(CanvasMotionGuide, want) {
			t.Errorf("the motion guide doesn't say %q", want)
		}
	}
	for _, bundled := range []string{"https://cdnjs.cloudflare.com/ajax/libs/animejs/3.2.2/anime.min.js", "https://cdn.jsdelivr.net/npm/three@0.160.0/build/three.module.js"} {
		if !strings.Contains(CanvasGuide, bundled) || !strings.Contains(CanvasMotionGuide, bundled) {
			t.Errorf("a guide doesn't give %s", bundled)
		}
	}
}

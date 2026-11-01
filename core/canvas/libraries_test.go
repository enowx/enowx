package canvas

import (
	"encoding/base64"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/enowx/enowx/proto"
)

const (
	animeTag   = `<script src="https://cdnjs.cloudflare.com/ajax/libs/animejs/3.2.2/anime.min.js"></script>`
	threeMap   = `<script type="importmap">{"imports":{"three":"https://cdn.jsdelivr.net/npm/three@0.160.0/build/three.module.js"}}</script>`
	threeBare  = `<script type="module">import * as THREE from 'three';</script>`
	cdnAddress = `https://(cdn\.jsdelivr\.net|cdnjs\.cloudflare\.com|unpkg\.com)`
)

func frameOf(html string) proto.CanvasFrame {
	return proto.CanvasFrame{ID: "f", Name: "F", Width: 800, Height: 600, HTML: html}
}

// A design naming the bundled libraries carries them, whole, their notices
// inside, and asks no network for them.
func TestBundledLibrariesNeedNoNetwork(t *testing.T) {
	doc := FrameDocument(frameOf(animeTag+threeMap+threeBare), nil)
	if regexp.MustCompile(cdnAddress).MatchString(doc) {
		t.Fatal("a CDN address is left")
	}
	for _, l := range []struct {
		lib    *library
		notice string
	}{{animeLibrary, "Released under the MIT license"}, {threeLibrary, "SPDX-License-Identifier: MIT"}} {
		if !strings.Contains(doc, l.lib.dataURL()) {
			t.Error("a library isn't in the document")
		}
		raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(l.lib.dataURL(), "data:text/javascript;base64,"))
		if err != nil || !strings.Contains(string(raw), l.notice) {
			t.Errorf("%q isn't in the library as carried: %v", l.notice, err)
		}
	}
	for _, path := range []string{"lib/anime/LICENSE.md", "lib/three/LICENSE"} {
		if data, err := os.ReadFile(path); err != nil || !strings.Contains(string(data), "MIT") {
			t.Errorf("%s: %v", path, err)
		}
	}
}

// Three by its bare name works however the design maps it, or doesn't; another
// version stays what the design asked for.
func TestThreeByItsBareName(t *testing.T) {
	data := threeLibrary.dataURL()
	if doc := FrameDocument(frameOf(threeBare), nil); !strings.Contains(doc, `<script type="importmap">{"imports":{"three":"`+data) {
		t.Error("no import map was added")
	}
	double := `<script type="module">import {Scene} from "three";</script>`
	if doc := FrameDocument(frameOf(double), nil); !strings.Contains(doc, `"three":"`+data) {
		t.Error("double quotes aren't read as an import")
	}
	own := `<script type="importmap">{"imports":{"x":"data:text/javascript,"}}</script>` + threeBare
	if doc := FrameDocument(frameOf(own), nil); !strings.Contains(doc, `"imports":{"three":"`+data+`","x"`) || strings.Count(doc, "importmap") != 1 {
		t.Error("three wasn't added to the design's own map")
	}
	other := `<script type="importmap">{"imports":{"three":"https://cdn.jsdelivr.net/npm/three@0.170.0/build/three.module.js"}}</script>` + threeBare
	doc := FrameDocument(frameOf(other), nil)
	if !strings.Contains(doc, "three@0.170.0") || strings.Contains(doc, data) {
		t.Error("another version was replaced")
	}
}

// An image of the design's and a library both go in, and the library's text is
// never read for "assets/".
func TestLibrariesWithAssets(t *testing.T) {
	assets := func(string) ([]byte, error) { return []byte("png"), nil }
	doc := FrameDocument(frameOf(`<img src="assets/a.png">`+animeTag), assets)
	if !strings.Contains(doc, "base64,"+base64.StdEncoding.EncodeToString([]byte("png"))) || !strings.Contains(doc, animeLibrary.dataURL()) {
		t.Fatal("one of them is missing")
	}
}

package canvas

import (
	_ "embed"
	"encoding/base64"
	"regexp"
	"strings"
	"sync"
)

// The motion libraries travel with enowX, so a design using them works with no
// network: at document time their CDN addresses become data: URLs, in a design
// that names them and no other. A data: URL rather than the source in a <script>
// because it needs no escaping in a shared page's srcdoc, can't hold a "</script>"
// or "</body>" the composing code would trip on, and loads the same under a
// design's stranger origin and a sandboxed frame's opaque one, where a blob: made
// by the page around it wouldn't. Base64 costs a third more than the source.
//
// lib/anime/anime.min.js:        cdnjs.cloudflare.com/ajax/libs/animejs/3.2.2/anime.min.js
// sha256 bceef94f964481f7680d95e7fbbe5a8c20d3945a926a754874898a578db7c7ab
// lib/three/three.module.min.js: cdn.jsdelivr.net/npm/three@0.160.0/build/three.module.min.js
// sha256 3e690ac7d180b0aadf0891bea39eec643e29e2d3e75c99b18689518665f69ba6
// Both MIT; each file keeps its header, and its LICENSE lies beside it.

//go:embed lib/anime/anime.min.js
var animeSource []byte

//go:embed lib/three/three.module.min.js
var threeSource []byte

// library is a bundled script and the addresses that stand for it, its exact
// version on the CDNs. A design naming another version asked for that one.
type library struct {
	source []byte
	urls   []string
	once   sync.Once
	data   string
}

func (l *library) dataURL() string {
	l.once.Do(func() {
		l.data = "data:text/javascript;base64," + base64.StdEncoding.EncodeToString(l.source)
	})
	return l.data
}

var (
	animeLibrary = &library{source: animeSource, urls: []string{
		"https://cdnjs.cloudflare.com/ajax/libs/animejs/3.2.2/anime.min.js",
		"https://cdn.jsdelivr.net/npm/animejs@3.2.2/lib/anime.min.js",
		"https://unpkg.com/animejs@3.2.2/lib/anime.min.js",
	}}
	threeLibrary = &library{source: threeSource, urls: []string{
		"https://cdn.jsdelivr.net/npm/three@0.160.0/build/three.module.js",
		"https://cdn.jsdelivr.net/npm/three@0.160.0/build/three.module.min.js",
		"https://unpkg.com/three@0.160.0/build/three.module.js",
		"https://unpkg.com/three@0.160.0/build/three.module.min.js",
	}}
	bundledLibraries = []*library{animeLibrary, threeLibrary}
)

// bareThree finds a module importing three by its bare name.
var bareThree = regexp.MustCompile(`(?:\bfrom\s*|\bimport\s*\(?\s*)['"]three['"]`)

// importMapOpen finds where a design's import map starts listing; threeInMap,
// that it lists three.
var (
	importMapOpen = regexp.MustCompile(`"imports"\s*:\s*\{`)
	threeInMap    = regexp.MustCompile(`"three"\s*:`)
)

// inlineLibraries has page carry the bundled libraries it names. Last of what
// composes a page: nothing after it should read the source as the design's.
func inlineLibraries(page string) string {
	for _, l := range bundledLibraries {
		for _, u := range l.urls {
			if strings.Contains(page, u) {
				page = strings.ReplaceAll(page, u, l.dataURL())
			}
		}
	}
	if !bareThree.MatchString(page) {
		return page
	}
	three := `"three":"` + threeLibrary.dataURL() + `"`
	switch {
	case threeInMap.MatchString(page):
		return page // its own map says where three comes from
	case strings.Contains(page, `importmap`):
		// A page can only count on one map: WebKit took a second only lately.
		if at := importMapOpen.FindStringIndex(page); at != nil {
			sep := ","
			if strings.HasPrefix(strings.TrimSpace(page[at[1]:]), "}") {
				sep = ""
			}
			return page[:at[1]] + three + sep + page[at[1]:]
		}
		return page
	}
	return WithHead(page, `<script type="importmap">{"imports":{`+three+`}}</script>`)
}

// usesLibrary says whether html names a bundled library.
func usesLibrary(html string) bool {
	for _, l := range bundledLibraries {
		for _, u := range l.urls {
			if strings.Contains(html, u) {
				return true
			}
		}
	}
	return bareThree.MatchString(html)
}

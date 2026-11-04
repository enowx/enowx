package canvas

import (
	"encoding/base64"
	"errors"
	"strings"

	"github.com/enowx/enowx/proto"
)

// ErrDocumentTooBig says what a frame's document comes to is over its budget.
var ErrDocumentTooBig = errors.New("canvas: this design's document is too big")

// FrameDocumentWithin is FrameDocument for a frame that isn't the user's own: a
// few bytes of it can name a file or a library again and again, each one inlined
// whole, so what it comes to is worked out before anything is made.
func FrameDocumentWithin(f proto.CanvasFrame, assets Assets, limit int64) (string, error) {
	if documentBound(f, assets) > limit {
		return "", ErrDocumentTooBig
	}
	doc := FrameDocument(f, assets)
	if int64(len(doc)) > limit {
		return "", ErrDocumentTooBig
	}
	return doc, nil
}

// What a shape costs in its SVG beyond its strings, and what escaping or a line of
// its text can make of a byte.
const (
	shapeOverhead = 512
	escapedByte   = 6
	lineOverhead  = 64
)

// documentBound is at least what FrameDocument(f, assets) comes to, reading each
// file once and writing nothing. It stops counting once a sum could overflow: an
// int64 of bytes is far beyond any budget.
func documentBound(f proto.CanvasFrame, assets Assets) int64 {
	inlined := map[string]int64{}
	sizeOf := func(name string) int64 {
		if n, seen := inlined[name]; seen {
			return n
		}
		var n int64
		if name != "" && !strings.Contains(name, ":") && assets != nil {
			if data, err := assets(name); err == nil {
				n = int64(base64.StdEncoding.EncodedLen(len(data))) + 128 // its data: prefix
			}
		}
		inlined[name] = n
		return n
	}
	total := int64(len(f.HTML)) + 4096 + int64(len(f.Background))*2*escapedByte
	if assets != nil {
		for page := f.HTML; ; {
			i, end := nextAsset(page)
			if i < 0 {
				break
			}
			total += sizeOf(page[i+len(assetPrefix) : end])
			page = page[end:]
		}
	}
	for _, l := range bundledLibraries {
		for _, u := range l.urls {
			if n := strings.Count(f.HTML, u); n > 0 {
				total += int64(n) * int64(len(l.dataURL()))
			}
		}
	}
	if bareThree.MatchString(f.HTML) {
		total += int64(len(threeLibrary.dataURL())) + 128
	}
	if usesPrototype(f.HTML) {
		total += int64(len(prototypeRuntime))
	}
	if UsesMotion(f.HTML) {
		total += int64(len(motionRuntime()))
	}
	for _, e := range f.Elements {
		total += shapeOverhead + sizeOf(e.Src)
		total += int64(len(e.ID)+len(e.Fill)+len(e.Stroke)+len(e.Color)+len(e.Src)+len(e.Text)) * escapedByte
		total += int64(strings.Count(e.Text, "\n")) * lineOverhead
	}
	return total
}

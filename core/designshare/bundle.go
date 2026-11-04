// Package designshare makes a canvas's design into what the community library
// keeps, and checks it: a bundle of the design alone (format enowx-design/1),
// what can't be shared and what the creator has to agree to. The host runs it
// before anything is sent, and the cloud runs the same checks again, since it
// doesn't take a host's word for them.
package designshare

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/enowx/enowx/proto"
)

// Format names what a bundle is.
const Format = "enowx-design/1"

// Bundle is what a version of a design is: the frames and the files they show.
// A canvas's comments, brief, history, sessions and agents' conversations have
// no field here, so none of them can come along.
type Bundle struct {
	Format string `json:"format"`
	Frame  Frame  `json:"frame"`
	// Phone is the design shown on a phone: a second design of the same page.
	Phone  *Frame            `json:"phone,omitempty"`
	Assets map[string][]byte `json:"assets,omitempty"`
}

// Frame is what a bundle keeps of a design.
type Frame struct {
	Name       string                `json:"name"`
	Width      int                   `json:"width"`
	Height     int                   `json:"height"`
	Background string                `json:"background,omitempty"`
	HTML       string                `json:"html,omitempty"`
	Elements   []proto.CanvasElement `json:"elements,omitempty"`
}

// Canvas is f as a canvas's frame, for the code that draws one.
func (f Frame) Canvas() proto.CanvasFrame {
	return proto.CanvasFrame{Name: f.Name, Width: f.Width, Height: f.Height, Background: f.Background, HTML: f.HTML, Elements: f.Elements}
}

func frameOf(f proto.CanvasFrame) Frame {
	return Frame{Name: f.Name, Width: f.Width, Height: f.Height, Background: f.Background, HTML: f.HTML, Elements: f.Elements}
}

var assetRef = regexp.MustCompile(`assets/([^"'\s)>]+)`)

// assetsOf are the names of the files f shows.
func assetsOf(f proto.CanvasFrame) []string {
	var out []string
	for _, m := range assetRef.FindAllStringSubmatch(f.HTML, -1) {
		out = append(out, m[1])
	}
	for _, e := range f.Elements {
		if e.Src != "" && !strings.Contains(e.Src, ":") {
			out = append(out, e.Src)
		}
	}
	return out
}

// Build is the bundle of design frame, with phone (nil for none), holding only
// the files they show, read with read. A file that can't be read is left out:
// the design shows it missing, as it does on the canvas.
func Build(frame proto.CanvasFrame, phone *proto.CanvasFrame, read func(name string) ([]byte, error)) Bundle {
	b := Bundle{Format: Format, Frame: frameOf(frame), Assets: map[string][]byte{}}
	names := assetsOf(frame)
	if phone != nil {
		p := frameOf(*phone)
		b.Phone = &p
		names = append(names, assetsOf(*phone)...)
	}
	for _, name := range names {
		if _, had := b.Assets[name]; had || read == nil {
			continue
		}
		if data, err := read(name); err == nil {
			b.Assets[name] = data
		}
	}
	return b
}

// Marshal is b as its bytes in the library.
func (b Bundle) Marshal() ([]byte, error) {
	data, err := json.Marshal(b)
	if err != nil {
		return nil, fmt.Errorf("designshare: bundle: %w", err)
	}
	return data, nil
}

// Parse reads a bundle, of a format it knows, holding what a design needs.
func Parse(data []byte) (Bundle, error) {
	var b Bundle
	if err := json.Unmarshal(data, &b); err != nil {
		return b, errors.New("designshare: this isn't a design's bundle")
	}
	if b.Format != Format {
		return b, fmt.Errorf("designshare: a bundle of format %q isn't known here", b.Format)
	}
	for _, f := range b.frames() {
		if f.Width <= 0 || f.Height <= 0 || f.Width > 8000 || f.Height > 40000 {
			return b, errors.New("designshare: a design's size is out of bounds")
		}
	}
	return b, nil
}

func (b Bundle) frames() []Frame {
	out := []Frame{b.Frame}
	if b.Phone != nil {
		out = append(out, *b.Phone)
	}
	return out
}

// AssetReader reads the bundle's files, as canvas's documents want them.
func (b Bundle) AssetReader() func(string) ([]byte, error) {
	return func(name string) ([]byte, error) {
		if data, ok := b.Assets[name]; ok {
			return data, nil
		}
		return nil, fmt.Errorf("designshare: no file %q", name)
	}
}

// Slug is title as a part of an address.
func Slug(title string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(title) {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			dash = false
			b.WriteRune(r)
		default:
			dash = true
		}
		if b.Len() >= 60 {
			break
		}
	}
	if b.Len() == 0 {
		return "design"
	}
	return b.String()
}

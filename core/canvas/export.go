package canvas

import (
	"encoding/base64"
	"fmt"
	"html"
	"mime"
	"path/filepath"
	"strings"

	"github.com/enowx/enowx/proto"
)

// Assets reads a canvas's file by name, for exports to carry its images.
type Assets func(name string) ([]byte, error)

// imageSrc is what an image's Src shows as in a page: an asset inlined, a
// URL as it is.
func imageSrc(src string, assets Assets) string {
	if src == "" || strings.Contains(src, ":") || assets == nil {
		return src
	}
	data, err := assets(src)
	if err != nil {
		return src
	}
	typ := mime.TypeByExtension(filepath.Ext(src))
	if typ == "" {
		typ = "application/octet-stream"
	}
	return "data:" + typ + ";base64," + base64.StdEncoding.EncodeToString(data)
}

// SVG is frame f's shapes as an SVG document, f's size; withHTML puts
// its mockup under them (a foreignObject, which browsers draw).
func SVG(f proto.CanvasFrame, assets Assets, withHTML bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" font-family="Inter, system-ui, sans-serif">`, f.Width, f.Height, f.Width, f.Height)
	b.WriteString(`<defs><marker id="arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse"><path d="M0,0 L10,5 L0,10 z" fill="context-stroke"/></marker></defs>`)
	if f.Background != "" {
		fmt.Fprintf(&b, `<rect width="100%%" height="100%%" fill="%s"/>`, attrEsc(f.Background))
	}
	if withHTML && strings.TrimSpace(f.HTML) != "" {
		fmt.Fprintf(&b, `<foreignObject x="0" y="0" width="%d" height="%d"><div xmlns="http://www.w3.org/1999/xhtml">%s</div></foreignObject>`, f.Width, f.Height, f.HTML)
	}
	for _, el := range f.Elements {
		svgElement(&b, f, el, assets)
	}
	b.WriteString(`</svg>`)
	return b.String()
}

func attrEsc(s string) string { return html.EscapeString(s) }

func svgElement(b *strings.Builder, f proto.CanvasFrame, el proto.CanvasElement, assets Assets) {
	st := StyleOf(el)
	paint := func() string {
		fill, stroke := st.Fill, st.Stroke
		if fill == "" {
			fill = "none"
		}
		if stroke == "" {
			stroke = "none"
		}
		return fmt.Sprintf(`fill="%s" stroke="%s" stroke-width="%g"`, attrEsc(fill), attrEsc(stroke), st.StrokeWidth)
	}
	fmt.Fprintf(b, `<g id="%s">`, attrEsc(el.ID))
	switch el.Type {
	case proto.ShapeRect, proto.ShapeSticky, proto.ShapeText:
		if st.Fill != "" || st.Stroke != "" {
			fmt.Fprintf(b, `<rect x="%g" y="%g" width="%g" height="%g" rx="%g" %s/>`, el.X, el.Y, el.W, el.H, el.Radius, paint())
		}
	case proto.ShapeEllipse:
		fmt.Fprintf(b, `<ellipse cx="%g" cy="%g" rx="%g" ry="%g" %s/>`, el.X+el.W/2, el.Y+el.H/2, el.W/2, el.H/2, paint())
	case proto.ShapeDiamond:
		cx, cy := el.X+el.W/2, el.Y+el.H/2
		fmt.Fprintf(b, `<polygon points="%g,%g %g,%g %g,%g %g,%g" %s/>`, cx, el.Y, el.X+el.W, cy, cx, el.Y+el.H, el.X, cy, paint())
	case proto.ShapeLine, proto.ShapeArrow:
		x1, y1, x2, y2 := Ends(f, el)
		marker := ""
		if el.Type == proto.ShapeArrow {
			marker = ` marker-end="url(#arrow)"`
		}
		fmt.Fprintf(b, `<line x1="%g" y1="%g" x2="%g" y2="%g" %s%s/>`, x1, y1, x2, y2, paint(), marker)
	case proto.ShapeImage:
		fmt.Fprintf(b, `<image x="%g" y="%g" width="%g" height="%g" href="%s" preserveAspectRatio="xMidYMid slice"/>`, el.X, el.Y, el.W, el.H, attrEsc(imageSrc(el.Src, assets)))
	}
	if el.Text != "" {
		x, anchor := el.X+el.W/2, "middle"
		switch el.Align {
		case "left":
			x, anchor = el.X+8, "start"
		case "right":
			x, anchor = el.X+el.W-8, "end"
		}
		if el.Type == proto.ShapeLine || el.Type == proto.ShapeArrow {
			x1, y1, x2, y2 := Ends(f, el)
			x, anchor = (x1+x2)/2, "middle"
			el.Y, el.H = (y1+y2)/2-st.FontSize, st.FontSize
		}
		weight := "400"
		if el.Bold {
			weight = "700"
		}
		lines := strings.Split(el.Text, "\n")
		lh := st.FontSize * 1.3
		y := el.Y + el.H/2 - lh*float64(len(lines)-1)/2
		fmt.Fprintf(b, `<text x="%g" y="%g" fill="%s" font-size="%g" font-weight="%s" text-anchor="%s" dominant-baseline="middle">`, x, y, attrEsc(st.Color), st.FontSize, weight, anchor)
		for i, line := range lines {
			fmt.Fprintf(b, `<tspan x="%g" dy="%g">%s</tspan>`, x, map[bool]float64{true: 0, false: lh}[i == 0], html.EscapeString(line))
		}
		b.WriteString(`</text>`)
	}
	b.WriteString(`</g>`)
}

// FrameDocument is frame f as a page of its own: its mockup, its shapes
// over it, at f's size. Pages render it, and screenshots are of it.
func FrameDocument(f proto.CanvasFrame, assets Assets) string {
	bg := f.Background
	if bg == "" {
		bg = "#ffffff"
	}
	overlay := ""
	if len(f.Elements) > 0 {
		overlay = fmt.Sprintf(`<div style="position:absolute;left:0;top:0;width:%dpx;height:%dpx;pointer-events:none;z-index:2147483647">%s</div>`, f.Width, f.Height, SVG(proto.CanvasFrame{Width: f.Width, Height: f.Height, Elements: f.Elements}, assets, false))
	}
	base := fmt.Sprintf(`<style>html,body{margin:0;min-height:%dpx;background:%s}body{position:relative;width:%dpx}</style>`, f.Height, attrEsc(bg), f.Width)
	if usesPrototype(f.HTML) {
		base += prototypeRuntime
	}
	if UsesMotion(f.HTML) {
		base += motionRuntime()
	}
	page := inlineLibraries(inlineAssets(f.HTML, assets))
	if strings.Contains(strings.ToLower(page), "</body>") {
		i := strings.LastIndex(strings.ToLower(page), "</body>")
		page = page[:i] + overlay + page[i:]
		if j := strings.Index(strings.ToLower(page), "<head>"); j >= 0 {
			return page[:j+6] + base + page[j+6:]
		}
		return base + page
	}
	return `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=` + fmt.Sprint(f.Width) + `">` + base + `</head><body>` + page + overlay + `</body></html>`
}

// inlineAssets puts a canvas's files a mockup shows ("assets/<name>") in
// it, so it shows them anywhere.
func inlineAssets(page string, assets Assets) string {
	if assets == nil || !strings.Contains(page, assetPrefix) {
		return page
	}
	var b strings.Builder
	for {
		i, end := nextAsset(page)
		if i < 0 {
			b.WriteString(page)
			return b.String()
		}
		name := page[i+len(assetPrefix) : end]
		b.WriteString(page[:i])
		if src := imageSrc(name, assets); src != name {
			b.WriteString(src)
		} else {
			b.WriteString(page[i:end])
		}
		page = page[end:]
	}
}

const assetPrefix = "assets/"

// nextAsset is where page's next "assets/<name>" starts and ends, -1 for none.
func nextAsset(page string) (start, end int) {
	start = strings.Index(page, assetPrefix)
	if start < 0 {
		return -1, -1
	}
	end = start + len(assetPrefix)
	for end < len(page) && !strings.ContainsRune("\"' )>", rune(page[end])) {
		end++
	}
	return start, end
}

// Standalone is the whole canvas as one read-only page to share: each
// frame at its size, under its name, with no app or connection needed.
func Standalone(c proto.Canvas, assets Assets) string {
	var b strings.Builder
	b.WriteString(`<!doctype html><html><head><meta charset="utf-8"><title>` + html.EscapeString(c.Title) + `</title>`)
	b.WriteString(`<style>body{margin:0;background:#f1f5f9;font-family:Inter,system-ui,sans-serif;color:#0f172a}header{padding:24px 32px}h1{margin:0;font-size:22px}p{margin:6px 0 0;color:#64748b;font-size:13px}section{padding:0 32px 40px}h2{font-size:14px;color:#475569;margin:0 0 10px}iframe{border:0;background:#fff;box-shadow:0 1px 3px rgba(15,23,42,.12),0 8px 24px rgba(15,23,42,.08);border-radius:8px;max-width:100%}</style></head><body>`)
	b.WriteString(shareStyle)
	fmt.Fprintf(&b, `<header><h1>%s</h1><p>%s · version %d · read-only</p>`+shareControls+`</header>`, html.EscapeString(c.Title), html.EscapeString(c.Kind), c.Version)
	moving := false
	for i, f := range c.Frames {
		doc := FrameDocument(f, assets)
		if j := strings.LastIndex(strings.ToLower(doc), "</body>"); j >= 0 {
			doc = doc[:j] + shareGoto + doc[j:]
		} else {
			doc += shareGoto
		}
		about := ""
		if f.Description != "" {
			about = `<p>` + html.EscapeString(f.Description) + `</p>`
		}
		bar := ""
		if UsesMotion(f.HTML) {
			bar, moving = shareMotionBar, true
		}
		fmt.Fprintf(&b, `<section id="design-%d" data-name="%s"><h2>%d · %s · %d×%d</h2>%s<iframe title="%s" width="%d" height="%d" sandbox="%s" srcdoc="%s"></iframe>%s</section>`,
			i+1, attrEsc(f.Name), i+1, html.EscapeString(f.Name), f.Width, f.Height, about, attrEsc(f.Name), f.Width, f.Height, shareFrameSandbox, attrEsc(doc), bar)
	}
	b.WriteString(shareHost)
	if moving {
		b.WriteString(shareMotion)
	}
	b.WriteString(`</body></html>`)
	return b.String()
}

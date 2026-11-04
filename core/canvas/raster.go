package canvas

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif" // assets an image shows
	_ "image/jpeg"
	"image/png"
	"math"
	"strings"
	"sync"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/vector"

	"github.com/enowx/enowx/proto"
)

// The daemon draws a frame of shapes itself, so agents see what they made
// on a server without a screen or an app connected; a mockup's HTML needs
// a web engine, the app's.

// ErrNeedsBrowser is a frame with HTML, which only a web engine renders.
var ErrNeedsBrowser = errors.New("canvas: the frame has HTML, which the enowX app renders, and no app is connected to this host: open the app, and check it reaches this host")

// PNG draws frame f's shapes as a PNG, f's size times scale.
func PNG(f proto.CanvasFrame, assets Assets, scale float64) ([]byte, error) {
	if strings.TrimSpace(f.HTML) != "" {
		return nil, ErrNeedsBrowser
	}
	img := Raster(f, assets, scale)
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// BoardColor is the bare board's, under a design without a page or a
// background of its own, as the app shows it in its dark theme.
const BoardColor = "#141413"

// Raster draws frame f's shapes, f's size times scale.
func Raster(f proto.CanvasFrame, assets Assets, scale float64) *image.RGBA {
	if scale <= 0 {
		scale = 1
	}
	w, h := max(1, int(float64(f.Width)*scale)), max(1, int(float64(f.Height)*scale))
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	bg := color.RGBA{255, 255, 255, 255}
	if c, ok := ParseColor(f.Background); ok {
		bg = c
	}
	draw.Draw(img, img.Bounds(), image.NewUniform(bg), image.Point{}, draw.Src)
	r := rasterizer{img: img, s: scale}
	for _, el := range f.Elements {
		r.element(f, el, assets)
	}
	return img
}

type rasterizer struct {
	img *image.RGBA
	s   float64
}

type pt struct{ x, y float64 }

func (r rasterizer) fill(poly []pt, col color.RGBA) {
	if len(poly) < 3 {
		return
	}
	b := r.img.Bounds()
	z := vector.NewRasterizer(b.Dx(), b.Dy())
	z.MoveTo(float32(poly[0].x*r.s), float32(poly[0].y*r.s))
	for _, p := range poly[1:] {
		z.LineTo(float32(p.x*r.s), float32(p.y*r.s))
	}
	z.ClosePath()
	z.Draw(r.img, b, image.NewUniform(col), image.Point{})
}

// line draws a stroke from a to b, width wide, as a quad.
func (r rasterizer) line(a, b pt, width float64, col color.RGBA) {
	dx, dy := b.x-a.x, b.y-a.y
	l := math.Hypot(dx, dy)
	if l == 0 {
		return
	}
	nx, ny := -dy/l*width/2, dx/l*width/2
	r.fill([]pt{{a.x + nx, a.y + ny}, {b.x + nx, b.y + ny}, {b.x - nx, b.y - ny}, {a.x - nx, a.y - ny}}, col)
}

func (r rasterizer) outline(poly []pt, width float64, col color.RGBA) {
	for i := range poly {
		r.line(poly[i], poly[(i+1)%len(poly)], width, col)
	}
}

// rounded is a box's outline, its corners rounded by radius.
func rounded(x, y, w, h, radius float64) []pt {
	radius = math.Min(radius, math.Min(w, h)/2)
	if radius <= 0 {
		return []pt{{x, y}, {x + w, y}, {x + w, y + h}, {x, y + h}}
	}
	var out []pt
	corner := func(cx, cy, from float64) {
		for i := 0; i <= 6; i++ {
			a := from + float64(i)*math.Pi/12
			out = append(out, pt{cx + radius*math.Cos(a), cy + radius*math.Sin(a)})
		}
	}
	corner(x+w-radius, y+radius, -math.Pi/2)
	corner(x+w-radius, y+h-radius, 0)
	corner(x+radius, y+h-radius, math.Pi/2)
	corner(x+radius, y+radius, math.Pi)
	return out
}

func ellipse(x, y, w, h float64) []pt {
	out := make([]pt, 48)
	for i := range out {
		a := float64(i) * 2 * math.Pi / 48
		out[i] = pt{x + w/2 + w/2*math.Cos(a), y + h/2 + h/2*math.Sin(a)}
	}
	return out
}

func (r rasterizer) element(f proto.CanvasFrame, el proto.CanvasElement, assets Assets) {
	st := StyleOf(el)
	fill, hasFill := ParseColor(st.Fill)
	stroke, hasStroke := ParseColor(st.Stroke)
	var shape []pt
	switch el.Type {
	case proto.ShapeRect, proto.ShapeSticky, proto.ShapeText:
		shape = rounded(el.X, el.Y, el.W, el.H, el.Radius)
	case proto.ShapeEllipse:
		shape = ellipse(el.X, el.Y, el.W, el.H)
	case proto.ShapeDiamond:
		cx, cy := el.X+el.W/2, el.Y+el.H/2
		shape = []pt{{cx, el.Y}, {el.X + el.W, cy}, {cx, el.Y + el.H}, {el.X, cy}}
	case proto.ShapeLine, proto.ShapeArrow:
		x1, y1, x2, y2 := Ends(f, el)
		if hasStroke {
			r.line(pt{x1, y1}, pt{x2, y2}, st.StrokeWidth, stroke)
			if el.Type == proto.ShapeArrow {
				ax, ay, bx, by := ArrowHead(x1, y1, x2, y2, 8+st.StrokeWidth*2)
				r.fill([]pt{{x2, y2}, {ax, ay}, {bx, by}}, stroke)
			}
		}
		if el.Text != "" {
			el.X, el.Y, el.W, el.H = (x1+x2)/2-100, (y1+y2)/2-st.FontSize*1.6, 200, st.FontSize*1.4
			r.text(el, st)
		}
		return
	case proto.ShapeImage:
		r.image(el, assets)
		shape = rounded(el.X, el.Y, el.W, el.H, el.Radius)
		hasFill = false
	}
	if hasFill {
		r.fill(shape, fill)
	}
	if hasStroke {
		r.outline(shape, st.StrokeWidth, stroke)
	}
	if el.Text != "" {
		r.text(el, st)
	}
}

func (r rasterizer) image(el proto.CanvasElement, assets Assets) {
	if assets == nil || el.Src == "" || strings.Contains(el.Src, ":") {
		r.fill(rounded(el.X, el.Y, el.W, el.H, el.Radius), color.RGBA{226, 232, 240, 255})
		return
	}
	data, err := assets(el.Src)
	if err != nil {
		return
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return
	}
	dst := image.Rect(int(el.X*r.s), int(el.Y*r.s), int((el.X+el.W)*r.s), int((el.Y+el.H)*r.s))
	xdraw.CatmullRom.Scale(r.img, dst, src, src.Bounds(), draw.Over, nil)
}

var (
	fontOnce        sync.Once
	regular, boldTT *opentype.Font
)

func loadFonts() {
	regular, _ = opentype.Parse(goregular.TTF)
	boldTT, _ = opentype.Parse(gobold.TTF)
}

// text draws el's text in its box, wrapped at its width, centered as it
// says.
func (r rasterizer) text(el proto.CanvasElement, st Style) {
	fontOnce.Do(loadFonts)
	tt := regular
	if el.Bold {
		tt = boldTT
	}
	if tt == nil {
		return
	}
	face, err := opentype.NewFace(tt, &opentype.FaceOptions{Size: st.FontSize * r.s, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		return
	}
	defer face.Close()
	col, _ := ParseColor(st.Color)
	maxW := fixed.I(int(math.Max(el.W-16, 10) * r.s))
	var lines []string
	for _, para := range strings.Split(el.Text, "\n") {
		line := ""
		for _, word := range strings.Fields(para) {
			try := strings.TrimSpace(line + " " + word)
			if line != "" && font.MeasureString(face, try) > maxW {
				lines = append(lines, line)
				line = word
				continue
			}
			line = try
		}
		lines = append(lines, line)
	}
	lh := st.FontSize * 1.3 * r.s
	m := face.Metrics()
	top := (el.Y+el.H/2)*r.s - lh*float64(len(lines))/2
	d := font.Drawer{Dst: r.img, Src: image.NewUniform(col), Face: face}
	for i, line := range lines {
		w := font.MeasureString(face, line)
		x := fixed.I(int((el.X+el.W/2)*r.s)) - w/2
		switch el.Align {
		case "left":
			x = fixed.I(int((el.X + 8) * r.s))
		case "right":
			x = fixed.I(int((el.X+el.W-8)*r.s)) - w
		}
		base := top + lh*float64(i) + (lh+float64(m.Ascent.Round()-m.Descent.Round()))/2
		d.Dot = fixed.Point26_6{X: x, Y: fixed.I(int(base))}
		d.DrawString(line)
	}
}

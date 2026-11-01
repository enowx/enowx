package canvasview

import (
	"hash/fnv"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/canvas"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
)

// Designs show small (the strip, the grid) and side by side (compare) as
// previews: a mockup as the app renders it, a picture fetched once its
// HTML stopped changing a moment; shapes drawn as they are.

// thumb is a design's picture, of its HTML as of key; the HTML seen last,
// since when; and the one whose picture was asked for.
type thumb struct {
	key    string
	img    ui.ImageSource
	seen   string
	seenAt time.Time
	asked  string
}

// settleFor is how long a design's HTML stays the same before its picture
// is made anew: an agent writing it changes it often.
const settleFor = 1500 * time.Millisecond

func htmlKey(f proto.CanvasFrame) string {
	h := fnv.New64a()
	h.Write([]byte(f.HTML))
	return f.ID + ":" + strconv.FormatUint(h.Sum64(), 16) + ":" + strconv.Itoa(f.Width) + "x" + strconv.Itoa(f.Height)
}

// picture is design f's picture, made when its HTML settled; nil while
// there is none, or it has no HTML.
func (v *View) picture(c *ui.Context, o Options, f proto.CanvasFrame) ui.ImageSource {
	return v.pictureAt(c, o, f, 0)
}

// pictureAt is design f's picture reflowed at width (0 its own).
func (v *View) pictureAt(c *ui.Context, o Options, f proto.CanvasFrame, width int) ui.ImageSource {
	if strings.TrimSpace(f.HTML) == "" || o.Render == nil || v.canvas == nil || width > 0 && o.RenderWidth == nil {
		return nil
	}
	if v.thumbs == nil {
		v.thumbs = map[string]*thumb{}
	}
	id := f.ID
	if width > 0 {
		id += "@" + strconv.Itoa(width)
	}
	t := v.thumbs[id]
	if t == nil {
		t = &thumb{}
		v.thumbs[id] = t
	}
	key := htmlKey(f) + "@" + strconv.Itoa(width)
	if t.key == key || t.asked == key {
		return t.img
	}
	if t.seen != key {
		t.seen, t.seenAt = key, time.Now()
	}
	if wait := settleFor - time.Since(t.seenAt); t.img != nil && wait > 0 {
		c.After(wait + 20*time.Millisecond)
		return t.img
	}
	t.asked = key
	done := func(data []byte, err error) {
		if err != nil || t.asked != key {
			return
		}
		if bmp, err := ui.DecodeBitmap(data); err == nil {
			t.img, t.key = bmp, key
		}
	}
	if width > 0 {
		o.RenderWidth(v.canvas.ID, f.ID, width, done)
	} else {
		o.Render(v.canvas.ID, f.ID, proto.RenderPNG, done)
	}
	return t.img
}

// preview shows design f w wide: its picture, else its shapes and its
// mockup's elements drawn small.
func (v *View) designPreview(c *ui.Context, o Options, f proto.CanvasFrame, w float32) ui.Element {
	s := float64(w) / math.Max(float64(f.Width), 1)
	h := float32(float64(f.Height) * s)
	box := ui.Box(c).Size(w, h).Shrink(0).Radius(theme.RadiusTail).Clip().Background(color(f.Background, ui.RGB(255, 255, 255)))
	if img := v.picture(c, o, f); img != nil {
		box.Children(func() { ui.Image(c, img).Size(w, h) })
		return box
	}
	outline := canvas.Outline(f.HTML)
	box.Draw(func(p *ui.Painter, r ui.Rect) {
		y := r.Y + 10*float32(s)/float32(math.Max(s, 0.15))
		size := float32(math.Max(14*s, 7))
		for _, el := range outline {
			if y > r.Y+r.H-size {
				break
			}
			if el.Text != "" && el.Depth >= 1 {
				p.Text(r.X+8+float32(el.Depth)*6, y, el.Text, size, ui.RGB(71, 85, 105))
				y += size * 1.6
			}
		}
		drawShapes(p, r, f, s)
	})
	return box
}

// drawShapes draws f's shapes into r, scaled by s, without their
// handles: a picture of them.
func drawShapes(p *ui.Painter, r ui.Rect, f proto.CanvasFrame, s float64) {
	for _, el := range f.Elements {
		st := canvas.StyleOf(el)
		fill, stroke := color(st.Fill, ui.Color{}), color(st.Stroke, ui.Color{})
		x, y, w, h := r.X+float32(el.X*s), r.Y+float32(el.Y*s), float32(el.W*s), float32(el.H*s)
		box := ui.Rect{X: x, Y: y, W: w, H: h}
		width := float32(math.Max(st.StrokeWidth*s, 0.75))
		switch el.Type {
		case proto.ShapeLine, proto.ShapeArrow:
			x1, y1, x2, y2 := canvas.Ends(f, el)
			p.Line(r.X+float32(x1*s), r.Y+float32(y1*s), r.X+float32(x2*s), r.Y+float32(y2*s), width, stroke)
			continue
		case proto.ShapeEllipse, proto.ShapeDiamond:
			var path ui.Path
			if el.Type == proto.ShapeEllipse {
				ellipsePath(&path, box)
			} else {
				path.MoveTo(x+w/2, y).LineTo(x+w, y+h/2).LineTo(x+w/2, y+h).LineTo(x, y+h/2).Close()
			}
			if st.Fill != "" {
				p.FillPath(&path, fill)
			}
			if st.Stroke != "" {
				p.StrokePath(&path, width, stroke)
			}
		case proto.ShapeImage:
			p.Fill(box, ui.RGB(226, 232, 240), float32(el.Radius*s))
		default:
			if st.Fill != "" {
				p.Fill(box, fill, float32(el.Radius*s))
			}
			if st.Stroke != "" {
				p.Stroke(box, stroke, float32(el.Radius*s), width)
			}
		}
		if el.Text != "" && st.FontSize*s >= 5 {
			size := float32(st.FontSize * s)
			line := strings.SplitN(el.Text, "\n", 2)[0]
			p.Text(x+4, y+h/2-size/2, line, size, color(st.Color, theme.TextStrong))
		}
	}
}

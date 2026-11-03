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
// HTML stopped changing a moment, kept on this device until it changes again;
// shapes drawn as they are. A preview is never the design's words: until its
// picture comes, its frame says it is on its way.

// thumb is a design's picture, of its HTML as of key; the HTML seen last,
// since when; and the one whose picture was asked for.
type thumb struct {
	key    string
	img    ui.ImageSource
	seen   string
	seenAt time.Time
	asked  string
	// failed is the key whose picture couldn't be made, why and when.
	failed   string
	failure  string
	failedAt time.Time
	// place is where its card last was, and looked that it has been at all.
	place  ui.Rect
	looked bool
}

const (
	// thumbsAtOnce is how many pictures are asked for at a time: each takes a
	// hidden window and a moment on this device, and the rest can wait their turn.
	thumbsAtOnce = 2
	// thumbRetry is how long a failed picture rests before it is tried again.
	thumbRetry = 30 * time.Second
	// thumbMoved is a look at a card just after it moved: what shows in the window
	// is known a frame late, and a scroll that stopped draws no more of them.
	thumbMoved = 60 * time.Millisecond
)

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
	return v.pictureIf(c, o, f, width, true)
}

// pictureIf is pictureAt, asking for a picture it lacks only if wanted: a card out
// of sight has no use for one yet.
func (v *View) pictureIf(c *ui.Context, o Options, f proto.CanvasFrame, width int, wanted bool) ui.ImageSource {
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
	key := v.keyOf(f) + "@" + strconv.Itoa(width)
	if t.key == key {
		return t.img
	}
	if t.failed == key {
		rest := thumbRetry - time.Since(t.failedAt)
		if !wanted {
			return t.img
		}
		if rest > 0 {
			c.After(rest)
			return t.img
		}
		t.failed, t.asked = "", ""
	}
	if t.asked == key || !wanted {
		return t.img
	}
	if t.seen != key {
		t.seen, t.seenAt = key, time.Now()
	}
	if wait := settleFor - time.Since(t.seenAt); t.img != nil && wait > 0 {
		c.After(wait + 20*time.Millisecond)
		return t.img
	}
	if v.thumbsBusy >= thumbsAtOnce {
		return t.img // the end of one in hand draws a frame, and this one's turn
	}
	v.thumbsBusy++
	t.asked = key
	canvasID, frameID := v.canvas.ID, f.ID
	store := canvasID + ":" + key
	// A picture is decoded off the main thread: a canvas may hold a hundred designs.
	decode := func(data []byte, done func(error)) {
		var bmp *ui.Bitmap
		var err error
		work := func() { bmp, err = decodePicture(data) }
		then := func() {
			if err == nil && t.asked == key {
				t.img, t.key = bmp, key
			}
			done(err)
		}
		if o.OffMain == nil {
			work()
			then()
			return
		}
		o.OffMain(work, then)
	}
	failed := func(err error) {
		if t.asked == key {
			t.failed, t.failure, t.failedAt = key, err.Error(), time.Now()
		}
	}
	rendered := func(data []byte, err error) {
		if err != nil {
			v.thumbsBusy--
			failed(err)
			return
		}
		decode(data, func(err error) {
			v.thumbsBusy--
			if err != nil {
				failed(err)
				return
			}
			if o.PictureKeep != nil {
				o.PictureKeep(store, data)
			}
		})
	}
	render := func() {
		if width > 0 {
			o.RenderWidth(canvasID, frameID, width, rendered)
		} else {
			o.Render(canvasID, frameID, proto.RenderPNG, rendered)
		}
	}
	if o.PictureCached == nil {
		render()
		return t.img
	}
	o.PictureCached(store, func(data []byte) {
		if len(data) == 0 {
			render() // none kept
			return
		}
		decode(data, func(err error) {
			if err != nil {
				render() // what was kept doesn't read
				return
			}
			v.thumbsBusy--
		})
	})
	return t.img
}

// thumbFailure is why design f's own picture couldn't be made, "" if it wasn't.
func (v *View) thumbFailure(f proto.CanvasFrame) string {
	if t := v.thumbs[f.ID]; t != nil && t.failed == v.keyOf(f)+"@0" {
		return t.failure
	}
	return ""
}

// lookAgainMoved has a card with no picture yet looked at again once it moved, or
// first showed: whether it is in sight is known only the frame after.
func (v *View) lookAgainMoved(c *ui.Context, f proto.CanvasFrame, place ui.Rect) {
	t := v.thumbs[f.ID]
	if t == nil || t.img != nil || t.looked && t.place == place {
		return
	}
	t.place, t.looked = place, true
	c.After(thumbMoved)
}

// designPreview shows design f w wide: its picture, its shapes when it is drawn
// rather than written, else in its frame that the picture is on its way or failed.
func (v *View) designPreview(c *ui.Context, o Options, f proto.CanvasFrame, w float32) ui.Element {
	s := float64(w) / math.Max(float64(f.Width), 1)
	h := float32(float64(f.Height) * s)
	box := ui.Box(c).Size(w, h).Shrink(0).Radius(theme.RadiusTail).Clip().Background(color(f.Background, ui.RGB(255, 255, 255)))
	if strings.TrimSpace(f.HTML) == "" {
		box.Draw(func(p *ui.Painter, r ui.Rect) { drawShapes(p, r, f, s) })
		return box
	}
	img := v.pictureIf(c, o, f, 0, box.InView())
	v.lookAgainMoved(c, f, box.Bounds())
	if img != nil {
		box.Children(func() { ui.Image(c, img).Size(w, h) })
		return box
	}
	word, why := "Loading preview", ""
	switch failure := v.thumbFailure(f); {
	case failure != "":
		word, why = "Preview failed", failure
	case o.Render == nil:
		word = "No preview here"
	}
	box.Background(theme.Sunken).Children(func() {
		ui.Row(c.Key("state")).Size(w, h).Center().Children(func() {
			t := ui.Text(c.Key("word"), word).FontSize(theme.TextLabel).TextColor(theme.TextSoft).MaxLines(1)
			if why != "" {
				t.Tooltip(why)
			}
		})
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

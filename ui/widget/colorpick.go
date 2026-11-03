package widget

import (
	"math"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/theme"
)

// colorPickWidth is the pop-over's, and colorPickArea the height of its square.
const (
	colorPickWidth = 240
	colorPickArea  = 160
)

// colorPickState is what a picker keeps between frames: hue, kept apart from
// the color as black or a grey has none, and the drag under way.
type colorPickState struct {
	open    bool
	h, s, v float64
	last    ui.Color
	held    bool
}

// ColorPick is a swatch of value that opens a pop-over choosing a color, with no
// typing: a square of saturation, across, and brightness, up, and a slider of
// hue under it. changed gets each color while dragging; done is true on release,
// with the color to keep.
func ColorPick(c *ui.Context, label string, value ui.Color, changed func(col ui.Color, done bool)) ui.Element {
	b := ui.ButtonBase(c).Size(theme.ControlHeight, theme.ControlHeight).Radius(theme.RadiusFull).Shrink(0).
		Cursor(ui.CursorPointer).Label(label).Tooltip(label).InsetShadow(0, 0, 0, 0.5, theme.BorderStrong)
	st := ui.Local[colorPickState](b, "color-pick", func() colorPickState {
		s := colorPickState{}
		s.h, s.s, s.v = colorToHSV(value)
		s.last = value
		return s
	})
	if st.last != value && !st.held {
		// Set from outside: a swatch, or back to the theme's own. A grey keeps
		// the hue the slider has. While dragging, value is still what was saved.
		h, s, v := colorToHSV(value)
		if s > 0 {
			st.h = h
		}
		st.s, st.v, st.last = s, v, value
	}
	b.Background(st.last)
	if b.Clicked() {
		st.open = !st.open
	}
	ui.PopoverBase(c, b, &st.open, func(panel ui.Element) {
		Popup(panel).Width(colorPickWidth).Padding(theme.S3).Gap(theme.S2).Children(func() {
			set := func() {
				col := colorFromHSV(st.h, st.s, st.v)
				if col != st.last {
					st.last = col
					changed(col, false)
				}
			}
			area := ui.Box(c.Key("area")).FillWidth().Height(colorPickArea).Radius(theme.RadiusSmall).Cursor(ui.CursorCrosshair).
				Label("Saturation and brightness")
			area.Dragged() // holds on to the pointer past the square's edge
			hue := colorPickHue(c, &st.h)
			if area.Pressed() {
				if x, y, _ := area.PointerPosition(); area.Bounds().W > 0 && area.Bounds().H > 0 {
					st.s = clamp01(float64(x / area.Bounds().W))
					st.v = 1 - clamp01(float64(y/area.Bounds().H))
				}
				set()
			}
			if hue.Changed() {
				set()
			}
			s, v := st.s, st.v
			pure := colorFromHSV(st.h, 1, 1)
			area.Draw(func(p *ui.Painter, r ui.Rect) {
				p.FillGradient(r, ui.LinearGradient{From: ui.RGB(255, 255, 255), To: pure, Angle: 90}, theme.RadiusSmall)
				p.FillGradient(r, ui.LinearGradient{From: ui.RGBA(0, 0, 0, 0), To: ui.RGB(0, 0, 0), Angle: 180}, theme.RadiusSmall)
				const d = 14
				cx, cy := r.X+r.W*float32(s), r.Y+r.H*float32(1-v)
				p.Stroke(ui.Rect{X: cx - d/2, Y: cy - d/2, W: d, H: d}, ui.RGB(255, 255, 255), d/2, 2)
				p.Stroke(ui.Rect{X: cx - d/2 - 1, Y: cy - d/2 - 1, W: d + 2, H: d + 2}, ui.RGBA(0, 0, 0, 0.3), d/2+1, 1)
			})
			held := area.Pressed() || hue.Pressed()
			if st.held && !held {
				changed(st.last, true)
			}
			st.held = held
		})
	})
	return b
}

// colorPickHue is the picker's slider of hue, 0 to 360.
func colorPickHue(c *ui.Context, h *float64) ui.Element {
	const knob, track = 16, 8
	s := ui.SliderBase(c.Key("hue"), h, 0, 360).FillWidth().Height(knob).PaddingX(knob / 2).Label("Hue").FocusRing(false)
	at := float32(*h / 360)
	s.Draw(func(p *ui.Painter, r ui.Rect) {
		bar := ui.Rect{X: r.X, Y: r.Y + (r.H-track)/2, W: r.W, H: track}
		p.Clip(bar, track/2, func() {
			const parts = 6
			w := bar.W / parts
			for i := range parts {
				from, to := colorFromHSV(float64(i)*60, 1, 1), colorFromHSV(float64(i+1)*60, 1, 1)
				p.FillGradient(ui.Rect{X: bar.X + w*float32(i), Y: bar.Y, W: w + 0.5, H: bar.H}, ui.LinearGradient{From: from, To: to, Angle: 90}, 0)
			}
		})
		x := r.X + knob/2 + (r.W-knob)*at
		p.Fill(ui.Rect{X: x - knob/2, Y: r.Y, W: knob, H: knob}, ui.RGB(255, 255, 255), knob/2)
		p.Stroke(ui.Rect{X: x - knob/2, Y: r.Y, W: knob, H: knob}, ui.RGBA(0, 0, 0, 0.3), knob/2, 1)
	})
	return s
}

func clamp01(v float64) float64 { return min(max(v, 0), 1) }

// colorToHSV is c's hue, 0 to 360, saturation and brightness, 0 to 1.
func colorToHSV(c ui.Color) (h, s, v float64) {
	r, g, b := float64(c.R)/255, float64(c.G)/255, float64(c.B)/255
	hi, lo := max(r, g, b), min(r, g, b)
	v = hi
	if hi == 0 {
		return 0, 0, 0
	}
	s = (hi - lo) / hi
	if hi == lo {
		return 0, s, v
	}
	switch hi {
	case r:
		h = math.Mod((g-b)/(hi-lo)+6, 6)
	case g:
		h = (b-r)/(hi-lo) + 2
	default:
		h = (r-g)/(hi-lo) + 4
	}
	return h * 60, s, v
}

// colorFromHSV is the opaque color of hue h, 0 to 360, saturation s and
// brightness v, 0 to 1.
func colorFromHSV(h, s, v float64) ui.Color {
	c := v * s
	sector := math.Mod(h, 360) / 60
	x := c * (1 - math.Abs(math.Mod(sector, 2)-1))
	var r, g, b float64
	switch int(sector) {
	case 0:
		r, g = c, x
	case 1:
		r, g = x, c
	case 2:
		g, b = c, x
	case 3:
		g, b = x, c
	case 4:
		r, b = x, c
	default:
		r, b = c, x
	}
	m := v - c
	to := func(f float64) uint8 { return uint8(math.Round((f + m) * 255)) }
	return ui.RGB(to(r), to(g), to(b))
}

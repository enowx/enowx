package canvasview

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// Export's MP4: what to render of a design that moves. The app makes it in the
// background, with a card of its own, so this closes as soon as it's asked for.

// videoForm is the dialog's state.
type videoForm struct {
	open        bool
	frame       string
	name        string
	w, h        int
	fps, size   int // indexes of the choices
	width       string
	from, to    string
	whyNot, err string
}

var (
	videoFPS   = []int{30, 60}
	videoSizes = []string{"The frame's", "Twice that", "A width"}
)

// videoMenuItem offers the MP4, off with the reason for a design that doesn't move.
func (v *View) videoMenuItem(c *ui.Context, o Options, f proto.CanvasFrame, done func()) {
	if o.ExportVideo == nil {
		return
	}
	if !v.usesMotion(f.HTML) {
		widget.MenuItemOff(c.Key("mp4"), nil, "MP4 (this design doesn't move)")
		return
	}
	if widget.MenuItemSub(c.Key("mp4"), nil, "MP4…", "Renders the motion", false) {
		why := ""
		if o.VideoWhyNot != nil {
			why = o.VideoWhyNot()
		}
		v.video = &videoForm{open: true, frame: f.ID, name: f.Name, w: f.Width, h: f.Height, whyNot: why}
		done()
	}
}

// videoParams reads the form; an error says what's wrong with it.
func (f *videoForm) params(id string) (proto.CanvasVideoParams, error) {
	p := proto.CanvasVideoParams{ID: id, Frame: f.frame, FPS: videoFPS[f.fps]}
	switch f.size {
	case 1:
		p.Width = f.w * 2
	case 2:
		n, err := strconv.Atoi(strings.TrimSpace(f.width))
		if err != nil || n < 16 || n > 7680 {
			return p, fmt.Errorf("a width is from 16 to 7680")
		}
		p.Width = n
	}
	seconds := func(text string) (float64, error) {
		text = strings.TrimSpace(text)
		if text == "" {
			return 0, nil
		}
		n, err := strconv.ParseFloat(text, 64)
		if err != nil || n < 0 {
			return 0, fmt.Errorf("from and to are seconds, or empty for all of it")
		}
		return n * 1000, nil
	}
	var err error
	if p.From, err = seconds(f.from); err != nil {
		return p, err
	}
	if p.To, err = seconds(f.to); err != nil {
		return p, err
	}
	if p.To > 0 && p.To <= p.From {
		return p, fmt.Errorf("the range ends before it starts")
	}
	return p, nil
}

// videoDialog is what to render.
func (v *View) videoDialog(c *ui.Context, o Options, cv proto.Canvas) {
	f := v.video
	if f == nil {
		return
	}
	if !f.open {
		v.video = nil
		return
	}
	widget.FormSheet(c, &f.open, 460, "Export "+f.name+" as MP4", "Frame by frame, the same each time, in the background. A loop renders one cycle.", func() {
		if f.whyNot != "" {
			ui.Text(c.Key("why"), f.whyNot).FontSize(theme.TextUI).TextColor(theme.Warning)
			return
		}
		widget.Field(c.Key("fps"), "Frames a second", func() {
			f.fps = widget.Segmented(c.Key("fps-pick"), []string{"30", "60"}, f.fps, nil)
		})
		widget.Field(c.Key("size"), fmt.Sprintf("Size (the frame is %d × %d)", f.w, f.h), func() {
			f.size = widget.Segmented(c.Key("size-pick"), videoSizes, f.size, nil)
			if f.size == 2 {
				widget.Input(c.Key("width"), &f.width, "Width, in pixels").FillWidth()
			}
		})
		widget.Field(c.Key("range"), "Range, in seconds (empty for all of it)", func() {
			ui.Row(c.Key("range-row")).FillWidth().Gap(theme.S2).Children(func() {
				widget.Input(c.Key("from"), &f.from, "From").Grow(1).Basis(0).MinWidth(0)
				widget.Input(c.Key("to"), &f.to, "To").Grow(1).Basis(0).MinWidth(0)
			})
		})
		if f.err != "" {
			ui.Text(c.Key("err"), f.err).FontSize(theme.TextUI).TextColor(theme.Error)
		}
	}, func() {
		if f.whyNot != "" {
			return
		}
		if !widget.PrimaryButton(c.Key("render"), nil, "Render", "").Clicked() {
			return
		}
		p, err := f.params(cv.ID)
		if err != nil {
			f.err = err.Error()
			return
		}
		f.open = false
		// The card in the window's corner says how it goes.
		o.ExportVideo(p, func(error) {})
	})
}

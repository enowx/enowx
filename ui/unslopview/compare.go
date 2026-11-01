package unslopview

import (
	"fmt"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// An unslop's before and after, one laid on the other: a divider dragged
// across shows the before on its left and the after on its right, at each
// size the page was pictured at.

// Shot is a size's pictures, before the unslop and after.
type Shot struct {
	Label         string
	Before, After *ui.Bitmap
}

// Compare is what the comparison remembers. It belongs to the main thread.
type Compare struct {
	at       float32 // the divider, from the left edge (0) to the right (1)
	dragging bool
	size     int // the shot shown
}

// CompareOptions are what the comparison shows.
type CompareOptions struct {
	Shots []Shot
	// Loading says the pictures are on their way; Err that they didn't come.
	Loading  bool
	Err      string
	From, To int // the scores before and after
	Close    func()
}

// Draw draws the comparison as a dialog over the window.
func (v *Compare) Draw(c *ui.Context, o CompareOptions) {
	if v.at == 0 {
		v.at = 0.5
	}
	open := true
	ui.DialogBase(c.Key("unslop-compare"), &open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.82))
		w, h := c.Size()
		panel.Gap(theme.S3).AlignItems(ui.Center)
		v.bar(c, o)
		maxW, maxH := max(w-64, 200), max(h-150, 200)
		switch {
		case o.Err != "":
			ui.Text(c, o.Err).FontSize(theme.TextUI).TextColor(theme.Error).Padding(24)
		case o.Loading || len(o.Shots) == 0:
			ui.Row(c).Gap(8).AlignItems(ui.Center).Padding(24).Children(func() {
				widget.Spinner(c, 14)
				ui.Text(c, "Getting the pictures…").FontSize(theme.TextUI).TextColor(theme.TextSoft)
			})
		default:
			v.size = min(v.size, len(o.Shots)-1)
			v.slider(c, o.Shots[v.size], maxW, maxH)
		}
	})
	if !open && o.Close != nil {
		o.Close()
	}
}

// bar is the comparison's top: the sizes, the scores, closing it.
func (v *Compare) bar(c *ui.Context, o CompareOptions) {
	widget.Rim(ui.Row(c.Key("compare-bar")).Gap(theme.S3).AlignItems(ui.Center).Height(48).Padding(0, theme.S2, 0, theme.S4).Radius(theme.RadiusFull).Background(theme.Popover)).Children(func() {
		ui.Text(c, "Before and after").FontSize(theme.TextUI).FontWeight(600).TextColor(theme.TextStrong).SingleLine()
		if len(o.Shots) > 1 {
			labels := make([]string, len(o.Shots))
			for i, s := range o.Shots {
				labels[i] = s.Label
			}
			if i := widget.Segmented(c, labels, min(v.size, len(labels)-1), nil); i >= 0 {
				v.size = i // -1: none clicked
			}
		}
		ui.Row(c).Gap(4).AlignItems(ui.End).Children(func() {
			ui.Text(c, fmt.Sprint(o.From)).FontSize(theme.TextUI).TextColor(scoreColor(o.From))
			ui.Icon(c, icon.ArrowRight).FontSize(12).TextColor(theme.TextSoft)
			ui.Text(c, fmt.Sprint(o.To)).FontSize(theme.TextUI).FontWeight(500).TextColor(scoreColor(o.To))
			ui.Text(c, "/100").FontSize(theme.TextLabel).TextColor(theme.TextSoft)
		})
		if widget.IconButton(c, icon.Close, "Close", false).Clicked() && o.Close != nil {
			o.Close()
		}
	})
}

// slider is shot's pictures, fitted in maxW × maxH, and the divider.
func (v *Compare) slider(c *ui.Context, s Shot, maxW, maxH float32) {
	bw, bh := picSize(s.Before)
	aw, ah := picSize(s.After)
	if s.After == nil {
		aw, ah = bw, bh
	}
	scale := min(maxW/max(bw, aw), maxH/max(bh, ah), 1)
	w, h := max(bw, aw)*scale, max(bh, ah)*scale
	box := ui.Box(c.Key("compare-"+s.Label)).Size(w, h).Radius(theme.RadiusCard).Clip().Background(theme.Sunken).
		Cursor(ui.CursorResizeEW).Label("Drag to compare before and after")
	x, _, over := box.PointerPosition()
	switch pressed := box.Pressed(); {
	case pressed && (v.dragging || over):
		v.dragging = true
		v.at = min(max(x/w, 0), 1)
	case !pressed:
		v.dragging = false
	}
	cut := w * v.at
	box.Children(func() {
		if s.After != nil {
			ui.Image(c, s.After).Absolute().Left(0).Top(0).Size(aw*scale, ah*scale)
		}
		ui.Box(c).Absolute().Left(0).Top(0).Size(cut, h).Clip().Children(func() {
			ui.Image(c, s.Before).Size(bw*scale, bh*scale)
		})
		ui.Box(c).Absolute().Left(cut-1).Top(0).Size(2, h).Background(ui.RGB(255, 255, 255))
		ui.Row(c).Absolute().Left(cut-15).Top(h/2-15).Size(30, 30).Radius(theme.RadiusFull).Background(ui.RGB(255, 255, 255)).
			Justify(ui.Center).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icon.ChevronLeft).FontSize(12).TextColor(ui.RGB(30, 30, 30))
			ui.Icon(c, icon.ChevronRight).FontSize(12).TextColor(ui.RGB(30, 30, 30))
		})
		compareTag(c, "Before").Left(10)
		if s.After != nil {
			compareTag(c, "After").Right(10)
		} else {
			compareTag(c, "No after yet").Right(10)
		}
	})
}

func compareTag(c *ui.Context, label string) ui.Element {
	return ui.Text(c, label).Absolute().Top(10).FontSize(theme.TextLabel).FontWeight(500).TextColor(ui.RGB(255, 255, 255)).
		Padding(theme.S1, theme.S2).Radius(theme.RadiusFull).Background(ui.RGBA(0, 0, 0, 0.6))
}

func picSize(b *ui.Bitmap) (float32, float32) {
	if b == nil {
		return 1, 1
	}
	w, h := b.Size()
	return float32(max(w, 1)), float32(max(h, 1))
}

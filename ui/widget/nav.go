package widget

import (
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/theme"
)

// Breadcrumb shows where a view sits: muted parents, then the current
// place.
func Breadcrumb(c *ui.Context, parts ...string) {
	ui.Row(c).Gap(theme.S1).MinWidth(0).Shrink(1).Children(func() {
		for i, p := range parts {
			if i > 0 {
				ui.Text(c, "/").FontSize(theme.TextLabel).TextColor(theme.TextSoft)
			}
			t := ui.Text(c, p).FontSize(theme.TextLabel).SingleLine().MinWidth(0)
			if i == len(parts)-1 {
				t.TextColor(theme.Text).Shrink(1)
			} else {
				// The parts before the last give way first: where the view
				// is says the most.
				t.TextColor(theme.TextSoft).Shrink(20)
			}
		}
	})
}

// Segmented is a row of mutually exclusive options, a well of capsules; it
// returns the option clicked, or -1. disabled options are dimmed and can't
// be picked.
func Segmented(c *ui.Context, options []string, current int, disabled func(i int) bool) int {
	clicked := -1
	ui.Row(c).Shrink(0).Padding(theme.S1).Radius(theme.RadiusFull).Background(theme.Sunken).Children(func() {
		for i, label := range options {
			off := disabled != nil && disabled(i)
			b := ui.ButtonBase(c.Key(label)).Height(28).PaddingX(theme.S3).Radius(theme.RadiusFull).Label(label)
			bg, fg, weight := theme.Fade(theme.Selected, 0), theme.TextSoft, 400
			switch {
			case i == current:
				bg, fg, weight = theme.Selected, theme.TextStrong, 500
			case off:
				b.Opacity(0.45)
			case b.Hovered():
				bg, fg = theme.Hover, theme.Text
			}
			if !off && i != current {
				b.Cursor(ui.CursorPointer)
			}
			b.Background(QuickTint(b, "bg", bg)).TextColor(QuickTint(b, "fg", fg))
			b.Children(func() { ui.Text(c, label).FontSize(theme.TextUI).FontWeight(weight).SingleLine() })
			if !off && i != current && b.Clicked() {
				clicked = i
			}
		}
	})
	return clicked
}

// The rail's entries keep one geometry whether it is wide or narrow:
// RailInset from its left edge, RailItemSize tall, the icon RailIconSize
// wide and as far into the entry as it is from the entry's top. The rail
// only gets wider or narrower to its right, so nothing moves sideways as
// it folds, the narrow rail being RailInset+RailItemSize+RailInset wide.
const (
	RailInset    = 8
	RailItemSize = 32
	RailIconSize = 14
	railIconPad  = (RailItemSize - RailIconSize) / 2
)

// RailButton is a square button of the host rail. on marks the current
// one, with the pill of group sliding to it; status, when not zero, is a
// connection dot at its corner.
func RailButton(c *ui.Context, svg *ui.SVG, label, group string, on bool, status ui.Color) ui.Element {
	return RailItem(c, svg, label, group, false, on, status)
}

// RailItem is an entry of the rail: a square with its icon while the rail
// is narrow, the same with its label (and the status dot after it) while
// wide. The icon sits at the same place in both.
func RailItem(c *ui.Context, svg *ui.SVG, label, group string, wide, on bool, status ui.Color) ui.Element {
	b := RailEntry(ui.ButtonBase(c), wide).Label(label).Cursor(ui.CursorPointer)
	if !wide {
		b.Tooltip(label)
	}
	return railFace(c, b, svg, label, group, on, status, wide)
}

// RailEntry gives e the rail's entry geometry: square while narrow, the
// rail's width while wide, its content starting where the icon goes.
func RailEntry(e ui.Element, wide bool) ui.Element {
	e.Height(RailItemSize).Shrink(0).PaddingX(railIconPad).Gap(theme.S2).Radius(theme.RadiusFull).
		AlignItems(ui.Center).Justify(ui.Start)
	if wide {
		return e.FillWidth()
	}
	return e.Width(RailItemSize)
}

// railFace draws a rail entry's colors, pill and content.
func railFace(c *ui.Context, b ui.Element, svg *ui.SVG, label, group string, on bool, status ui.Color, wide bool) ui.Element {
	bg, fg := theme.Fade(theme.Hover, 0), theme.TextSoft
	switch {
	case on:
		fg = theme.TextStrong
	case b.Hovered():
		bg, fg = theme.Hover, theme.Text
	}
	b.Background(QuickTint(b, "bg", bg)).TextColor(QuickTint(b, "fg", fg))
	b.Children(func() {
		if on {
			SelectionPill(c, group, theme.RadiusFull)
		}
		ui.Icon(c, svg).FontSize(RailIconSize)
		if !wide {
			if status.A != 0 {
				ui.Box(c).Size(8, 8).Radius(theme.RadiusFull).Background(status).
					AttachTo(b, ui.AnchorBottomRight, ui.AnchorBottomRight).Margin(0, 2, 2, 0)
			}
			return
		}
		ui.Text(c, label).FontSize(theme.TextUI).SingleLine().Shrink(1)
		ui.Spacer(c)
		if status.A != 0 {
			ui.Box(c).Size(8, 8).Radius(theme.RadiusFull).Background(status).Shrink(0)
		}
	})
	return Pressable(b)
}

// RailSection starts a group of the rail. It is as tall wide as narrow,
// so the entries under it stay put as the rail folds: its label while
// wide, which fades in, and a short hairline under the icons while narrow,
// both starting where the icons do.
func RailSection(c *ui.Context, label string, wide bool) {
	ui.Row(c).Height(32).FillWidth().Shrink(0).AlignItems(ui.End).
		Padding(0, theme.S3, theme.S1, theme.S3).Children(func() {
		if wide {
			SectionLabel(c.Key("label"), label, theme.TextSoft).
				Transition(ui.ElementTransition{Duration: 260 * time.Millisecond, Ease: Out, Enter: &ui.Motion{X: -6, Opacity: 0}})
			return
		}
		ui.Box(c.Key("line")).Size(RailIconSize, 0.5).Margin(0, 0, theme.S1, 0).Background(theme.Border)
	})
}

// SectionLabel is the small label over a group.
func SectionLabel(c *ui.Context, label string, col ui.Color) ui.Element {
	return ui.Text(c, label).FontSize(theme.TextLabel).FontWeight(500).TextColor(col).SingleLine()
}

// ListRow styles a clickable row of a list and fills it with fn: hovered,
// or selected with the pill of group, which slides from row to row.
func ListRow(c *ui.Context, row ui.Element, selected bool, group string, fn func()) ui.Element {
	row.FillWidth().MinHeight(36).Padding(theme.S1, theme.S3).Gap(theme.S2).Radius(theme.RadiusCard).Cursor(ui.CursorPointer)
	bg := theme.Fade(theme.Hover, 0)
	if !selected && row.Hovered() {
		bg = theme.Hover
	}
	row.Background(QuickTint(row, "bg", bg)).Children(func() {
		if selected {
			SelectionPill(c, group, theme.RadiusCard)
		}
		fn()
	})
	return row
}

// Avatar is the tile of an agent or host: an icon on a round well.
func Avatar(c *ui.Context, svg *ui.SVG, size float32, col ui.Color) {
	ui.Box(c).Size(size, size).Shrink(0).Radius(theme.RadiusFull).Background(theme.Sunken).
		Center().TextColor(col).Children(func() {
		ui.Icon(c, svg).FontSize(size * 0.55)
	})
}

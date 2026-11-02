package widget

import (
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/theme"
)

// The parts of a page that lists things and details the one picked.

// Pick is an entry of a page's list of things to pick from: a raised row
// with a title and a quieter line, which run inside it, lit while on.
func Pick(c *ui.Context, svg *ui.SVG, title, sub string, on bool, right func()) ui.Element {
	return PickFace(c, title+sub, title, on, func() {
		if svg != nil {
			ui.Icon(c, svg).FontSize(16).Shrink(0)
		}
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			Running(c, title, RunStyle{Weight: 500, Always: on})
			if sub != "" {
				Running(c.Key("sub"), sub, RunStyle{Size: theme.TextLabel, Color: theme.TextSoft, Always: on})
			}
		})
		if right != nil {
			ui.Row(c).Shrink(0).Gap(theme.S2).Children(right)
		}
	})
}

// PickFace is a Pick whose inside the caller draws, for entries that aren't a
// title and a line.
func PickFace(c *ui.Context, key, label string, on bool, face func()) ui.Element {
	b := ui.ButtonBase(c.Key("pick:"+key)).FillWidth().Shrink(0).MinHeight(44).Padding(theme.S3, theme.S4).Gap(theme.S3).Radius(theme.RadiusControl).
		Justify(ui.Start).Label(label).Cursor(ui.CursorPointer)
	bg := theme.Raised
	switch {
	case on:
		bg = theme.Selected
	case b.Hovered():
		bg = theme.Fade(theme.TextStrong, 0.12)
	}
	b.Background(QuickTint(b, "bg", bg)).InsetShadow(0, 0.5, 0, 0, theme.Border).Children(face)
	return b
}

// Heading names a part of a page, over its cards.
func Heading(c *ui.Context, text string) {
	ui.Row(c.Key("heading:" + text)).FillWidth().Shrink(0).MinHeight(20).Children(func() {
		Running(c, text, RunStyle{Weight: 500, Color: theme.TextSoft})
	})
}

// PageFill is what sits under a PageHeader when it fills the frame rather
// than scrolling: a master and its detail, each scrolling on its own.
func PageFill(c *ui.Context, fn func()) {
	ui.Column(c.Key("page-fill")).Grow(1).FillWidth().MinHeight(0).Padding(theme.S4, theme.S5, theme.S5, theme.S5).Gap(theme.S4).Children(fn)
}

// MasterDetail is a list of the given width, scrolling on its own, beside
// the detail of what's picked, which fills what is left of the height.
func MasterDetail(c *ui.Context, width float32, list, detail func()) {
	ui.Row(c.Key("master-detail")).Grow(1).FillWidth().MinHeight(0).AlignItems(ui.Stretch).Gap(theme.S3).Children(func() {
		master := ui.Scroll(c.Key("master")).Width(width).Shrink(0).FillHeight()
		master.Children(func() {
			// At least the scroll's height, so a last card can grow to the bottom.
			ui.Column(c).FillWidth().MinHeight(master.Bounds().H).Gap(theme.S2).Children(list)
		})
		ui.Column(c.Key("detail")).Grow(1).Basis(0).MinWidth(0).FillHeight().Gap(theme.S3).Children(detail)
	})
}

// Cols is a grid of cards in n columns of the same width.
func Cols(c *ui.Context, n int) ui.Element {
	return ui.Grid(c).FillWidth().Columns(n).Gap(theme.S3)
}

// FactCard is a small card of a grid: a title, a chip and a quieter line.
func FactCard(c *ui.Context, title, chip, sub string) {
	ui.Column(c.Key("fact:"+title+sub)).MinWidth(0).Padding(theme.S3, theme.S4).Gap(2).Radius(theme.RadiusCard).Background(theme.Raised).Children(func() {
		ui.Row(c).FillWidth().Gap(theme.S2).Children(func() {
			ui.Row(c).Grow(1).MinWidth(0).Children(func() { Running(c, title, RunStyle{Weight: 500}) })
			if chip != "" {
				Chip(c, chip, ui.Color{})
			}
		})
		if sub != "" {
			Running(c.Key("sub"), sub, RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
		}
	})
}

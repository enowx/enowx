package widget

import (
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/theme"
)

// The parts of a page of cards under its figures: a grid sharing the frame's
// height among its rows, and what a card is made of.

// FillGrid is a page's grid of n cards in cols columns. Its rows share the
// height of the frame, at least minRow each; with more cards than fit it
// scrolls. Each row has the height it's given whatever its cards hold, so a
// card holds its content, not the other way around.
func FillGrid(c *ui.Context, cols, n int, minRow float32, cards func()) {
	scroll := ui.Scroll(c.Key("fill-grid")).Grow(1).FillWidth().MinHeight(0)
	rows := max((n+cols-1)/cols, 1)
	row := minRow
	if h := scroll.Bounds().H; h > 0 {
		room := h - theme.S4 - theme.S5 - float32(rows-1)*theme.S3
		row = max(minRow, room/float32(rows))
	}
	tracks := make([]ui.Track, rows)
	for i := range tracks {
		tracks[i] = ui.Fixed(row)
	}
	scroll.Children(func() {
		ui.Grid(c).FillWidth().Padding(theme.S4, theme.S5, theme.S5, theme.S5).Columns(cols).RowTracks(tracks...).Gap(theme.S3).Children(cards)
	})
}

// FigsBand is a row of figures in its place between a page's header and its
// cards.
func FigsBand(c *ui.Context, figs []Fig) {
	ui.Column(c.Key("figs-band")).FillWidth().Shrink(0).Padding(theme.S4, theme.S5, 0, theme.S5).Children(func() { Figs(c, figs) })
}

// GridCard is a card filling its cell in a grid: what doesn't fit is clipped
// at its rounded edge rather than growing it.
func GridCard(c *ui.Context) ui.Element {
	return Card(c).MinHeight(0).Clip()
}

// CardTitle is a card's title, which runs when it can't fit, then what follows
// it at the right.
func CardTitle(c *ui.Context, svg *ui.SVG, title string, right func()) {
	ui.Row(c.Key("card-title")).FillWidth().Shrink(0).MinHeight(24).Gap(theme.S2).Children(func() {
		if svg != nil {
			ui.Icon(c, svg).FontSize(16).TextColor(theme.TextSoft)
		}
		ui.Row(c).Grow(1).MinWidth(0).Children(func() {
			Running(c, title, RunStyle{Size: theme.TextRead, Weight: 600})
		})
		if right != nil {
			right()
		}
	})
}

// StatusLine is a dot and what it says.
func StatusLine(c *ui.Context, col ui.Color, hollow bool, text string) {
	ui.Row(c.Key("status-line")).MinWidth(0).Shrink(1).Height(16).Gap(theme.S1).Children(func() {
		Dot(c, col, hollow, 8)
		ui.Row(c).MinWidth(0).Shrink(1).Children(func() { Running(c, text, RunStyle{Size: theme.TextLabel}) })
	})
}

// CardNote is a card's quiet line, wrapped inside it; at most lines of it show
// when lines isn't 0.
func CardNote(c *ui.Context, text string, lines int) ui.Element {
	t := ui.Text(c, text).FontSize(theme.TextLabel).TextColor(theme.TextSoft).FillWidth().MinWidth(0)
	if lines > 0 {
		t.MaxLines(lines)
	}
	return t
}

// CardFoot is a card's row of actions, at the bottom of a card whose middle
// grows: a Scroll, or a ui.Spacer.
func CardFoot(c *ui.Context, fn func()) {
	ui.Row(c.Key("card-foot")).FillWidth().Shrink(0).Gap(theme.S2).Children(fn)
}

// Stat is a figure of a card, in a row of equal ones.
type Stat struct{ Value, Label string }

// CardStats is a card's row of figures.
func CardStats(c *ui.Context, stats []Stat) {
	ui.Row(c.Key("card-stats")).FillWidth().Shrink(0).Gap(theme.S2).Children(func() {
		for _, s := range stats {
			ui.Column(c.Key(s.Label)).Grow(1).Basis(0).MinWidth(0).Gap(theme.S1).Children(func() {
				Running(c.Key("v"), s.Value, RunStyle{Size: theme.TextRead, Weight: 500})
				Running(c.Key("l"), s.Label, RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
			})
		}
	})
}

// CardLines is a list of a card: rows divided by hairlines.
func CardLines(c *ui.Context, fn func()) {
	ui.Column(c.Key("card-lines")).FillWidth().MinWidth(0).Children(fn)
}

// CardLine is one row of CardLines: a title and a quieter line, which run
// inside the row, and what follows at its right.
func CardLine(c *ui.Context, first bool, title, sub string, right func()) ui.Element {
	r := ui.Row(c).FillWidth().Shrink(0).MinHeight(32).Padding(theme.S1, 0).Gap(theme.S2)
	if !first {
		r.BorderWidth(0.5, 0, 0, 0).BorderColor(theme.Border)
	}
	r.Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			Running(c, title, RunStyle{Size: theme.TextLabel})
			if sub != "" {
				Running(c.Key("sub"), sub, RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
			}
		})
		if right != nil {
			ui.Row(c).Shrink(0).Gap(theme.S2).Children(right)
		}
	})
	return r
}

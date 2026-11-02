package widget

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
)

// PageBodyGrid is a wide PageBody around a grid of n cards in cols columns,
// whose rows share the height the frame has left under before and over after,
// at least minRow each; what doesn't fit scrolls. The rows' height comes from
// the frame and never from the cards, so a card holds its content (GridCard).
func PageBodyGrid(c *ui.Context, cols, n int, minRow float32, before, cards, after func()) {
	scroll := ui.Scroll(c.Key("page-body")).Grow(1).FillWidth().MinHeight(0)
	scroll.Children(func() {
		col := PagePad(ui.Column(c.Key("page-column")).FillWidth()).Gap(theme.S3)
		col.Children(func() {
			if before != nil {
				before()
			}
			grid := Columns(ui.Grid(c.Key("page-grid")).FillWidth(), cols).Gap(theme.S3)
			rows := max((n+cols-1)/cols, 1)
			row := minRow
			if h := scroll.Bounds().H; h > 0 {
				rest := h - (col.Bounds().H - grid.Bounds().H) // last frame's, as the rows only follow the frame
				row = max(minRow, (rest-float32(rows-1)*theme.S3)/float32(rows))
			}
			tracks := make([]ui.Track, rows)
			for i := range tracks {
				tracks[i] = ui.Fixed(row)
			}
			grid.RowTracks(tracks...).Children(cards)
			if after != nil {
				after()
			}
		})
	})
}

// PageBodyRest is a wide PageBody whose last part takes the height the frame
// has left under before, at least minRest; what doesn't fit scrolls. The height
// comes from the frame and never from the content, so a page's lowest cards
// reach its bottom edge.
func PageBodyRest(c *ui.Context, minRest float32, before func(), rest func(height float32)) {
	scroll := ui.Scroll(c.Key("page-body")).Grow(1).FillWidth().MinHeight(0)
	scroll.Children(func() {
		col := PagePad(ui.Column(c.Key("page-column")).FillWidth()).Gap(theme.S3)
		col.Children(func() {
			before()
			box := ui.Column(c.Key("page-rest")).FillWidth().MinWidth(0)
			h := minRest
			if sh := scroll.Bounds().H; sh > 0 {
				h = max(minRest, sh-(col.Bounds().H-box.Bounds().H)) // last frame's, as it only follows the frame
			}
			box.Height(h).Children(func() { rest(h) })
		})
	})
}

// SheetHead is a Sheet's header as FormSheet draws it, for a sheet whose footer
// isn't Cancel and a button: its title, a line about it and the ×.
func SheetHead(c *ui.Context, open *bool, title, description string) {
	ui.Column(c).Grow(1).MinWidth(0).Gap(theme.S1).Children(func() {
		ui.Text(c, title).FontSize(theme.TextTitle).FontWeight(600)
		if description != "" {
			ui.Text(c, description).FontSize(theme.TextUI).TextColor(theme.TextSoft)
		}
	})
	if IconButton(c, icon.Close, "Close", false).Clicked() {
		*open = false
	}
}

// InitialAvatar is a person's initial in a round well.
func InitialAvatar(c *ui.Context, name string, size float32) {
	initial := "?"
	if name != "" {
		initial = strings.ToUpper(string([]rune(name)[0]))
	}
	ui.Box(c).Size(size, size).Shrink(0).Radius(theme.RadiusFull).Background(theme.Hover).Center().Children(func() {
		ui.Text(c, initial).FontSize(theme.TextUI).FontWeight(600)
	})
}

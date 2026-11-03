package widget

import (
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/theme"
)

// RowsLeft are the row tracks of a grid at the end of a page's column, whose
// rows share what the frame has left below whatever comes before it, at least
// minRow each. shown is how much of the column its scroll shows; what grid and
// column measured last frame says what comes before. With more cards than fit
// the rows stay at minRow and the column scrolls.
func RowsLeft(grid, column ui.Element, shown float32, rows int, minRow float32) []ui.Track {
	rows = max(rows, 1)
	row := minRow
	if shown > 0 {
		above := grid.Bounds().Y - column.Bounds().Y
		row = max(minRow, (shown-above-theme.S5-float32(rows-1)*theme.S3)/float32(rows))
	}
	tracks := make([]ui.Track, rows)
	for i := range tracks {
		tracks[i] = ui.Fixed(row)
	}
	return tracks
}

package widget

import (
	"strconv"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/theme"
)

// The parts of a page that reports what happened: counts by kind, the latest
// records, a log's cards and what stands out above them.

// TwoParts lays a page's lower parts side by side, the second the wider.
func TwoParts(c *ui.Context, left, right func()) {
	ui.Row(c.Key("two-parts")).FillWidth().Shrink(0).Gap(theme.S4).AlignItems(ui.Start).Children(func() {
		ui.Column(c.Key("left")).Grow(5).Basis(0).MinWidth(0).Children(left)
		ui.Column(c.Key("right")).Grow(7).Basis(0).MinWidth(0).Children(right)
	})
}

// TallyRow is a count among others: what it counts, a meter against the
// largest of them, the count.
func TallyRow(c *ui.Context, label string, n, largest int) {
	ui.Row(c.Key("tally:" + label)).FillWidth().Shrink(0).MinHeight(theme.TabHeight).Gap(theme.S2).Children(func() {
		ui.Row(c).Width(172).Shrink(0).Children(func() { Running(c, label, RunStyle{Size: theme.TextLabel}) })
		ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
			fraction := 0.0
			if largest > 0 {
				fraction = float64(n) / float64(largest)
			}
			Meter(c, Segment{Fraction: fraction, Color: theme.Working})
		})
		ui.Row(c).Width(24).Shrink(0).Justify(ui.End).Children(func() {
			ui.Text(c, strconv.Itoa(n)).FontSize(theme.TextLabel).SingleLine()
		})
	})
}

// RecordRow is a line of a list of the latest: when, what, and how it went,
// under a hairline. What doesn't fit runs inside its row.
func RecordRow(c *ui.Context, when, what string, col ui.Color, status string) {
	ui.Row(c).FillWidth().Shrink(0).MinHeight(theme.TabHeight).Gap(theme.S2).BorderWidth(theme.Rule, 0, 0, 0).BorderColor(theme.Border).Children(func() {
		ui.Row(c).Width(44).Shrink(0).Children(func() { Running(c.Key("when"), when, RunStyle{Size: theme.TextLabel, Color: theme.TextSoft}) })
		ui.Row(c).Grow(1).Basis(0).MinWidth(0).Children(func() { Running(c.Key("what"), what, RunStyle{Size: theme.TextLabel}) })
		ui.Row(c).MaxWidth(160).Shrink(0).Children(func() { StatusLine(c, col, false, status) })
	})
}

// LogCardHeight is what a LogCard takes: its rows of a grid are all this.
const LogCardHeight = 100

// LogGrid is a log's cards, two a row, each row as tall as a LogCard whatever
// the card holds.
func LogGrid(c *ui.Context, n int, cards func()) {
	tracks := make([]ui.Track, (n+1)/2)
	for i := range tracks {
		tracks[i] = ui.Fixed(LogCardHeight)
	}
	ui.Grid(c.Key("log-grid")).FillWidth().Shrink(0).Columns(2).RowTracks(tracks...).GapX(theme.S3).GapY(theme.S2).Children(cards)
}

// LogCard is a record of a log: its title and how it went, a quieter line, and
// what's kept below. mono says the title is a command.
func LogCard(c *ui.Context, title string, mono bool, col ui.Color, status, meta string, tags, body func()) {
	ui.Column(c).MinWidth(0).MinHeight(0).Clip().Padding(theme.S3, theme.S4).Gap(2).Radius(theme.RadiusCard).Background(theme.Raised).Children(func() {
		ui.Row(c.Key("head")).FillWidth().Shrink(0).MinHeight(20).Gap(theme.S2).Children(func() {
			ui.Row(c).Grow(1).Basis(0).MinWidth(0).Children(func() { Running(c, title, RunStyle{Weight: 500, Mono: mono}) })
			if tags != nil {
				tags()
			}
			ui.Row(c).MaxWidth(160).Shrink(0).Children(func() { StatusLine(c, col, false, status) })
		})
		Running(c.Key("meta"), meta, RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
		if body != nil {
			body()
		}
	})
}

// BannerCard says what stands out at more length than a Banner: a card tinted
// by col with a title, a quieter note, what follows them, and what to do about
// it at the right.
func BannerCard(c *ui.Context, col ui.Color, title, note string, more, actions func()) {
	ui.Row(c).FillWidth().Shrink(0).Padding(theme.S3, theme.S4).Gap(theme.S3).Radius(theme.RadiusCard).Background(theme.Fade(col, 0.13)).Children(func() {
		ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, title).FontSize(theme.TextUI).FontWeight(500).FillWidth().MinWidth(0)
			if note != "" {
				CardNote(c, note, 0)
			}
			if more != nil {
				more()
			}
		})
		if actions != nil {
			ui.Row(c).Shrink(0).Gap(theme.S2).Children(actions)
		}
	})
}

// RunningChip is a Chip that gives way to its neighbours: its words run inside
// what room it gets.
func RunningChip(c *ui.Context, label string) {
	ui.Row(c.Key("chip:" + label)).Height(24).MinWidth(0).Shrink(1).PaddingX(theme.S2).Radius(theme.RadiusFull).Background(theme.Sunken).Children(func() {
		Running(c, label, RunStyle{Size: theme.TextLabel})
	})
}

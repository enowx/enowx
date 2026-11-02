package widget

import (
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/theme"
)

// Card is a raised surface for a group of facts.
func Card(c *ui.Context) ui.Element {
	return Raise(ui.Column(c).MinWidth(0).Gap(theme.S3).Padding(theme.S4).Radius(theme.RadiusCard).Background(theme.Raised))
}

// Segment is one part of a Meter.
type Segment struct {
	Fraction float64 // of the whole track, 0 to 1
	Color    ui.Color
}

// Meter is a thin track filled with segments, in order.
func Meter(c *ui.Context, parts ...Segment) {
	rest := 1.0
	ui.Row(c).FillWidth().Height(8).Radius(theme.RadiusFull).Clip().Background(theme.Sunken).Children(func() {
		for i, p := range parts {
			f := min(max(p.Fraction, 0), rest)
			rest -= f
			if f > 0 {
				ui.Box(c.Key(i)).Height(8).Radius(theme.RadiusFull).Grow(float32(f)).Background(p.Color)
			}
		}
		if rest > 0 {
			ui.Box(c.Key("rest")).Height(8).Grow(float32(rest))
		}
	})
}

// Legend is a meter's key: a dot and what it stands for.
func Legend(c *ui.Context, col ui.Color, label string) {
	ui.Row(c).Gap(theme.S1).Shrink(0).Children(func() {
		Dot(c, col, false, 8)
		ui.Text(c, label).FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine()
	})
}

// Level colors a fill by how full it is.
func Level(fill float64) ui.Color {
	switch {
	case fill > 0.9:
		return theme.Error
	case fill > 0.75:
		return theme.Warning
	}
	return theme.Working
}

// OptionCard is one of a group of exclusive choices with a title and a
// line about it; it reports whether it was clicked.
func OptionCard(c *ui.Context, on bool, title, body string) bool {
	card := ui.ButtonBase(c).MinWidth(0).Padding(theme.S3, theme.S4).Gap(theme.S2).Radius(theme.RadiusCard).
		AlignItems(ui.Start).Cursor(ui.CursorPointer).Label(title)
	bg := theme.Sunken
	switch {
	case on:
		bg = theme.Selected
	case card.Hovered():
		bg = theme.Fade(theme.TextStrong, 0.1)
	}
	card.Background(QuickTint(card, "bg", bg))
	card.Children(func() {
		ui.Column(c).Gap(theme.S1).Grow(1).MinWidth(0).Children(func() {
			ui.Text(c, title).FontSize(theme.TextUI).FontWeight(500)
			if body != "" {
				ui.Text(c, body).FontSize(theme.TextLabel).TextColor(theme.TextSoft)
			}
		})
	})
	return card.Clicked()
}

// Notice is a plate that tells something about the form it sits in; col
// colors its title and icon when not zero.
func Notice(c *ui.Context, svg *ui.SVG, col ui.Color, title, body string, extra func()) {
	titleCol := theme.Text
	if col.A != 0 {
		titleCol = col
	} else {
		col = theme.TextSoft
	}
	ui.Row(c).FillWidth().Gap(theme.S3).Padding(theme.S3, theme.S4).Radius(theme.RadiusCard).Background(theme.Sunken).
		AlignItems(ui.Start).Children(func() {
		if svg != nil {
			ui.Icon(c, svg).FontSize(16).TextColor(col).Margin(2, 0, 0, 0)
		}
		ui.Column(c).Gap(theme.S1).Grow(1).MinWidth(0).Children(func() {
			ui.Text(c, title).FontSize(theme.TextUI).FontWeight(500).TextColor(titleCol)
			if body != "" {
				ui.Text(c, body).FontSize(theme.TextLabel).TextColor(theme.TextSoft)
			}
			if extra != nil {
				extra()
			}
		})
	})
}

// StateCard tells what state a host is in: a badge colored by it, a title
// and message, then what can be done.
func StateCard(c *ui.Context, col ui.Color, svg *ui.SVG, title, msg string, badge, actions func()) {
	Rim(ui.Column(c).Width(460).MaxWidth(9999).Shrink(1).MinWidth(0).Padding(theme.S5).Gap(theme.S3).Radius(theme.RadiusPanel).
		Background(theme.Frame)).Children(func() {
		ui.Row(c).FillWidth().Gap(theme.S3).AlignItems(ui.Start).Children(func() {
			ui.Box(c).Size(40, 40).Shrink(0).Radius(theme.RadiusFull).Background(theme.Sunken).
				Center().TextColor(col).Children(func() {
				if badge != nil {
					badge()
				} else {
					ui.Icon(c, svg).FontSize(18)
				}
			})
			ui.Column(c).Grow(1).MinWidth(0).Gap(theme.S1).Children(func() {
				ui.Text(c, title).FontSize(theme.TextTitle).FontWeight(600)
				ui.Text(c, msg).FontSize(theme.TextUI).TextColor(theme.TextSoft)
			})
		})
		if actions != nil {
			ui.Row(c).FillWidth().Gap(theme.S2).Wrap().Justify(ui.End).Children(actions)
		}
	})
}

// EmptyState fills a view that has nothing yet: an icon, what is missing,
// and the one action that fixes it.
func EmptyState(c *ui.Context, svg *ui.SVG, title, body string, action func()) {
	ui.Column(c).Grow(1).FillWidth().Center().Gap(theme.S3).Children(func() {
		ui.Box(c).Size(48, 48).Radius(theme.RadiusFull).Background(theme.Sunken).Center().TextColor(theme.TextSoft).Children(func() {
			ui.Icon(c, svg).FontSize(20)
		})
		ui.Text(c, title).FontSize(theme.TextTitle).FontWeight(600)
		ui.Text(c, body).FontSize(theme.TextUI).TextColor(theme.TextSoft).MaxWidth(420).TextAlign(ui.Center)
		if action != nil {
			action()
		}
	})
}

// Spinner says something is under way: MyGo's turning spokes, as high as
// size.
func Spinner(c *ui.Context, size float32) {
	ui.Spinner(c).FontSize(size).TextColor(theme.Now).Shrink(0)
}

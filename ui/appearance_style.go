package ui

import (
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// stylePicker is the app's styles, each a card of the same fixed size with a sample
// in that style's own shape and colors, whichever is in use.
func (a *App) stylePicker(c *ui.Context) {
	ui.Column(c.Key("style-picker")).FillWidth().Gap(theme.S2).Children(func() {
		ui.Text(c, "Style").FontSize(theme.TextUI).FontWeight(500)
		ui.Grid(c).FillWidth().Columns(4).Gap(theme.S3).Children(func() {
			current := theme.StyleByID(a.state.UIStyle).ID
			for _, s := range theme.Styles {
				if a.styleCard(c.Key(s.ID), s, s.ID == current) && s.ID != current {
					a.state.UIStyle = s.ID
					if s.ID == theme.LiquidGlass.ID {
						a.state.UIStyle = ""
					}
					a.applyTheme()
				}
			}
		})
	})
}

// styleCard is one style to pick; it says whether it was clicked.
func (a *App) styleCard(c *ui.Context, s theme.Style, on bool) bool {
	card := ui.ButtonBase(c).MinWidth(0).Padding(theme.S2).Radius(theme.RadiusCard).Label(s.Name).Cursor(ui.CursorPointer)
	switch {
	case on:
		card.Background(theme.Sunken).InsetShadow(0, 0, 0, 2, theme.Text)
	case card.Hovered():
		card.Background(theme.Hover)
	default:
		card.Background(theme.Sunken)
	}
	ground, pane, accent := theme.StyleColors(s, a.state.palette(a.lightAppearance()))
	card.Children(func() {
		ui.Column(c).FillWidth().MinWidth(0).Gap(theme.S2).Children(func() {
			// The sample: two panes on the style's ground, spaced and cornered as it
			// spaces and corners the window's.
			ui.Row(c).FillWidth().Height(56).Radius(theme.S3).Clip().Background(theme.Solid(ground)).
				Padding(min(s.WindowInset, 6)).Gap(min(s.BentoGap, 6)).Children(func() {
				corner := min(s.RadiusPanel, 10)
				ui.Box(c).Grow(1).Basis(0).FillHeight().Radius(corner).Background(pane)
				ui.Box(c).Grow(2).Basis(0).FillHeight().Radius(corner).Background(pane).Center().Children(func() {
					ui.Box(c).Size(20, 6).Radius(min(s.RadiusFull, 3)).Background(accent)
				})
			})
			widget.Running(c, s.Name, widget.RunStyle{Size: theme.TextLabel})
		})
	})
	return card.Clicked()
}

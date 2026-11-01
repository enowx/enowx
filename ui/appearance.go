package ui

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// applyTheme sets the palette the state names, in every view and
// terminal at once.
func (a *App) applyTheme() {
	theme.Apply(a.state.palette(a.lightAppearance()))
	a.theme = theme.Window()
	t := theme.Terminal()
	for _, p := range a.panes {
		if p != nil {
			p.term.SetTheme(t, t)
		}
	}
	if a.win != nil {
		_ = a.win.SetBackgroundColor(theme.Hex(theme.Background))
		a.win.Invalidate()
	}
	if a.floating != nil {
		a.floating.Invalidate()
	}
}

// appearancePage picks the palette and, optionally, an accent of the
// user's own.
func (a *App) appearancePage(c *ui.Context) {
	a.viewHeader(c, func() {
		ui.Row(c).Grow(1).MinWidth(0).Children(func() { widget.Breadcrumb(c, "Settings", "Appearance") })
		if widget.IconButton(c, icon.Close, "Back to sessions", false).Clicked() {
			a.page = ""
		}
	})
	ui.Scroll(c).Grow(1).FillWidth().Children(func() {
		ui.Column(c).FillWidth().Padding(20).Gap(14).Children(func() {
			a.appearanceMode(c)
			widget.SectionLabel(c, "THEME", theme.Muted)
			widget.FitGrid(c, 260, 4).Gap(12).Children(func() {
				for _, preset := range theme.Palettes {
					p := theme.LookupAppearance(preset.ID, ui.Color{}, a.lightAppearance())
					if a.paletteCard(c.Key(p.ID), p) {
						a.state.Theme = p.ID
						if p.ID == theme.Default.ID {
							a.state.Theme = ""
						}
						a.applyTheme()
					}
				}
			})

			widget.SectionLabel(c, "ACCENT", theme.Muted).Margin(14, 0, 0, 0)
			ui.Text(c, "Give any theme an accent of your own: its buttons, selection and focus take it.").
				FontSize(12).TextColor(theme.Muted)
			ui.Row(c).FillWidth().Gap(10).Children(func() {
				own := a.state.Accent == ""
				if a.swatch(c.Key("default"), theme.LookupAppearance(a.state.Theme, ui.Color{}, a.lightAppearance()).Accent, own, "Theme's own") {
					a.setAccent("")
				}
				ui.Box(c).Width(1).Height(24).Background(theme.Border).Margin(0, 4)
				for _, col := range theme.Accents {
					hex := theme.Hex(col)
					if a.swatch(c.Key(hex), col, strings.EqualFold(a.state.Accent, hex), hex) {
						a.setAccent(hex)
					}
				}
			})
			ui.Row(c).Gap(10).Children(func() {
				ui.Text(c, "Custom").FontSize(theme.TextLabel).TextColor(theme.TextSoft)
				in := widget.Input(c, &a.accentInput, "#D8C3A0").Width(130).Font(theme.MonoFont)
				if in.Changed() {
					if hex, ok := parseAccent(a.accentInput); ok {
						a.setAccent(hex)
					}
				}
				if a.accentInput != "" {
					if _, ok := parseAccent(a.accentInput); !ok {
						ui.Text(c, "Type a color as #RRGGBB.").FontSize(theme.TextLabel).TextColor(theme.Warning)
					}
				}
			})
		})
	})
}

func (a *App) setAccent(hex string) {
	a.state.Accent = hex
	a.applyTheme()
}

// parseAccent reads #RRGGBB, with or without the #.
func parseAccent(s string) (string, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) != 6 || strings.Trim(strings.ToLower(s), "0123456789abcdef") != "" {
		return "", false
	}
	return "#" + strings.ToUpper(s), true
}

// paletteCard previews a palette with its own colors, whatever the
// current one; it reports whether it was clicked.
func (a *App) paletteCard(c *ui.Context, p theme.Palette) bool {
	on := theme.Lookup(a.state.Theme, ui.Color{}).ID == p.ID
	card := ui.ButtonBase(c).MinWidth(0).Padding(10).Gap(10).Radius(theme.RadiusCard).Cursor(ui.CursorPointer).Label(p.Name).
		Background(theme.Raised)
	switch {
	case on:
		card.Border(1, theme.AccentLine)
	case card.Hovered():
		card.Border(1, theme.BorderStrong)
	default:
		card.Border(1, theme.Border)
	}
	card.Children(func() {
		ui.Column(c).FillWidth().Children(func() {
			ui.Row(c).FillWidth().Height(74).Radius(theme.RadiusSmall).Clip().Background(p.Background).Border(1, p.Border).Children(func() {
				ui.Column(c).Width(18).FillHeight().Background(p.Panel).Padding(5, 0).Gap(4).AlignItems(ui.Center).Children(func() {
					ui.Box(c).Size(9, 9).Radius(theme.RadiusTail).Background(p.Selected).Border(1, p.Accent)
					ui.Box(c).Size(9, 9).Radius(theme.RadiusTail).Background(p.Selected)
				})
				ui.Column(c).Width(50).FillHeight().Background(p.Panel).Padding(6).Gap(4).
					BorderWidth(0, 1, 0, 1).BorderColor(p.Border).Children(func() {
					for i, col := range []ui.Color{p.Working, p.Warning, p.Idle} {
						row := ui.Row(c.Key(i)).FillWidth().Height(10).Radius(theme.RadiusTail).Gap(3).PaddingX(3)
						if i == 0 {
							row.Background(p.Selected)
						}
						row.Children(func() {
							ui.Box(c).Size(4, 4).Radius(theme.RadiusTail).Background(col)
							ui.Box(c).Height(2).Grow(1).Radius(theme.RadiusTail).Background(p.Muted)
						})
					}
				})
				ui.Column(c).Grow(1).FillHeight().Padding(7).Gap(4).Children(func() {
					ui.Box(c).Height(2).Width(44).Radius(theme.RadiusTail).Background(p.TextSoft)
					ui.Box(c).Height(2).Width(30).Radius(theme.RadiusTail).Background(p.Muted)
					ui.Spacer(c)
					ui.Row(c).Gap(4).Children(func() {
						ui.Box(c).Size(30, 10).Radius(theme.RadiusTail).Background(p.Accent)
						ui.Box(c).Size(22, 10).Radius(theme.RadiusTail).Border(1, p.BorderStrong)
					})
				})
			})
		})
		ui.Row(c).FillWidth().Gap(8).Children(func() {
			ui.Text(c, p.Name).FontSize(theme.TextUI).Grow(1).SingleLine()
			if on {
				ui.Icon(c, icon.Check).FontSize(13).TextColor(theme.Accent)
			}
		})
	})
	return card.Clicked()
}

// swatch is one accent to pick; it reports whether it was clicked.
func (a *App) swatch(c *ui.Context, col ui.Color, on bool, label string) bool {
	b := ui.ButtonBase(c).Size(30, 30).Radius(theme.RadiusCard).Center().Shrink(0).Cursor(ui.CursorPointer).Label(label).Tooltip(label)
	if theme.Current().Light {
		b.Border(1, theme.BorderStrong)
	}
	switch {
	case on:
		b.Border(1.5, theme.Text)
	case b.Hovered():
		b.Border(1.5, theme.BorderStrong)
	}
	b.Children(func() {
		ui.Box(c).Size(22, 22).Radius(theme.RadiusSmall).Background(col).InsetShadow(0, 1, 0, 0, ui.RGBA(255, 255, 255, 0.3))
	})
	return b.Clicked()
}

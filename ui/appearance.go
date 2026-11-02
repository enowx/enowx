package ui

import (
	"strings"

	"github.com/egoist/mygo/ui"

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

// appearancePage picks the palette, the mode and, optionally, an accent of the
// user's own, beside how they look together.
func (a *App) appearancePage(c *ui.Context) {
	widget.PageHeader(c, "Appearance", "On this device", nil)
	body := ui.Scroll(c.Key("appearance-body")).Grow(1).FillWidth().MinHeight(0)
	room := body.Bounds().H
	body.Children(func() {
		ui.Row(c).FillWidth().MinHeight(room).Padding(theme.S4, theme.S5, theme.S5, theme.S5).Gap(theme.S4).AlignItems(ui.Stretch).Children(func() {
			ui.Column(c).Grow(7).Basis(0).MinWidth(0).Gap(theme.S4).Children(func() {
				a.themePicker(c)
				ui.Grid(c).FillWidth().Grow(1).Columns(2).Gap(theme.S3).AlignItems(ui.Stretch).Children(func() {
					widget.Card(c).Children(func() { a.appearanceMode(c) })
					widget.Card(c).Children(func() { a.accentPicker(c) })
				})
			})
			widget.Card(c).Grow(5).Basis(0).Children(func() { a.themePreview(c) })
		})
	})
}

// themePicker is the palettes, each in its own colors whatever the current one.
func (a *App) themePicker(c *ui.Context) {
	ui.Column(c).FillWidth().Gap(theme.S2).Children(func() {
		ui.Text(c, "Theme").FontSize(theme.TextUI).FontWeight(500)
		ui.Grid(c).FillWidth().Columns(len(theme.Palettes)).Gap(theme.S3).Children(func() {
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
	})
}

// accentPicker is the accent: the theme's own, one of the app's, or one typed.
func (a *App) accentPicker(c *ui.Context) {
	widget.CardTitle(c, nil, "Accent", nil)
	ui.Text(c, "Give any theme an accent of your own: its buttons, selection and focus take it.").
		FontSize(theme.TextLabel).TextColor(theme.TextSoft)
	ui.Row(c).FillWidth().Wrap().Gap(theme.S2).Children(func() {
		if widget.Button(c.Key("default"), widget.Secondary, nil, "Theme's own", "").Clicked() {
			a.setAccent("")
		}
		for _, col := range theme.Accents {
			hex := theme.Hex(col)
			if a.swatch(c.Key(hex), col, strings.EqualFold(a.state.Accent, hex), hex) {
				a.setAccent(hex)
			}
		}
	})
	widget.Field(c, "Custom", func() {
		in := widget.Input(c, &a.accentInput, "#RRGGBB").FillWidth().Font(theme.MonoFont).Label("Custom accent")
		if in.Changed() {
			if hex, ok := parseAccent(a.accentInput); ok {
				a.setAccent(hex)
			}
		}
		col := theme.TextSoft
		if a.accentInput != "" {
			if _, ok := parseAccent(a.accentInput); !ok {
				col = theme.Warning
			}
		}
		ui.Text(c, "Type a color as #RRGGBB.").FontSize(theme.TextLabel).TextColor(col)
	})
}

// themePreview shows the theme on the controls and states the app is made of.
// Nothing in it does anything: it is there to be looked at.
func (a *App) themePreview(c *ui.Context) {
	widget.CardTitle(c, nil, "With this theme", nil)
	ui.Row(c).FillWidth().Wrap().Gap(theme.S2).Children(func() {
		widget.PrimaryButton(c, nil, "Primary", "")
		widget.Button(c, widget.Secondary, nil, "Secondary", "")
		on := true
		widget.Switch(c, &on, "A switch")
		widget.Segmented(c.Key("sample"), []string{"Selected", "Not selected"}, 0, nil)
	})
	ui.Column(c).FillWidth().Children(func() {
		for i, s := range []struct {
			name, state string
			col         ui.Color
		}{
			{"GOA session tabs", "Working", theme.Working},
			{"Host keys to the vault", "Waiting on you", theme.Warning},
			{"Dialog footers", "Failed", theme.Error},
		} {
			row := ui.Row(c.Key(s.name)).FillWidth().MinHeight(32).Gap(theme.S2)
			if i > 0 {
				row.BorderWidth(0.5, 0, 0, 0).BorderColor(theme.Border)
			}
			row.Children(func() {
				ui.Text(c, s.name).FontSize(theme.TextLabel).Grow(1).MinWidth(0).SingleLine()
				widget.Legend(c, s.col, s.state)
			})
		}
	})
	widget.CodeBlockText(c, "$ go test ./ui\nok  github.com/enowx/enowx/ui  6.1s").Grow(1).Clip()
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

// paletteCard is a palette's ground and accent, and its name; it reports whether
// it was clicked.
func (a *App) paletteCard(c *ui.Context, p theme.Palette) bool {
	on := theme.Lookup(a.state.Theme, ui.Color{}).ID == p.ID
	card := ui.ButtonBase(c).MinWidth(0).Padding(theme.S2).Radius(theme.RadiusCard).Cursor(ui.CursorPointer)
	switch {
	case on:
		card.Background(theme.Sunken).InsetShadow(0, 0, 0, 2, theme.Text)
	case card.Hovered():
		card.Background(theme.Hover)
	default:
		card.Background(theme.Sunken)
	}
	card.Children(func() {
		ui.Column(c).FillWidth().MinWidth(0).Gap(theme.S2).Children(func() {
			ui.Row(c).FillWidth().Height(56).Radius(theme.S3).Clip().Children(func() {
				ui.Box(c).Grow(2).Basis(0).FillHeight().Background(p.Background)
				ui.Box(c).Grow(1).Basis(0).FillHeight().Background(p.Accent)
			})
			widget.Running(c, p.Name, widget.RunStyle{Size: theme.TextLabel})
		})
	})
	return card.Clicked()
}

// swatch is one accent to pick; it reports whether it was clicked.
func (a *App) swatch(c *ui.Context, col ui.Color, on bool, label string) bool {
	b := ui.ButtonBase(c).Size(32, 32).Radius(theme.RadiusFull).Shrink(0).Cursor(ui.CursorPointer).Label(label).Tooltip(label).Background(col)
	switch {
	case on:
		b.InsetShadow(0, 0, 0, 2, theme.Text)
	case b.Hovered():
		b.InsetShadow(0, 0, 0, 2, theme.TextSoft)
	default:
		b.InsetShadow(0, 0, 0, 0.5, theme.Border)
	}
	return b.Clicked()
}

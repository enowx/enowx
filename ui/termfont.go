package ui

import (
	"slices"
	"strings"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// The font terminals draw with: Settings › Terminal picks it among this
// device's monospaced fonts, so a Nerd Font shows the icons agents print.

// termFont is the terminals' font: the one picked, the system's monospaced
// font when none.
func (a *App) termFont() terminal.Font {
	return terminal.Font{Family: a.state.TermFont, Size: a.state.TermFontSize}
}

// setTermFont changes the font of the terminals already open, and the one
// the next ones take.
func (a *App) setTermFont(family string) {
	if a.state.TermFont == family {
		return
	}
	a.state.TermFont = family
	a.applyTermFont()
}

// applyTermFont gives every open terminal the font, a shared session's
// watched too.
func (a *App) applyTermFont() {
	f := a.termFont()
	for _, p := range a.panes {
		if p != nil {
			p.term.SetFont(f)
		}
	}
	if a.visit != nil && a.visit.watch != nil && a.visit.watch.term != nil {
		a.visit.watch.term.SetFont(f)
	}
}

// fontFamilies are the monospaced fonts of this device, read once in the
// background: asking the system for them takes a while, and only
// monospaced ones suit a terminal's grid. Nil until they come, and after,
// on a device with none.
func (a *App) fontFamilies() []ui.FontFamily {
	if !a.fontsLoaded && !a.fontsLoading {
		a.fontsLoading = true
		go func() {
			var mono []ui.FontFamily
			for _, f := range ui.FontFamilies() {
				if f.Monospaced {
					mono = append(mono, f)
				}
			}
			slices.SortFunc(mono, func(x, y ui.FontFamily) int {
				return strings.Compare(strings.ToLower(x.Name), strings.ToLower(y.Name))
			})
			a.onMain(func() {
				a.fonts, a.fontsLoaded, a.fontsLoading = mono, true, false
			})
		}()
	}
	return a.fonts
}

// fontSection picks the font terminals draw with. Terminals are drawn on
// this device, whatever host runs them, so the fonts offered are this
// device's.
func (a *App) fontSection(c *ui.Context) {
	ui.Column(c.Key("font")).FillWidth().Gap(theme.S2).Children(func() {
		widget.Heading(c, "Font")
		families := a.fontFamilies()
		widget.SearchField(c, &a.fontQuery, "Find a font", icon.Search)
		a.fontList(c, families)
	})
}

// fontList lists the fonts the search matches, the system's first.
func (a *App) fontList(c *ui.Context, families []ui.FontFamily) {
	q := strings.ToLower(strings.TrimSpace(a.fontQuery))
	var rows []ui.FontFamily
	for _, f := range families {
		if q == "" || strings.Contains(strings.ToLower(f.Name), q) {
			rows = append(rows, f)
		}
	}
	widget.Cols(c, 2).Gap(theme.S2).Children(func() {
		a.fontRow(c, "", "System monospaced font")
		for _, f := range rows {
			a.fontRow(c, f.Name, f.Name)
		}
	})
	switch {
	case !a.fontsLoaded:
		widget.CardNote(c, "Looking for the fonts of this device…", 0)
	case len(families) == 0:
		widget.CardNote(c, "This device has no other monospaced font.", 0)
	case len(rows) == 0:
		widget.CardNote(c, "No monospaced font has that name.", 0)
	}
}

// fontRow is one font of the section's list, its name drawn in it.
func (a *App) fontRow(c *ui.Context, family, label string) {
	on := a.state.TermFont == family
	b := widget.PickFace(c, "font:"+family, label, on, func() {
		ui.Row(c).Grow(1).MinWidth(0).Children(func() {
			ui.Text(c, label).Font(family).FontSize(theme.TextUI).FontWeight(500).SingleLine().Shrink(1).MinWidth(0)
		})
		ui.Text(c, "Aa 0123").Font(family).FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine().Shrink(0)
	})
	if b.Clicked() && !on {
		a.setTermFont(family)
	}
}

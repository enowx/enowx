package ui

import (
	"math"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"

	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

func (a *App) floatingAccent() ui.Color {
	if hex, ok := parseAccent(a.state.FloatingAccent); ok {
		if theme.Current().Light {
			return theme.LookupAppearance(a.state.Theme, ui.Hex(hex), true).Accent
		}
		return ui.Hex(hex)
	}
	return theme.Accent
}

func (a *App) floatingTheme() *ui.Theme {
	if a.state.FloatingAccent == "" {
		return a.theme
	}
	t := *a.theme
	t.Accent = a.floatingAccent()
	t.AccentHover = theme.Mix(t.Accent, theme.TextStrong, 0.15)
	t.AccentPressed = theme.Mix(t.Accent, theme.Background, 0.2)
	t.Focus = theme.Fade(t.Accent, 0.5)
	return &t
}

func (a *App) floatingSurface() ui.Color {
	// Opaque: the panel is sliced out of a window of its own, with nothing
	// under it to tint.
	base := theme.Solid(theme.Panel)
	if a.state.FloatingTint {
		return theme.Mix(base, a.floatingAccent(), 0.12)
	}
	return base
}

func (a *App) floatingScale() float32 {
	if a.state.FloatingCompact {
		return 0.9
	}
	return 1
}

func (a *App) floatingWindowSize() int {
	return int(math.Ceil(float64(floatingSize) * float64(a.floatingScale())))
}

func (a *App) refreshFloatingAppearance() {
	if a.floating != nil {
		size := a.floatingWindowSize()
		a.floating.SetSize(size, size)
		a.floating.Invalidate()
	}
}

func (a *App) floatingAnimate(e ui.Element, key string, target float32, animation ui.Animation) float32 {
	if a.state.FloatingReduceMotion {
		e.Jump(key, target)
		return target
	}
	return e.AnimateTo(key, target, animation)
}

func (a *App) floatingAppearance(c *ui.Context) {
	ui.Column(c.Key("floating-appearance")).FillWidth().Gap(12).Margin(12, 0, 0, 0).TextColor(theme.Text).Children(func() {
		input := ui.Local[string](c.Root(), "floating-accent-input", func() string { return a.state.FloatingAccent })
		ui.Text(c, "Menu color").FontSize(12.5).TextColor(theme.TextStrong)
		ui.Text(c, "Choose a color for this menu, or follow the app theme.").FontSize(11.5).TextColor(theme.Muted)
		ui.Row(c).FillWidth().Gap(8).Children(func() {
			if a.swatch(c.Key("theme"), theme.Accent, a.state.FloatingAccent == "", "Follow app theme") {
				a.state.FloatingAccent, *input = "", ""
				a.refreshFloatingAppearance()
			}
			ui.Box(c).Width(1).Height(24).Background(theme.Border).Margin(0, 4)
			for _, color := range theme.Accents {
				hex := theme.Hex(color)
				if a.swatch(c.Key(hex), color, strings.EqualFold(a.state.FloatingAccent, hex), "Floating color "+hex) {
					a.state.FloatingAccent, *input = hex, hex
					a.refreshFloatingAppearance()
				}
			}
		})
		ui.Row(c).Gap(10).Children(func() {
			ui.Text(c, "Custom").FontSize(12).TextColor(theme.TextSoft)
			field := widget.Input(c, input, "#9DB6F5").Label("Custom floating color").Width(130).Font(theme.MonoFont).TextColor(theme.Text)
			if field.Changed() {
				if hex, ok := parseAccent(*input); ok {
					a.state.FloatingAccent = hex
					a.refreshFloatingAppearance()
				}
			}
			if *input != "" {
				if _, ok := parseAccent(*input); !ok {
					ui.Text(c, "Type a color as #RRGGBB.").FontSize(11.5).TextColor(theme.Warning)
				}
			}
		})
		if switchRowKey(c, "Tint menu segments switch", &a.state.FloatingTint, "Tint menu segments", "Add a subtle wash of your chosen color to the menu.") {
			a.refreshFloatingAppearance()
		}
		if switchRowKey(c, "Compact floating control switch", &a.state.FloatingCompact, "Compact size", "A smaller menu with the same actions and navigation.") {
			a.refreshFloatingAppearance()
		}
		if switchRowKey(c, "Reduce floating motion switch", &a.state.FloatingReduceMotion, "Reduce animation", "Open menus directly, without spinning or bouncing.") {
			a.refreshFloatingAppearance()
		}
		ui.Row(c).Gap(14).Children(func() {
			ui.Box(c).Size(100, 100).PassThrough().Draw(func(p *ui.Painter, r ui.Rect) {
				cx, cy := r.X+r.W/2, r.Y+r.H/2
				st := &floatingState{source: -1}
				for i, slice := range floatingSlicesFor(floatingWedges) {
					hover := float32(0)
					if i == 1 {
						hover = 1
					}
					p.FillPath(chart.Sector(cx, cy, 20, 46, slice), a.sliceFill(st, floatingAction{enabled: true}, hover))
				}
				hub := ui.Rect{X: cx - 15, Y: cy - 15, W: 30, H: 30}
				p.Fill(hub, theme.Solid(theme.Panel), 15)
				p.Stroke(hub, theme.BorderStrong, 15, 1)
				p.Icon(icon.Logo, ui.Rect{X: cx - 8, Y: cy - 8, W: 16, H: 16}, a.floatingAccent())
			})
			ui.Text(c, "Preview").FontSize(11.5).TextColor(theme.TextSoft)
		})
	})
}

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
	ui.Column(c.Key("floating-appearance")).FillWidth().Gap(theme.S3).TextColor(theme.Text).Children(func() {
		input := ui.Local[string](c.Root(), "floating-accent-input", func() string { return a.state.FloatingAccent })
		widget.CardNote(c, "Choose a color for this menu, or follow the app theme.", 0)
		follow := a.state.FloatingAccent == ""
		if switchRowKey(c, "Follow app theme switch", &follow, "Follow app theme", "") {
			a.state.FloatingAccent, *input = "", ""
			if !follow {
				a.state.FloatingAccent = theme.Hex(theme.Accents[0])
				*input = a.state.FloatingAccent
			}
			a.refreshFloatingAppearance()
		}
		// Wraps: a third of a narrow frame doesn't hold every swatch on a line.
		ui.Row(c).FillWidth().Gap(theme.S2).Wrap().Children(func() {
			for _, color := range theme.Accents {
				hex := theme.Hex(color)
				if a.swatch(c.Key(hex), color, strings.EqualFold(a.state.FloatingAccent, hex), "Floating color "+hex) {
					a.state.FloatingAccent, *input = hex, hex
					a.refreshFloatingAppearance()
				}
			}
		})
		widget.Field(c, "Custom", func() {
			field := widget.Input(c, input, "#9DB6F5").Label("Custom floating color").FillWidth().Font(theme.MonoFont).TextColor(theme.Text)
			if field.Changed() {
				if hex, ok := parseAccent(*input); ok {
					a.state.FloatingAccent = hex
					a.refreshFloatingAppearance()
				}
			}
			if *input != "" {
				if _, ok := parseAccent(*input); !ok {
					ui.Text(c, "Type a color as #RRGGBB.").FontSize(theme.TextLabel).TextColor(theme.Warning)
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
	})
}

// floatingPreview is the panel as the settings beside it draw it, centered.
func (a *App) floatingPreview(c *ui.Context) {
	const size, scale = 220, 2.2
	ui.Row(c.Key("floating-ring")).FillWidth().Shrink(0).Justify(ui.Center).Children(func() {
		ui.Box(c).Size(size, size).PassThrough().Draw(func(p *ui.Painter, r ui.Rect) {
			cx, cy := r.X+r.W/2, r.Y+r.H/2
			st := &floatingState{source: -1}
			for i, slice := range floatingSlicesFor(floatingWedges) {
				hover := float32(0)
				if i == 1 {
					hover = 1
				}
				p.FillPath(chart.Sector(cx, cy, 20*scale, 46*scale, slice), a.sliceFill(st, floatingAction{enabled: true}, hover))
			}
			const hubR, logoR = 15 * scale, 8 * scale
			hub := ui.Rect{X: cx - hubR, Y: cy - hubR, W: 2 * hubR, H: 2 * hubR}
			p.Fill(hub, theme.Solid(theme.Panel), hubR)
			p.Stroke(hub, theme.BorderStrong, hubR, 1)
			p.Icon(icon.Logo, ui.Rect{X: cx - logoR, Y: cy - logoR, W: 2 * logoR, H: 2 * logoR}, a.floatingAccent())
		})
	})
}

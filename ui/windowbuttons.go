package ui

import (
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
)

// The window's close, minimize and maximize buttons, drawn by the app so
// they look the same on macOS, Windows and Linux: three dots on the left,
// their signs showing while the pointer is over them, grey while the
// window isn't the focused one. The green one does what the system's
// does: full screen on macOS (Option+click zooms), maximize elsewhere.

// windowDots are the buttons' colors, as people know them.
var windowDots = [3]ui.Color{ui.Hex("#FF5F57"), ui.Hex("#FEBC2E"), ui.Hex("#28C840")}

// windowControl is what the buttons do to the window: a *mygo.Window,
// or a test's.
type windowControl interface {
	Close()
	Minimize()
	ToggleMaximize()
	ToggleFullScreen()
}

// windowButtons draws them in the title bar.
func (a *App) windowButtons(c *ui.Context) {
	win := a.winCtl
	if win == nil && a.win != nil {
		win = a.win
	}
	if win == nil {
		return
	}
	group := ui.Row(c.Key("window-buttons")).Gap(theme.S2).Shrink(0)
	over := group.Hovered()
	group.Children(func() {
		for i, label := range []string{"Close", "Minimize", "Zoom"} {
			col := windowDots[i]
			if a.blurred && !over {
				col = theme.Idle
			}
			b := ui.ButtonBase(c.Key(label)).Size(12, 12).Radius(theme.RadiusSmall).Shrink(0).Background(col).Label(label).Center()
			b.Children(func() {
				if !over {
					return
				}
				sign := ui.RGBA(0, 0, 0, 0.55)
				switch i {
				case 0:
					ui.Icon(c, icon.Close).FontSize(8).TextColor(sign)
				case 1:
					ui.Box(c).Size(6, 1.2).Background(sign)
				default:
					ui.Icon(c, icon.Plus).FontSize(8).TextColor(sign)
				}
			})
			if b.Clicked() {
				switch i {
				case 0:
					win.Close()
				case 1:
					win.Minimize()
				case 2:
					if zoomGoesFullScreen && b.ClickModifiers()&ui.Alt == 0 {
						win.ToggleFullScreen()
					} else {
						win.ToggleMaximize()
					}
				}
			}
		}
	})
}

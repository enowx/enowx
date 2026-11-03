package ui

import (
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// wallpaper is what the glass sits on: three soft lights on the palette's
// ground, placed by the window's size so that it doesn't matter how it is
// resized.
func (a *App) wallpaper(c *ui.Context) {
	a.keepBackground()
	if theme.Flat {
		return // what shows is the ground alone
	}
	if a.background.bmp != nil {
		a.backgroundImage(c)
		return
	}
	if a.state.BackgroundPlain {
		return
	}
	w, h := c.Size()
	sx, sy := w/1440, h/900
	wall := theme.Current().Wall
	ui.Box(c.Key("wallpaper")).Absolute().Left(0).Top(0).Right(0).Bottom(0).Clip().PassThrough().Children(func() {
		light := func(col ui.Color, x, y, lw, lh float32) {
			l := ui.Box(c).Absolute().Left(x*sx).Top(y*sy).Size(lw*sx, lh*sy).Radius(theme.RadiusFull).Background(col)
			if widget.Frosted {
				l.Blur(70)
			}
		}
		light(wall[0], 160, -80, 640, 420)
		light(wall[1], 900, 220, 620, 520)
		light(wall[2], -60, 570, 520, 420)
	})
}

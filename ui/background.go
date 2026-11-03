package ui

import (
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"

	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// The user's own background image, kept by path in this device's state: another
// device has its own files, so it isn't synced, and each device picks its own.

const (
	backgroundMaxFile   = 64 << 20  // what is read at all
	backgroundMaxPixels = 120 << 20 // what is decoded: a small file can claim a huge image
	backgroundMaxSide   = 2560      // what is kept, scaled down, of a larger one
	// The panels' opacity and the blur's sigma, until the user sets them.
	backgroundDefaultOpacity = 0.8
	backgroundDefaultBlur    = 12
	backgroundMaxBlur        = 40
)

// backgroundState is the image as read. It belongs to the main thread.
type backgroundState struct {
	path string // what bmp, err and the state's derived colors are of
	bmp  *ui.Bitmap
	err  string
}

// readBackground reads the image at path, scaled down so that a large one
// costs no more memory than a screen's worth.
func readBackground(path string) (image.Image, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("the background image isn't there: %s", filepath.Base(path))
	}
	if info.Size() > backgroundMaxFile {
		return nil, fmt.Errorf("the background image %s is over 64 MB: use a smaller one", filepath.Base(path))
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("the background image %s can't be read", filepath.Base(path))
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return nil, fmt.Errorf("%s isn't a PNG, JPEG, GIF or WebP image", filepath.Base(path))
	}
	if cfg.Width*cfg.Height > backgroundMaxPixels {
		return nil, fmt.Errorf("the background image %s is too large: %d×%d", filepath.Base(path), cfg.Width, cfg.Height)
	}
	if _, err := f.Seek(0, 0); err != nil {
		return nil, err
	}
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("the background image %s can't be read", filepath.Base(path))
	}
	b := img.Bounds()
	longest := max(b.Dx(), b.Dy())
	if longest <= backgroundMaxSide {
		return img, nil
	}
	out := image.NewRGBA(image.Rect(0, 0, b.Dx()*backgroundMaxSide/longest, b.Dy()*backgroundMaxSide/longest))
	xdraw.ApproxBiLinear.Scale(out, out.Bounds(), img, b, xdraw.Src, nil)
	return out, nil
}

// keepBackground reads the image the state names when it isn't what was read.
func (a *App) keepBackground() {
	path := a.state.BackgroundImage
	if path == a.background.path {
		return
	}
	a.background = backgroundState{path: path}
	if path == "" {
		a.setDerived(nil)
		return
	}
	go func() {
		img, err := readBackground(path)
		var bmp *ui.Bitmap
		var derived *theme.Derived
		if err == nil {
			bmp = ui.NewBitmap(img)
			d := theme.DeriveColors(img)
			derived = &d
		}
		a.onMain(func() {
			a.backgroundRead(path, bmp, err)
			if a.background.path == path {
				a.setDerived(derived)
			}
		})
	}()
}

// setDerived keeps the colors of the image read, and has a palette following
// them change at once. nil goes back to the palette last saved.
func (a *App) setDerived(d *theme.Derived) {
	a.state.derived = d
	if a.state.FollowBackground {
		a.applyTheme()
	}
}

// backgroundRead takes what was read of path, unless another was picked since.
// What can't be read leaves the theme's own background, and says why.
func (a *App) backgroundRead(path string, bmp *ui.Bitmap, err error) {
	if a.background.path != path {
		return
	}
	a.background.bmp = bmp
	if err != nil {
		a.background.err = err.Error()
		a.status = err.Error()
	}
}

// backgroundImage draws the image, blurred as the user set, with the scrim that
// keeps words readable. It reaches past the window by as far as the blur fades.
func (a *App) backgroundImage(c *ui.Context) {
	w, h := c.Size()
	sigma := float32(a.state.backgroundBlur())
	reach := 3 * sigma
	ui.Box(c.Key("wallpaper")).Absolute().Left(0).Top(0).Right(0).Bottom(0).Clip().PassThrough().Children(func() {
		img := ui.Image(c.Key("background"), a.background.bmp).Absolute().Left(-reach).Top(-reach).
			Size(w+2*reach, h+2*reach).Fit(ui.Cover)
		if widget.Frosted && sigma > 0 {
			img.Blur(sigma)
		}
		ui.Box(c.Key("scrim")).Absolute().Left(0).Top(0).Right(0).Bottom(0).Background(theme.Fade(theme.Ground, theme.BackdropScrim*theme.SurfaceOpacity))
	})
}

// chooseBackground lets the user pick an image of this device.
func (a *App) chooseBackground() {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.win, Title: "Choose a background image",
			Filters: []mygo.FileFilter{{Name: "Images", Extensions: []string{"png", "jpg", "jpeg", "gif", "webp"}}}})
		a.onMain(func() {
			if err != nil {
				a.status = err.Error()
				return
			}
			if len(paths) == 0 {
				return
			}
			a.state.BackgroundImage, a.state.BackgroundPlain = paths[0], false
			a.applyTheme()
		})
	}()
}

// backgroundButton is one of the background's actions, disabled in a style without
// glass.
func backgroundButton(c *ui.Context, label string, off bool) ui.Element {
	b := widget.Button(c, widget.Secondary, nil, label, "")
	if off {
		b.Disabled(true).Opacity(0.45)
	}
	return b
}

// backgroundCard is the Appearance page's image, its opacity and blur: the
// window behind it is their preview, as they change.
func (a *App) backgroundCard(c *ui.Context) {
	path := a.state.BackgroundImage
	flat := a.state.flatStyle()
	widget.Card(c.Key("background-card")).FillWidth().Shrink(0).Children(func() {
		widget.CardTitle(c, nil, "Background", func() {
			if (path != "" || a.state.BackgroundPlain) && backgroundButton(c.Key("default"), "Use the default", flat).Clicked() && !flat {
				a.state.BackgroundImage, a.state.BackgroundPlain = "", false
				a.applyTheme()
			}
			if backgroundButton(c.Key("choose"), "Choose an image…", flat).Clicked() && !flat {
				a.chooseBackground()
			}
		})
		switch {
		case flat:
			ui.Text(c, "A background image and the panels' opacity show in Liquid Glass only: this style has no glass to see them through. Yours is kept for when you go back.").
				FontSize(theme.TextLabel).TextColor(theme.TextSoft)
		case a.background.err != "":
			widget.Notice(c, icon.Alert, theme.Warning, a.background.err, "Showing the theme's own background.", nil)
		case path == "":
			ui.Text(c, "The theme's own background. An image of yours stays on this device: each device picks its own.").
				FontSize(theme.TextLabel).TextColor(theme.TextSoft)
		default:
			widget.Running(c.Key("name"), filepath.Base(path), widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
		}
		if path == "" {
			return
		}
		ui.Row(c.Key("background-sliders")).FillWidth().MinWidth(0).Gap(theme.S4).Children(func() {
			ui.Column(c.Key("opacity")).Grow(1).Basis(0).MinWidth(0).Children(func() {
				widget.Field(c, "Panels' opacity", func() {
					ui.Row(c).FillWidth().MinWidth(0).Gap(theme.S2).Children(func() {
						v := a.state.backgroundOpacity()
						if ui.Slider(c, &v, 0, 1).Grow(1).MinWidth(0).Label("Panels' opacity").Disabled(flat).Changed() && !flat {
							a.state.BackgroundOpacity = &v
							a.applyTheme()
						}
						ui.Text(c, fmt.Sprintf("%.0f%%", v*100)).FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine().Shrink(0)
					})
				})
			})
			ui.Column(c.Key("blur")).Grow(1).Basis(0).MinWidth(0).Children(func() {
				widget.Field(c, "Blur", func() {
					ui.Row(c).FillWidth().MinWidth(0).Gap(theme.S2).Children(func() {
						v := a.state.backgroundBlur()
						if ui.Slider(c, &v, 0, backgroundMaxBlur).Grow(1).MinWidth(0).Label("Blur").Disabled(flat).Changed() && !flat {
							a.state.BackgroundBlur = &v
						}
						ui.Text(c, fmt.Sprintf("%.0f", v)).FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine().Shrink(0)
					})
				})
			})
		})
		ui.Text(c, "From fully opaque down to the image alone, terminals and code included: at the low end, how readable it is depends on your image.").
			FontSize(theme.TextLabel).TextColor(theme.TextSoft)
	})
}

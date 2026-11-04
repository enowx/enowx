package canvasview

import (
	"bytes"
	"image"
	imagecolor "image/color"
	_ "image/png" // the pictures the app captures

	"github.com/egoist/mygo/ui"
)

// previewPixels is the most a design's picture holds in memory. It shows small,
// and at the screen's scale a tall page is hundreds of MB of pixels otherwise,
// kept for every design looked at. The disk keeps the picture whole.
const previewPixels = 4 << 20

// decodePicture is the picture in data, made smaller to fit previewPixels.
func decodePicture(data []byte) (*ui.Bitmap, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width*cfg.Height <= previewPixels {
		return ui.DecodeBitmap(data) // what it can't size, it says why
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return ui.NewBitmap(shrinkToPixels(img, previewPixels)), nil
}

// shrinkToPixels is img averaged down by a whole factor to at most limit pixels.
func shrinkToPixels(img image.Image, limit int) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w*h <= limit {
		return img
	}
	n := 2
	for (w/n)*(h/n) > limit {
		n++
	}
	out := image.NewNRGBA(image.Rect(0, 0, max(w/n, 1), max(h/n, 1)))
	for y := range out.Rect.Dy() {
		for x := range out.Rect.Dx() {
			var r, g, bl, a uint64
			for sy := range n {
				for sx := range n {
					c := imagecolor.NRGBA64Model.Convert(img.At(b.Min.X+x*n+sx, b.Min.Y+y*n+sy)).(imagecolor.NRGBA64)
					r, g, bl, a = r+uint64(c.R), g+uint64(c.G), bl+uint64(c.B), a+uint64(c.A)
				}
			}
			d := uint64(n*n) * 257
			out.SetNRGBA(x, y, imagecolor.NRGBA{uint8(r / d), uint8(g / d), uint8(bl / d), uint8(a / d)})
		}
	}
	return out
}

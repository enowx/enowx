package canvasview

import (
	"bytes"
	"image"
	imagecolor "image/color"
	"image/png"
	"testing"
)

// A tall page's picture holds no more in memory than the limit, and looks the same.
func TestLargePictureIsKeptSmall(t *testing.T) {
	const w, h = 2880, 6000 // a page at a Retina screen's scale: 66 MB of pixels
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetNRGBA(x, y, imagecolor.NRGBA{200, 10, 90, 255})
		}
	}
	var data bytes.Buffer
	if err := png.Encode(&data, img); err != nil {
		t.Fatal(err)
	}
	bmp, err := decodePicture(data.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	gotW, gotH := bmp.Size()
	if gotW*gotH > previewPixels || gotW < 200 {
		t.Fatalf("kept %d×%d", gotW, gotH)
	}
	if ratio := float64(gotH) / float64(gotW); ratio < 2.05 || ratio > 2.12 {
		t.Fatalf("its shape changed: %d×%d", gotW, gotH)
	}
	if c := shrinkToPixels(img, previewPixels).At(10, 10); c != (imagecolor.NRGBA{200, 10, 90, 255}) {
		t.Fatalf("its color changed: %v", c)
	}
	if small := image.NewNRGBA(image.Rect(0, 0, 100, 100)); shrinkToPixels(small, previewPixels) != image.Image(small) {
		t.Fatal("a small picture was remade")
	}
}

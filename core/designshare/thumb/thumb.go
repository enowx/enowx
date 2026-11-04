// Package thumb makes a design's picture for the community library. It stands
// apart from designshare because the cloud imports that one, and doesn't make
// pictures.
package thumb

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	_ "image/png" // what a render is

	"github.com/gen2brain/webp"
	"golang.org/x/image/draw"

	"github.com/enowx/enowx/cloud/api"
)

// WebP is the picture (a PNG) as what the library takes: at most 1200 pixels
// wide and 300 KB, losing what quality it must to get there.
func WebP(picture []byte) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(picture))
	if err != nil {
		return nil, fmt.Errorf("thumb: the picture doesn't open: %w", err)
	}
	for _, width := range []int{api.MaxLibraryThumbWidth, 800, 480} {
		small := scaled(img, width)
		for _, quality := range []int{80, 60, 40} {
			var out bytes.Buffer
			if err := webp.Encode(&out, small, webp.Options{Quality: quality, Method: 4}); err != nil {
				return nil, fmt.Errorf("thumb: make it WebP: %w", err)
			}
			if out.Len() <= api.MaxLibraryThumb {
				return out.Bytes(), nil
			}
		}
	}
	return nil, errors.New("thumb: this design's picture won't fit in 300 KB")
}

func scaled(img image.Image, width int) image.Image {
	b := img.Bounds()
	if b.Dx() <= width {
		return img
	}
	out := image.NewNRGBA(image.Rect(0, 0, width, max(1, b.Dy()*width/b.Dx())))
	draw.CatmullRom.Scale(out, out.Bounds(), img, b, draw.Over, nil)
	return out
}

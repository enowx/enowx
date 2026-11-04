package library

import (
	"bytes"
	"fmt"
	"image"
	"math"
	"math/bits"
	"slices"
	"strconv"

	"golang.org/x/image/webp"
)

// A picture's perceptual hash: 64 bits saying where it's lighter than its usual
// at its coarsest detail, so a copy resized or recompressed differs by a few bits.
// Pure Go, as the cloud's image has no CGO.

const (
	hashSide   = 32 // the picture, shrunk, the transform is made of
	hashDetail = 8  // the lowest frequencies kept, per side
)

// leastContrast is how much a shrunk picture's brightness has to vary for its
// hash to say anything: every near-blank picture would otherwise look like every
// other.
const leastContrast = 6.0

// picturePhash is the hash of a WebP as hex, "" when it can't be read or is too
// plain to tell by.
func picturePhash(data []byte) string {
	img, err := webp.Decode(bytes.NewReader(data))
	if err != nil {
		return ""
	}
	h, ok := phash(img)
	if !ok {
		return ""
	}
	return fmt.Sprintf("%016x", h)
}

// cellSamples is how many samples a shrunk picture's cell takes per side.
const cellSamples = 4

// shrunkGray is img's brightness, 0 to 255, averaged into side × side cells. It
// goes by the cells, not the pixels, so a picture smaller than the grid leaves none
// empty, which would read as contrast it doesn't have.
func shrunkGray(img image.Image, side int) []float64 {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	out := make([]float64, side*side)
	per := side * cellSamples
	for sy := range per {
		y := sy * h / per
		for sx := range per {
			x := sx * w / per
			r, g, bl, a := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
			// Transparency shows over white, as a picture does on a page.
			white := float64(0xffff - a)
			gray := 0.299*(float64(r)+white) + 0.587*(float64(g)+white) + 0.114*(float64(bl)+white)
			out[(sy/cellSamples)*side+sx/cellSamples] += gray / 257 / (cellSamples * cellSamples)
		}
	}
	return out
}

// phash is img's hash; ok false when it's too plain to tell by.
func phash(img image.Image) (uint64, bool) {
	if img.Bounds().Dx() < 8 || img.Bounds().Dy() < 8 {
		return 0, false
	}
	gray := shrunkGray(img, hashSide)
	mean, spread := 0.0, 0.0
	for _, v := range gray {
		mean += v
	}
	mean /= float64(len(gray))
	for _, v := range gray {
		spread += (v - mean) * (v - mean)
	}
	if math.Sqrt(spread/float64(len(gray))) < leastContrast {
		return 0, false
	}
	// A discrete cosine transform of the lowest frequencies, without the first, which
	// is only the average.
	var cos [hashDetail][hashSide]float64
	for u := range hashDetail {
		for x := range hashSide {
			cos[u][x] = math.Cos(float64(2*x+1) * float64(u) * math.Pi / (2 * hashSide))
		}
	}
	values := make([]float64, 0, hashDetail*hashDetail)
	for v := range hashDetail {
		for u := range hashDetail {
			total := 0.0
			for y := range hashSide {
				for x := range hashSide {
					total += gray[y*hashSide+x] * cos[u][x] * cos[v][y]
				}
			}
			values = append(values, total)
		}
	}
	sorted := slices.Clone(values[1:])
	slices.Sort(sorted)
	median := sorted[len(sorted)/2]
	var out uint64
	for i, v := range values {
		if i > 0 && v > median {
			out |= 1 << uint(i)
		}
	}
	return out, true
}

// hashDistance is how many bits two hashes in hex differ by; ok false when one
// doesn't read.
func hashDistance(a, b string) (int, bool) {
	x, err1 := strconv.ParseUint(a, 16, 64)
	y, err2 := strconv.ParseUint(b, 16, 64)
	if err1 != nil || err2 != nil {
		return 0, false
	}
	return bits.OnesCount64(x ^ y), true
}

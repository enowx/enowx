package theme

import (
	"cmp"
	"image"
	"slices"

	"github.com/egoist/mygo/ui"
)

// FollowID is the ID of the palette derived from the user's background image.
// It is no entry of Palettes: what is saved is the choice to follow, per device,
// beside the palette to go back to.
const FollowID = "follow"

// Derived are the colors a palette takes from an image.
type Derived struct {
	Ground ui.Color    // its main color, dark
	Wall   [3]ui.Color // its main colors, with how strong each light is
	Accent ui.Color    // its most colorful main color
}

const (
	// deriveGrid is about how many pixels along a side are looked at: an image's
	// main colors don't need more, and it runs once per image.
	deriveGrid = 128
	// deriveApart is how far apart, summed over channels, two main colors are.
	deriveApart = 96
	// How bright, at most in any channel, the ground and the lights are, as the
	// built-in palettes have them: words stay on dark glass.
	deriveGroundMax = 45
	deriveWallMax   = 200
)

// imageBucket is the pixels of one coarse color.
type imageBucket struct {
	key     uint32
	n       int
	r, g, b int
}

func (b imageBucket) color() ui.Color {
	return ui.RGB(uint8(b.r/b.n), uint8(b.g/b.n), uint8(b.b/b.n))
}

// DeriveColors is img's main colors. An image with nothing to see takes the
// default palette's. It costs a look at a grid of pixels, so it runs off the
// main thread, once per image.
func DeriveColors(img image.Image) Derived {
	buckets := imageBuckets(img)
	if len(buckets) == 0 {
		return Derived{Ground: Default.Ground, Wall: Default.Wall, Accent: Default.Accent}
	}
	main := buckets[0].color()
	var picked []ui.Color
	for _, b := range buckets {
		col := b.color()
		if !slices.ContainsFunc(picked, func(o ui.Color) bool { return colorDistance(o, col) < deriveApart }) {
			picked = append(picked, col)
		}
		if len(picked) == 3 {
			break
		}
	}
	// Few colors, as a flat image: lighter shades of the main one.
	for i := 1; len(picked) < 3; i++ {
		picked = append(picked, Mix(main, ui.Hex("#FFFFFF"), 0.2*float64(i)))
	}
	alphas := [3]float32{0.45, 0.4, 0.45}
	var wall [3]ui.Color
	for i, col := range picked {
		wall[i] = ui.RGBA(0, 0, 0, alphas[i])
		c := noBrighterThan(col, deriveWallMax)
		wall[i].R, wall[i].G, wall[i].B = c.R, c.G, c.B
	}
	return Derived{Ground: noBrighterThan(main, deriveGroundMax), Wall: wall, Accent: mostColorful(buckets)}
}

// imageBuckets counts img's pixels by coarse color, the most common first, in
// the same order for the same image.
func imageBuckets(img image.Image) []imageBucket {
	box := img.Bounds()
	step := max(1, max(box.Dx(), box.Dy())/deriveGrid)
	seen := map[uint32]*imageBucket{}
	for y := box.Min.Y; y < box.Max.Y; y += step {
		for x := box.Min.X; x < box.Max.X; x += step {
			r, g, b, a := img.At(x, y).RGBA()
			if a < 0x8000 {
				continue // what shows through says nothing of the image
			}
			r8, g8, b8 := int(r>>8), int(g>>8), int(b>>8)
			key := uint32(r8>>4)<<8 | uint32(g8>>4)<<4 | uint32(b8>>4)
			e := seen[key]
			if e == nil {
				e = &imageBucket{key: key}
				seen[key] = e
			}
			e.n++
			e.r, e.g, e.b = e.r+r8, e.g+g8, e.b+b8
		}
	}
	out := make([]imageBucket, 0, len(seen))
	for _, e := range seen {
		out = append(out, *e)
	}
	slices.SortFunc(out, func(a, b imageBucket) int {
		return cmp.Or(cmp.Compare(b.n, a.n), cmp.Compare(a.key, b.key))
	})
	return out
}

// mostColorful is the color the most pixels have the most color in: a large
// grey sky doesn't beat a smaller red roof, nor one red pixel a blue sea.
func mostColorful(buckets []imageBucket) ui.Color {
	best, score := buckets[0].color(), -1.0
	for _, b := range buckets {
		col := b.color()
		s := saturation(col)
		s = s * s * float64(b.n)
		if s > score {
			best, score = col, s
		}
	}
	return best
}

// saturation is how far c is from a grey, 0 to 1.
func saturation(c ui.Color) float64 {
	hi, lo := max(c.R, c.G, c.B), min(c.R, c.G, c.B)
	if hi == 0 {
		return 0
	}
	return float64(hi-lo) / float64(hi)
}

func colorDistance(a, b ui.Color) int {
	d := func(x, y uint8) int { return max(int(x)-int(y), int(y)-int(x)) }
	return d(a.R, b.R) + d(a.G, b.G) + d(a.B, b.B)
}

// noBrighterThan is c, its hue kept, with no channel over most.
func noBrighterThan(c ui.Color, most uint8) ui.Color {
	hi := max(c.R, c.G, c.B)
	if hi <= most {
		return ui.RGB(c.R, c.G, c.B)
	}
	scale := func(v uint8) uint8 { return uint8(int(v) * int(most) / int(hi)) }
	return ui.RGB(scale(c.R), scale(c.G), scale(c.B))
}

// DerivedPalette is the palette of d on the existing glass recipe, in the
// appearance asked for; a non-zero accent is the user's and replaces d's. Its
// own accent is lifted as far as it takes to read as words on the frame under
// the pointer, or, in the light appearance, deepened as any pale accent is.
func DerivedPalette(d Derived, accent ui.Color, light bool) Palette {
	own := d.Accent
	dark := glass(FollowID, "Follow background", Hex(d.Ground), d.Wall, Hex(own), Hex(contrastInk(own)))
	on := over(dark.Hover, over(dark.Frame, dark.Ground))
	for contrast(own, on) < 4.5 {
		own = Mix(own, ui.Hex("#FFFFFF"), 0.06)
	}
	dark.Accent, dark.OnAccent = own, contrastInk(own)
	if light {
		return lightOf(dark, accent)
	}
	if accent.A != 0 {
		dark.Accent, dark.OnAccent = accent, contrastInk(accent)
	}
	return dark
}

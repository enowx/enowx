package theme

import (
	"image"
	"image/color"
	"testing"

	"github.com/egoist/mygo/ui"
)

// paint is an image, top to bottom, of bands of colors with these shares.
func paint(bands []color.RGBA, shares []int) image.Image {
	total := 0
	for _, s := range shares {
		total += s
	}
	img := image.NewRGBA(image.Rect(0, 0, 200, total))
	y := 0
	for i, s := range shares {
		for range s {
			for x := 0; x < 200; x++ {
				img.SetRGBA(x, y, bands[i])
			}
			y++
		}
	}
	return img
}

// The most common color makes the ground, dark and of its hue; the most colorful
// the accent, though less common than a grey; the same image, the same colors.
func TestDeriveColorsFromAnImage(t *testing.T) {
	grey, red := color.RGBA{R: 120, G: 120, B: 130, A: 255}, color.RGBA{R: 210, G: 30, B: 30, A: 255}
	sea := color.RGBA{R: 60, G: 80, B: 100, A: 255}
	img := paint([]color.RGBA{sea, grey, red}, []int{120, 60, 40})
	d := DeriveColors(img)
	if max(d.Ground.R, d.Ground.G, d.Ground.B) > deriveGroundMax || d.Ground.B <= d.Ground.R {
		t.Fatalf("ground %s isn't a dark blue", Hex(d.Ground))
	}
	if d.Accent.R <= d.Accent.B {
		t.Fatalf("accent %s isn't the red", Hex(d.Accent))
	}
	for i, w := range d.Wall {
		if w.A == 0 || max(w.R, w.G, w.B) > deriveWallMax {
			t.Fatalf("wall %d: %+v", i, w)
		}
	}
	if again := DeriveColors(img); again != d {
		t.Fatalf("not the same twice: %+v, %+v", d, again)
	}
	// A few red pixels don't beat a blue sea.
	blue := color.RGBA{R: 20, G: 90, B: 160, A: 255}
	if a := DeriveColors(paint([]color.RGBA{blue, red}, []int{120, 20})).Accent; a.B <= a.R {
		t.Fatalf("accent %s isn't the sea", Hex(a))
	}
}

// A flat image still makes three lights; one with nothing to see, the default's.
func TestDeriveColorsFromAlmostNothing(t *testing.T) {
	flat := DeriveColors(paint([]color.RGBA{{R: 80, G: 80, B: 80, A: 255}}, []int{10}))
	if flat.Wall[0].A == 0 || flat.Wall[1].A == 0 || flat.Wall[2].A == 0 {
		t.Fatalf("flat: %+v", flat.Wall)
	}
	clear := DeriveColors(image.NewRGBA(image.Rect(0, 0, 10, 10)))
	if clear.Ground != Default.Ground || clear.Accent != Default.Accent {
		t.Fatalf("nothing to see: %+v", clear)
	}
}

// Whatever the image, words and the accent read, in dark and light, at the
// default opacity too; a custom accent replaces the derived one; and terminals
// stay on a dark ground.
func TestDerivedPaletteReads(t *testing.T) {
	old := Current()
	defer Apply(old)
	for _, band := range []color.RGBA{
		{R: 255, G: 255, B: 255, A: 255}, {A: 255}, {R: 255, G: 240, B: 0, A: 255},
		{R: 10, G: 10, B: 120, A: 255}, {R: 128, G: 128, B: 128, A: 255},
	} {
		d := DeriveColors(paint([]color.RGBA{band}, []int{10}))
		for _, light := range []bool{false, true} {
			p := DerivedPalette(d, ui.Color{}, light)
			if p.ID != FollowID || p.Light != light {
				t.Fatalf("%+v: %s, light %v", band, p.ID, p.Light)
			}
			if r := contrast(p.OnAccent, p.Accent); r < 4.5 {
				t.Errorf("%+v light=%v: on accent %.2f", band, light, r)
			}
			on := over(p.Hover, over(p.Frame, p.Ground))
			if r := contrast(p.Accent, on); r < 4.5 {
				t.Errorf("%+v light=%v: accent %s on %s: %.2f", band, light, Hex(p.Accent), Hex(on), r)
			}
			b := WithBackdrop(p, defaultBackdropOpacityForTest)
			for _, image := range []ui.Color{ui.Hex("#FFFFFF"), ui.Hex("#000000")} {
				under := over(Fade(b.Ground, BackdropScrim*defaultBackdropOpacityForTest), image)
				for _, surface := range []ui.Color{b.Panel, b.Frame} {
					if r := contrast(b.Muted, over(surface, under)); r < 4.5 {
						t.Errorf("%+v light=%v: words over %s: %.2f", band, light, Hex(image), r)
					}
				}
			}
			Apply(p)
			if tt := Terminal(); contrast(tt.Foreground, tt.Background) < 4.5 || tt.Background != ui.Hex("#0D0E16") {
				t.Errorf("%+v light=%v: terminal on %s", band, light, Hex(tt.Background))
			}
		}
		custom := ui.Hex("#FF00AA")
		if got := DerivedPalette(d, custom, false).Accent; got != custom {
			t.Errorf("custom accent replaced by %s", Hex(got))
		}
	}
}

// The saved palettes' IDs are untouched by there being a derived one.
func TestFollowIsNoSavedPalette(t *testing.T) {
	for _, p := range Palettes {
		if p.ID == FollowID {
			t.Fatal("a listed palette has the derived palette's ID")
		}
	}
	if got := Lookup(FollowID, ui.Color{}).ID; got != Default.ID {
		t.Fatalf("looked up by name: %s", got)
	}
}

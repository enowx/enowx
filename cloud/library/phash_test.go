package library

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"testing"

	"github.com/enowx/enowx/cloud/api"
	"github.com/enowx/enowx/cloud/store"
)

func page(blocks []image.Rectangle, w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	for _, b := range blocks {
		// Blocks are in thousandths of the picture.
		r := image.Rect(b.Min.X*w/1000, b.Min.Y*h/1000, b.Max.X*w/1000, b.Max.Y*h/1000)
		draw.Draw(img, r, image.NewUniform(color.RGBA{20, 40, 80, 255}), image.Point{}, draw.Src)
	}
	return img
}

var (
	oneLayout   = []image.Rectangle{image.Rect(0, 0, 250, 1000), image.Rect(300, 100, 950, 300), image.Rect(300, 400, 600, 900), image.Rect(650, 400, 950, 900)}
	otherLayout = []image.Rectangle{image.Rect(100, 50, 900, 200), image.Rect(50, 600, 300, 950), image.Rect(700, 300, 950, 500), image.Rect(350, 700, 600, 800)}
)

func hashOf(t *testing.T, img image.Image) string {
	t.Helper()
	h, ok := phash(img)
	if !ok {
		t.Fatal("no hash")
	}
	return fmt.Sprintf("%016x", h)
}

func TestPhashTellsCopiesFromOthers(t *testing.T) {
	a := hashOf(t, page(oneLayout, 1200, 750))
	if d, _ := hashDistance(a, hashOf(t, page(oneLayout, 1200, 750))); d != 0 {
		t.Fatalf("the same picture differs by %d", d)
	}
	if d, _ := hashDistance(a, hashOf(t, page(oneLayout, 400, 250))); d > similarWithin {
		t.Fatalf("a smaller copy differs by %d", d)
	}
	if d, _ := hashDistance(a, hashOf(t, page(otherLayout, 1200, 750))); d <= similarWithin {
		t.Fatalf("another layout differs by only %d", d)
	}
}

func TestPhashSaysNothingOfPlainPictures(t *testing.T) {
	if _, ok := phash(page(nil, 800, 500)); ok {
		t.Fatal("a blank picture has a hash")
	}
	// Smaller than the grid it's shrunk to, transparent or not.
	if _, ok := phash(image.NewRGBA(image.Rect(0, 0, 40, 20))); ok {
		t.Fatal("a small blank picture has a hash")
	}
	if _, ok := phash(page(nil, 40, 20)); ok {
		t.Fatal("a small white picture has a hash")
	}
	if _, ok := phash(image.NewRGBA(image.Rect(0, 0, 4, 4))); ok {
		t.Fatal("a speck has a hash")
	}
	if picturePhash([]byte("not a picture")) != "" {
		t.Fatal("bytes that aren't a picture have a hash")
	}
	if _, ok := hashDistance("zz", "00"); ok {
		t.Fatal("that isn't a hash")
	}
}

func TestClosestToIsOnePerItemClosestFirst(t *testing.T) {
	const base = "0000000000000000"
	me := store.LibraryHash{ItemID: "me", Owner: "o", Phash: base}
	all := []store.LibraryHash{me,
		{ItemID: "far", Phash: "ffffffffffffffff"},
		{ItemID: "b", Owner: "o", Phash: "00000000000000ff"},
		{ItemID: "a", Owner: "x", Version: 2, Phash: "0000000000000001"},
		{ItemID: "a", Owner: "x", Version: 1, Phash: "0000000000000003"},
		{ItemID: "bad", Phash: "nope"},
	}
	got := closestTo(me, all)
	want := []api.LibrarySimilar{{ItemID: "a", Version: 2, Distance: 1}, {ItemID: "b", SameCreator: true, Distance: 8}}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("%+v", got)
	}
}

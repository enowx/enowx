package communityview

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/gen2brain/webp"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
)

func shot(t *testing.T, tt *ui.Tester, name string) {
	dir := os.Getenv("SHOTS")
	if dir == "" {
		return
	}
	tt.Advance(time.Second)
	tt.Frame()
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	_ = png.Encode(f, tt.Image())
}

func shows(tt *ui.Tester, text string) bool {
	return strings.Contains(strings.Join(tt.Texts(), "\n"), text)
}

// sample stands in for a picture: a page of colors, tall as a page is.
func sample(n int) *ui.Bitmap {
	img := image.NewRGBA(image.Rect(0, 0, 400, 300))
	tones := []color.RGBA{{0x1d, 0x4e, 0x5f, 255}, {0xc2, 0x4f, 0x2a, 255}, {0x3a, 0x2f, 0x8f, 255}, {0xe8, 0xb9, 0x4a, 255}}
	for y := range 300 {
		for x := range 400 {
			c := tones[n%len(tones)]
			if y > 60 && y < 240 && x > 40 && x < 360 {
				c = color.RGBA{0xf4, 0xf1, 0xe8, 255}
			}
			img.SetRGBA(x, y, c)
		}
	}
	return ui.NewBitmap(img)
}

func listing(id, title, license, kind string, a proto.LibraryActions) proto.LibraryListing {
	return proto.LibraryListing{ItemID: id, Title: title, License: license, Kind: kind, Handle: "ada", Version: 1, Category: "dashboard", Actions: a, URL: "https://ui.example.com/d/" + id + "/x",
		Description: "A page to look at.", Tags: []string{"grid", "dark"}}
}

var allowsAll = proto.LibraryActions{CopyPrompt: true, UseReference: true, OpenInEnowX: true}

func catalog() []proto.LibraryListing {
	return []proto.LibraryListing{
		listing("a", "Harbor ops dashboard", "mit", "page", allowsAll),
		listing("b", "Order flow diagram with a title too long for any card to hold it on one line at all", "cc-by-4.0", "page", allowsAll),
		listing("c", "Pricing table", "mit", "component", proto.LibraryActions{CopyPrompt: true, UseComponent: true, OpenInEnowX: true}),
		listing("d", "Name your workspace", "view-only", "page", proto.LibraryActions{}),
	}
}

type fake struct {
	asked   []proto.LibrarySearchParams
	results proto.LibraryResults
	opened  string
	prompts int
}

func (f *fake) options() Options {
	return Options{
		Search: func(p proto.LibrarySearchParams, done func(proto.LibraryResults, error)) {
			f.asked = append(f.asked, p)
			done(f.results, nil)
		},
		Prompt: func(id string, done func(string, error)) {
			f.prompts++
			done("Build it\n\nDesign: x by ada", nil)
		},
		Picture: func(it proto.LibraryListing, done func(*ui.Bitmap, error)) {
			done(sample(int(it.ItemID[0])), nil)
		},
		OpenURL: func(u string) { f.opened = u },
	}
}

func tester(f *fake, v *View, width int) *ui.Tester {
	o := f.options()
	return ui.NewTester(func(c *ui.Context) {
		c.SetTheme(theme.Window())
		ui.Column(c).Grow(1).FillWidth().FillHeight().Background(theme.Ground).TextColor(theme.Text).Children(func() { v.Draw(c, o) })
	}, width, 800)
}

// The page searches, narrows by category and kind, and shows what came.
func TestBrowseSearchesAndNarrows(t *testing.T) {
	f := &fake{results: proto.LibraryResults{On: true, Items: catalog(), Next: "page2"}}
	v := &View{}
	tt := tester(f, v, 1280)
	tt.Frame()
	tt.Frame()
	for _, want := range []string{"Community library", "Type of item", "Category", "Harbor ops dashboard", "View only", "Show more"} {
		if !shows(tt, want) {
			t.Errorf("no %q in %q", want, tt.Texts())
		}
	}
	shot(t, tt, "community-grid.png")
	if err := tt.Click("Dashboard"); err != nil {
		t.Fatalf("no category: %v %q", err, tt.Texts())
	}
	tt.Frame()
	if last := f.asked[len(f.asked)-1]; last.Category != "dashboard" {
		t.Fatalf("not narrowed: %+v", last)
	}
	if err := tt.Click("Component"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if last := f.asked[len(f.asked)-1]; last.Kind != "component" || last.Category != "dashboard" {
		t.Fatalf("not narrowed by kind: %+v", last)
	}
	// Typing searches once it rests, not at each key.
	n := len(f.asked)
	v.query = "harbor"
	tt.Frame()
	if len(f.asked) != n {
		t.Fatal("searched while typing")
	}
	tt.Advance(searchQuiet + time.Millisecond)
	tt.Frame()
	if len(f.asked) != n+1 || f.asked[n].Query != "harbor" {
		t.Fatalf("after typing: %+v", f.asked[n:])
	}
	// The next page keeps what's there; an item it repeats, pushed down by what
	// went live meanwhile, shows once.
	n = len(f.asked)
	f.results.Items = []proto.LibraryListing{catalog()[3], listing("e", "Payment received loader", "mit", "component", allowsAll)}
	f.results.Next = ""
	if err := tt.Click("Show more"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	if f.asked[n].Cursor != "page2" || len(v.items) != len(catalog())+1 || shows(tt, "Show more") {
		t.Fatalf("more: %+v, %d items", f.asked[n], len(v.items))
	}
}

// A sheet offers what the item allows: a view-only design nothing to copy, its
// page in the browser all the same.
func TestDetailOffersWhatTheItemAllows(t *testing.T) {
	f := &fake{results: proto.LibraryResults{On: true, Items: catalog()}}
	v := &View{}
	tt := tester(f, v, 1280)
	tt.Frame()
	tt.Frame()
	if err := tt.Click("Open Harbor ops dashboard"); err != nil {
		t.Fatalf("no card: %v", err)
	}
	tt.Frame()
	tt.Frame()
	for _, want := range []string{"What this design allows", "Copy its prompt", "Get prompt", "Open in browser", "MIT", "don't run in the app"} {
		if !shows(tt, want) {
			t.Errorf("no %q in %q", want, tt.Texts())
		}
	}
	shot(t, tt, "community-detail.png")
	if err := tt.Click("Get prompt"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if f.prompts != 1 || !shows(tt, "Copy prompt") {
		t.Fatalf("prompt: %d %q", f.prompts, tt.Texts())
	}
	if err := tt.Click("Open in browser"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if f.opened != "https://ui.example.com/d/a/x" {
		t.Fatalf("opened %q", f.opened)
	}

	v.detail = &detail{open: true, item: catalog()[3]}
	tt.Frame()
	tt.Frame()
	if shows(tt, "Get prompt") || shows(tt, "Copy prompt") || shows(tt, "Copy its prompt") {
		t.Fatalf("a view-only design offers its prompt: %q", tt.Texts())
	}
	if !shows(tt, "View only: look at it") || !shows(tt, "Open in browser") {
		t.Fatalf("view only: %q", tt.Texts())
	}
	shot(t, tt, "community-view-only.png")
}

// No library on the cloud: nothing to browse, and nothing else asked for.
func TestNothingShowsWithoutALibrary(t *testing.T) {
	f := &fake{}
	v := &View{}
	tt := tester(f, v, 1280)
	tt.Frame()
	tt.Frame()
	if !shows(tt, "has no community library") || shows(tt, "Search designs") || shows(tt, "Type of item") {
		t.Fatalf("texts: %q", tt.Texts())
	}
	if len(f.asked) != 1 {
		t.Fatalf("asked %d times", len(f.asked))
	}
}

// At phone width the side list is a row, and nothing sticks out.
func TestBrowseAtPhoneWidth(t *testing.T) {
	f := &fake{results: proto.LibraryResults{On: true, Items: catalog()}}
	v := &View{}
	tt := tester(f, v, 390)
	tt.Frame()
	tt.Frame()
	tt.Frame()
	if shows(tt, "Type of item") {
		t.Fatalf("the side list at phone width: %q", tt.Texts())
	}
	for _, want := range []string{"Harbor ops dashboard", "Any", "All"} {
		box, ok := tt.Find(want)
		if !ok || box.W <= 0 {
			t.Errorf("%s doesn't show: %+v %q", want, box, tt.Texts())
		}
		if box.X < 0 || box.X+box.W > 390 {
			t.Errorf("%s sticks out: %+v", want, box)
		}
	}
	shot(t, tt, "community-phone.png")
	v.detail = &detail{open: true, item: catalog()[0]}
	tt.Frame()
	tt.Frame()
	if box, ok := tt.Find("Open in browser"); !ok || box.X < 0 || box.X+box.W > 390 {
		t.Errorf("the sheet's button: %v %+v", ok, box)
	}
	shot(t, tt, "community-phone-detail.png")
}

// A picture too large to be what the library allows isn't decoded; a real one is.
func TestDecodePicture(t *testing.T) {
	encode := func(w, h int) []byte {
		var out bytes.Buffer
		if err := webp.Encode(&out, image.NewRGBA(image.Rect(0, 0, w, h)), webp.Options{Quality: 50}); err != nil {
			t.Fatal(err)
		}
		return out.Bytes()
	}
	if bmp, err := DecodePicture(encode(40, 20)); err != nil || bmp == nil {
		t.Fatalf("a picture: %v", err)
	}
	if _, err := DecodePicture(encode(maxPictureSide+1, 1)); err == nil {
		t.Fatal("decoded a picture past the limit")
	}
	if _, err := DecodePicture([]byte("not a picture")); err == nil {
		t.Fatal("decoded nonsense")
	}
}

// A slow answer overtaken by a later search doesn't replace it.
func TestAnOvertakenSearchIsDropped(t *testing.T) {
	var answers []func(proto.LibraryResults, error)
	v := &View{}
	o := Options{Search: func(p proto.LibrarySearchParams, done func(proto.LibraryResults, error)) {
		answers = append(answers, done)
	}}
	v.search(o)
	v.search(o)
	answers[1](proto.LibraryResults{On: true, Items: catalog()[:1]}, nil)
	answers[0](proto.LibraryResults{On: true, Items: catalog()}, nil)
	if len(v.items) != 1 {
		t.Fatalf("items: %d", len(v.items))
	}
}

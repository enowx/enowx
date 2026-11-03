package browserview

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
)

// A capture at twice the page's pixels is cut to the element, with room
// around it.
func TestPickedCropsToTheElement(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 1200, 800))
	for y := 80; y < 162; y++ {
		for x := 80; x < 256; x++ {
			img.Set(x, y, color.RGBA{37, 99, 235, 255})
		}
	}
	var capture bytes.Buffer
	if err := png.Encode(&capture, img); err != nil {
		t.Fatal(err)
	}
	p := Picked{URL: "http://localhost:5173/", Selector: "#buy", HTML: `<button id="buy">Buy now</button>`,
		Styles: "color: rgb(255, 255, 255)", Rect: [4]float64{40, 40, 88, 41}, Viewport: 600}
	shot, err := p.Crop(capture.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	cut, err := png.Decode(bytes.NewReader(shot))
	if err != nil {
		t.Fatal(err)
	}
	if b := cut.Bounds(); b.Dx() != (88+16)*2 || b.Dy() != (41+16)*2 {
		t.Fatalf("cut %v", b)
	}
	about := p.Describe()
	for _, want := range []string{"http://localhost:5173/", "#buy", "Buy now", "color: rgb(255, 255, 255)"} {
		if !strings.Contains(about, want) {
			t.Fatalf("description lacks %q:\n%s", want, about)
		}
	}
}

// pickingPage is a page that answers Pick when told to.
type pickingPage struct {
	fakePage
	answer  chan pickAnswer
	stopped int
}

type pickAnswer struct {
	picked *Picked
	err    error
}

func (p *pickingPage) Pick(context.Context) (*Picked, error) {
	a := <-p.answer
	return a.picked, a.err
}
func (p *pickingPage) StopPick() { p.stopped++; p.answer <- pickAnswer{} }
func (p *pickingPage) Capture() ([]byte, error) {
	var b bytes.Buffer
	err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 100, 100)))
	return b.Bytes(), err
}

// The toolbar's Pick an element turns to Stop picking while the page takes the
// click, and back once it stops, the agent told nothing; what is picked goes to
// the agent, and a page that fails says so. The outline itself is the page's
// (a real web view's).
func TestPickButton(t *testing.T) {
	page := &pickingPage{answer: make(chan pickAnswer, 1)}
	updates := make(chan func(), 4)
	o := options(&page.fakePage, new(string))
	o.NewPage = func(url string, done func(Page, error)) { page.url = url; done(page, nil) }
	o.Update = func(fn func()) { updates <- fn }
	var attached []string
	o.AttachElement = func(about string, _ []byte) { attached = append(attached, about) }
	v := New()
	v.loaded = true
	v.setPorts(o, []proto.Port{{Port: 5173}})
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(theme.Window())
		ui.Column(c).Width(1000).Height(600).Background(theme.Frame).TextColor(theme.Text).Children(func() { v.Draw(c, o) })
	}, 1000, 600)
	tt.Frame()
	settle := func() {
		t.Helper()
		select {
		case fn := <-updates:
			fn()
		case <-time.After(5 * time.Second):
			t.Fatal("picking didn't end")
		}
		tt.Frame()
	}

	if err := tt.Click("Pick an element"); err != nil {
		t.Fatalf("%v: %q", err, tt.Texts())
	}
	tt.Frame()
	lookShot(t, tt, "browser-picking")
	if err := tt.Click("Stop picking"); err != nil {
		t.Fatalf("%v: %q", err, tt.Texts())
	}
	settle()
	if _, ok := tt.Find("Pick an element"); !ok || page.stopped != 1 || len(attached) != 0 {
		t.Fatalf("after stopping: stopped %d, attached %q, shows %q", page.stopped, attached, tt.Texts())
	}

	if err := tt.Click("Pick an element"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	page.answer <- pickAnswer{picked: &Picked{Selector: "#a", Rect: [4]float64{0, 0, 10, 10}, Viewport: 100}}
	settle()
	if len(attached) != 1 || !strings.Contains(attached[0], "#a") {
		t.Fatalf("what was picked: %q", attached)
	}

	if err := tt.Click("Pick an element"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	page.answer <- pickAnswer{err: errors.New("the page went away")}
	settle()
	if !tt.HasText("Couldn't pick the element: the page went away") || len(attached) != 1 {
		t.Fatalf("a failed pick: attached %q, shows %q", attached, tt.Texts())
	}
}

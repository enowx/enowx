package browserview

import (
	"strconv"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
)

// fakePage records what the view asks of it.
type fakePage struct {
	url      string
	reloads  int
	devtools int
	closed   bool
}

func (p *fakePage) Show(c *ui.Context) ui.Element { return ui.Text(c, "page "+p.url) }
func (p *fakePage) Load(url string)               { p.url = url }
func (p *fakePage) Reload()                       { p.reloads++ }
func (p *fakePage) Back()                         {}
func (p *fakePage) Forward()                      {}
func (p *fakePage) CanBack() bool                 { return false }
func (p *fakePage) CanForward() bool              { return false }
func (p *fakePage) URL() string                   { return p.url }
func (p *fakePage) Loading() bool                 { return false }
func (p *fakePage) ToggleDevTools()               { p.devtools++ }
func (p *fakePage) Capture() ([]byte, error)      { return []byte("png"), nil }
func (p *fakePage) Close()                        { p.closed = true }

func options(page *fakePage, remembered *string) Options {
	return Options{
		Supported: true,
		NewPage: func(url string, done func(Page, error)) {
			page.url = url
			done(page, nil)
		},
		Origin:   func(p proto.Port) (string, error) { return "http://localhost:" + strconv.Itoa(p.Port), nil },
		OpenURL:  func(string) {},
		Attach:   func([]byte) {},
		Remember: func(port int, path string) { *remembered = strconv.Itoa(port) + path },
	}
}

func TestBrowserFollowsTheSessionsDevServer(t *testing.T) {
	page := &fakePage{}
	var remembered string
	o := options(page, &remembered)
	v := New()
	tt := ui.NewTester(func(c *ui.Context) { v.Draw(c, o) }, 600, 500)
	v.loaded = true
	tt.Frame()
	tt.Frame()
	// Before a dev server, the home page, its address left blank.
	if page.url != HomePage || !tt.HasText("page "+HomePage) || v.address != "" {
		t.Fatalf("before a port: url %q, field %q, %q", page.url, v.address, tt.Texts())
	}

	vite := proto.Port{Port: 5173, Process: "node"}
	api := proto.Port{Port: 8080, Process: "go"}
	v.setPorts(o, []proto.Port{vite, api})
	tt.Frame()
	if page.url != "http://localhost:5173/" || !tt.HasText("page http://localhost:5173/") {
		t.Fatalf("first port: url %q, %q", page.url, tt.Texts())
	}

	// Another port, picked.
	if err := tt.Click("Ports"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("localhost:8080 · go"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if page.url != "http://localhost:8080/" || remembered != "8080/" {
		t.Fatalf("picked 8080: url %q, remembered %q", page.url, remembered)
	}

	// The dev server restarts: the page reloads when it is back.
	v.setPorts(o, []proto.Port{vite})
	tt.Frame()
	if !tt.HasText("The dev server on :8080 stopped. The page comes back when it does.") {
		t.Fatalf("stopped: %q", tt.Texts())
	}
	v.setPorts(o, []proto.Port{vite, api})
	tt.Frame()
	if page.reloads != 1 {
		t.Fatalf("reloads = %d", page.reloads)
	}

	if err := tt.Click("More for the page"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("DevTools"); err != nil {
		t.Fatal(err)
	}
	if page.devtools != 1 {
		t.Fatal("DevTools didn't open")
	}
	v.Close()
	if !page.closed {
		t.Fatal("the page outlived its session")
	}
}

func TestBrowserStartsWhereItWasLeft(t *testing.T) {
	page := &fakePage{}
	var remembered string
	o := options(page, &remembered)
	o.Port, o.Path = 3000, "/settings"
	v := New()
	v.setPorts(o, []proto.Port{{Port: 5173}, {Port: 3000}})
	if page.url != "http://localhost:3000/settings" {
		t.Fatalf("url = %q", page.url)
	}
}

func TestBrowserWithoutAWebViewOffersTheBrowser(t *testing.T) {
	o := options(&fakePage{}, new(string))
	o.Supported = false
	v := New()
	v.loaded = true
	tt := ui.NewTester(func(c *ui.Context) { v.Draw(c, o) }, 600, 400)
	tt.Frame()
	if !tt.HasText("This device has no web view to show pages in. Open them in the browser instead.") {
		t.Fatalf("%q", tt.Texts())
	}
}

func TestPageWidthsAsDevices(t *testing.T) {
	page := &fakePage{}
	o := options(page, new(string))
	v := New()
	v.loaded = true
	v.setPorts(o, []proto.Port{{Port: 5173}})
	tt := ui.NewTester(func(c *ui.Context) { v.Draw(c, o) }, 900, 500)
	tt.Frame()
	if err := tt.Click("Page width: Fill the tab"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("Phone, 390"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if v.width != 390 {
		t.Fatalf("width %d", v.width)
	}
	r, ok := tt.Find("page http://localhost:5173/")
	if !ok || r.W > 390 {
		t.Fatalf("the page is %+v wide", r)
	}
}

func TestAddressTakesHostsAndURLs(t *testing.T) {
	page := &fakePage{}
	var remembered string
	o := options(page, &remembered)
	v := New()
	v.loaded = true
	tt := ui.NewTester(func(c *ui.Context) { v.Draw(c, o) }, 700, 400)
	tt.Frame()

	// No dev server yet: a host typed opens a page all the same.
	if r, ok := tt.Find("Address"); ok {
		tt.ClickAt(r.X+r.W/2, r.Y+r.H/2)
	}
	tt.Frame()
	tt.Type("google.com")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if page.url != "https://google.com" {
		t.Fatalf("loaded %q", page.url)
	}
	tt.Frame()
	if v.address != "https://google.com" || !tt.HasText("page https://google.com") {
		t.Fatalf("field %q, %q", v.address, tt.Texts())
	}
	if remembered != "" {
		t.Fatalf("another site was remembered as the port's: %q", remembered)
	}

	// A dev server starting later waits in the picker, not over the page.
	v.setPorts(o, []proto.Port{{Port: 5173}})
	if page.url != "https://google.com" || v.port != 5173 {
		t.Fatalf("url %q, port %d", page.url, v.port)
	}

	for in, want := range map[string]string{
		"localhost:3000/x":      "http://localhost:3000/x",
		"/settings":             "http://localhost:5173/settings",
		"http://example.com/a":  "http://example.com/a",
		"docs.enowx.ai/preview": "https://docs.enowx.ai/preview",
		"not a url":             "",
		"word":                  "",
	} {
		if got := v.target(in); got != want {
			t.Errorf("%q → %q, want %q", in, got, want)
		}
	}
	v.address = "word"
	v.navigate(o)
	if v.err == "" {
		t.Fatal("no word of a bad address")
	}
}

// The toolbar puts the page on a canvas: its screenshot and its URL.
func TestPageToCanvasButton(t *testing.T) {
	page := &fakePage{}
	o := options(page, new(string))
	got := make(chan string, 1)
	o.ToCanvas = func(png []byte, url string, width int) { got <- string(png) + " " + url }
	v := New()
	v.loaded = true
	v.setPorts(o, []proto.Port{{Port: 5173}})
	tt := ui.NewTester(func(c *ui.Context) { v.Draw(c, o) }, 1000, 600)
	tt.Frame()
	if err := tt.Click("Put on a canvas"); err != nil {
		t.Fatalf("%v: %q", err, tt.Texts())
	}
	tt.Frame()
	select {
	case s := <-got:
		if s != "png "+page.url || page.url == "" {
			t.Fatalf("put %q", s)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("nothing put on a canvas")
	}
}

// The panel lists pages and places no web view: the center's tab does, and a
// page clicked goes there.
func TestPagesListOpensInTheTab(t *testing.T) {
	page := &fakePage{}
	o := options(page, new(string))
	opened := 0
	o.OpenTab = func() { opened++ }
	v := New()
	tt := ui.NewTester(func(c *ui.Context) { v.DrawPages(c, o) }, 320, 500)
	tt.Frame()
	if !tt.HasText("Looking for the session's dev server…") {
		t.Fatalf("not read yet: %q", tt.Texts())
	}

	v.loaded = true
	tt.Frame()
	if !tt.HasText("Waiting for a dev server: start one in the session (npm run dev, say) and it shows here.") || page.url != "" {
		t.Fatalf("no dev server: url %q, %q", page.url, tt.Texts())
	}
	if err := tt.Click("Open a page"); err != nil {
		t.Fatal(err)
	}
	if opened != 1 {
		t.Fatal("no tab for a page without a dev server")
	}

	v.setPorts(o, []proto.Port{{Port: 5173, Process: "node"}, {Port: 8080, Process: "go"}})
	tt.Frame()
	for _, want := range []string{"Pages", "Open in a tab", "localhost:5173", "node", "localhost:8080", "go"} {
		if !tt.HasText(want) {
			t.Errorf("no %q in %q", want, tt.Texts())
		}
	}
	if tt.HasText("page http://localhost:5173/") {
		t.Fatal("the panel placed the web view")
	}
	reloadsBefore := page.url
	if err := tt.Click("localhost:5173"); err != nil {
		t.Fatal(err)
	}
	if opened != 2 || page.url != reloadsBefore {
		t.Fatalf("the page shown: tabs %d, url %q", opened, page.url)
	}
	if err := tt.Click("localhost:8080"); err != nil {
		t.Fatal(err)
	}
	if opened != 3 || page.url != "http://localhost:8080/" {
		t.Fatalf("another page: tabs %d, url %q", opened, page.url)
	}

	// Its server stops: the page stays listed, to come back.
	v.setPorts(o, []proto.Port{{Port: 5173, Process: "node"}})
	tt.Frame()
	if !tt.HasText("localhost:8080") || !tt.HasText("Stopped: the page comes back when it does") {
		t.Fatalf("stopped: %q", tt.Texts())
	}

	// A site an agent went to shows first.
	v.setPorts(o, []proto.Port{{Port: 5173, Process: "node"}, {Port: 8080, Process: "go"}})
	v.address = "example.com"
	v.navigate(o)
	tt.Frame()
	if !tt.HasText("example.com") || !tt.HasText("Another site") {
		t.Fatalf("another site: %q", tt.Texts())
	}
}

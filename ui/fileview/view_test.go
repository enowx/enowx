package fileview

import (
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
)

func TestTreeUnfoldsAndOpensFiles(t *testing.T) {
	v := New()
	v.loaded, v.root = true, "/work/app"
	v.dirs[""] = proto.FileListing{Root: "/work/app", Entries: []proto.FileEntry{
		{Name: "src", Dir: true, Status: "M"},
		{Name: "build", Dir: true, Ignored: true},
		{Name: "README.md", Size: 12},
	}}
	v.dirs["src"] = proto.FileListing{Entries: []proto.FileEntry{{Name: "main.go", Status: "M"}, {Name: "new.go", Status: "?"}}}
	v.layout()
	var opened []string
	o := Options{OpenFile: func(p string) { opened = append(opened, p) }}
	tt := ui.NewTester(func(c *ui.Context) { v.Draw(c, o) }, 480, 500)
	tt.Frame()
	if !tt.HasText("src") || !tt.HasText("README.md") || tt.HasText("main.go") {
		t.Fatalf("folded tree: %q", tt.Texts())
	}
	if err := tt.Click("src"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("main.go") || !tt.HasText("U") {
		t.Fatalf("unfolded src: %q", tt.Texts())
	}
	// Files open in tabs of their own: the tree stays.
	if err := tt.Click("src/main.go"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(opened) != 1 || opened[0] != "src/main.go" || !tt.HasText("README.md") {
		t.Fatalf("opened %q, tree %q", opened, tt.Texts())
	}
}

func TestDocShowsTheFile(t *testing.T) {
	d := NewDoc("src/main.go")
	d.content = &proto.FileContent{Path: "src/main.go", Size: 23, Text: "package main\nfunc main() {}\n"}
	d.lines = splitLines(d.content.Text)
	tt := ui.NewTester(func(c *ui.Context) { d.Draw(c, Options{Session: proto.SessionInfo{Dir: "/work/app"}}) }, 480, 300)
	tt.Frame()
	for _, want := range []string{"src/", "main.go", "package main", "func main() {}", "23 B"} {
		if !tt.HasText(want) {
			t.Errorf("no %q in %q", want, tt.Texts())
		}
	}
}

func TestMarkdownRendersWithRawAtHand(t *testing.T) {
	d := NewDoc("docs/api.md")
	d.content = &proto.FileContent{Path: "docs/api.md", Size: 40, Text: "# API contract\n\nMoney is **always** a string.\n"}
	d.lines = splitLines(d.content.Text)
	tt := ui.NewTester(func(c *ui.Context) { d.Draw(c, Options{}) }, 600, 400)
	tt.Frame()
	if !tt.HasText("Preview") || !tt.HasText("Raw") || tt.HasText("# API contract") {
		t.Fatalf("preview: %q", tt.Texts())
	}
	if !tt.HasText("API contract") {
		t.Fatalf("no heading in %q", tt.Texts())
	}
	if err := tt.Click("Raw"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("# API contract") {
		t.Fatalf("raw: %q", tt.Texts())
	}
	// Other files have no such choice.
	other := NewDoc("main.go")
	other.content = &proto.FileContent{Text: "package main\n"}
	other.lines = splitLines(other.content.Text)
	tt2 := ui.NewTester(func(c *ui.Context) { other.Draw(c, Options{}) }, 600, 300)
	tt2.Frame()
	if tt2.HasText("Preview") {
		t.Fatal("a Go file offers a preview")
	}
}

func TestHTMLShowsAsCodeOrAsAPage(t *testing.T) {
	d := NewDoc("site/index.html")
	d.content = &proto.FileContent{Path: "site/index.html", Size: 20, Text: "<h1>Hello</h1>\n"}
	d.lines = splitLines(d.content.Text)
	paged := ""
	o := Options{Page: func(c *ui.Context, p string, _ time.Time) {
		paged = p
		ui.Text(c, "the page")
	}}
	tt := ui.NewTester(func(c *ui.Context) { d.Draw(c, o) }, 600, 400)
	tt.Frame()
	if !tt.HasText("Code") || !tt.HasText("Page") || !tt.HasText("<h1>Hello</h1>") {
		t.Fatalf("code: %q", tt.Texts())
	}
	if err := tt.Click("Page"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if paged != "site/index.html" || !tt.HasText("the page") {
		t.Fatalf("page %q: %q", paged, tt.Texts())
	}
	// Where pages can't show, there is no switch.
	tt2 := ui.NewTester(func(c *ui.Context) { NewDoc("a.html").Draw(c, Options{}) }, 600, 300)
	tt2.Frame()
	if tt2.HasText("Page") {
		t.Fatal("a page is offered without web views")
	}
}

func TestPDFShowsAsAPage(t *testing.T) {
	d := NewDoc("docs/spec.pdf")
	d.content = &proto.FileContent{Path: "docs/spec.pdf", Size: 900, Binary: true}
	paged := ""
	o := Options{Page: func(c *ui.Context, p string, _ time.Time) { paged = p }}
	tt := ui.NewTester(func(c *ui.Context) { d.Draw(c, o) }, 600, 300)
	tt.Frame()
	if paged != "docs/spec.pdf" || tt.HasText("A binary file") {
		t.Fatalf("paged %q: %q", paged, tt.Texts())
	}
}

// A folder's menu adds files into it, a file's into the folder it is in,
// the space below the rows into the project folder, as the toolbar does.
func TestAddFilesFromTheTree(t *testing.T) {
	v := New()
	v.loaded, v.root = true, "/work/app"
	v.dirs[""] = proto.FileListing{Root: "/work/app", Entries: []proto.FileEntry{{Name: "docs", Dir: true}, {Name: "README.md"}}}
	v.layout()
	var adds []string
	o := Options{AddFiles: func(folder string, done func()) {
		adds = append(adds, folder)
		done()
	}}
	tt := ui.NewTester(func(c *ui.Context) { v.Draw(c, o) }, 480, 400)
	tt.Frame()
	choose := func(right func() error, item string) {
		t.Helper()
		if err := right(); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if err := tt.Click(item); err != nil {
			t.Fatalf("%v: %q", err, tt.Texts())
		}
		tt.Frame()
	}
	choose(func() error { return tt.RightClick("docs") }, "Add files to docs…")
	choose(func() error { return tt.RightClick("README.md") }, "Add files to the project folder…")
	choose(func() error { tt.RightClickAt(240, 300); return nil }, "Add files to the project folder…")
	choose(func() error { return tt.Click("More") }, "Add files from this device")
	want := []string{"docs", "", "", ""}
	if !slices.Equal(adds, want) {
		t.Fatalf("added into %q, want %q", adds, want)
	}
	if !v.open["docs"] {
		t.Fatal("docs didn't unfold to show what came in")
	}
}

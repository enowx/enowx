package fileview

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
)

// lookShot keeps what tt shows as name under $SHOTS, to compare with the design.
func lookShot(t *testing.T, tt *ui.Tester, name string) {
	t.Helper()
	dir := os.Getenv("SHOTS")
	if dir == "" {
		return
	}
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	_ = png.Encode(f, tt.Image())
}

// The tree in a panel's width beside a file's tab, through the tab's states.
func TestFilesStatesInAPanelAndATab(t *testing.T) {
	v := New()
	v.loaded, v.root = true, "/work/enowx"
	v.dirs[""] = proto.FileListing{Root: "/work/enowx", Entries: []proto.FileEntry{
		{Name: "docs", Dir: true}, {Name: "ui", Dir: true, Status: "M"}, {Name: "build", Dir: true, Ignored: true}, {Name: "go.mod"},
	}}
	v.dirs["docs"] = proto.FileListing{Entries: []proto.FileEntry{{Name: "goa.md", Status: "M"}, {Name: "session-tab-strip-and-center-viewers-behaviour-notes.md"}}}
	v.dirs["ui"] = proto.FileListing{Entries: []proto.FileEntry{{Name: "goa_tabs.go", Status: "A"}, {Name: "goa_test.go", Status: "?"}, {Name: "gone.go", Status: "D"}, {Name: "moved.go", Status: "R"}}}
	v.open["docs"], v.open["ui"] = true, true
	v.layout()
	shown := ""
	o := Options{OpenFile: func(string) {}, GoToFile: func() {}, AddFiles: func(string, func()) {}, Status: v.StatusOf, ShowChanges: func(p string) { shown = p },
		Session: proto.SessionInfo{Dir: "/work/enowx"}, OpenInEditor: func(string, int) {}, EditorName: "Zed", Current: func() string { return "ui/goa_tabs.go" }}
	draw := func(name string, d *Doc, want ...string) *ui.Tester {
		t.Helper()
		tt := ui.NewTester(func(c *ui.Context) {
			c.SetTheme(theme.Window())
			ui.Row(c).Width(1060).Height(640).Background(theme.Frame).TextColor(theme.Text).Children(func() {
				ui.Column(c).Width(740).FillHeight().Children(func() { d.Draw(c, o) })
				ui.Column(c).Width(320).FillHeight().Children(func() { v.Draw(c, o) })
			})
		}, 1060, 640)
		tt.Frame()
		tt.Frame()
		for _, w := range want {
			if !tt.HasText(w) {
				t.Errorf("%s: no %q in %q", name, w, tt.Texts())
			}
		}
		lookShot(t, tt, "files-"+name)
		return tt
	}
	text := func(p, body string, f proto.FileContent) *Doc {
		d := NewDoc(p)
		f.Text = body
		d.content, d.lines = &f, splitLines(body)
		return d
	}
	tt := draw("code", text("ui/goa_tabs.go", "package ui\n\nfunc a() {}\n", proto.FileContent{Size: 26}), "enowx", "goa_tabs.go", "A", "Show changes", "package ui")
	if err := tt.Click("Show changes"); err != nil {
		t.Fatal(err)
	}
	if shown != "ui/goa_tabs.go" {
		t.Fatalf("showed the changes of %q", shown)
	}
	draw("unchanged", text("go.mod", "module x\n", proto.FileContent{Size: 9}), "module x")
	if tt := draw("markdown", text("docs/goa.md", "# GOA\n\nClicking one opens it in a tab.\n", proto.FileContent{Size: 40}), "Preview", "Raw", "GOA"); tt.HasText("Show changes") == false {
		t.Error("a changed Markdown file doesn't offer its changes")
	}
	draw("cut", text("ui/moved.go", "a\nb\n", proto.FileContent{Truncated: true}), "…the rest of this file is too long to show here", "Copy displayed text")
	draw("empty", text("ui/empty.go", "", proto.FileContent{}), "This file is empty.")
	draw("binary", text("a.bin", "", proto.FileContent{Binary: true}), "A binary file: nothing to show as text.")
	d := NewDoc("gone.go")
	d.err = "the file no longer exists, and a failure long enough to have to wrap in the bar"
	draw("error", d, "the file no longer exists, and a failure long enough to have to wrap in the bar")
	draw("reading", NewDoc("a.go"), "Reading…")
	var pic bytes.Buffer
	if err := png.Encode(&pic, image.NewRGBA(image.Rect(0, 0, 400, 300))); err != nil {
		t.Fatal(err)
	}
	shown2 := NewDoc("docs/goa-tabs.png")
	shown2.content = &proto.FileContent{Image: "png", Size: int64(pic.Len())}
	img, w, h, err := decodeImage("png", pic.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	shown2.image, shown2.imgW, shown2.imgH = img, w, h
	draw("image", shown2, "400×300 · "+textfmt.Bytes(uint64(pic.Len())))
	long := text("docs/session-tab-strip-and-center-viewers-behaviour-notes-with-a-rather-long-name-indeed.md", "# x\n", proto.FileContent{Size: 4})
	draw("long", long, "Preview")
	v.err = "the host hung up"
	draw("tree-error", text("go.mod", "m\n", proto.FileContent{}), "the host hung up")
}

package notesview

import (
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
)

// shot saves what tt shows when SHOTS names a folder, to look at.
func shot(t *testing.T, tt *ui.Tester, name string) {
	dir := os.Getenv("SHOTS")
	if dir == "" {
		return
	}
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	_ = png.Encode(f, tt.Image())
}

var layoutPage = `::toc

## Canvas

:::tabs
::tab Summary
- [ ] sketch it
::tab Tests
- [ ] try it, then [[#Sync|go to sync]]
:::

:::toggle More about it
hidden detail
:::

:::callout warning Careful
Don't ship on a Friday {{High}}
:::

:::columns
::col
Left side
::col
Right side
:::

:::cards
::card Sync engine
One place for everything {{Done}}
::card Vault
Secrets {{WIP}}
:::

` + filler + `
## Sync

| Name | Size |
|---|---|
| bravo | 2 |
| alpha | 10 |
| charlie | 1 |

::progress
- [x] design
- [ ] build
`

var filler = strings.Repeat("Some text to scroll past.\n", 40)

// notesPage is the Notes page on notes kept in memory.
type notesPage struct {
	v     View
	notes map[string]proto.Note
	kept  map[string]string
}

func newNotesPage(body string) *notesPage {
	np := &notesPage{notes: map[string]proto.Note{"p1": {NoteInfo: proto.NoteInfo{ID: "p1", Title: "Roadmap"}, Body: body}}, kept: map[string]string{}}
	np.notes["c1"] = proto.Note{NoteInfo: proto.NoteInfo{ID: "c1", Title: "Sync design", Parent: "p1"}}
	return np
}

func (np *notesPage) options() Options {
	var list []proto.NoteInfo
	for _, id := range []string{"p1", "c1"} {
		list = append(list, np.notes[id].NoteInfo)
	}
	return Options{
		Host: "this Mac", Notes: list, Loaded: true,
		Load: func(id string, done func(proto.Note, error)) { done(np.notes[id], nil) },
		Save: func(n proto.Note, done func(proto.NoteInfo, error)) {
			np.notes[n.ID] = n
			done(n.NoteInfo, nil)
		},
		Kept: np.kept,
		Keep: func(k, v string) { np.kept[k] = v },
	}
}

func (np *notesPage) tester(w, h int) *ui.Tester {
	return ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Width(float32(w)).Height(float32(h)).Background(theme.Background).Children(func() { np.v.Draw(c, np.options()) })
	}, w, h)
}

func frames(tt *ui.Tester, n int) {
	for range n {
		tt.Advance(50 * time.Millisecond)
		tt.Frame()
	}
}

// A page laid out in blocks: tabs switch, a toggle opens, a to-do in a
// tab ticks, a table sorts by a column, and the outline, a link and Jump
// to section scroll to a section.
func TestLayoutBlocksOnAPage(t *testing.T) {
	np := newNotesPage(layoutPage)
	tt := np.tester(1300, 760)
	np.v.Select(np.options(), "p1")
	frames(tt, 3)
	for _, want := range []string{"Contents", "Summary", "sketch it", "More about it", "Careful", "Left side", "Right side", "Sync engine", "On this page", "Pages inside", "Sync design"} {
		if !tt.HasText(want) {
			t.Fatalf("no %q in %q", want, tt.Texts())
		}
	}
	if tt.HasText("hidden detail") || tt.HasText("try it, then ") {
		t.Fatal("a folded toggle or a tab not picked shows")
	}
	shot(t, tt, "notes-layout.png")

	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatalf("%v: %q", err, tt.Texts())
		}
		frames(tt, 2)
	}
	click("Tab Tests")
	click("Toggle More about it")
	if !tt.HasText("hidden detail") || tt.HasText("sketch it") {
		t.Fatalf("after picking a tab and opening a toggle: %q", tt.Texts())
	}
	if np.kept["p1 tab tabs::Summary|Tests"] != "Tests" {
		t.Errorf("kept %v", np.kept)
	}
	click("Done") // the to-do in the tab
	if md := np.v.page.ed.markdown(); !strings.Contains(md, "- [x] try it") {
		t.Fatalf("the to-do in a tab: %s", md)
	}

	p := np.v.page
	click("Section Careful")
	frames(tt, 20)
	shot(t, tt, "notes-layout-blocks.png")
	click("Section Sync")
	frames(tt, 20)
	if p.scroll.Y < 300 {
		t.Fatalf("the outline didn't scroll to Sync: at %v", p.scroll.Y)
	}
	click("Sort by Size")
	a, _ := tt.Find("alpha")
	ch, _ := tt.Find("charlie")
	if !(ch.Y < a.Y) {
		t.Errorf("sorted by size: charlie at %v, alpha at %v", ch.Y, a.Y)
	}
	shot(t, tt, "notes-layout-sync.png")

	click("Back to top")
	frames(tt, 20)
	if p.scroll.Y > 1 {
		t.Fatalf("back to top: at %v", p.scroll.Y)
	}
	np.v.OpenJump()
	frames(tt, 2)
	tt.Type("sync")
	tt.Key(0, ui.KeyEnter)
	frames(tt, 20)
	if p.scroll.Y < 300 {
		t.Fatalf("jump to section: at %v", p.scroll.Y)
	}
}

// Read mode opens nothing for typing; Markdown mode shows the page's
// Markdown, and what is typed there is the page.
func TestPageModes(t *testing.T) {
	np := newNotesPage("## One\ntext\n:::toggle Hidden\nx\n:::\n")
	tt := np.tester(1000, 600)
	np.v.Select(np.options(), "p1")
	frames(tt, 2)
	if err := tt.Click("Read mode"); err != nil {
		t.Fatal(err)
	}
	frames(tt, 2)
	if err := tt.Click("text"); err != nil {
		t.Fatal(err)
	}
	frames(tt, 2)
	if np.v.page.ed.open >= 0 {
		t.Fatal("a block opened in read mode")
	}
	if err := tt.Click("Markdown mode"); err != nil {
		t.Fatal(err)
	}
	frames(tt, 2)
	if _, ok := tt.Find("Markdown"); !ok || !strings.Contains(np.v.page.src, ":::toggle Hidden\nx\n:::") {
		t.Fatalf("no Markdown: %q", np.v.page.src)
	}
	np.v.page.src += "- [ ] more\n"
	np.v.page.ed.blocks = parseBlocks(np.v.page.src)
	if err := tt.Click("Edit mode"); err != nil {
		t.Fatal(err)
	}
	frames(tt, 2)
	if !tt.HasText("more") {
		t.Fatalf("what was written as Markdown: %q", tt.Texts())
	}
}

// On a narrow page, columns go one under another.
func TestColumnsStackWhenNarrow(t *testing.T) {
	np := newNotesPage(":::columns\n::col\nLeft side\n::col\nRight side\n:::\n")
	tt := np.tester(560, 500)
	np.v.Select(np.options(), "p1")
	frames(tt, 3)
	l, _ := tt.Find("Left side")
	r, _ := tt.Find("Right side")
	if !(r.Y > l.Y+10) {
		t.Fatalf("columns side by side when narrow: %v %v", l, r)
	}
	shot(t, tt, "notes-layout-narrow.png")
}

// Pages inside show a page's SVG icon as an icon, not its name, and one with
// no icon as a file.
func TestPagesInsideShowIcons(t *testing.T) {
	e := &editor{}
	e.env.children = []proto.NoteInfo{{ID: "a", Title: "With an icon", Icon: "lucide:rocket"}, {ID: "b", Title: "With none"}}
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Width(600).Height(300).Background(theme.Background).Children(func() { e.pagesView(c) })
	}, 600, 300)
	frames(tt, 2)
	if tt.HasText("lucide:rocket") {
		t.Fatalf("the icon's name shows as text: %q", tt.Texts())
	}
	if !tt.HasText("With an icon") || !tt.HasText("With none") {
		t.Fatalf("the pages: %q", tt.Texts())
	}
}

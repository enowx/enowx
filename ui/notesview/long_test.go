package notesview

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/framecheck"
)

func longNoteTester(np *notesPage, o Options) *ui.Tester {
	return ui.NewTester(func(c *ui.Context) {
		c.After(time.Millisecond)
		ui.Column(c).Width(1000).Height(700).Children(func() { np.v.Draw(c, o) })
	}, 1000, 700)
}

// A note of ten megabytes in each shape it can take stays quick, and nothing of it is lost
// for being drawn a part at a time.
func TestLongNotesStayQuick(t *testing.T) {
	lines, line := framecheck.Lines(), framecheck.Line()
	for name, body := range map[string]string{
		"10 MB of lines":   lines,
		"a 1 MB line":      line + "\n",
		"10 MB of code":    "```\n" + lines + "```\n",
		"a 1 MB code line": "```\n" + line + "\n```\n",
		"10 MB of table":   "| a | b |\n|---|---|\n" + strings.Repeat("| x | y |\n", 1<<20),
		"10 MB of layout":  ":::toggle Big\n" + lines + ":::\n",
	} {
		np := newNotesPage(body)
		o := np.options()
		np.v.Select(o, "p1")
		tt := longNoteTester(np, o)
		framecheck.Quick(t, name, tt)
		if got := np.v.page.ed.markdown(); len(got) < len(body)-8 {
			t.Errorf("%s: %d bytes of %d left", name, len(got), len(body))
		}
	}
}

// As Markdown, a note too long for one text area can be read and copied.
func TestLongNoteAsMarkdownStaysQuick(t *testing.T) {
	np := newNotesPage(framecheck.Lines())
	o := np.options()
	np.v.Select(o, "p1")
	tt := longNoteTester(np, o)
	tt.Image()
	np.v.SetMode(modeSource)
	framecheck.Quick(t, "as Markdown", tt)
	if !tt.HasText("Too long to edit as Markdown here") {
		t.Errorf("no word that it's too long: %q", tt.Texts())
	}
}

// A long note draws a part at a time, with the rest a click away. A block too long to type in
// doesn't open, and the page around it still saves whole.
func TestLongNoteShowsMoreAndKeepsItsBlocks(t *testing.T) {
	body := framecheck.Lines()
	np := newNotesPage(body)
	o := np.options()
	np.v.Select(o, "p1")
	tt := longNoteTester(np, o)
	tt.Image()
	e := np.v.page.ed
	if e.drawnTo() != blocksStep {
		t.Fatalf("drawn: %d", e.drawnTo())
	}
	if !tt.HasText("Show 300 more") {
		t.Fatalf("nothing offers the rest: %q", tt.Texts())
	}
	// The offer is below the blocks, out of the window: clicked here by itself.
	more := ui.NewTester(func(c *ui.Context) { e.showMore(c, e.drawnTo()) }, 600, 200)
	more.Frame()
	if err := more.Click("Show 300 more"); err != nil {
		t.Fatal(err)
	}
	more.Frame()
	if e.drawnTo() != 2*blocksStep {
		t.Fatalf("drawn after Show more: %d", e.drawnTo())
	}
	far := len(e.blocks) - 1
	e.openAt(far, 0)
	if e.open != far || e.drawnTo() != far+1 {
		t.Fatalf("a block far down, opened: open %d, drawn %d", e.open, e.drawnTo())
	}
	e.close()

	long := newEditor(framecheck.Line() + "\nshort\n")
	long.openAt(0, -1)
	if long.open >= 0 {
		t.Fatal("a block of a megabyte opened to type in")
	}
	long.openAt(1, -1)
	if long.open != 1 {
		t.Fatalf("the block beside it: open %d", long.open)
	}
	long.blocks[1].text = "changed"
	if got := long.markdown(); !strings.HasSuffix(got, "\nchanged\n") || len(got) < 1<<20 {
		t.Fatalf("after typing beside it: %d bytes ending %q", len(got), got[len(got)-16:])
	}
}

// Worked out once, the Markdown still follows every change to the blocks, however made.
func TestMarkdownFollowsTheBlocks(t *testing.T) {
	e := newEditor("one\ntwo\n")
	if got := e.markdown(); got != "one\ntwo\n" {
		t.Fatalf("read: %q", got)
	}
	e.blocks[1].text = "three"
	if got := e.markdown(); got != "one\nthree\n" {
		t.Fatalf("typed in: %q", got)
	}
	e.blocks = parseBlocks("- [ ] a\n")
	if done, total := e.progress(); done != 0 || total != 1 {
		t.Fatalf("progress: %d of %d", done, total)
	}
	e.blocks[0].done = true
	if done, total := e.progress(); done != 1 || total != 1 {
		t.Fatalf("progress once ticked: %d of %d", done, total)
	}
	e.blocks = parseBlocks("## Title\n")
	if out := e.outline(); len(out) != 1 || out[0].Title != "Title" {
		t.Fatalf("outline: %+v", out)
	}
}

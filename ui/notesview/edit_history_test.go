package notesview

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/enowx/enowx/ui/theme"
)

func editingPage(t *testing.T, body string) (*notesPage, *ui.Tester) {
	t.Helper()
	np := newNotesPage(body)
	np.v.Select(np.options(), "p1")
	data := imageTestData(t)
	tt := ui.NewTester(func(c *ui.Context) {
		o := np.options()
		o.Asset = func(string) ([]byte, error) { return data, nil }
		ui.Column(c).Width(1100).Height(700).Background(theme.Background).Children(func() { np.v.Draw(c, o) })
	}, 1100, 700)
	waitForTestImage(t, tt)
	frames(tt, 2)
	return np, tt
}

func TestNotesUndoRedoTypingAndNewBranch(t *testing.T) {
	np, tt := editingPage(t, "Before\n")
	p := np.v.page
	p.ed.openAt(0, -1)
	tt.Frame()
	tt.Type(" after")
	tt.Frame()
	if p.ed.markdown() != "Before after\n" {
		t.Fatalf("typing: %q", p.ed.markdown())
	}
	tt.Key(ui.Cmd, ui.KeyZ)
	tt.Frame()
	if p.ed.markdown() != "Before\n" {
		t.Fatalf("undo: %q", p.ed.markdown())
	}
	tt.Key(ui.Cmd|ui.Shift, ui.KeyZ)
	tt.Frame()
	if p.ed.markdown() != "Before after\n" {
		t.Fatalf("redo: %q", p.ed.markdown())
	}
	tt.Key(ui.Cmd, ui.KeyZ)
	tt.Frame()
	tt.Type(" different")
	tt.Frame()
	if len(p.edits.future) != 0 || !strings.Contains(p.ed.markdown(), "different") {
		t.Fatalf("typing after undo kept a stale redo: %q", p.ed.markdown())
	}
	if ui.Cmd == ui.Ctrl {
		tt.Key(ui.Cmd, ui.KeyZ)
		tt.Frame()
		tt.Key(ui.Ctrl, ui.KeyY)
		tt.Frame()
		if !strings.Contains(p.ed.markdown(), "different") {
			t.Fatal("Ctrl+Y did not redo")
		}
	}
}

func TestNotesUndoRedoImageDeletionAndCompanionTyping(t *testing.T) {
	const original = "![Image](assets/history/shot.png) Caption\n"
	np, tt := editingPage(t, original)
	p := np.v.page
	if err := tt.Click("Delete Image 1"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if strings.Contains(p.ed.markdown(), "![Image]") {
		t.Fatal("image was not deleted")
	}
	tt.Key(ui.Cmd, ui.KeyZ)
	tt.Frame()
	if p.ed.markdown() != original {
		t.Fatalf("undo lost the image or caption: %q", p.ed.markdown())
	}
	tt.Key(ui.Cmd|ui.Shift, ui.KeyZ)
	tt.Frame()
	if strings.Contains(p.ed.markdown(), "![Image]") || !strings.Contains(p.ed.markdown(), "Caption") {
		t.Fatalf("redo: %q", p.ed.markdown())
	}
	tt.Key(ui.Cmd, ui.KeyZ)
	tt.Frame()
	p.ed.openAt(0, -1)
	tt.Frame()
	tt.Type(" more")
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyZ)
	tt.Frame()
	if p.ed.markdown() != original {
		t.Fatalf("companion undo did not keep its image: %q", p.ed.markdown())
	}
}

func TestNotesUndoTitleAndMarkdown(t *testing.T) {
	np, tt := editingPage(t, "Original body\n")
	p := np.v.page
	if err := tt.Click("Title"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("New title")
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyZ)
	tt.Frame()
	if p.title != "Roadmap" {
		t.Fatalf("title undo: %q", p.title)
	}
	tt.Key(ui.Cmd|ui.Shift, ui.KeyZ)
	tt.Frame()
	if p.title != "New title" {
		t.Fatalf("title redo: %q", p.title)
	}
	tt.Click("Markdown")
	tt.Frame()
	p.sourceFocus = true
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("## Replacement\n")
	tt.Frame()
	if p.src != "## Replacement\n" {
		t.Fatalf("Markdown typing: %q", p.src)
	}
	tt.Key(ui.Cmd, ui.KeyZ)
	tt.Frame()
	if p.src != "Original body\n" {
		t.Fatalf("Markdown undo: %q", p.src)
	}
	tt.Key(ui.Cmd|ui.Shift, ui.KeyZ)
	tt.Frame()
	if p.src != "## Replacement\n" {
		t.Fatalf("Markdown redo: %q", p.src)
	}
}

func TestNotesUndoDoesNotTakeFilterShortcuts(t *testing.T) {
	np, tt := editingPage(t, "Before\n")
	p := np.v.page
	p.ed.openAt(0, -1)
	tt.Frame()
	tt.Type(" change")
	tt.Frame()
	before := p.ed.markdown()
	tt.Click("Filter notes")
	tt.Type("unmatched")
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyZ)
	tt.Frame()
	if np.v.filter != "" {
		t.Fatalf("filter lost its native undo: %q", np.v.filter)
	}
	if p.ed.markdown() != before {
		t.Fatal("filter undo changed the note")
	}
}

func TestNotesUndoImageInsertionAndRename(t *testing.T) {
	np, tt := editingPage(t, "Text\n")
	p := np.v.page
	np.v.imageDone(np.options(), p)("assets/history-insert/photo.png", nil)
	tt.Frame()
	inserted := p.ed.markdown()
	tt.Click("Undo edit")
	tt.Frame()
	if p.ed.markdown() != "Text\n" {
		t.Fatalf("image insertion undo: %q", p.ed.markdown())
	}
	tt.Click("Redo edit")
	tt.Frame()
	if p.ed.markdown() != inserted {
		t.Fatal("image insertion redo failed")
	}
	if err := p.ed.renameImage("assets/history-insert/photo.png", "Plan"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Click("Undo edit")
	tt.Frame()
	if p.ed.markdown() != inserted {
		t.Fatalf("rename undo: %q", p.ed.markdown())
	}
	waitForTestImage(t, tt)
	shot(t, tt, "notes-undo-redo.png")
}

func TestNotesUndoBlockSplitAndReadMode(t *testing.T) {
	np, tt := editingPage(t, "One block\n")
	p := np.v.page
	p.ed.openAt(0, -1)
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if len(p.ed.blocks) != 2 {
		t.Fatal("Enter did not make a block")
	}
	tt.Key(ui.Cmd, ui.KeyZ)
	tt.Frame()
	if p.ed.markdown() != "One block\n" || len(p.ed.blocks) != 1 {
		t.Fatalf("split undo: %q", p.ed.markdown())
	}
	tt.Key(ui.Cmd|ui.Shift, ui.KeyZ)
	tt.Frame()
	if len(p.ed.blocks) != 2 {
		t.Fatal("split redo did not restore the block")
	}
	tt.Click("Read")
	tt.Frame()
	before := p.ed.markdown()
	tt.Key(ui.Cmd, ui.KeyZ)
	tt.Frame()
	if p.ed.markdown() != before || tt.HasText("Undo edit") {
		t.Fatal("Read mode allowed document edits")
	}
}

func TestNotesRenameDialogKeepsItsOwnUndo(t *testing.T) {
	np, tt := editingPage(t, "![Image](assets/dialog-history/photo.png)\n")
	p := np.v.page
	waitForTestImage(t, tt)
	tt.RightClick("Block 1")
	tt.Frame()
	tt.Click("Rename image…")
	frames(tt, 3)
	before := p.ed.markdown()
	tt.Type("New name")
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyZ)
	tt.Frame()
	if p.ed.markdown() != before || p.ed.rename == nil || !p.ed.rename.open || p.ed.rename.name != "Image 1" {
		t.Fatalf("rename dialog undo: body=%q before=%q rename=%+v", p.ed.markdown(), before, p.ed.rename)
	}
}

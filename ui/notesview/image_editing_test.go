package notesview

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/enowx/enowx/ui/theme"
)

func imageEditingTester(t *testing.T, e *editor) *ui.Tester {
	t.Helper()
	data := imageTestData(t)
	e.env = pageEnv{width: 720, asset: func(string) ([]byte, error) { return data, nil }}
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Width(740).Height(680).Background(theme.Background).Children(func() { e.draw(c) })
	}, 740, 680)
	waitForTestImage(t, tt)
	return tt
}

func TestSlashBlocksBesideImage(t *testing.T) {
	for _, tc := range []struct{ query, item, mark string }{
		{"/todo", "To-do list", "- [ ] "},
		{"/heading", "Heading 1", "# "},
		{"/bulleted", "Bulleted list", "- "},
		{"/code", "Code", "```"},
		{"/tabs", "Tabs", ":::tabs"},
	} {
		t.Run(tc.item, func(t *testing.T) {
			e := newEditor("![Image](assets/slash-blocks/shot.png)\n")
			tt := imageEditingTester(t, e)
			e.openAt(0, -1)
			tt.Frame()
			tt.Type(tc.query)
			tt.Frame()
			if !tt.HasText(tc.item) {
				t.Fatalf("no slash choices beside image: %q", tt.Texts())
			}
			if err := tt.Click(tc.item); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
			tt.Type("Check this")
			tt.Frame()
			body := e.markdown()
			if !strings.Contains(body, "![Image](assets/slash-blocks/shot.png)") || !strings.Contains(body, tc.mark) || !strings.Contains(body, "Check this") {
				t.Fatalf("image or companion block was lost: %q", body)
			}
			if !strings.HasPrefix(body, ":::columns\n") {
				t.Fatalf("companion did not persist as columns: %q", body)
			}
			if tc.item == "To-do list" {
				tt.Key(0, ui.KeyEscape)
				tt.Frame()
				if err := tt.Click("Done"); err != nil {
					t.Fatal(err)
				}
				tt.Frame()
				if !strings.Contains(e.markdown(), "- [x] Check this") {
					t.Fatal("companion checkbox did not save")
				}
				shot(t, tt, "image-companion-todo.png")
			}
			reopened := newEditor(e.markdown())
			if _, ok := imageBlockParts(reopened.blocks[0]); !ok {
				t.Fatal("saved image columns did not reopen")
			}
			reopened.env.read = true
			read := imageEditingTester(t, reopened)
			reopened.env.read = true
			read.Frame()
			if !read.HasText("Check this") || read.HasText(":::columns") {
				t.Fatalf("companion was not readable after reopening: %q", read.Texts())
			}
		})
	}
}

func TestRenameImageFromContextMenu(t *testing.T) {
	const ref = "assets/rename-menu/shot.png"
	e := newEditor("![Image](" + ref + ") Caption\n\nSee [@Image 1](" + ref + ")\n")
	tt := imageEditingTester(t, e)
	waitForTestImage(t, tt)
	if err := tt.RightClick("Block 1"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	shot(t, tt, "image-menu.png")
	if err := tt.Click("Rename image…"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("Rename image") {
		t.Fatal("rename dialog did not open")
	}
	shot(t, tt, "image-rename.png")
	tt.Type("Architecture")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if e.rename.open {
		t.Fatalf("rename did not finish: %s", e.rename.err)
	}
	if !strings.Contains(e.markdown(), "![Architecture]("+ref+") Caption") || !strings.Contains(e.markdown(), "[@Architecture]("+ref+")") {
		t.Fatalf("image or mention was not renamed: %q", e.markdown())
	}
	if got := newEditor(e.markdown()).noteImages(); len(got) != 1 || got[0].name != "Architecture" || got[0].ref != ref {
		t.Fatalf("renamed image did not persist: %+v", got)
	}
	shot(t, tt, "image-renamed.png")
}

func TestImageRenameRejectsInvalidName(t *testing.T) {
	e := newEditor("![Image](assets/rename/shot.png)\n")
	before := e.markdown()
	for _, name := range []string{"", "  ", "[broken]", strings.Repeat("x", 81)} {
		if err := e.renameImage("assets/rename/shot.png", name); err == nil {
			t.Fatalf("accepted invalid name %q", name)
		}
		if e.markdown() != before {
			t.Fatal("invalid rename changed the note")
		}
	}
}

func TestImageRenameLeavesCodeExamplesAlone(t *testing.T) {
	const ref = "assets/rename-code/shot.png"
	const example = "`[@Image 1](" + ref + ")`"
	e := newEditor("![Image](" + ref + ")\n" + example + "\n\n[@Image 1](" + ref + ")\n")
	if err := e.renameImage(ref, "Plan"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(e.markdown(), example) || !strings.Contains(e.markdown(), "[@Plan]("+ref+")") {
		t.Fatalf("rename changed a code example: %q", e.markdown())
	}
}

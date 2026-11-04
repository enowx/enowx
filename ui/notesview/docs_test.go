package notesview

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/projectdocs"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
)

// docsTester draws the page with o changed by change.
func docsTester(np *notesPage, w, h int, change func(*Options)) *ui.Tester {
	return ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Width(float32(w)).Height(float32(h)).Background(theme.Background).Children(func() {
			o := np.options()
			change(&o)
			np.v.Draw(c, o)
		})
	}, w, h)
}

// A page of Project Docs gets the hook, without its first line on the page; an
// ordinary page gets neither.
func TestDocsPanelOnlyOnProjectDocsPages(t *testing.T) {
	line := projectdocs.Meta{Kind: "PRD", Status: projectdocs.StatusDraft, Version: 1}.Line()
	np := newNotesPage(line + "\n\nFirst words\n")
	var got []DocsPage
	tt := docsTester(np, 1000, 600, func(o *Options) {
		o.DocsPanel = func(c *ui.Context, p DocsPage) {
			got = append(got, p)
			ui.Text(c, "the panel")
		}
	})
	np.v.Select(np.options(), "p1")
	frames(tt, 3)
	if !tt.HasText("the panel") || len(got) == 0 {
		t.Fatalf("no panel: %q", tt.Texts())
	}
	if !strings.HasPrefix(got[0].Note.Body, line) {
		t.Errorf("its body: %q", got[0].Note.Body)
	}
	if tt.HasText("Project Docs:") || tt.HasText(projectdocs.StatusDraft) {
		t.Errorf("the line shows on the page: %q", tt.Texts())
	}
	if !tt.HasText("First words") {
		t.Errorf("the body: %q", tt.Texts())
	}
	// It stays in the note.
	if md := np.v.page.ed.markdown(); !strings.HasPrefix(md, line) {
		t.Errorf("markdown: %q", md)
	}
	// Markdown mode shows it whole.
	if err := tt.Click("Markdown mode"); err != nil {
		t.Fatal(err)
	}
	frames(tt, 2)
	if !strings.HasPrefix(np.v.page.src, line) {
		t.Errorf("source: %q", np.v.page.src)
	}

	plain := newNotesPage("Project Docs is a nice name\nmore\n")
	plain.notes["p1"] = proto.Note{NoteInfo: proto.NoteInfo{ID: "p1", Title: "Plain"}, Body: "Just words\nmore\n"}
	called := false
	tp := docsTester(plain, 1000, 600, func(o *Options) {
		o.DocsPanel = func(*ui.Context, DocsPage) { called = true }
	})
	plain.v.Select(plain.options(), "p1")
	frames(tp, 3)
	if called {
		t.Error("an ordinary page got the panel")
	}
	if !tp.HasText("Just words") {
		t.Errorf("an ordinary page's first line: %q", tp.Texts())
	}
}

// What's written through the hook is saved as the page's edits are, and says
// so while typed edits are waiting.
func TestDocsPanelRewriteSaves(t *testing.T) {
	line := projectdocs.Meta{Kind: "PRD", Status: projectdocs.StatusDraft, Version: 1}.Line()
	np := newNotesPage(line + "\n\nFirst words\n")
	var page DocsPage
	tt := docsTester(np, 1000, 600, func(o *Options) {
		o.DocsPanel = func(_ *ui.Context, p DocsPage) { page = p }
	})
	np.v.Select(np.options(), "p1")
	frames(tt, 3)
	if page.Unsaved {
		t.Fatal("unsaved before any edit")
	}
	np.v.page.ed.changed = true
	frames(tt, 1)
	if !page.Unsaved {
		t.Fatal("an edit doesn't say unsaved")
	}
	page.Flush()
	frames(tt, 2)
	if page.Unsaved {
		t.Fatal("flush left it unsaved")
	}
	want, err := projectdocs.SetStatus(page.Note.Body, projectdocs.StatusReview)
	if err != nil {
		t.Fatal(err)
	}
	page.Rewrite(want)
	frames(tt, 3)
	if m, _ := projectdocs.Parse(np.notes["p1"].Body); m.Status != projectdocs.StatusReview {
		t.Fatalf("saved: %q", np.notes["p1"].Body)
	}
	if !tt.HasText("First words") {
		t.Errorf("the page: %q", tt.Texts())
	}
}

// Enter in the title goes on in the body, not in the line kept out of sight.
func TestTitleGoesOnPastTheLine(t *testing.T) {
	line := projectdocs.Meta{Kind: "PRD", Status: projectdocs.StatusDraft, Version: 1}.Line()
	e := newEditor(line + "\n\nFirst words\n")
	if e.firstShown() != 1 {
		t.Errorf("first shown: %d", e.firstShown())
	}
	if newEditor("words\n\nmore\n").firstShown() != 0 {
		t.Error("an ordinary page's first block is kept out of sight")
	}
}

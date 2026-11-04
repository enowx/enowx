package notesview

import (
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/projectdocs"
	"github.com/enowx/enowx/proto"
)

// DocsPage is the page open, for what Project Docs shows above it.
type DocsPage struct {
	Note proto.Note // its Body as edited now
	// Unsaved says its edits aren't all on the host yet; Flush saves them now.
	Unsaved bool
	Flush   func()
	// Rewrite makes body the page's and saves it as the user's edits are, so
	// that nothing else writes the page while it's open. Undo starts again from
	// it: a status isn't an edit to take back.
	Rewrite func(body string)
	// Proposals is how many agents' changes wait for it.
	Proposals int
}

// docsPanel asks for what Project Docs shows above the page. The first block
// tells without writing out the whole body, which an ordinary page need not.
func (v *View) docsPanel(c *ui.Context, o Options, p *page) {
	if o.DocsPanel == nil || !isDocsLine(p.ed.blocks) {
		return
	}
	o.DocsPanel(c, DocsPage{
		Note:      proto.Note{NoteInfo: p.note.NoteInfo, Body: p.ed.markdown()},
		Unsaved:   p.loading || p.saving || p.dirty || p.ed.changed,
		Flush:     func() { v.flush(o) },
		Rewrite:   func(body string) { p.rewrite(body) },
		Proposals: len(v.proposalsFor(o, p.id)),
	})
}

// rewrite makes body the page's and has it saved at the next chance.
func (p *page) rewrite(body string) {
	ed := newEditor(body)
	ed.env = p.ed.env
	p.ed, p.edits = ed, editHistory{}
	if p.mode == modeSource {
		p.src = body
	}
	p.dirty, p.edited = true, time.Time{}
	p.editGeneration++
}

// isDocsLine says blocks start with a Project Docs line.
func isDocsLine(blocks []block) bool {
	if len(blocks) == 0 || blocks[0].kind != kText {
		return false
	}
	_, ok := projectdocs.Parse(blocks[0].text)
	return ok
}

// firstShown is the first block a page shows: a Project Docs line stays in the
// note, but its strip says it.
func (e *editor) firstShown() int {
	if len(e.blocks) > 1 && isDocsLine(e.blocks) {
		return 1
	}
	return 0
}

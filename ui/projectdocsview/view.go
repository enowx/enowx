// Package projectdocsview is what the Notes page shows of Project Docs: making
// a project, its summary on the project's page, and a strip on each document's.
// It reads and writes through the notes calls the app gives it, so a new app
// on an old daemon works, and the documents stay ordinary notes.
package projectdocsview

import (
	"slices"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/projectdocs"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/widget"
)

// Options are what the view shows and does. Done funcs run on the main thread.
type Options struct {
	Host   string
	Team   string // the space shown, "" for the user's own
	Notes  []proto.NoteInfo
	Load   func(id string, done func(proto.Note, error))
	Save   func(n proto.Note, done func(proto.NoteInfo, error))
	Delete func(id string, done func(error))
	// Read reads a note; it blocks, and is called off the main thread.
	Read func(id string) (proto.Note, error)
	// Main runs fn on the main thread.
	Main func(fn func())
	// Open opens a note on the page.
	Open func(id string)
	// AttachProjects opens attaching a note to projects, and Attach adds
	// projects to one; nil leaves the summary without them.
	AttachProjects func(n proto.NoteInfo)
	Attach         func(id string, projects []proto.NoteProject, done func(proto.NoteInfo, error))
	// ExportRepo writes the project as files in folder on the host, and
	// overwrites files not exported from Notes only with force; nil offers none.
	ExportRepo func(id, folder string, force bool, done func(written string, err error))
	// Folder is where project p is on this host, "" when it isn't here.
	Folder func(p proto.NoteProject) string
}

// Page is the page open as the page has it this frame.
type Page struct {
	Note proto.Note // its Body as edited now
	// Unsaved says its edits aren't all on the host yet; Flush saves them now.
	Unsaved bool
	Flush   func()
	// Rewrite makes body the page's and saves it, as the user's edits are:
	// the page alone writes itself, so nothing overtakes what is typed.
	Rewrite func(body string)
	// Proposals is how many agents' changes wait for this page.
	Proposals int
}

// deriveEvery is how often a project is read again at most, while its notes
// change.
const deriveEvery = 1500 * time.Millisecond

// View is the view's state. It belongs to the main thread, but for what a
// derive holds while it runs: reads is handed to its goroutine and back.
type View struct {
	// For tests: the clock, and what makes a project.
	now         func() time.Time
	makeProject func(projectdocs.Options, projectdocs.Saver) (proto.NoteInfo, []string, error)

	// The project as last derived, and what it was derived from.
	model     *model
	openID    string
	sumList   uint64
	sumTree   uint64
	wantLoad  bool
	loading   *struct{} // the derive running: results of any other are dropped
	started   time.Time
	loadErr   string
	reads     map[string]proto.Note
	crumbs    []proto.NoteInfo
	crumbsFor string
	crumbsSum uint64

	// The open document's body, and what it says.
	seenBody string
	seenDoc  docView

	pending *pendingWrite
	note    string // says how the last action went
	noteErr bool

	menu    widget.ContextMenu
	form    *newForm
	export  *exportForm
	confirm *confirmation
	apply   *applyState
}

// pendingWrite is a change to the page's body that waits for its edits to be
// saved first.
type pendingWrite struct {
	id     string
	change func(body string) (string, error)
}

func (v *View) clock() time.Time {
	if v.now != nil {
		return v.now()
	}
	return time.Now()
}

// Panel draws what the page shows above a project note or a document, for
// Page pg, and has its writes done.
func (v *View) Panel(c *ui.Context, o Options, pg Page) {
	m, ok := projectdocs.Parse(pg.Note.Body)
	if !ok || m.Kind == projectdocs.KindTemplates {
		return
	}
	v.follow(c, o, pg.Note.ID)
	v.runWrite(c, pg)
	switch {
	case m.IsProject():
		v.summaryPanel(c, o, pg, m)
	case m.IsDocument():
		v.stripPanel(c, o, pg)
	default:
		v.breadcrumb(c, o, pg.Note.ID)
	}
}

// Dialogs draws the view's dialogs, while the Notes page shows, whichever page
// is open.
func (v *View) Dialogs(c *ui.Context, o Options) {
	v.newDialog(c, o)
	v.exportDialog(c, o)
	v.confirmDialog(c)
}

// current is the project as derived, when it holds the note.
func (v *View) current(id string) *model {
	if v.model.has(id) {
		return v.model
	}
	return nil
}

// follow reads the project again when a note of it changed. It costs nothing
// while they don't, and no more often than deriveEvery, as a project of sixty
// documents is that many calls to the host and as many hashes.
func (v *View) follow(c *ui.Context, o Options, id string) {
	sum := listSum(o.Notes)
	if id != v.openID || sum != v.sumList {
		tree := treeSum(o.Notes, id)
		if tree != v.sumTree || !v.model.has(id) {
			v.wantLoad = true
		}
		v.openID, v.sumList, v.sumTree = id, sum, tree
	}
	if !v.wantLoad || v.loading != nil {
		return
	}
	if wait := deriveEvery - v.clock().Sub(v.started); wait > 0 && !v.started.IsZero() {
		c.After(wait)
		return
	}
	v.derive(o, id)
}

// derive reads and works out the project of note id, off the main thread.
// What's read stays between derives, so that only the notes changed are read
// again.
func (v *View) derive(o Options, id string) {
	run := &struct{}{}
	v.loading, v.wantLoad, v.started = run, false, v.clock()
	list, cache, read := slices.Clone(o.Notes), v.reads, o.Read
	v.reads = nil
	go func() {
		updated := make(map[string]time.Time, len(list))
		for _, n := range list {
			updated[n.ID] = n.Updated
		}
		fresh := make(map[string]proto.Note, len(cache))
		p, err := projectdocs.Load(list, id, func(n string) (proto.Note, error) {
			if had, ok := cache[n]; ok && had.Updated.Equal(updated[n]) {
				fresh[n] = had
				return had, nil
			}
			got, err := read(n)
			if err == nil {
				fresh[n] = got
			}
			return got, err
		})
		var m *model
		if err == nil {
			m = modelOf(p)
		}
		o.Main(func() {
			if v.loading != run {
				return // another host's, or replaced since
			}
			v.loading, v.reads = nil, fresh
			if err != nil {
				v.loadErr = err.Error()
				return
			}
			v.model, v.loadErr = m, ""
		})
	}()
}

// write asks for a change to the page's body, which waits for what's typed.
func (v *View) write(id string, change func(body string) (string, error)) {
	v.pending = &pendingWrite{id: id, change: change}
}

// runWrite does the write waiting, once the page's edits are saved: a status
// must not take the place of what the user just typed, nor be overwritten by
// the save of a body from before it.
func (v *View) runWrite(c *ui.Context, pg Page) {
	w := v.pending
	if w == nil {
		return
	}
	if w.id != pg.Note.ID {
		v.pending = nil
		return
	}
	if pg.Unsaved {
		pg.Flush()
		c.After(100 * time.Millisecond)
		return
	}
	v.pending = nil
	body, err := w.change(pg.Note.Body)
	if err != nil {
		v.tell(err.Error(), true)
		return
	}
	v.tell("", false)
	if body != pg.Note.Body {
		pg.Rewrite(body)
		v.seenBody = "" // says the new status at once
	}
}

func (v *View) tell(msg string, failed bool) { v.note, v.noteErr = msg, failed }

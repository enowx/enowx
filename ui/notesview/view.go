// Package notesview is the Notes page: the user's pages on the connected
// host, nested like a notebook's, each edited a block at a time. The app
// gives it the pages and how to read and write them through Options.
package notesview

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// Options are what the page shows and does: the host's pages, and calls
// to it, whose done funcs run on the main thread (the app's Update).
type Options struct {
	// ListOutside: the app draws the pages' tree in its own sidebar
	// (DrawList), so the page leaves it out.
	ListOutside bool
	// OpenLink follows a link the app knows (canvas://…), false for others,
	// which the system opens.
	OpenLink func(link string) bool
	Host     string // its name, said where notes are kept
	// Space is the name of the team whose space Notes shows, "" for the
	// user's own; Team its ID. Notes are that space's alone.
	Space, Team string
	Notes       []proto.NoteInfo
	Loaded      bool   // Notes came at least once
	Err         string // why they didn't
	Load        func(id string, done func(proto.Note, error))
	Save        func(n proto.Note, done func(proto.NoteInfo, error))
	Delete      func(id string, done func(error))
	Move        func(id, parent, before string, done func(error))
	// Proposals are the changes agents proposed, which Accept makes and
	// Reject forgets.
	Proposals []Proposal
	Accept    func(id string, done func(proto.NoteInfo, error))
	Reject    func(id string, done func(error))
	// CanAsk offers Ask an agent, whose panel the app draws beside the
	// page while AssistantOpen.
	CanAsk bool
	// Groups are the groups of notes, like folders, which the calls after
	// change; nil calls leave the page without groups.
	Groups      []proto.NoteGroup
	SaveGroup   func(from string, g proto.NoteGroup, done func(error))
	DeleteGroup func(name string, done func(error))
	MoveGroup   func(name, before string, done func(error))
	SetGroup    func(id, group string, done func(error))
	// Spaces are the spaces the page may switch to, the user's own first;
	// PickSpace switches to one.
	Spaces    []widget.Space
	PickSpace func(team string)
	// SyncState says how the notes are synced with the other hosts, and
	// whether that calls for the user; OpenSync shows the details.
	SyncState func() (string, bool)
	OpenSync  func()
	// History lists a page's earlier versions, Version reads one and
	// Restore brings it back; nil History offers none.
	// Asset reads a file a note shows (an image an import brought); it
	// blocks, and is called off the main thread.
	Asset func(ref string) ([]byte, error)
	// PickImage chooses and uploads a device image; PasteImage takes a
	// clipboard image. An empty reference means the picker was canceled.
	PickImage  func(id string, done func(ref string, err error))
	PasteImage func(id string, done func(ref string, err error)) bool
	History    func(id string, done func([]proto.NoteVersion, error))
	Version    func(id, version string, done func(proto.Note, error))
	Restore    func(id, version string, done func(error))
	// Kept is how the user left pages' views (tabs picked, toggles open,
	// the outline folded), which Keep changes: kept by the app, per
	// device, not in the notes.
	Kept map[string]string
	Keep func(key, value string)
	// IconPrefs reads the icon picker's preferences, which SaveIconPrefs
	// keeps; OpenSVG reads an SVG file the user picks, nil for no file.
	IconPrefs     func(done func(proto.NoteIconPrefs, error))
	SaveIconPrefs func(p proto.NoteIconPrefs, done func(proto.NoteIconPrefs, error))
	OpenSVG       func(done func([]byte, error))
	// AttachProjects opens the attaching of page n to projects, and
	// ProjectState says how its project p is: ProjectOnline, ProjectOffline
	// or ProjectBroken; nil AttachProjects offers none.
	AttachProjects func(n proto.NoteInfo)
	ProjectState   func(p proto.NoteProject) string
	// Import brings in notes from other apps, Export writes the notes a
	// pick names as a file, and Drop imports files dropped on the page;
	// nil leaves the page without them.
	Import func()
	Export func(p ExportPick)
	Drop   func(paths []string)
	// MoveSpace opens moving page n, with those under it, to another
	// space (the user's own or a team's); nil while there is none.
	MoveSpace func(n proto.NoteInfo)
	// DocsPanel draws what Project Docs shows above a page of theirs, and
	// NewDocs opens making a project; nil leaves the page without them.
	DocsPanel func(c *ui.Context, p DocsPage)
	NewDocs   func()
}

// ExportPick is the notes an export takes: a page with those under it, a
// group's, or all; Title names them.
type ExportPick struct {
	ID, Group string
	All       bool
	Title     string
}

// saveAfter is how long after the last change a page is saved.
const saveAfter = 700 * time.Millisecond

// View is the Notes page's state. It belongs to the main thread.
type View struct {
	// treeInSidebar: the tree draws in the app's sidebar (DrawList).
	treeInSidebar bool
	selected      string
	page          *page
	filter        string
	folded        map[string]bool // pages whose pages under them are hidden
	menu          widget.ContextMenu
	deleting      *proto.NoteInfo
	history       *historyDialog
	narrow        bool // too narrow for the tree beside the page
	treeOpen      bool // the tree shown though narrow
	err           string
	// assistant shows the agent's panel beside the page.
	assistant bool
	undo      *undoAccept
	// groupFilter shows one group's pages alone, "" all of them.
	groupFilter string
	// projectFilter shows one project's pages, one host's, those of none,
	// or groups them by project (projects.go); "" shows them all.
	projectFilter   string
	allBadges       map[string]bool // pages whose project badges all show
	groupForm       *groupForm
	deletingGroup   *proto.NoteGroup
	ungroupedFolded bool
	jump            jumpState
	outlineHidden   bool
	kept            map[string]string // Kept as changed here, before the app has it
	picker          iconPicker
	preview         *imagePreview
}

// AssistantOpen says the agent's panel shows; the app follows its chat
// only then.
func (v *View) AssistantOpen() bool { return v.assistant && v.page != nil }

// SetAssistant opens or closes the agent's panel beside the page.
func (v *View) SetAssistant(open bool) { v.assistant = open }

// SelectedNote is the page open, "" for none.
func (v *View) SelectedNote() string {
	if v.page == nil {
		return ""
	}
	return v.page.id
}

// reload reads page pg again, as after a change made elsewhere.
func (v *View) reload(o Options, pg *page) {
	pg.loading = true
	o.Load(pg.id, func(n proto.Note, err error) {
		pg.loading = false
		if err == nil && !pg.dirty {
			pg.note, pg.title, pg.icon, pg.ed = n, n.Title, n.Icon, newEditor(n.Body)
			pg.edits = editHistory{}
		}
	})
}

// page is the page open: as last read, and as being edited.
type page struct {
	id                      string
	note                    proto.Note // as the host last had it
	title                   string
	icon                    string
	ed                      *editor
	loading                 bool
	saving                  bool
	uploading               bool // an image is being selected or uploaded
	dirty                   bool
	edited                  time.Time
	icons                   bool // the icon picker is open
	listed                  bool // the host's list had it: one made now isn't yet
	mode                    int  // modeEdit, modeRead or modeSource
	src                     string
	edits                   editHistory
	editGeneration          int
	titleFocus, sourceFocus bool
	// Where the page was last frame: scrolled, its scroll's box, its
	// area's and its text's widths.
	scroll   ui.ScrollState
	viewport ui.Rect
	area     float32
	width    float32
	// jumpTo is the section to scroll to, once laid out; revealed says
	// its tab or toggle was opened.
	jumpTo   string
	revealed bool
}

// Select opens page id, as a link to it does.
func (v *View) Select(o Options, id string) {
	if v.page != nil && v.page.id == id {
		return
	}
	v.flush(o)
	v.selected = id
	v.page = &page{id: id, loading: true}
	p := v.page
	o.Load(id, func(n proto.Note, err error) {
		p.loading = false
		if err != nil {
			v.err = err.Error()
			return
		}
		p.note, p.title, p.icon, p.ed = n, n.Title, n.Icon, newEditor(n.Body)
	})
}

// Draw builds the page: the pages' tree beside the page open.
func (v *View) Draw(c *ui.Context, o Options) {
	v.follow(o)
	row := ui.Row(c).Grow(1).FillWidth().FillHeight().MinHeight(0)
	// Narrow, the page open takes the room and the tree folds away, a
	// button away.
	v.narrow = row.Bounds().W > 0 && row.Bounds().W < 640
	row.Children(func() {
		if (!v.narrow || v.page == nil || v.treeOpen) && !o.ListOutside {
			v.sidebar(c, o)
		}
		ui.Column(c).Grow(1).MinWidth(0).FillHeight().Background(theme.Background).Children(func() {
			if v.page == nil {
				v.empty(c, o)
				return
			}
			v.pageView(c, o)
		})
	})
	v.deleteDialog(c, o)
	v.historySheet(c, o)
	v.groupDialog(c, o)
	v.deleteGroupDialog(c, o)
	v.imageDialog(c, o)
	v.save(c, o)
}

// follow takes in what changed on the host: the page open, changed by
// another app or an agent while not being edited here, is read again; a
// page gone closes.
func (v *View) follow(o Options) {
	p := v.page
	if p == nil || p.loading || p.saving || p.dirty {
		return
	}
	for _, n := range o.Notes {
		if n.ID != p.id {
			continue
		}
		p.listed = true
		if n.Updated.After(p.note.Updated) && (p.ed == nil || p.ed.open < 0) {
			p.loading = true
			o.Load(p.id, func(full proto.Note, err error) {
				p.loading = false
				if err == nil && !p.dirty {
					p.note, p.title, p.icon, p.ed = full, full.Title, full.Icon, newEditor(full.Body)
					p.edits = editHistory{}
				}
			})
		}
		return
	}
	if p.listed {
		v.page, v.selected = nil, "" // deleted
	}
}

// save writes the page open once it has rested from changes.
func (v *View) save(c *ui.Context, o Options) {
	p := v.page
	if p == nil || p.ed == nil {
		return
	}
	if p.ed.changed {
		p.ed.changed, p.dirty, p.edited = false, true, time.Now()
	}
	if !p.dirty || p.saving {
		return
	}
	if wait := saveAfter - time.Since(p.edited); wait > 0 {
		c.After(wait)
		return
	}
	v.write(o, p)
}

// flush saves the page open now, if it changed, as another opens.
func (v *View) flush(o Options) {
	if p := v.page; p != nil && p.ed != nil && (p.dirty || p.ed.changed) && !p.saving {
		v.write(o, p)
	}
}

func (v *View) write(o Options, p *page) {
	n := p.note
	n.Title, n.Icon, n.Body = strings.TrimSpace(p.title), p.icon, p.ed.markdown()
	p.saving, p.dirty, p.ed.changed = true, false, false
	o.Save(n, func(info proto.NoteInfo, err error) {
		p.saving = false
		if err != nil {
			v.err, p.dirty = err.Error(), true
			return
		}
		v.err = ""
		p.note.NoteInfo, p.note.Body = info, n.Body
	})
}

// newPage makes a page under parent, or at the top in group ("" for
// none), and opens it.
func (v *View) newPage(o Options, parent, group string) {
	if parent != "" {
		delete(v.folded, parent)
	}
	o.Save(proto.Note{NoteInfo: proto.NoteInfo{Parent: parent, Group: group}}, func(info proto.NoteInfo, err error) {
		if err != nil {
			v.err = err.Error()
			return
		}
		v.Select(o, info.ID)
		if v.page != nil {
			v.page.note.NoteInfo = info
		}
	})
}

func (v *View) empty(c *ui.Context, o Options) {
	ui.Column(c).Grow(1).FillWidth().Center().Gap(12).Padding(32).Children(func() {
		ui.Icon(c, icon.Notebook).FontSize(30).TextColor(theme.Muted)
		msg := "Pages of notes, plans and checklists, kept on " + o.Host + ". Your agents read and write them too."
		if o.Team != "" {
			msg = o.Space + "'s pages: notes, plans and checklists of the team, kept on " + o.Host + ". Your agents read and write them too."
		}
		if !o.Loaded && o.Err == "" {
			msg = "Reading the notes…"
		}
		ui.Text(c, msg).FontSize(12.5).TextColor(theme.Muted).TextAlign(ui.Center).MaxWidth(420)
		if o.Err != "" {
			ui.Text(c, o.Err).FontSize(12).TextColor(theme.Error).TextAlign(ui.Center)
		}
		if o.Loaded && widget.PrimaryButton(c, icon.Plus, "New page", "").Clicked() {
			v.newPage(o, "", "")
		}
		if o.Loaded && o.NewDocs != nil && widget.GhostButton(c, "New project docs…").Clicked() {
			o.NewDocs()
		}
	})
}

func titleOf(n proto.NoteInfo) string {
	if strings.TrimSpace(n.Title) == "" {
		return "Untitled"
	}
	return n.Title
}

// pageItems are a page's menu, in the tree and over the page.
func (v *View) pageItems(c *ui.Context, o Options, n proto.NoteInfo, done func()) {
	if widget.MenuItem(c, icon.Plus, "Add a page inside", false) {
		v.newPage(o, n.ID, "")
		done()
	}
	if o.SetGroup != nil && len(o.Groups) > 0 {
		for _, g := range o.Groups {
			if g.Name != n.Group && widget.MenuItem(c.Key("to:"+g.Name), icon.Folder, "Move to "+g.Name, false) {
				o.SetGroup(n.ID, g.Name, v.failed)
				done()
			}
		}
		if (n.Group != "" || n.Parent != "") && widget.MenuItem(c, icon.Close, "Move out of groups", false) {
			o.SetGroup(n.ID, "", v.failed)
			done()
		}
	}
	if n.Parent != "" && widget.MenuItem(c, icon.ArrowUp, "Move to the top level", false) {
		o.Move(n.ID, "", "", v.failed)
		done()
	}
	if o.AttachProjects != nil && widget.MenuItem(c, icon.Git, "Attach to projects…", false) {
		o.AttachProjects(n)
		done()
	}
	if o.History != nil && widget.MenuItem(c, icon.File, "Version history…", false) {
		v.openHistory(o, n)
		done()
	}
	if o.MoveSpace != nil && n.Parent == "" && widget.MenuItem(c, icon.ArrowRight, "Move or copy to a space…", false) {
		o.MoveSpace(n)
		done()
	}
	if o.Export != nil && widget.MenuItem(c, icon.ArrowUp, "Export…", false) {
		o.Export(ExportPick{ID: n.ID, Title: titleOf(n)})
		done()
	}
	widget.MenuSeparator(c)
	if widget.MenuItem(c, icon.Trash, "Delete…", true) {
		v.deleting = &n
		done()
	}
}

func (v *View) failed(err error) {
	if err != nil {
		v.err = err.Error()
	}
}

// deleteDialog confirms deleting a page, and those under it.
func (v *View) deleteDialog(c *ui.Context, o Options) {
	if v.deleting == nil {
		return
	}
	n := *v.deleting
	under := 0
	for changed, gone := true, map[string]bool{n.ID: true}; changed; {
		changed = false
		for _, x := range o.Notes {
			if !gone[x.ID] && gone[x.Parent] {
				gone[x.ID], changed = true, true
				under++
			}
		}
	}
	msg := "It can't be brought back."
	if under > 0 {
		msg = "The pages under it go too. It can't be brought back."
	}
	open := true
	switch ui.AlertDialog(c, &open, "Delete “"+titleOf(n)+"”?", msg, "Cancel", "Delete") {
	case 1:
		if v.page != nil && v.page.id == n.ID {
			v.page, v.selected = nil, ""
		}
		o.Delete(n.ID, v.failed)
		v.deleting = nil
	case 0, ui.AlertDismissed:
		v.deleting = nil
	}
}

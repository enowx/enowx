package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/iconlib"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/notesview"
	"github.com/enowx/enowx/ui/projectdocsview"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// pageNotes is the rail's Notes page: the connected host's notes.
const pageNotes = "notes"

// hostNotes is the connected host's notes, as last listed.
type hostNotes struct {
	host      string
	list      []proto.NoteInfo
	proposals []proto.NoteProposal // agents' changes waiting for the user
	groups    []proto.NoteGroup
	loaded    bool
	err       string
}

// notesOf is the connected host's notes, listed anew when the host
// changed; the notes event keeps them current.
func (a *App) notesOf() *hostNotes {
	if a.noteList == nil || a.noteList.host != a.host.ID {
		a.noteList = &hostNotes{host: a.host.ID}
		a.notes = notesview.View{}
		a.projectDocs = projectdocsview.View{}
		hn, cl := a.noteList, a.client
		if cl != nil {
			go func() {
				list, err := cl.Notes()
				props, _ := cl.NoteProposals() // none from a daemon of before
				groups, _ := cl.NoteGroups()
				a.onMain(func() {
					hn.list, hn.proposals, hn.groups, hn.loaded = list, props, groups, err == nil
					if err != nil {
						hn.err = client.Explain(err)
					}
				})
			}()
		}
	}
	return a.noteList
}

// notesEvent takes the host's notes as they changed.
func (a *App) notesEvent(data json.RawMessage) func() {
	var list []proto.NoteInfo
	if json.Unmarshal(data, &list) != nil {
		return nil
	}
	return func() {
		if hn := a.noteList; hn != nil && hn.host == a.host.ID {
			hn.list, hn.loaded, hn.err = list, true, ""
		}
	}
}

func (a *App) notesPage(c *ui.Context) {
	a.loadCatalogs()
	defer a.attachDialog(c) // the Notes page alone attaches notes
	defer func() { a.projectDocs.Dialogs(c, a.projectDocsOptions()) }()
	a.notes.Draw(c, a.notesOptions())
}

func (a *App) notesOptions() notesview.Options {
	hn, cl := a.notesOf(), a.client
	// call runs a call to the host off the main thread, its result back on
	// it.
	call := func(fn func() error, done func(error)) {
		go func() {
			err := fn()
			a.onMain(func() { done(err) })
		}()
	}
	team := a.space()
	return notesview.Options{
		ListOutside: a.navShown(),
		OpenLink:    a.openCanvasLink,
		Host:        a.host.Name,
		Notes:       a.notesInSpace(hn.list),
		Space:       a.spaceName(a.space()),
		Team:        a.space(),
		Loaded:      hn.loaded,
		Err:         hn.err,
		Load: func(id string, done func(proto.Note, error)) {
			var n proto.Note
			call(func() (err error) { n, err = cl.Note(id); return err }, func(err error) { done(n, err) })
		},
		Save: func(n proto.Note, done func(proto.NoteInfo, error)) {
			var info proto.NoteInfo
			if n.ID == "" && n.Parent == "" {
				n.Team = a.space() // made in the space shown
			}
			call(func() (err error) { info, err = cl.SaveNote(n); return err }, func(err error) { done(info, err) })
		},
		Delete: func(id string, done func(error)) { call(func() error { return cl.DeleteNote(id) }, done) },
		Move: func(id, parent, before string, done func(error)) {
			call(func() error { return cl.MoveNote(id, parent, before) }, done)
		},
		Asset:      cl.NoteAsset,
		PickImage:  func(id string, done func(string, error)) { a.pickNoteImage(cl, id, done) },
		PasteImage: func(id string, done func(string, error)) bool { return a.pasteNoteImage(cl, id, done) },
		History: func(id string, done func([]proto.NoteVersion, error)) {
			var list []proto.NoteVersion
			call(func() (err error) { list, err = cl.NoteHistory(id); return err }, func(err error) { done(list, err) })
		},
		Version: func(id, version string, done func(proto.Note, error)) {
			var n proto.Note
			call(func() (err error) { n, err = cl.NoteVersion(id, version); return err }, func(err error) { done(n, err) })
		},
		Restore: func(id, version string, done func(error)) {
			call(func() error { _, err := cl.RestoreNote(id, version); return err }, done)
		},
		Proposals: a.notesProposals(),
		Accept: func(id string, done func(proto.NoteInfo, error)) {
			var info proto.NoteInfo
			call(func() (err error) { info, err = cl.AcceptNoteProposal(id); return err }, func(err error) { done(info, err) })
		},
		Reject:         func(id string, done func(error)) { call(func() error { return cl.RejectNoteProposal(id) }, done) },
		CanAsk:         a.featureOn("notes-agent"),
		SyncState:      a.notesSyncState,
		Spaces:         a.spaceChoices(),
		PickSpace:      a.showSpace,
		AttachProjects: a.openAttach,
		ProjectState:   a.projectState,
		OpenSync: func() {
			if team := a.space(); team != "" {
				a.syncSpaceNow(team)
				return
			}
			a.page, a.settingsSection = pageSettings, sectionSync
		},
		Groups: a.groupsInSpace(hn.groups),
		SaveGroup: func(from string, g proto.NoteGroup, done func(error)) {
			g.Team = a.space()
			call(func() error { _, err := cl.SaveNoteGroup(from, g); return err }, done)
		},
		DeleteGroup: func(name string, done func(error)) {
			call(func() error { _, err := cl.DeleteNoteGroup(team, name); return err }, done)
		},
		MoveGroup: func(name, before string, done func(error)) {
			call(func() error { _, err := cl.MoveNoteGroup(team, name, before); return err }, done)
		},
		SetGroup: func(id, group string, done func(error)) {
			call(func() error { _, err := cl.SetNoteGroup(id, group); return err }, done)
		},
		Kept: a.state.NotesView,
		Keep: func(key, value string) {
			if a.state.NotesView == nil {
				a.state.NotesView = map[string]string{}
			}
			if value == "" {
				delete(a.state.NotesView, key)
			} else {
				a.state.NotesView[key] = value
			}
		},
		IconPrefs: func(done func(proto.NoteIconPrefs, error)) {
			var p proto.NoteIconPrefs
			call(func() (err error) { p, err = cl.NoteIconPrefs(); return err }, func(err error) { done(p, err) })
		},
		SaveIconPrefs: func(p proto.NoteIconPrefs, done func(proto.NoteIconPrefs, error)) {
			var out proto.NoteIconPrefs
			call(func() (err error) { out, err = cl.SaveNoteIconPrefs(p); return err }, func(err error) { done(out, err) })
		},
		OpenSVG: func(done func([]byte, error)) {
			go func() {
				data, err := a.openSVGFile()
				if err == nil && data == nil {
					return // canceled
				}
				a.onMain(func() { done(data, err) })
			}()
		},
		MoveSpace: a.moveSpaceOption(),
		Import:    func() { a.openNoteImport(nil) },
		Export:    a.openNoteExport,
		Drop:      a.openNoteImport,
		DocsPanel: a.projectDocsPanel,
		NewDocs:   a.projectDocs.OpenNew,
	}
}

// openSVGFile asks the user for an SVG file of this device and reads it;
// nil when they canceled.
func (a *App) openSVGFile() ([]byte, error) {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.win, Title: "An SVG for the icon",
		Filters: []mygo.FileFilter{{Name: "SVG", Extensions: []string{"svg"}}}})
	if err != nil || len(paths) == 0 {
		return nil, err
	}
	info, err := os.Stat(paths[0])
	if err != nil {
		return nil, err
	}
	if info.Size() > iconlib.MaxSVG {
		return nil, fmt.Errorf("%s is %d KB: an icon's SVG is %d KB at most", filepath.Base(paths[0]), info.Size()>>10, iconlib.MaxSVG>>10)
	}
	return os.ReadFile(paths[0])
}

// groupsEvent takes the groups of notes as they changed.
func (a *App) groupsEvent(data json.RawMessage) func() {
	var list []proto.NoteGroup
	if json.Unmarshal(data, &list) != nil {
		return nil
	}
	return func() {
		if hn := a.noteList; hn != nil && hn.host == a.host.ID {
			hn.groups = list
		}
	}
}

// notesAgentPanel is the Notes page's agent, beside the workspace as the
// side panel is beside a session, while the page asks for it.
func (a *App) notesAgentPanel(c *ui.Context) {
	if a.page != pageNotes || a.client == nil || !a.notes.AssistantOpen() || a.noteList == nil {
		return
	}
	id := a.notes.SelectedNote()
	var n proto.NoteInfo
	for _, x := range a.noteList.list {
		if x.ID == id {
			n = x
		}
	}
	if n.ID == "" {
		return
	}
	widget.Bento(ui.Column(c.Key("notes-agent")).Width(min(400, max(300, a.winW*0.4))).MinWidth(minPanelWidth).Shrink(1).FillHeight(), theme.Background).
		Transition(widget.PanelMotion(24)).Children(func() { a.notesAssistant(c, n) })
}

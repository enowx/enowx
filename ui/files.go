package ui

import (
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/fileview"
	"github.com/enowx/enowx/ui/textfmt"
)

// The Files tool of the side panel (see package fileview), and the files
// opened from it, each in a tab of the panel's top row. Tabs belong to
// their session, as the panel follows the selected one.

// fileTabPrefix starts a center tab for a file: "file:<path>".
const fileTabPrefix = "file:"

func (a *App) fileViewOf(id string) *fileview.View {
	if a.fileViews == nil {
		a.fileViews = map[string]*fileview.View{}
	}
	v := a.fileViews[id]
	if v == nil {
		v = fileview.New()
		a.fileViews[id] = v
	}
	return v
}

// fileOptions connect session s's file views to the app; wanted tells
// whether one still shows.
func (a *App) fileOptions(s proto.SessionInfo, wanted func() bool) fileview.Options {
	ed := a.preferredEditor()
	return fileview.Options{
		Client:       a.client,
		Session:      s,
		Update:       a.onMain,
		Wanted:       wanted,
		EditorName:   ed.name,
		OpenInEditor: func(path string, line int) { a.openInEditor(ed, path, line) },
		OpenFile:     func(p string) { a.openFileTab(s, p) },
		Page:         a.pageOption(s),
		AddFiles:     func(folder string, done func()) { a.addFiles(s, folder, done) },
		GoToFile:     a.openQuickOpen,
		Current:      func() string { p, _ := strings.CutPrefix(a.centerFront(s), fileTabPrefix); return p },
	}
}

// addFiles asks for files and folders of this device and copies them
// into folder of session s's folder, on its host, telling how it goes.
func (a *App) addFiles(s proto.SessionInfo, folder string, done func()) {
	cl := a.client
	if cl == nil {
		return
	}
	where := s.Dir
	if folder != "" {
		where = path.Join(s.Dir, folder)
	}
	about := "Copied into " + textfmt.ShortHome(where) + " on " + a.host.Name + ", folders with all they hold. Names taken there get a number."
	a.localPick.Open("Add to "+path.Base(where), about, func(paths []string) {
		go func() {
			n := 0
			err := cl.AddFiles(s.ID, paths, folder, func(string) {
				n++
				if n%10 == 1 {
					count := n
					a.onMain(func() {
						a.status = fmt.Sprintf("Copying into %s… %d %s", path.Base(where), count, textfmt.Plural(count, "file", "files"))
					})
				}
			})
			a.onMain(func() {
				if err != nil {
					a.status = "Couldn't add the files: " + client.Explain(err)
				} else {
					a.notify(fmt.Sprintf("Added %d %s to %s", n, textfmt.Plural(n, "file", "files"), path.Base(where)))
				}
				done()
			})
		}()
	})
}

// filesTab draws the Files tool for session s.
func (a *App) filesTab(c *ui.Context, s proto.SessionInfo) {
	v := a.fileViewOf(s.ID)
	gen := a.gen
	v.Draw(c, a.fileOptions(s, func() bool {
		return gen == a.gen && a.panelShows(panelFiles) && a.selected == s.ID && a.fileViews[s.ID] == v
	}))
}

// openFileTab shows the file at p of session s's folder in a center tab,
// opening one for it unless it has one.
func (a *App) openFileTab(s proto.SessionInfo, p string) { a.openCenter(s, fileTabPrefix+p) }

// openFileAt shows the file at p of session s's folder at lines from to
// to, marked.
func (a *App) openFileAt(s proto.SessionInfo, p string, from, to int) {
	if a.docs == nil {
		a.docs = map[string]*fileview.Doc{}
	}
	key := s.ID + "\x00" + p
	if a.docs[key] == nil {
		a.docs[key] = fileview.NewDoc(p)
	}
	if from > 0 {
		a.docs[key].GoTo(from, to)
	}
	a.openFileTab(s, p)
}

// showFile shows the file at p of session s's folder in its tab.
func (a *App) showFile(s proto.SessionInfo, p string) { a.openFileTab(s, p) }

// pageOption is how session s's HTML files show as pages, nil where this
// device has no web view.
func (a *App) pageOption(s proto.SessionInfo) func(c *ui.Context, p string, modified time.Time) {
	if !webViewAvailable() {
		return nil
	}
	return func(c *ui.Context, p string, modified time.Time) { a.filePage(c, s, p, modified) }
}

// docTab draws session s's tab of the file at p.
func (a *App) docTab(c *ui.Context, s proto.SessionInfo, p string) {
	if a.docs == nil {
		a.docs = map[string]*fileview.Doc{}
	}
	key := s.ID + "\x00" + p
	d := a.docs[key]
	if d == nil {
		d = fileview.NewDoc(p)
		a.docs[key] = d
	}
	gen := a.gen
	d.Draw(c, a.fileOptions(s, func() bool {
		return gen == a.gen && a.centerShows(s.ID, fileTabPrefix+p) && a.selected == s.ID && a.docs[key] == d
	}))
}

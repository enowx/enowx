// Package fileview is the side panel's Files tool: the tree of a session's
// folder, with what git says changed (View), and the text of a file opened
// from it in a tab of its own (Doc). The daemon reads them, so they work
// the same for a session on a server; both read again every few seconds
// while they show, since the agent keeps changing them.
package fileview

import (
	"path"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/widget"
)

// refreshEvery is how often a shown view reads its open folders again.
const refreshEvery = 4 * time.Second

// Options connect a view to the app.
type Options struct {
	Client  *client.Client
	Session proto.SessionInfo
	// Update runs a function on the main thread; Wanted, called there,
	// reports whether the view still shows, else its poller stops.
	Update func(func())
	Wanted func() bool
	// OpenInEditor opens an absolute path of the host at a line (0 for
	// none) in the editor named EditorName.
	OpenInEditor func(path string, line int)
	EditorName   string
	// OpenFile opens the file at a path of the session's folder, in a tab.
	OpenFile func(path string)
	// Page draws the HTML file at a path as a web page, reloading it when
	// the file changed (modified); nil where pages can't show.
	Page func(c *ui.Context, path string, modified time.Time)
	// AddFiles copies files and folders the user picks on this device into
	// a folder of the session's folder ("" is the folder itself), wherever
	// the session runs, and calls done after; nil hides it.
	AddFiles func(folder string, done func())
	// GoToFile finds a file by name; Current is the file in front, to mark it
	// in the tree.
	GoToFile func()
	Current  func() string
	// Status is git's mark on the file at a path, "" for none or not known;
	// ShowChanges shows what changed in it.
	Status      func(path string) string
	ShowChanges func(path string)
}

// StatusOf is git's mark on the file at p, as the tree last read it.
func (v *View) StatusOf(p string) string {
	for _, r := range v.rows {
		if r.path == p {
			return r.entry.Status
		}
	}
	return ""
}

// Added reads the folder again and unfolds it: files were added to it.
func (v *View) Added(folder string) {
	v.open[folder] = true
	v.reload()
	v.layout()
}

// View is one session's file tree. It belongs to the main thread; its
// poller reports through Options.Update.
type View struct {
	root    string                       // the session's folder, absolute
	dirs    map[string]proto.FileListing // folders read, by path ("" is the root)
	open    map[string]bool              // folders unfolded
	err     string
	loaded  bool
	polling bool
	refresh chan struct{} // asks the poller to read now

	rows []row
	list ui.ListState
	menu widget.ContextMenu
	ask  *asking // the dialog a menu opened
	// rowMenu: a row took this frame's right-click, which the space
	// around the rows doesn't then take too.
	rowMenu bool
}

// New returns a view with nothing read.
func New() *View {
	return &View{dirs: map[string]proto.FileListing{}, open: map[string]bool{"": true}, refresh: make(chan struct{}, 1)}
}

// follow reads the open folders while the view is wanted, unless it
// already does.
func (v *View) follow(o Options) {
	if v.polling || o.Client == nil {
		return
	}
	v.polling = true
	cl, id := o.Client, o.Session.ID
	go func() {
		for {
			var paths []string
			keep := make(chan bool, 1)
			o.Update(func() {
				if !o.Wanted() {
					v.polling = false
					keep <- false
					return
				}
				for p := range v.open {
					paths = append(paths, p)
				}
				keep <- true
			})
			if !<-keep {
				return
			}
			read := map[string]proto.FileListing{}
			var failed error
			for _, p := range paths {
				l, err := cl.ListFiles(id, p)
				if err != nil {
					if p == "" {
						failed = err
					}
					continue
				}
				read[p] = l
			}
			o.Update(func() {
				v.loaded = true
				v.err = ""
				if failed != nil {
					v.err = client.Explain(failed)
				}
				for p, l := range read {
					v.dirs[p] = l
					v.root = l.Root
				}
				v.layout()
			})
			select {
			case <-time.After(refreshEvery):
			case <-v.refresh:
			}
		}
	}()
}

// reload asks the poller to read now.
func (v *View) reload() {
	select {
	case v.refresh <- struct{}{}:
	default:
	}
}

// toggle unfolds the folder at p, reading it, or folds it.
func (v *View) toggle(p string) {
	if v.open[p] {
		delete(v.open, p)
	} else {
		v.open[p] = true
		v.reload()
	}
	v.layout()
}

// foldAll folds every folder but the root.
func (v *View) foldAll() {
	v.open = map[string]bool{"": true}
	v.layout()
}

// row is a row of the tree: an entry of a folder read, at a depth.
type row struct {
	path  string
	depth int
	entry proto.FileEntry
}

// layout lays the tree out again, as a new slice: the list may still be
// drawing the old one this frame.
func (v *View) layout() {
	var rows []row
	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		for _, e := range v.dirs[dir].Entries {
			p := path.Join(dir, e.Name)
			rows = append(rows, row{path: p, depth: depth, entry: e})
			if e.Dir && v.open[p] {
				walk(p, depth+1)
			}
		}
	}
	walk("", 0)
	v.rows = rows
}

func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	lines := []string{}
	start := 0
	for i := 0; i < len(text); i++ {
		if text[i] == '\n' {
			lines = append(lines, text[start:i])
			start = i + 1
		}
	}
	if start < len(text) {
		lines = append(lines, text[start:])
	}
	return lines
}

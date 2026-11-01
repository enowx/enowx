package fileview

import (
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// Changing the folder's files from the tree's menus, through the session's
// daemon: new files and folders, renames, copies, deletes, and what a
// file is. Each asks what it needs in a dialog of its own.

// asking is the dialog open: a name to type, a delete to confirm, or a
// file's properties.
type asking struct {
	change string // a proto.File* change, or "stat"
	path   string // what it is about: the folder for new ones
	dir    bool
	name   string // as typed
	busy   bool
	err    string
	stat   *proto.FileStat
}

// rowItems are a row's menu: what to make in its folder, and what to do
// with it.
func (v *View) rowItems(c *ui.Context, o Options, r row, into string, done func()) {
	full := r.path
	v.newItems(c, o, into, done)
	widget.MenuSeparator(c)
	if widget.MenuItem(c, icon.Pencil, "Rename…", false) {
		done()
		v.ask = &asking{change: proto.FileRename, path: full, dir: r.entry.Dir, name: r.entry.Name}
	}
	if widget.MenuItem(c, icon.Copy, "Duplicate", false) {
		done()
		v.change(o, proto.FileDuplicate, full, "")
	}
	if widget.MenuItem(c, icon.Copy, "Copy path", false) {
		done()
		c.WriteClipboard(path.Join(v.root, full))
	}
	if widget.MenuItem(c, icon.Copy, "Copy relative path", false) {
		done()
		c.WriteClipboard(full)
	}
	if widget.MenuItem(c, icon.List, "Properties", false) {
		done()
		v.ask = &asking{change: "stat", path: full, dir: r.entry.Dir}
		v.stat(o, full)
	}
	widget.MenuSeparator(c)
	if widget.MenuItem(c, icon.Trash, "Delete…", true) {
		done()
		v.ask = &asking{change: proto.FileDelete, path: full, dir: r.entry.Dir}
	}
}

// newItems make a file or folder in folder into, or bring files in.
func (v *View) newItems(c *ui.Context, o Options, into string, done func()) {
	where := "the project folder"
	if into != "" {
		where = path.Base(into)
	}
	if widget.MenuItem(c, icon.File, "New file in "+where+"…", false) {
		done()
		v.ask = &asking{change: proto.FileNewFile, path: into}
	}
	if widget.MenuItem(c, icon.Folder, "New folder in "+where+"…", false) {
		done()
		v.ask = &asking{change: proto.FileNewFolder, path: into, dir: true}
	}
	if o.AddFiles != nil && widget.MenuItem(c, icon.FolderPlus, "Add files to "+where+"…", false) {
		done()
		o.AddFiles(into, func() { v.Added(into) })
	}
}

// change makes a change through the daemon, then reads the folder again;
// a new file opens.
func (v *View) change(o Options, change, p, name string) {
	cl, id := o.Client, o.Session.ID
	if cl == nil {
		return
	}
	if v.ask != nil {
		v.ask.busy, v.ask.err = true, ""
	}
	go func() {
		got, err := cl.ChangeFile(id, change, p, name)
		o.Update(func() {
			if err != nil {
				if v.ask != nil {
					v.ask.busy, v.ask.err = false, client.Explain(err)
				} else {
					v.err = client.Explain(err)
				}
				return
			}
			v.ask = nil
			parent := path.Dir(got)
			switch change {
			case proto.FileNewFolder, proto.FileNewFile:
				parent = p
			case proto.FileDelete:
				parent = path.Dir(p)
			}
			if parent == "." {
				parent = ""
			}
			v.Added(parent)
			if change == proto.FileNewFile && o.OpenFile != nil {
				o.OpenFile(got)
			}
		})
	}()
}

func (v *View) stat(o Options, p string) {
	cl, id := o.Client, o.Session.ID
	if cl == nil {
		return
	}
	go func() {
		st, err := cl.StatFile(id, p)
		o.Update(func() {
			if v.ask == nil || v.ask.change != "stat" || v.ask.path != p {
				return
			}
			if err != nil {
				v.ask.err = client.Explain(err)
				return
			}
			v.ask.stat = &st
		})
	}()
}

// askDialog draws the open dialog.
func (v *View) askDialog(c *ui.Context, o Options) {
	a := v.ask
	if a == nil {
		return
	}
	open := true
	switch a.change {
	case "stat":
		v.properties(c, a, &open)
	case proto.FileDelete:
		widget.Dialog(c, &open, 440, 12, func() {
			what := "file"
			if a.dir {
				what = "folder, with all it holds"
			}
			widget.DialogHeader(c, "Delete "+path.Base(a.path)+"?", "The "+what+" goes for good: it isn't moved to a trash. Git can bring back what it tracks.", false)
			v.askError(c, a)
			widget.DialogFooter(c, "", func() {
				if widget.GhostButton(c, "Cancel").Clicked() {
					open = false
				}
				if widget.DangerButton(c, "Delete").Disabled(a.busy).Clicked() && !a.busy {
					v.change(o, proto.FileDelete, a.path, "")
				}
			})
		})
	default:
		title, button := "New file", "Create"
		switch a.change {
		case proto.FileNewFolder:
			title = "New folder"
		case proto.FileRename:
			title, button = "Rename "+path.Base(a.path), "Rename"
		}
		where := "the project folder"
		if dir := a.path; a.change == proto.FileRename {
			dir = path.Dir(a.path)
			if dir != "." {
				where = dir
			}
		} else if dir != "" {
			where = dir
		}
		widget.Dialog(c, &open, 440, 12, func() {
			widget.DialogHeader(c, title, "In "+where+".", false)
			in := widget.Input(c, &a.name, "name").FillWidth().AutoFocus().Font(theme.MonoFont)
			v.askError(c, a)
			submit := in.Submitted()
			widget.DialogFooter(c, "", func() {
				if widget.GhostButton(c, "Cancel").Clicked() {
					open = false
				}
				if widget.PrimaryButton(c, nil, button, "").Disabled(a.busy).Clicked() {
					submit = true
				}
			})
			if submit && !a.busy && strings.TrimSpace(a.name) != "" {
				v.change(o, a.change, a.path, a.name)
			}
		})
	}
	if !open && v.ask == a {
		v.ask = nil
	}
}

func (v *View) askError(c *ui.Context, a *asking) {
	if a.err != "" {
		ui.Text(c, a.err).FontSize(theme.TextLabel).TextColor(theme.Error)
	}
}

// properties shows what a file or folder is.
func (v *View) properties(c *ui.Context, a *asking, open *bool) {
	widget.Dialog(c, open, 480, 12, func() {
		widget.DialogHeader(c, path.Base(a.path), a.path, false)
		v.askError(c, a)
		st := a.stat
		if st == nil {
			if a.err == "" {
				widget.Spinner(c, 14)
			}
			return
		}
		kind := "File"
		size := textfmt.Bytes(uint64(max(st.Size, 0)))
		if st.Dir {
			kind = "Folder"
			size = fmt.Sprintf("%s in %d %s, %d %s", size, st.Files, textfmt.Plural(st.Files, "file", "files"), st.Folders, textfmt.Plural(st.Folders, "folder", "folders"))
			if st.Partial {
				size += ", at least"
			}
		}
		if st.Link != "" {
			kind += ", a link to " + st.Link
		}
		rows := [][2]string{{"Kind", kind}, {"Size", size}, {"Where", st.Abs}, {"Modified", st.Modified.Local().Format("Jan 2, 2006 15:04") + " · " + textfmt.Age(st.Modified, time.Now()) + " ago"},
			{"Permissions", st.Mode}}
		if st.Owner != "" {
			rows = append(rows, [2]string{"Owner", st.Owner})
		}
		switch {
		case st.Ignored:
			rows = append(rows, [2]string{"Git", "ignored"})
		case st.Status != "":
			rows = append(rows, [2]string{"Git", gitWords(st.Status)})
		}
		ui.Column(c).FillWidth().Gap(6).Children(func() {
			for _, r := range rows {
				ui.Row(c.Key(r[0])).FillWidth().Gap(12).Children(func() {
					ui.Text(c, r[0]).FontSize(theme.TextLabel).TextColor(theme.Muted).Width(90).Shrink(0)
					ui.Text(c, r[1]).FontSize(theme.TextUI).Selectable().Grow(1).Shrink(1)
				})
			}
		})
		widget.DialogFooter(c, "", func() {
			if widget.GhostButton(c, "Close").Clicked() {
				*open = false
			}
		})
	})
}

func gitWords(status string) string {
	switch status {
	case "M":
		return "modified"
	case "A":
		return "added"
	case "?":
		return "new, untracked"
	case "D":
		return "deleted"
	case "R":
		return "renamed"
	}
	return status
}

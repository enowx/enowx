// Package folderpicker is the Open a folder dialog, and its Choose a file
// variant. It browses the connected host's folders through its daemon, so
// picking works the same on this device and on a server.
package folderpicker

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// Picker is the dialog. It belongs to the main thread; listings arriving
// late are dropped by seq.
type Picker struct {
	open    bool
	client  *client.Client
	update  func(func()) // runs a function on the main thread
	listing proto.FolderListing
	loaded  bool
	loading bool
	err     string
	path    string // the path bar, as typed
	hidden  bool
	seq     int

	naming  bool // the new folder row is showing
	newName string

	// files picks a file rather than a folder: title and about say which;
	// chosen is the one picked.
	files        bool
	title, about string
	chosen       string
}

// Open shows the dialog at the home folder of cl's host; update runs a
// function on the main thread.
func (f *Picker) Open(cl *client.Client, update func(func())) {
	*f = Picker{open: true, client: cl, update: update}
	f.browse("~")
}

// OpenFile shows the dialog choosing a file of cl's host, starting at
// home, with title and about saying what for. A macOS app bundle (.app)
// counts as a file.
func (f *Picker) OpenFile(cl *client.Client, update func(func()), title, about string) {
	*f = Picker{open: true, client: cl, update: update, files: true, title: title, about: about}
	f.browse("~")
}

// Close hides the dialog.
func (f *Picker) Close() { f.open = false }

func (f *Picker) browse(path string) {
	f.seq++
	seq, cl, params := f.seq, f.client, proto.FolderListParams{Path: path, Hidden: f.hidden, WithFiles: f.files}
	f.loading, f.err, f.naming, f.chosen = true, "", false, ""
	go func() {
		l, err := cl.FolderList(params)
		f.update(func() {
			if f.seq != seq || !f.open {
				return
			}
			f.loading = false
			if err != nil {
				f.err = strings.TrimPrefix(err.Error(), "folders: ")
				return
			}
			f.listing, f.loaded = l, true
			f.path = textfmt.TildePath(l.Path, l.Home)
		})
	}()
}

func (f *Picker) makeFolder() {
	name := strings.TrimSpace(f.newName)
	if name == "" || strings.ContainsAny(name, `/\`) {
		f.err = "Type a name for the new folder, without slashes."
		return
	}
	cl, path := f.client, textfmt.JoinPath(f.listing.Path, name)
	go func() {
		made, err := cl.MakeFolder(path)
		f.update(func() {
			if !f.open {
				return
			}
			if err != nil {
				f.err = strings.TrimPrefix(err.Error(), "folders: ")
				return
			}
			f.newName = ""
			f.browse(made)
		})
	}()
}

// View draws the dialog while it is open. hostName names the host it
// browses; pick is called with the folder the user opens.
func (f *Picker) View(c *ui.Context, hostName string, pick func(path string)) {
	if !f.open {
		return
	}
	title, about := "Open a folder", "Each folder is a project; a git repository gets branches and worktrees."
	if f.files {
		title, about = f.title, f.about
	}
	widget.Dialog(c, &f.open, 620, 12, func() {
		if widget.DialogHeader(c, title, about, true, hostName, title) {
			f.open = false
		}
		f.toolbar(c)
		f.list(c)
		if f.err != "" {
			ui.Text(c, f.err).FontSize(theme.TextLabel).TextColor(theme.Error)
		}
		f.footer(c, pick)
	})
}

func (f *Picker) toolbar(c *ui.Context) {
	ui.Row(c).FillWidth().Gap(6).Children(func() {
		up := widget.IconButton(c, icon.ArrowUp, "Up one folder", true)
		if up.Clicked() && f.listing.Parent != "" {
			f.browse(f.listing.Parent)
		}
		if widget.IconButton(c, icon.Home, "Home folder", true).Clicked() {
			f.browse("~")
		}
		in := widget.Input(c, &f.path, "Folder path").Font(theme.MonoFont).Grow(1)
		if in.Submitted() {
			f.browse(f.path)
		}
		if f.listing.Git {
			widget.Tag(c, "git", theme.Done)
		}
	})
}

func (f *Picker) list(c *ui.Context) {
	ui.Scroll(c).FillWidth().Height(320).Radius(theme.RadiusCard).Background(theme.Sunken).Border(1, theme.BorderStrong).Children(func() {
		ui.Column(c).FillWidth().Padding(4).Gap(1).Children(func() {
			switch {
			case !f.loaded && f.loading:
				ui.Text(c, "Reading…").TextColor(theme.Muted).Padding(12)
				return
			case !f.loaded:
				return
			}
			if f.naming {
				f.newFolderRow(c)
			}
			for _, e := range f.listing.Folders {
				if f.files && isBundle(e.Name) {
					f.fileRow(c.Key(e.Name), e)
				} else {
					f.row(c.Key(e.Name), e)
				}
			}
			if f.files {
				for _, e := range f.listing.FileNames {
					f.fileRow(c.Key("f:"+e.Name), e)
				}
				if len(f.listing.Folders)+len(f.listing.FileNames) == 0 {
					ui.Text(c, "Nothing here.").TextColor(theme.Muted).Padding(12)
				}
				return
			}
			if len(f.listing.Folders) == 0 && !f.naming {
				empty := "No folders here."
				if f.listing.Files > 0 {
					empty = fmt.Sprintf("No folders here, %d %s.", f.listing.Files, textfmt.Plural(f.listing.Files, "file", "files"))
				}
				ui.Text(c, empty).TextColor(theme.Muted).Padding(12)
			}
		})
	})
}

func (f *Picker) row(c *ui.Context, e proto.FolderEntry) {
	row := ui.ButtonBase(c).Label(e.Name)
	widget.ListRow(c, row, false, "", func() {
		svg, col := icon.Folder, theme.TextSoft
		if e.Git {
			svg, col = icon.Git, theme.Done
		}
		ui.Icon(c, svg).FontSize(14).TextColor(col)
		ui.Text(c, e.Name).SingleLine().Grow(1).Shrink(1)
		if e.Git {
			widget.Tag(c, "git", theme.Done)
		}
		ui.Icon(c, icon.ChevronRight).FontSize(12).TextColor(theme.Muted)
	})
	if row.Clicked() {
		f.browse(textfmt.JoinPath(f.listing.Path, e.Name))
	}
}

// isBundle is a macOS app bundle, which is picked as a file.
func isBundle(name string) bool { return strings.HasSuffix(name, ".app") }

func (f *Picker) fileRow(c *ui.Context, e proto.FolderEntry) {
	path := textfmt.JoinPath(f.listing.Path, e.Name)
	row := ui.ButtonBase(c).Label(e.Name)
	widget.ListRow(c, row, f.chosen == path, "", func() {
		svg := icon.File
		if isBundle(e.Name) {
			svg = icon.AppWindow
		}
		ui.Icon(c, svg).FontSize(14).TextColor(theme.TextSoft)
		ui.Text(c, e.Name).SingleLine().Grow(1).Shrink(1)
	})
	if row.Clicked() {
		f.chosen = path
	}
}

func (f *Picker) newFolderRow(c *ui.Context) {
	ui.Row(c).FillWidth().Gap(8).Padding(4, 6).Children(func() {
		ui.Icon(c, icon.FolderPlus).FontSize(14).TextColor(theme.Accent)
		in := widget.Input(c, &f.newName, "New folder name").AutoFocus().Grow(1)
		if in.Submitted() {
			f.makeFolder()
		}
		if widget.GhostButton(c, "Create").Clicked() {
			f.makeFolder()
		}
		if widget.GhostButton(c, "Cancel").Clicked() {
			f.naming, f.newName = false, ""
		}
	})
}

func (f *Picker) footer(c *ui.Context, pick func(path string)) {
	if f.files {
		ui.Row(c).FillWidth().Gap(8).Children(func() {
			if widget.Switch(c, &f.hidden, "Show hidden files") {
				f.browse(f.listing.Path)
			}
			ui.Text(c, "Hidden files").FontSize(theme.TextLabel).TextColor(theme.TextSoft)
			ui.Box(c).Grow(1)
			if widget.GhostButton(c, "Cancel").Clicked() {
				f.open = false
			}
			label := "Choose"
			if f.chosen != "" {
				label = "Choose " + textfmt.LastPathPart(f.chosen)
			}
			b := widget.PrimaryButton(c, icon.Check, label, "")
			if f.chosen == "" {
				b.Disabled(true).Opacity(0.5)
			} else if b.Clicked() {
				f.open = false
				pick(f.chosen)
			}
		})
		return
	}
	ui.Row(c).FillWidth().Gap(8).Children(func() {
		if widget.Switch(c, &f.hidden, "Show hidden folders") {
			f.browse(f.listing.Path)
		}
		ui.Text(c, "Hidden folders").FontSize(theme.TextLabel).TextColor(theme.TextSoft)
		ui.Box(c).Grow(1)
		if widget.GhostButton(c, "New folder").Clicked() && f.loaded {
			f.naming = true
		}
		if widget.GhostButton(c, "Cancel").Clicked() {
			f.open = false
		}
		name := textfmt.LastPathPart(f.listing.Path)
		b := widget.PrimaryButton(c, icon.FolderPlus, "Open "+name, "")
		if !f.loaded || f.loading {
			b.Disabled(true).Opacity(0.5)
		} else if b.Clicked() {
			f.open = false
			pick(f.listing.Path)
		}
	})
}

package fileview

import (
	"path"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// rowHeight is the height of a tree row; lineHeight, of a line of a file.
const (
	rowHeight  = 28
	lineHeight = 19
	indent     = 16
)

// Draw shows the tree.
func (v *View) Draw(c *ui.Context, o Options) {
	v.follow(o)
	v.askDialog(c, o)
	ui.Column(c).Grow(1).FillWidth().MinHeight(0).Padding(0, theme.S3, theme.S3, theme.S3).Gap(theme.S2).Children(func() {
		v.toolbar(c, o)
		if v.err != "" {
			widget.Notice(c, nil, theme.Del, v.err, "", nil)
		}
		switch {
		case !v.loaded:
			centered(c, "Reading the folder…")
		case len(v.rows) == 0:
			centered(c, "This folder is empty.")
		default:
			v.tree(c, o)
		}
	})
}

func centered(c *ui.Context, text string) {
	ui.Column(c).Grow(1).FillWidth().Center().Padding(theme.S5).Children(func() {
		ui.Text(c, text).FontSize(theme.TextUI).TextColor(theme.TextSoft).TextAlign(ui.Center)
	})
}

// toolbar names the folder and offers to add files to it, fold it or
// read it again.
func (v *View) toolbar(c *ui.Context, o Options) {
	ui.Row(c).Height(theme.ControlHeight).Shrink(0).FillWidth().PaddingX(theme.S1).Gap(theme.S2).Children(func() {
		ui.Icon(c, icon.Folder).FontSize(14).TextColor(theme.TextSoft)
		name := filepath.Base(v.root)
		if v.root == "" {
			name = "Files"
		}
		ui.Row(c).Grow(1).MinWidth(0).Children(func() {
			widget.Running(c, name, widget.RunStyle{Weight: 500}).Tooltip(textfmt.ShortHome(v.root))
		})
		if o.GoToFile != nil && widget.IconButton(c, icon.Search, "Go to a file", false).Clicked() {
			o.GoToFile()
		}
		more := widget.IconButton(c, icon.More, "More", false)
		v.menu.OpenBelow(more, "more", theme.ControlHeight)
		v.menu.Show(c, more, "more", func(done func()) {
			if o.AddFiles != nil && widget.MenuItem(c, icon.FolderPlus, "Add files from this device", false) {
				o.AddFiles("", func() { v.Added("") })
				done()
			}
			if widget.MenuItem(c, icon.Collapse, "Fold every folder", false) {
				v.foldAll()
				done()
			}
			if widget.MenuItem(c, icon.Sync, "Read again", false) {
				v.reload()
				done()
			}
		})
	})
}

// tree is the virtual list of the folders unfolded.
func (v *View) tree(c *ui.Context, o Options) {
	rows := v.rows
	v.list.Key = func(i int) any { return rows[i].path }
	v.rowMenu = false
	list := widget.List(c.Key("tree"), &v.list, len(rows), func(i int) {
		v.treeRow(c, o, rows[i])
	}).Grow(1).FillWidth().MinHeight(0)
	// The space around the rows is the session's folder's.
	if !v.rowMenu {
		v.menu.Open(list, "into:")
	}
	v.menu.Show(c, list, "into:", func(done func()) { v.newItems(c, o, "", done) })
}

// treeRow is a folder, which unfolds, or a file, which opens in the center.
// A long name runs inside its row.
func (v *View) treeRow(c *ui.Context, o Options, r row) {
	e := r.entry
	on := !e.Dir && o.Current != nil && o.Current() == r.path
	b := ui.ButtonBase(c).FillWidth().Height(rowHeight).Padding(0, theme.S3, 0, theme.S3+float32(r.depth*indent)).Gap(theme.S2).
		Radius(theme.RadiusCard).Justify(ui.Start).Cursor(ui.CursorPointer).Label(r.path)
	bg := theme.Fade(theme.Selected, 0)
	switch {
	case on:
		bg = theme.Selected
	case b.Hovered():
		bg = theme.Hover
	}
	b.Background(widget.QuickTint(b, "bg", bg)).Children(func() {
		svg := icon.File
		switch {
		case e.Dir:
			svg = icon.Folder
		case isPicture(e.Name):
			svg = icon.Image
		}
		letter, col := statusLetter(e.Status)
		name := theme.Text
		if e.Ignored {
			name = theme.SoftOn(on)
		}
		ui.Icon(c, svg).FontSize(14).TextColor(theme.SoftOn(on))
		ui.Row(c).Grow(1).MinWidth(0).Children(func() {
			widget.Running(c, e.Name, widget.RunStyle{Color: name, Always: on})
		})
		if letter != "" {
			ui.Text(c, letter).FontSize(theme.TextLabel).FontWeight(500).TextColor(col).SingleLine().Shrink(0)
		}
	})
	// Its menu: into the folder, or the one the file is in.
	into := r.path
	if !e.Dir {
		into = path.Dir(r.path)
		if into == "." {
			into = ""
		}
	}
	if b.RightClicked() {
		v.rowMenu = true
	}
	v.menu.Open(b, "row:"+r.path)
	v.menu.Show(c, b, "row:"+r.path, func(done func()) { v.rowItems(c, o, r, into, done) })
	if b.Clicked() {
		switch {
		case e.Dir:
			v.toggle(r.path)
		case o.OpenFile != nil:
			o.OpenFile(r.path)
		}
	}
}

// isPicture says the file at p is an image, by its name.
func isPicture(p string) bool {
	switch strings.ToLower(path.Ext(p)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".ico", ".bmp":
		return true
	}
	return false
}

// statusLetter is how the tree marks git's status, and its color; "" for
// none.
func statusLetter(status string) (string, ui.Color) {
	switch status {
	case "":
		return "", theme.Text
	case "A":
		return "A", theme.Done
	case "?":
		return "U", theme.Done // untracked: new to git
	case "D":
		return "D", theme.Error
	case "R":
		return "R", theme.Accent
	}
	return "M", theme.Warning
}

package diffview

import (
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// rowHeight is the height of a diff line: the list's rows all share it,
// so a long diff scrolls without measuring every line.
const rowHeight = 20

// Kinds of row.
const (
	rowHunk    = iota // "@@ -1,3 +1,4 @@"
	rowLine           // a line of a hunk
	rowPair           // a removed and an added line side by side
	rowBinary         // a file shown without its lines
	rowCut            // the file goes on past what the daemon sent
	rowComment        // a review comment, under its line
	rowDraft          // the comment being written
)

// row is a row of the view's list: the lines of the file open. In a pair,
// line and other are the left and right lines, -1 for none.
type row struct {
	kind    int
	file    int // index in the diff's files
	hunk    int
	line    int
	other   int
	comment int // index in the comments
}

// setDiff shows d.
func (v *View) setDiff(d proto.Diff) {
	v.diff = d
	v.layoutRows()
}

// showFile makes the rows those of the file at path.
func (v *View) showFile(path string, split bool) {
	if path == v.file && split == v.split {
		return
	}
	if path != v.file {
		v.fileList.ScrollTo(0, ui.Start)
	}
	v.file, v.split = path, split
	v.layoutRows()
}

// layoutRows lays the rows out again. It makes a new slice: the list may
// still be drawing the old one this frame.
func (v *View) layoutRows() {
	v.rows = nil
	fi := v.fileIndex(v.file)
	if fi < 0 {
		return
	}
	f := v.diff.Files[fi]
	if f.Binary || len(f.Hunks) == 0 {
		v.rows = append(v.rows, row{kind: rowBinary, file: fi})
		return
	}
	for hi, h := range f.Hunks {
		v.rows = append(v.rows, row{kind: rowHunk, file: fi, hunk: hi})
		if v.split {
			v.rows = append(v.rows, pairRows(fi, hi, h.Lines)...)
			continue
		}
		for li, l := range h.Lines {
			v.rows = append(v.rows, row{kind: rowLine, file: fi, hunk: hi, line: li})
			for ci, c := range v.comments {
				if c.at(f.Path, l) {
					v.rows = append(v.rows, row{kind: rowComment, file: fi, comment: ci})
				}
			}
			if d := v.draft; d != nil && d.at(f.Path, l) {
				v.rows = append(v.rows, row{kind: rowDraft, file: fi, hunk: hi, line: li})
			}
		}
	}
	if f.Truncated {
		v.rows = append(v.rows, row{kind: rowCut, file: fi})
	}
}

// pairRows lines a hunk up side by side: lines removed beside the lines added
// after them, unchanged ones on both sides.
func pairRows(fi, hi int, lines []proto.DiffLine) []row {
	var out []row
	for i := 0; i < len(lines); {
		if lines[i].Kind != '-' && lines[i].Kind != '+' {
			out = append(out, row{kind: rowPair, file: fi, hunk: hi, line: i, other: i})
			i++
			continue
		}
		var del, add []int
		for ; i < len(lines) && lines[i].Kind == '-'; i++ {
			del = append(del, i)
		}
		for ; i < len(lines) && lines[i].Kind == '+'; i++ {
			add = append(add, i)
		}
		for k := 0; k < max(len(del), len(add)); k++ {
			r := row{kind: rowPair, file: fi, hunk: hi, line: -1, other: -1}
			if k < len(del) {
				r.line = del[k]
			}
			if k < len(add) {
				r.other = add[k]
			}
			out = append(out, r)
		}
	}
	return out
}

// PathOf is the changed file path names, as an agent names it: itself, or the
// one it ends in; path itself if it is none.
func (v *View) PathOf(path string) string {
	for _, f := range v.diff.Files {
		if f.Path == path || strings.HasSuffix(path, "/"+f.Path) {
			return f.Path
		}
	}
	return path
}

// fileIndex is the index of the file at path in the diff, or -1.
func (v *View) fileIndex(path string) int {
	if path == "" {
		return -1
	}
	for i, f := range v.diff.Files {
		if f.Path == path {
			return i
		}
	}
	return -1
}

// fileRows are the changed files, for the panel.
func (v *View) fileRows(c *ui.Context, o Options) {
	current := ""
	if o.Current != nil {
		current = o.Current()
	}
	widget.Scroll(c).Grow(1).FillWidth().MinHeight(0).Children(func() {
		ui.Column(c).FillWidth().Gap(2).Children(func() {
			for _, f := range v.diff.Files {
				v.fileItem(c.Key(f.Path), o, f, f.Path == current)
			}
		})
	})
}

// fileItem is a changed file in the panel: a dot by what happened to it, its
// path, which runs when too long, and the lines it added and removed.
func (v *View) fileItem(c *ui.Context, o Options, f proto.DiffFile, on bool) {
	b := ui.ButtonBase(c).FillWidth().MinHeight(36).Padding(theme.S1, theme.S3).Gap(theme.S2).Radius(theme.RadiusCard).Justify(ui.Start).
		Cursor(ui.CursorPointer).Label(f.Path)
	bg := theme.Fade(theme.Selected, 0)
	switch {
	case on:
		bg = theme.Selected
	case b.Hovered():
		bg = theme.Hover
	}
	b.Background(widget.QuickTint(b, "bg", bg)).Children(func() {
		col := theme.Working
		if f.Status == proto.FileAdded {
			col = theme.Add
		}
		widget.Dot(c, col, false, 8)
		ui.Row(c).Grow(1).MinWidth(0).Children(func() {
			widget.Running(c, f.Path, widget.RunStyle{Mono: true, Always: on})
		})
		if n := v.commentsOn(f.Path); n > 0 {
			ui.Icon(c, icon.Chat).FontSize(12).TextColor(theme.Now)
			ui.Text(c, strconv.Itoa(n)).FontSize(theme.TextLabel).TextColor(theme.Now).SingleLine().Shrink(0)
		}
		counts(c, f)
	})
	if b.Clicked() && o.OpenDiff != nil {
		o.OpenDiff(f.Path)
	}
}

// counts are a file's added and removed lines, as +n −n.
func counts(c *ui.Context, f proto.DiffFile) {
	if f.Added > 0 {
		ui.Text(c, "+"+strconv.Itoa(f.Added)).FontSize(theme.TextUI).TextColor(theme.Add).SingleLine().Shrink(0)
	}
	if f.Removed > 0 {
		ui.Text(c, "−"+strconv.Itoa(f.Removed)).FontSize(theme.TextUI).TextColor(theme.Del).SingleLine().Shrink(0)
	}
}

// statusLetter is how git's short status writes a file's change.
func statusLetter(status string) (string, ui.Color) {
	switch status {
	case proto.FileAdded:
		return "A", theme.Add
	case proto.FileDeleted:
		return "D", theme.Del
	case proto.FileRenamed:
		return "R", theme.Now
	}
	return "M", theme.Warning
}

// The code's colors, on its own dark ground in every palette.
var (
	diffGround  = ui.RGBA(10, 11, 18, 0.94)
	diffText    = ui.Hex("#E4E6EE")
	diffNumber  = ui.Hex("#6E7386")
	diffAdd     = ui.RGBA(46, 160, 67, 0.2)
	diffAddText = ui.Hex("#7EE29A")
	diffDel     = ui.RGBA(248, 81, 73, 0.2)
	diffDelText = ui.Hex("#FF9D97")
	diffHunk    = ui.Hex("#6CC4FF")
)

// lineColors are the ground and color of a line of kind.
func lineColors(kind byte) (bg, fg ui.Color) {
	switch kind {
	case '+':
		return diffAdd, diffAddText
	case '-':
		return diffDel, diffDelText
	}
	return ui.Transparent, diffText
}

func lineNumber(n int) string {
	if n == 0 {
		return ""
	}
	return strconv.Itoa(n)
}

// diffLine is a line with its old and new numbers, tinted by what it is.
// Clicking it opens a review comment on it.
func (v *View) diffLine(c *ui.Context, file string, l proto.DiffLine) {
	bg, fg := lineColors(byte(l.Kind))
	mark := " "
	switch l.Kind {
	case '+':
		mark = "+"
	case '-':
		mark = "−"
	}
	b := ui.ButtonBase(c).FillWidth().Height(rowHeight).Background(bg).Clip().Justify(ui.Start).Gap(0).
		Cursor(ui.CursorPointer).Label("Comment on line " + lineNumber(max(l.New, l.Old)))
	if b.Hovered() {
		b.Background(theme.Fade(theme.Now, 0.14))
	}
	if b.Clicked() {
		v.startDraft(file, l)
	}
	b.Children(func() {
		for _, n := range []int{l.Old, l.New} {
			ui.Text(c, lineNumber(n)).Font(theme.MonoFont).FontSize(theme.TextUI).TextColor(diffNumber).Width(44).Padding(0, theme.S2, 0, 0).
				TextAlign(ui.End).SingleLine().Shrink(0)
		}
		ui.Text(c, mark).Font(theme.MonoFont).FontSize(theme.TextUI).TextColor(fg).Width(9).SingleLine().Shrink(0)
		ui.Text(c, l.Text).Font(theme.MonoFont).FontSize(theme.TextUI).TextColor(fg).SingleLine().Shrink(1).MinWidth(0)
	})
}

// pairLine is one side of a row side by side: a half of the row, tinted.
func pairLine(c *ui.Context, lines []proto.DiffLine, i int, left bool) {
	cell := ui.Row(c).Grow(1).Basis(0).MinWidth(0).Height(rowHeight).Clip()
	if i < 0 {
		cell.Background(ui.RGBA(255, 255, 255, 0.03))
		return
	}
	l := lines[i]
	bg, fg := lineColors(byte(l.Kind))
	n := l.New
	if left {
		n = l.Old
	}
	cell.Background(bg).Children(func() {
		ui.Text(c, lineNumber(n)).Font(theme.MonoFont).FontSize(theme.TextUI).TextColor(diffNumber).Width(44).Padding(0, theme.S2, 0, 0).
			TextAlign(ui.End).SingleLine().Shrink(0)
		ui.Text(c, l.Text).Font(theme.MonoFont).FontSize(theme.TextUI).TextColor(fg).SingleLine().Shrink(1).MinWidth(0)
	})
}

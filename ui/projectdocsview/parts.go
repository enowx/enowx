package projectdocsview

import (
	"strconv"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/projectdocs"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// The parts both panels are made of. Their frames have fixed sizes: what they
// hold fits them, by running.

// Fixed heights of the rows.
const (
	lineHeight = 24
	rowHeight  = 30
	noteHeight = 18
	crumbGap   = 15 // a chevron and the gaps around it
)

// narrowWidth is under what width a panel lays out for little room.
const narrowWidth = 420

// statusStyle is how a status is marked.
func statusStyle(status string) widget.Status {
	s := widget.Status{Label: status}
	switch status {
	case projectdocs.StatusApproved:
		s.Color = theme.Done
	case projectdocs.StatusReview:
		s.Color = theme.Accent
	case projectdocs.StatusOutdated:
		s.Color = theme.Warning
	default:
		s.Color, s.Hollow = theme.Muted, true
	}
	return s
}

// line is a one-line row: an icon, then text that runs when too long.
func line(c *ui.Context, svg *ui.SVG, text string, col ui.Color) {
	ui.Row(c).FillWidth().Height(lineHeight).Gap(7).AlignItems(ui.Center).MinWidth(0).Children(func() {
		if svg != nil {
			ui.Icon(c, svg).FontSize(12).TextColor(col).Shrink(0)
		}
		running(c, text, runStyle{Size: 12, Color: col}).Grow(1)
	})
}

// progress is a bar of done of total; a Row of two parts rather than a
// measured width, so it fits its frame from the first frame.
func progress(c *ui.Context, done, total int) {
	ui.Row(c).FillWidth().Height(6).Radius(3).Clip().Background(theme.Selected).Children(func() {
		if total > 0 && done > 0 {
			ui.Box(c).Grow(float32(done)).FillHeight().Background(theme.Done)
		}
		if rest := total - done; rest > 0 || total == 0 {
			ui.Box(c).Grow(float32(max(rest, 1))).FillHeight()
		}
	})
}

// cell is a number over what it counts, in a frame of its own.
func cell(c *ui.Context, n int, label string, col ui.Color) {
	ui.Column(c).Height(46).MinWidth(0).PaddingX(10).Justify(ui.Center).Gap(1).Radius(theme.RadiusControl).Background(theme.Sunken).Clip().Children(func() {
		ui.Row(c).FillWidth().Height(20).AlignItems(ui.Center).MinWidth(0).Children(func() {
			ui.Text(c, strconv.Itoa(n)).FontSize(15).FontWeight(600).TextColor(col).SingleLine().Shrink(0)
		})
		ui.Row(c).FillWidth().Height(15).AlignItems(ui.Center).MinWidth(0).Children(func() {
			running(c, label, runStyle{Size: 11, Color: theme.Muted}).Grow(1)
		})
	})
}

// sectionTitle is a label over a part of a panel.
func sectionTitle(c *ui.Context, text string) {
	ui.Row(c).FillWidth().Height(20).AlignItems(ui.Center).MinWidth(0).Children(func() {
		running(c, text, runStyle{Size: 11.5, Weight: 600, Color: theme.TextSoft}).Grow(1)
	})
}

// noteLine says how the last action went, in a row kept for it so nothing
// moves when it does.
func (v *View) noteLine(c *ui.Context) {
	ui.Row(c).FillWidth().Height(noteHeight).AlignItems(ui.Center).MinWidth(0).Children(func() {
		if v.note == "" {
			return
		}
		col := theme.Muted
		if v.noteErr {
			col = theme.Error
		}
		running(c, v.note, runStyle{Size: 11.5, Color: col}).Grow(1)
	})
}

// breadcrumb is project > area > document, each part opening its note. The
// parts share the row and run, so a long title takes no room from the others.
func (v *View) breadcrumb(c *ui.Context, o Options, id string) {
	project := ""
	if m := v.current(id); m != nil {
		project = m.project.Note.ID
	}
	if v.crumbsFor != id+"\x00"+project || v.crumbsSum != v.sumList {
		v.crumbs, v.crumbsFor, v.crumbsSum = crumbsOf(o.Notes, id, project), id+"\x00"+project, v.sumList
	}
	if len(v.crumbs) < 2 {
		return
	}
	row := ui.Row(c).FillWidth().Height(rowHeight - 6).Gap(3).AlignItems(ui.Center).MinWidth(0).Clip().Label("Where this is")
	// An equal share at most: shrinking alone would take a short part down
	// to nothing beside two long ones.
	share := (row.Bounds().W - float32(len(v.crumbs)-1)*crumbGap) / float32(len(v.crumbs))
	row.Children(func() {
		for i, n := range v.crumbs {
			if i > 0 {
				ui.Icon(c, icon.ChevronRight).FontSize(9).TextColor(theme.Muted).Shrink(0)
			}
			crumb(c.Key(n.ID), o, n, share)
		}
	})
}

func crumb(c *ui.Context, o Options, n proto.NoteInfo, share float32) {
	b := ui.ButtonBase(c).Height(22).PaddingX(4).Gap(4).Radius(5).Cursor(ui.CursorPointer).Label("Open " + titleOf(n)).MinWidth(0).Shrink(1).AlignItems(ui.Center)
	if share > 0 {
		b.MaxWidth(share)
	}
	if b.Hovered() {
		b.Background(theme.Hover)
	}
	b.Children(func() {
		widget.NoteIcon(c, n.Icon, 12, theme.Muted, nil).Shrink(0)
		running(c, titleOf(n), runStyle{Size: 11.5, Color: theme.TextSoft})
	})
	if b.Clicked() {
		o.Open(n.ID)
	}
}

// entryIcon is a catalog entry's icon for a menu.
func entryIcon(ref string) *ui.SVG {
	if svg, _, _ := icon.Note(ref); svg != nil {
		return svg
	}
	return icon.File
}

// confirmation is a question over the page: what an action will lose, asked
// before it.
type confirmation struct {
	open               bool
	title, msg, action string
	danger             bool
	do                 func()
}

// ask opens a confirmation.
func (v *View) ask(title, msg, action string, danger bool, do func()) {
	v.confirm = &confirmation{open: true, title: title, msg: msg, action: action, danger: danger, do: do}
}

func (v *View) confirmDialog(c *ui.Context) {
	q := v.confirm
	if q == nil {
		return
	}
	if !q.open {
		v.confirm = nil
		return
	}
	widget.FormSheet(c.Key("confirm"), &q.open, 440, q.title, "", func() {
		ui.Text(c, q.msg).FontSize(12.5).TextColor(theme.TextSoft).FillWidth()
	}, func() {
		kind := widget.Primary
		if q.danger {
			kind = widget.Danger
		}
		if widget.Button(c, kind, nil, q.action, "").Clicked() {
			v.confirm = nil
			q.do()
		}
	})
}

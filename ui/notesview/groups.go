package notesview

import (
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// Groups in the tree: each a header, folding, with its pages under it;
// pages dragged onto a header go in that group, groups dragged onto one go
// before it. "No group" follows them, with the pages in none.

// groupColors are the colors a group is given from.
var groupColors = []string{"#E5484D", "#F76B15", "#FFC53D", "#46A758", "#12A594", "#0090FF", "#8E4EC6", "#D6409F"}

// groupForm makes a group, or changes group from.
type groupForm struct {
	open  bool
	from  string
	name  string
	icon  string
	color string
	err   string
	// picking shows the icon picker.
	picking bool
}

// inGroup counts group's pages the tree shows, those under them too.
func (v *View) inGroup(o Options, group string, shown map[string]bool) int {
	byID := map[string]proto.NoteInfo{}
	for _, n := range o.Notes {
		byID[n.ID] = n
	}
	count := 0
	for _, n := range o.Notes {
		top := n
		for top.Parent != "" {
			p, ok := byID[top.Parent]
			if !ok {
				break
			}
			top = p
		}
		if top.Group == group && (shown == nil || shown[n.ID]) {
			count++
		}
	}
	return count
}

// groupHeader is group g's line: a click folds it, a drag moves it.
func (v *View) groupHeader(c *ui.Context, o Options, g proto.NoteGroup, count int) {
	row := groupRow(c, "Group "+g.Name)
	row.Drag(noteDrag{group: g.Name})
	if d, ok := ui.Drop[noteDrag](row); ok {
		switch {
		case d.id != "":
			o.SetGroup(d.id, g.Name, v.failed)
		case d.group != g.Name:
			o.MoveGroup(d.group, g.Name, v.failed)
		}
	}
	if d, ok := ui.DragOver[noteDrag](row); ok && d.group != g.Name {
		if d.id != "" {
			row.Background(theme.AccentSoft)
		} else {
			row.BorderWidth(2, 0, 0, 0).BorderColor(theme.Accent)
		}
	} else if row.Hovered() {
		row.Background(theme.Hover)
	}
	row.Children(func() {
		if g.Icon != "" || g.Color != "" {
			groupMark(c, g)
		}
		ui.Row(c).Grow(1).MinWidth(0).Children(func() {
			widget.Running(c, g.Name, widget.RunStyle{Size: theme.TextLabel, Weight: 500, Color: theme.TextSoft})
		})
		if row.Hovered() {
			if widget.IconButton(c, icon.Plus, "New page in "+g.Name, false).Clicked() {
				v.newPage(o, "", g.Name)
			}
			more := widget.IconButton(c, icon.More, "More for "+g.Name, false)
			v.menu.OpenBelow(more, "group:"+g.Name, theme.ControlHeight)
		} else {
			ui.Text(c, strconv.Itoa(count)).FontSize(theme.TextLabel).TextColor(theme.TextSoft).Shrink(0)
		}
		foldMark(c, g.Folded)
	})
	key := "group:" + g.Name
	v.menu.Open(row, key)
	v.menu.Show(c, row, key, func(done func()) {
		if widget.MenuItem(c, icon.Plus, "New page", false) {
			v.newPage(o, "", g.Name)
			done()
		}
		if widget.MenuItem(c, icon.Pencil, "Edit group…", false) {
			v.groupForm = &groupForm{open: true, from: g.Name, name: g.Name, icon: g.Icon, color: g.Color}
			done()
		}
		if o.Export != nil && widget.MenuItem(c, icon.ArrowUp, "Export group…", false) {
			o.Export(ExportPick{Group: g.Name, Title: g.Name})
			done()
		}
		widget.MenuSeparator(c)
		if widget.MenuItem(c, icon.Trash, "Delete group…", true) {
			gg := g
			v.deletingGroup = &gg
			done()
		}
	})
	if row.Clicked() {
		g.Folded = !g.Folded
		o.SaveGroup(g.Name, g, v.failed)
	}
}

// groupRow is a line heading pages in the tree: what the design's lists head
// their groups with, that folds.
func groupRow(c *ui.Context, label string) ui.Element {
	return ui.ButtonBase(c).FillWidth().Height(theme.ControlHeight).Shrink(0).Padding(0, theme.S2, 0, theme.S3).Gap(theme.S2).Radius(theme.RadiusCard).
		Justify(ui.Start).Margin(theme.S1, 0, 0, 0).Cursor(ui.CursorPointer).Label(label).AlignItems(ui.Center)
}

// foldMark says whether what a line heads is folded.
func foldMark(c *ui.Context, folded bool) {
	svg := icon.ChevronDown
	if folded {
		svg = icon.ChevronRight
	}
	ui.Icon(c, svg).FontSize(12).TextColor(theme.TextSoft).Shrink(0)
}

// groupMark is a group's icon, or a folder, in its color.
func groupMark(c *ui.Context, g proto.NoteGroup) {
	ink := theme.Muted
	if g.Color != "" {
		ink = ui.Hex(g.Color)
	}
	widget.NoteIcon(c, g.Icon, 14, ink, icon.Folder)
}

// ungroupedHeader heads the pages in no group, which take pages dropped.
func (v *View) ungroupedHeader(c *ui.Context, o Options, count int) {
	row := groupRow(c, "No group")
	if d, ok := ui.Drop[noteDrag](row); ok && d.id != "" {
		o.SetGroup(d.id, "", v.failed)
	}
	if d, ok := ui.DragOver[noteDrag](row); ok && d.id != "" {
		row.Background(theme.AccentSoft)
	} else if row.Hovered() {
		row.Background(theme.Hover)
	}
	row.Children(func() {
		ui.Row(c).Grow(1).MinWidth(0).Children(func() {
			widget.Running(c, "No group", widget.RunStyle{Size: theme.TextLabel, Weight: 500, Color: theme.TextSoft})
		})
		ui.Text(c, strconv.Itoa(count)).FontSize(theme.TextLabel).TextColor(theme.TextSoft).Shrink(0)
		foldMark(c, v.ungroupedFolded)
	})
	if row.Clicked() {
		v.ungroupedFolded = !v.ungroupedFolded
	}
}

// groupFilterButton picks the group whose pages the tree shows alone.
func (v *View) groupFilterButton(c *ui.Context, o Options) {
	if len(o.Groups) == 0 {
		v.groupFilter = ""
		return
	}
	label := "All groups"
	if v.groupFilter != "" {
		label = v.groupFilter
	}
	b := widget.IconButton(c, icon.Folder, "Show: "+label, v.groupFilter != "")
	v.menu.OpenBelow(b, "group-filter", theme.ControlHeight)
	v.menu.Show(c, b, "group-filter", func(done func()) {
		if widget.MenuItem(c, icon.List, "All groups", false) {
			v.groupFilter = ""
			done()
		}
		for _, g := range o.Groups {
			if widget.MenuItem(c.Key(g.Name), icon.Folder, g.Name, false) {
				v.groupFilter = g.Name
				done()
			}
		}
	})
}

// groupDialog makes or edits a group: its name, emoji and color.
func (v *View) groupDialog(c *ui.Context, o Options) {
	f := v.groupForm
	if f == nil {
		return
	}
	if !f.open {
		v.groupForm = nil
		return
	}
	title := "New group"
	if f.from != "" {
		title = "Edit group"
	}
	save := false
	widget.FormSheet(c, &f.open, 440, title, "", func() {
		widget.Field(c, "Name", func() {
			save = widget.Input(c, &f.name, "Plans").FillWidth().AutoFocus().Submitted()
		})
		widget.Field(c, "Icon", func() {
			b := widget.Well(ui.ButtonBase(c).AlignSelf(ui.Start).Height(theme.ControlHeight).PaddingX(theme.S3).Gap(theme.S2).Radius(theme.RadiusFull).
				Cursor(ui.CursorPointer).Label("Group icon"), f.picking)
			b.Children(func() {
				ink := theme.Muted
				if f.color != "" {
					ink = ui.Hex(f.color)
				}
				widget.NoteIcon(c, f.icon, 16, ink, icon.Folder)
				label := "A folder"
				if f.icon != "" {
					label = "Change…"
				}
				ui.Text(c, label).FontSize(theme.TextUI).TextColor(theme.Text)
			})
			if b.Clicked() {
				if f.picking = !f.picking; f.picking {
					v.openPicker(o)
				}
			}
			// In the sheet itself: a pop-over would go under it.
			if f.picking {
				widget.Raise(ui.Column(c).FillWidth().Margin(theme.S2, 0, 0, 0).Radius(theme.RadiusCard).Background(theme.Raised)).Children(func() {
					v.iconPickerBody(c, o, f.icon, func(ref string) { f.icon, f.picking = ref, false })
				})
			}
		})
		widget.Field(c, "Color", func() {
			ui.Row(c).Gap(theme.S2).Wrap().Children(func() {
				for _, col := range append([]string{""}, groupColors...) {
					b := ui.ButtonBase(c.Key("c"+col)).Size(24, 24).Radius(theme.RadiusFull).Cursor(ui.CursorPointer).Label("Color " + col)
					if col == "" {
						b.Border(1, theme.BorderStrong)
					} else {
						b.Background(ui.Hex(col))
					}
					if f.color == col {
						b.Border(2, theme.TextStrong)
					}
					if b.Clicked() {
						f.color = col
					}
				}
			})
		})
		if f.err != "" {
			ui.Text(c, f.err).FontSize(theme.TextLabel).TextColor(theme.Error)
		}
	}, func() {
		label := "Create"
		if f.from != "" {
			label = "Save"
		}
		if widget.PrimaryButton(c, nil, label, "").Clicked() {
			save = true
		}
	})
	if !save {
		return
	}
	name := strings.TrimSpace(f.name)
	if name == "" {
		f.err = "Give it a name."
		return
	}
	g := proto.NoteGroup{Name: name, Icon: f.icon, Color: f.color}
	for _, old := range o.Groups {
		if old.Name == f.from {
			g.Folded = old.Folded
		}
	}
	from := f.from
	o.SaveGroup(from, g, func(err error) {
		if err != nil {
			f.err = err.Error()
			return
		}
		if v.groupFilter == from && from != "" {
			v.groupFilter = name
		}
		v.groupForm = nil
	})
}

// deleteGroupDialog confirms deleting a group; its pages stay.
func (v *View) deleteGroupDialog(c *ui.Context, o Options) {
	g := v.deletingGroup
	if g == nil {
		return
	}
	n := v.inGroup(o, g.Name, nil)
	msg := "It has no pages."
	if n > 0 {
		msg = "Its " + strconv.Itoa(n) + " pages stay, in no group."
	}
	open := true
	switch widget.AlertDialog(c, &open, "Delete the group “"+g.Name+"”?", msg, "Cancel", "Delete group") {
	case 1:
		if v.groupFilter == g.Name {
			v.groupFilter = ""
		}
		o.DeleteGroup(g.Name, v.failed)
		v.deletingGroup = nil
	case 0, ui.AlertDismissed:
		v.deletingGroup = nil
	}
}

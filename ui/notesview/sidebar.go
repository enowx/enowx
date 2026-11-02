package notesview

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// noteDrag is what dragging a page in the tree carries.
// noteDrag is what dragging a page (id) or a group (group) in the tree
// carries: one type, as an element takes values of one.
type noteDrag struct{ id, group string }

// sidebar is the pages' tree: each under its parent, folding; a filter
// shows the pages whose titles match, with those they are under.
func (v *View) sidebar(c *ui.Context, o Options) {
	children := map[string][]proto.NoteInfo{}
	for _, n := range o.Notes {
		children[n.Parent] = append(children[n.Parent], n)
	}
	shown := v.matching(o)
	var tree ui.Element
	if v.treeInSidebar {
		tree = ui.Column(c.Key("notes-tree")).FillWidth().Grow(1).MinHeight(0).Padding(theme.S2).Gap(theme.S1)
	} else {
		tree = ui.Column(c.Key("notes-tree")).Width(248).Shrink(0).FillHeight().Padding(theme.S2).Gap(theme.S1).BorderWidth(0, 0.5, 0, 0).BorderColor(theme.Border)
	}
	if o.Drop != nil {
		// Files from other apps dropped on the tree are imported.
		if files := tree.DroppedFiles(); len(files) > 0 {
			o.Drop(files)
		}
		if tree.FileDragOver() {
			tree.Background(theme.AccentSoft)
		}
	}
	tree.Children(func() {
		v.treeHead(c, o)
		if len(o.Spaces) > 1 && o.PickSpace != nil {
			name := o.Space
			if o.Team == "" {
				name = "Personal"
			}
			ui.Row(c).FillWidth().PaddingX(theme.S2).MinWidth(0).Children(func() {
				widget.SpacePicker(c, widget.Space{Team: o.Team, Name: name}, "on "+o.Host, theme.TextLabel, o.Spaces, o.PickSpace)
			})
		} else if o.Team != "" {
			ui.Row(c).FillWidth().PaddingX(theme.S3).Gap(theme.S2).MinWidth(0).Children(func() {
				widget.TeamMark(c, o.Team, o.Space, 12)
				ui.Row(c).Grow(1).MinWidth(0).Children(func() {
					widget.Running(c, o.Space+" · on "+o.Host, widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
				})
			})
		}
		ui.Row(c).FillWidth().Gap(theme.S1).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Children(func() {
				widget.Input(c, &v.filter, "Filter notes").FillWidth()
			})
			v.groupFilterButton(c, o)
			v.projectFilterButton(c, o)
		})
		widget.Scroll(c).Grow(1).FillWidth().MinHeight(0).Children(func() {
			ui.Column(c).FillWidth().Gap(2).Children(func() {
				if o.Loaded && len(o.Notes) == 0 {
					ui.Text(c, "No notes yet").FontSize(theme.TextUI).TextColor(theme.TextSoft).Padding(theme.S2, theme.S3)
				}
				if v.projectFilter == byProject {
					v.byProjectTree(c, o, shown)
					return
				}
				var walk func(parent string, depth int, group string)
				walk = func(parent string, depth int, group string) {
					for _, n := range children[parent] {
						if shown != nil && !shown[n.ID] || depth == 0 && n.Group != group {
							continue
						}
						v.treeRow(c.Key(n.ID), o, n, depth, len(children[n.ID]) > 0, children[parent])
						if len(children[n.ID]) > 0 && (shown != nil || !v.folded[n.ID]) {
							walk(n.ID, depth+1, group)
						}
					}
				}
				if len(o.Groups) == 0 {
					walk("", 0, "")
					return
				}
				for _, g := range o.Groups {
					if v.groupFilter != "" && g.Name != v.groupFilter {
						continue
					}
					n := v.inGroup(o, g.Name, shown)
					if shown != nil && n == 0 {
						continue
					}
					v.groupHeader(c.Key("group:"+g.Name), o, g, n)
					if !g.Folded || shown != nil {
						ui.Column(c).FillWidth().Gap(2).Children(func() { walk("", 0, g.Name) })
					}
				}
				if v.groupFilter == "" {
					v.ungroupedHeader(c, o, v.inGroup(o, "", shown))
					if !v.ungroupedFolded || shown != nil {
						walk("", 0, "")
					}
				}
			})
		})
	})
}

// matching is the pages a filter shows, nil without one.
func (v *View) matching(o Options) map[string]bool {
	q := strings.ToLower(strings.TrimSpace(v.filter))
	filtered := v.projectFilter != "" && v.projectFilter != byProject
	if q == "" && !filtered {
		return nil
	}
	byID := map[string]proto.NoteInfo{}
	for _, n := range o.Notes {
		byID[n.ID] = n
	}
	shown := map[string]bool{}
	for _, n := range o.Notes {
		if strings.Contains(strings.ToLower(n.Title), q) && v.inProjectFilter(n) {
			for x, ok := n, true; ok && !shown[x.ID]; x, ok = byID[x.Parent] {
				shown[x.ID] = true
			}
		}
	}
	return shown
}

// treeHead is the tree's title and what makes or brings notes.
func (v *View) treeHead(c *ui.Context, o Options) {
	ui.Row(c).FillWidth().MinHeight(theme.ControlHeight).Shrink(0).Padding(0, 0, 0, theme.S3).Gap(0).Children(func() {
		ui.Row(c).Grow(1).MinWidth(0).Children(func() {
			widget.Running(c, "Notes", widget.RunStyle{Weight: 600})
		})
		if o.SyncState != nil {
			state, alarm := o.SyncState()
			b := widget.IconButton(c, icon.Sync, state, false)
			if alarm {
				b.TextColor(theme.Warning)
			}
			if b.Clicked() && o.OpenSync != nil {
				o.OpenSync()
			}
		}
		add := widget.IconButton(c, icon.Plus, "New note", false)
		if !o.Loaded {
			add.Disabled(true).Opacity(0.45)
		} else if add.Clicked() {
			v.newPage(o, "", v.groupFilter)
		}
		if (o.Import != nil || o.Export != nil || o.SaveGroup != nil) && o.Loaded {
			more := widget.IconButton(c, icon.More, "Import and export", false)
			v.menu.OpenBelow(more, "notes-io", theme.ControlHeight)
			v.menu.Show(c, more, "notes-io", func(done func()) {
				if o.Import != nil && widget.MenuItem(c, icon.ArrowDown, "Import…", false) {
					o.Import()
					done()
				}
				if o.Export != nil && len(o.Notes) > 0 && widget.MenuItem(c, icon.ArrowUp, "Export…", false) {
					o.Export(ExportPick{All: true, Title: "All notes"})
					done()
				}
				if o.SaveGroup != nil {
					widget.MenuSeparator(c)
					if widget.MenuItem(c, icon.FolderPlus, "New group…", false) {
						v.groupForm = &groupForm{open: true}
						done()
					}
				}
			})
		}
	})
}

// treeRow is a page in the tree: dragging it onto another's upper or
// lower edge puts it before or after that one, onto its middle under it.
func (v *View) treeRow(c *ui.Context, o Options, n proto.NoteInfo, depth int, hasChildren bool, siblings []proto.NoteInfo) {
	on := n.ID == v.selected
	row := ui.ButtonBase(c).FillWidth().Height(36).Shrink(0).Padding(0, theme.S1, 0, theme.S3+float32(depth)*theme.S3).Gap(theme.S2).Radius(theme.RadiusCard).
		Justify(ui.Start).Cursor(ui.CursorPointer).Label(titleOf(n)).AlignItems(ui.Center)
	row.Drag(noteDrag{id: n.ID})
	bg := theme.Fade(theme.Selected, 0)
	switch {
	case on:
		bg = theme.Selected
	case row.Hovered():
		bg = theme.Hover
	}
	row.Background(widget.QuickTint(row, "bg", bg))
	v.dropOn(row, o, n, siblings)
	row.Children(func() {
		if n.Icon != "" {
			widget.NoteIcon(c, n.Icon, 14, theme.TextSoft, nil)
		}
		style := widget.RunStyle{Always: on}
		switch {
		case on:
			style.Weight, style.Color = 500, theme.TextStrong
		case strings.TrimSpace(n.Title) == "":
			style.Color = theme.Muted
		}
		ui.Row(c).Grow(1).MinWidth(0).Children(func() { widget.Running(c, titleOf(n), style) })
		if n.IconReview && !row.Hovered() {
			ui.Box(c).Size(6, 6).Radius(theme.RadiusFull).Background(theme.Warning).Shrink(0).Tooltip("Its icon stands in for an emoji: pick one")
		}
		if row.Hovered() {
			if widget.IconButton(c, icon.Plus, "Add a page inside", false).Clicked() {
				v.newPage(o, n.ID, "")
			}
		}
		if hasChildren {
			chev := ui.ButtonBase(c).Size(theme.ControlHeight-theme.S2, theme.ControlHeight-theme.S2).Radius(theme.RadiusFull).Center().Shrink(0).
				Cursor(ui.CursorPointer).Label("Fold " + titleOf(n))
			chev.Children(func() {
				svg := icon.ChevronDown
				if v.folded[n.ID] {
					svg = icon.ChevronRight
				}
				ui.Icon(c, svg).FontSize(12).TextColor(theme.TextSoft)
			})
			if chev.Clicked() {
				if v.folded == nil {
					v.folded = map[string]bool{}
				}
				v.folded[n.ID] = !v.folded[n.ID]
			}
		}
	})
	key := "note:" + n.ID
	v.menu.Open(row, key)
	v.menu.Show(c, row, key, func(done func()) { v.pageItems(c, o, n, done) })
	if row.Clicked() {
		v.Select(o, n.ID)
	}
}

// dropOn makes row take a page dropped on it, as treeRow says.
func (v *View) dropOn(row ui.Element, o Options, n proto.NoteInfo, siblings []proto.NoteInfo) {
	_, y, _ := row.PointerPosition()
	zone := 1 // 0 before, 1 inside, 2 after
	switch {
	case y < 10:
		zone = 0
	case y > 26:
		zone = 2
	}
	if d, ok := ui.Drop[noteDrag](row); ok && d.id != "" && d.id != n.ID {
		switch zone {
		case 0:
			o.Move(d.id, n.Parent, n.ID, v.failed)
		case 1:
			delete(v.folded, n.ID)
			o.Move(d.id, n.ID, "", v.failed)
		case 2:
			o.Move(d.id, n.Parent, after(siblings, n.ID, d.id), v.failed)
		}
	}
	if d, ok := ui.DragOver[noteDrag](row); ok && d.id != "" && d.id != n.ID {
		switch zone {
		case 0:
			row.BorderWidth(2, 0, 0, 0).BorderColor(theme.Accent)
		case 1:
			row.Background(theme.AccentSoft)
		case 2:
			row.BorderWidth(0, 0, 2, 0).BorderColor(theme.Accent)
		}
	}
}

// after is the sibling after id, which a page put after id goes before;
// "" when id is last. moving is left out, as it leaves its place.
func after(siblings []proto.NoteInfo, id, moving string) string {
	for i, s := range siblings {
		if s.ID != id {
			continue
		}
		for _, next := range siblings[i+1:] {
			if next.ID != moving {
				return next.ID
			}
		}
	}
	return ""
}

// DrawList draws the pages' tree where the app puts it (its sidebar),
// filling the room it is given.
func (v *View) DrawList(c *ui.Context, o Options) {
	v.treeInSidebar = true
	defer func() { v.treeInSidebar = false }()
	v.sidebar(c, o)
}

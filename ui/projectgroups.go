package ui

import (
	"fmt"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// The session list's projects in the user's order and groups, kept by the
// host: dragging a project or a group moves it, and their menus move them
// a step, put a project in a group or rename one.

// listEntry is a line of the list's projects: one without a group, or a
// group with its projects, where its first one is.
type listEntry struct {
	group    string
	projects []proto.ProjectInfo
}

// listDrag is what dragging a project's header (path) or a group's
// (group) carries: one type, as an element takes values of one.
type listDrag struct{ path, group string }

// groupForm puts a project in a group, or renames a group.
type groupForm struct {
	open    bool
	project string // the project going in a group, or "" renaming
	from    string // the group renamed
	name    string
}

// projectEntries are the projects as the list shows them: in order, a
// group's together where the first of them is.
func (a *App) projectEntries() []listEntry {
	var out []listEntry
	at := map[string]int{}
	for _, p := range a.projects {
		if p.Group == "" {
			out = append(out, listEntry{projects: []proto.ProjectInfo{p}})
			continue
		}
		if i, ok := at[p.Group]; ok {
			out[i].projects = append(out[i].projects, p)
			continue
		}
		at[p.Group] = len(out)
		out = append(out, listEntry{group: p.Group, projects: []proto.ProjectInfo{p}})
	}
	return out
}

// groupSection draws a group and its projects, and says how many projects
// it showed: while filtering, a group without a match is hidden.
func (a *App) groupSection(c *ui.Context, e listEntry) int {
	filtering := strings.TrimSpace(a.filter) != ""
	if filtering {
		n := 0
		for _, p := range e.projects {
			if len(a.matching(p, a.projectSessions(p.Path))) > 0 {
				n++
			}
		}
		if n == 0 {
			return 0
		}
	}
	open := filtering || !a.foldedGroups[e.group]
	ui.Row(c).Height(theme.ControlHeight).FillWidth().Gap(theme.S1).Children(func() {
		row := ui.ButtonBase(c).Grow(1).MinWidth(0).Height(theme.ControlHeight).PaddingX(theme.S2).Gap(theme.S1).Radius(theme.RadiusFull).Justify(ui.Start).Cursor(ui.CursorPointer).Label(e.group)
		row.Drag(listDrag{group: e.group})
		a.dropOnEntry(row, e)
		if row.Hovered() {
			row.Background(theme.Hover)
		}
		row.Children(func() {
			chev := icon.ChevronRight
			if open {
				chev = icon.ChevronDown
			}
			ui.Icon(c, chev).FontSize(12).TextColor(theme.TextSoft)
			ui.Icon(c, icon.Folder).FontSize(12).TextColor(theme.TextSoft)
			ui.Text(c, e.group).FontSize(theme.TextLabel).FontWeight(500).TextColor(theme.TextSoft).SingleLine().Shrink(1)
			ui.Spacer(c)
			ui.Text(c, fmt.Sprint(len(e.projects))).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine()
		})
		if row.Clicked() && !filtering {
			if a.foldedGroups == nil {
				a.foldedGroups = map[string]bool{}
			}
			a.foldedGroups[e.group] = open
		}
		key := "group:" + e.group
		a.menu.Open(row, key)
		a.menu.Show(c, row, key, func(done func()) { a.groupMenuItems(c, e, done) })
		more := smallIcon(c, icon.More, "More for "+e.group)
		a.menu.OpenBelow(more, "group-more:"+e.group, 24)
		a.menu.Show(c, more, "group-more:"+e.group, func(done func()) { a.groupMenuItems(c, e, done) })
	})
	if !open {
		return 1
	}
	shown := 0
	ui.Column(c).FillWidth().Padding(0, 0, 0, 10).Children(func() {
		for _, p := range e.projects {
			if a.projectGroup(c.Key(p.Path), p) {
				shown++
			}
		}
	})
	return shown
}

// dropOnEntry makes a project's or a group's header take a project or a
// group dropped on it, which goes before it: a project into its group.
func (a *App) dropOnEntry(row ui.Element, e listEntry) {
	first := e.projects[0].Path
	takes := func(d listDrag) bool {
		if d.path != "" {
			return d.path != first
		}
		return d.group != e.group
	}
	if d, ok := ui.Drop[listDrag](row); ok && takes(d) {
		if d.path != "" {
			a.arrangeProject(d.path, e.projects[0].Group, first)
		} else {
			a.moveGroup(d.group, first)
		}
	}
	if d, ok := ui.DragOver[listDrag](row); ok && takes(d) {
		row.BorderWidth(2, 0, 0, 0).BorderColor(theme.Accent)
	}
}

// dropOnProject is dropOnEntry for a project's header: a project dropped
// goes before it, into its group; a group, before the project's group.
func (a *App) dropOnProject(row ui.Element, p proto.ProjectInfo) {
	if p.Group == "" {
		a.dropOnEntry(row, listEntry{projects: []proto.ProjectInfo{p}})
		return
	}
	takes := func(d listDrag) bool { return d.path != "" && d.path != p.Path }
	if d, ok := ui.Drop[listDrag](row); ok && takes(d) {
		a.arrangeProject(d.path, p.Group, p.Path)
	}
	if d, ok := ui.DragOver[listDrag](row); ok && takes(d) {
		row.BorderWidth(2, 0, 0, 0).BorderColor(theme.Accent)
	}
}

// projectArrangeItems are a project's menu items that move it.
func (a *App) projectArrangeItems(c *ui.Context, p proto.ProjectInfo, done func()) {
	if up, ok := a.projectStep(p, -1); ok && widget.MenuItem(c, icon.ArrowUp, "Move up", false) {
		up()
		done()
	}
	if down, ok := a.projectStep(p, 1); ok && widget.MenuItem(c, icon.ArrowDown, "Move down", false) {
		down()
		done()
	}
	label := "Add to a group…"
	if p.Group != "" {
		label = "Move to another group…"
	}
	if widget.MenuItem(c, icon.Folder, label, false) {
		a.groupForm = &groupForm{open: true, project: p.Path}
		done()
	}
	if p.Group != "" && widget.MenuItem(c, icon.Close, "Take out of “"+p.Group+"”", false) {
		a.arrangeProject(p.Path, "", a.pathAfterEntry(p.Group))
		done()
	}
}

// groupMenuItems are a group's menu.
func (a *App) groupMenuItems(c *ui.Context, e listEntry, done func()) {
	entries := a.projectEntries()
	i := slices.IndexFunc(entries, func(x listEntry) bool { return x.group == e.group })
	if i > 0 && widget.MenuItem(c, icon.ArrowUp, "Move up", false) {
		a.moveGroup(e.group, entries[i-1].projects[0].Path)
		done()
	}
	if i >= 0 && i+1 < len(entries) && widget.MenuItem(c, icon.ArrowDown, "Move down", false) {
		before := ""
		if i+2 < len(entries) {
			before = entries[i+2].projects[0].Path
		}
		a.moveGroup(e.group, before)
		done()
	}
	if widget.MenuItem(c, icon.Pencil, "Rename group…", false) {
		a.groupForm = &groupForm{open: true, from: e.group, name: e.group}
		done()
	}
	widget.MenuSeparator(c)
	if widget.MenuItem(c, icon.Close, "Ungroup", false) {
		a.renameGroup(e.group, "")
		done()
	}
}

// projectStep is what moves p one step up (-1) or down (1) the list,
// within its group if it has one, and whether it can.
func (a *App) projectStep(p proto.ProjectInfo, dir int) (func(), bool) {
	entries := a.projectEntries()
	if p.Group != "" {
		i := slices.IndexFunc(entries, func(x listEntry) bool { return x.group == p.Group })
		if i < 0 {
			return nil, false
		}
		ps := entries[i].projects
		j := slices.IndexFunc(ps, func(x proto.ProjectInfo) bool { return x.Path == p.Path })
		switch {
		case dir < 0 && j > 0:
			return func() { a.arrangeProject(p.Path, p.Group, ps[j-1].Path) }, true
		case dir > 0 && j+1 < len(ps):
			return func() { a.arrangeProject(ps[j+1].Path, p.Group, p.Path) }, true
		}
		return nil, false
	}
	i := slices.IndexFunc(entries, func(x listEntry) bool { return x.group == "" && x.projects[0].Path == p.Path })
	switch {
	case i < 0:
		return nil, false
	case dir < 0 && i > 0:
		return func() { a.arrangeProject(p.Path, "", entries[i-1].projects[0].Path) }, true
	case dir > 0 && i+1 < len(entries):
		before := ""
		if i+2 < len(entries) {
			before = entries[i+2].projects[0].Path
		}
		return func() { a.arrangeProject(p.Path, "", before) }, true
	}
	return nil, false
}

// pathAfterEntry is the first project after group's, or "" when it is
// last or there is no such group: where a project taken out of it, or
// joining it, goes.
func (a *App) pathAfterEntry(group string) string {
	entries := a.projectEntries()
	i := slices.IndexFunc(entries, func(x listEntry) bool { return x.group == group })
	if i < 0 || i+1 >= len(entries) {
		return ""
	}
	return entries[i+1].projects[0].Path
}

// groupDialog names the group a project goes in, or a group's new name.
func (a *App) groupDialog(c *ui.Context) {
	f := a.groupForm
	if f == nil {
		return
	}
	if !f.open {
		a.groupForm = nil
		return
	}
	title, desc, verb := "Rename group", "Its projects move with it. A name another group has merges them.", "Rename"
	if f.project != "" {
		title, desc, verb = "Add to a group", "Groups keep projects together in the list, for every device of this host.", "Add"
	}
	save := false
	widget.FormSheet(c, &f.open, 420, title, desc, func() {
		widget.Field(c, "Group", func() {
			if widget.Input(c, &f.name, "Clients, Side projects…").FillWidth().Submitted() {
				save = true
			}
		})
		var groups []string
		for _, e := range a.projectEntries() {
			if e.group != "" && e.group != f.from {
				groups = append(groups, e.group)
			}
		}
		if len(groups) > 0 {
			ui.Row(c).FillWidth().Gap(6).Wrap().Children(func() {
				for _, g := range groups {
					if widget.GhostButton(c.Key(g), g).Label("Group “" + g + "”").Clicked() {
						f.name = g
					}
				}
			})
		}
	}, func() {
		b := widget.PrimaryButton(c, icon.Check, verb, "")
		if strings.TrimSpace(f.name) == "" {
			b.Disabled(true).Opacity(0.45)
		} else if b.Clicked() {
			save = true
		}
	})
	if !save || strings.TrimSpace(f.name) == "" {
		return
	}
	name := strings.TrimSpace(f.name)
	if f.project != "" {
		a.arrangeProject(f.project, name, a.pathAfterEntry(name))
	} else if name != f.from {
		a.renameGroup(f.from, name)
	}
	a.groupForm = nil
}

func (a *App) arrangeProject(path, group, before string) {
	a.callHost(func(cl *client.Client) error { return cl.ArrangeProject(path, group, before) })
}

func (a *App) moveGroup(group, before string) {
	a.callHost(func(cl *client.Client) error { return cl.MoveProjectGroup(group, before) })
}

func (a *App) renameGroup(from, to string) {
	if a.foldedGroups[from] {
		delete(a.foldedGroups, from)
		if to != "" {
			a.foldedGroups[to] = true
		}
	}
	a.callHost(func(cl *client.Client) error { return cl.RenameProjectGroup(from, to) })
}

// callHost runs call off the main thread, its error in the status bar; the
// projects event redraws the list.
func (a *App) callHost(call func(cl *client.Client) error) {
	cl := a.client
	if cl == nil {
		return
	}
	go func() {
		if err := call(cl); err != nil {
			a.onMain(func() { a.status = err.Error() })
		}
	}()
}

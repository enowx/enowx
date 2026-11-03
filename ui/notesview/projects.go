package notesview

import (
	"fmt"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// The projects pages are attached to: badges under a page's title, and
// the tree shown for one project, one host, or grouped by project.

// Project states, as Options.ProjectState says them.
const (
	ProjectOnline  = "online"
	ProjectOffline = "offline" // its host is out of reach
	ProjectBroken  = "broken"  // its host has it no more
)

// Project filters of the tree.
const (
	byProject   = "by-project"
	noProject   = "none"
	refFilter   = "ref:"
	hostFilter  = "host:"
	badgesShown = 3
)

// projectLabel is how a page's project reads: its name and host.
func projectLabel(p proto.NoteProject) string {
	if p.HostName == "" {
		return p.Name
	}
	return p.Name + " · " + p.HostName
}

// noteOf is page id as the host's list has it.
func noteOf(o Options, id string) (proto.NoteInfo, bool) {
	i := slices.IndexFunc(o.Notes, func(n proto.NoteInfo) bool { return n.ID == id })
	if i < 0 {
		return proto.NoteInfo{}, false
	}
	return o.Notes[i], true
}

// projectBadges are page n's projects under its title, the first few and
// +N for the rest, which a click shows; one whose project is gone offers
// to attach it again.
func (v *View) projectBadges(c *ui.Context, o Options, n proto.NoteInfo) {
	if len(n.Projects) == 0 && o.AttachProjects == nil {
		return
	}
	ui.Row(c).FillWidth().Gap(theme.S2).Padding(0, 0, theme.S2, theme.S1+2).Wrap().AlignItems(ui.Center).Children(func() {
		shown := n.Projects
		if len(shown) > badgesShown && !v.allBadges[n.ID] {
			shown = shown[:badgesShown]
		}
		for _, p := range shown {
			state := ProjectOnline
			if o.ProjectState != nil {
				state = o.ProjectState(p)
			}
			v.projectBadge(c.Key(p.Ref), o, n, p, state)
		}
		if extra := len(n.Projects) - len(shown); extra > 0 {
			more := ui.ButtonBase(c).Height(24).PaddingX(theme.S2).Radius(theme.RadiusFull).Background(theme.Sunken).Cursor(ui.CursorPointer).
				Label(fmt.Sprintf("%d more projects", extra))
			more.Children(func() { ui.Text(c, fmt.Sprintf("+%d", extra)).FontSize(theme.TextLabel).TextColor(theme.TextSoft) })
			if more.Clicked() {
				if v.allBadges == nil {
					v.allBadges = map[string]bool{}
				}
				v.allBadges[n.ID] = true
			}
		}
		if o.AttachProjects != nil {
			label := "Attach to a project"
			if len(n.Projects) > 0 {
				label = "Projects…"
			}
			add := ui.ButtonBase(c).Height(24).PaddingX(theme.S2).Gap(theme.S1).Radius(theme.RadiusFull).MinWidth(0).Shrink(1).Cursor(ui.CursorPointer).Label(label)
			if add.Hovered() {
				add.Background(theme.Hover)
			}
			add.Children(func() {
				ui.Icon(c, icon.Plus).FontSize(12).TextColor(theme.TextSoft)
				ui.Text(c, label).FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine().MinWidth(0).Shrink(1)
			})
			if add.Clicked() {
				o.AttachProjects(n)
			}
		}
	})
}

func (v *View) projectBadge(c *ui.Context, o Options, n proto.NoteInfo, p proto.NoteProject, state string) {
	col, tip := theme.TextSoft, p.Path
	switch state {
	case ProjectOffline:
		col, tip = theme.Muted, p.HostName+" is offline: "+p.Path
	case ProjectBroken:
		col, tip = theme.Error, "Not on "+p.HostName+" any more: click to attach it again"
	}
	b := ui.ButtonBase(c).Height(24).PaddingX(theme.S2).Gap(theme.S1).Radius(theme.RadiusFull).Background(theme.Sunken).MinWidth(0).Shrink(1).
		Cursor(ui.CursorPointer).Label("Project " + projectLabel(p)).Tooltip(tip).AlignItems(ui.Center)
	b.Children(func() {
		ui.Icon(c, icon.Folder).FontSize(12).TextColor(col)
		ui.Row(c).MinWidth(0).Shrink(1).Children(func() {
			widget.Running(c, projectLabel(p), widget.RunStyle{Size: theme.TextLabel})
		})
		if state != ProjectOnline {
			ui.Text(c, state).FontSize(theme.TextLabel).TextColor(col).Shrink(0)
		}
	})
	if b.Clicked() {
		if state == ProjectBroken && o.AttachProjects != nil {
			o.AttachProjects(n)
		} else {
			v.projectFilter = refFilter + p.Ref
		}
	}
}

// ShowProject shows the pages of project ref alone.
func (v *View) ShowProject(ref string) {
	v.projectFilter = refFilter + ref
	v.filter = ""
}

// projectChoices are the projects and hosts the pages are attached to.
func projectChoices(o Options) (projects []proto.NoteProject, hosts []string) {
	for _, n := range o.Notes {
		for _, p := range n.Projects {
			if !slices.ContainsFunc(projects, func(x proto.NoteProject) bool { return x.Ref == p.Ref }) {
				projects = append(projects, p)
			}
			if p.HostName != "" && !slices.Contains(hosts, p.HostName) {
				hosts = append(hosts, p.HostName)
			}
		}
	}
	slices.SortFunc(projects, func(a, b proto.NoteProject) int { return strings.Compare(projectLabel(a), projectLabel(b)) })
	slices.Sort(hosts)
	return projects, hosts
}

// projectFilterButton picks the project or host whose pages the tree
// shows, or shows them grouped by project.
func (v *View) projectFilterButton(c *ui.Context, o Options) {
	projects, hosts := projectChoices(o)
	if len(projects) == 0 {
		v.projectFilter = ""
		return
	}
	b := widget.IconButton(c, icon.Git, "Projects: "+v.projectFilterLabel(projects), v.projectFilter != "")
	v.menu.OpenBelow(b, "project-filter", theme.ControlHeight)
	v.menu.Show(c, b, "project-filter", func(done func()) {
		pick := func(c *ui.Context, svg *ui.SVG, label, value string) {
			if v.projectFilter == value {
				svg = icon.Check // the one shown
			}
			if widget.MenuItem(c, svg, label, false) {
				v.projectFilter = value
				done()
			}
		}
		pick(c.Key("all"), icon.List, "All pages", "")
		pick(c.Key("by"), icon.Folder, "Grouped by project", byProject)
		pick(c.Key("none"), icon.File, "Not attached", noProject)
		widget.MenuSeparator(c)
		for _, p := range projects {
			pick(c.Key("p"+p.Ref), icon.Git, projectLabel(p), refFilter+p.Ref)
		}
		if len(hosts) > 1 {
			widget.MenuSeparator(c)
			for _, h := range hosts {
				pick(c.Key("h"+h), icon.Server, "On "+h, hostFilter+h)
			}
		}
	})
}

func (v *View) projectFilterLabel(projects []proto.NoteProject) string {
	switch f := v.projectFilter; {
	case f == "":
		return "all pages"
	case f == byProject:
		return "grouped"
	case f == noProject:
		return "not attached"
	case strings.HasPrefix(f, hostFilter):
		return "on " + strings.TrimPrefix(f, hostFilter)
	default:
		for _, p := range projects {
			if refFilter+p.Ref == f {
				return projectLabel(p)
			}
		}
	}
	return "all pages"
}

// inProjectFilter reports whether page n is one the project filter shows.
func (v *View) inProjectFilter(n proto.NoteInfo) bool {
	switch f := v.projectFilter; {
	case f == "" || f == byProject:
		return true
	case f == noProject:
		return len(n.Projects) == 0
	case strings.HasPrefix(f, hostFilter):
		return slices.ContainsFunc(n.Projects, func(p proto.NoteProject) bool { return p.HostName == strings.TrimPrefix(f, hostFilter) })
	default:
		return slices.ContainsFunc(n.Projects, func(p proto.NoteProject) bool { return refFilter+p.Ref == f })
	}
}

// byProjectTree is the tree grouped by project: each project's pages
// under it (a page of several in each), then those of none.
func (v *View) byProjectTree(c *ui.Context, o Options, shown map[string]bool) {
	projects, _ := projectChoices(o)
	section := func(key, title string, svg *ui.SVG, in func(proto.NoteInfo) bool) {
		var pages []proto.NoteInfo
		for _, n := range o.Notes {
			if in(n) && (shown == nil || shown[n.ID]) {
				pages = append(pages, n)
			}
		}
		if len(pages) == 0 {
			return
		}
		ui.Row(c.Key(key)).FillWidth().Height(theme.ControlHeight).Shrink(0).PaddingX(theme.S3).Gap(theme.S2).Children(func() {
			ui.Icon(c, svg).FontSize(12).TextColor(theme.TextSoft)
			ui.Row(c).Grow(1).MinWidth(0).Children(func() {
				widget.Running(c, title, widget.RunStyle{Size: theme.TextLabel, Weight: 500, Color: theme.TextSoft})
			})
			ui.Text(c, fmt.Sprint(len(pages))).FontSize(theme.TextLabel).TextColor(theme.TextSoft).Shrink(0)
		})
		ui.Column(c.Key(key + ":pages")).FillWidth().Gap(2).Children(func() {
			for _, n := range pages {
				v.treeRow(c.Key(n.ID), o, n, 0, false, nil)
			}
		})
	}
	for _, p := range projects {
		ref := p.Ref
		section("project:"+ref, projectLabel(p), icon.Git, func(n proto.NoteInfo) bool {
			return slices.ContainsFunc(n.Projects, func(x proto.NoteProject) bool { return x.Ref == ref })
		})
	}
	section("project:none", "No project", icon.File, func(n proto.NoteInfo) bool { return len(n.Projects) == 0 })
}

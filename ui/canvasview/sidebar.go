package canvasview

import (
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// kindIcon is a kind of canvas's icon.
func kindIcon(kind string) *ui.SVG {
	switch kind {
	case proto.CanvasDiagram:
		return icon.ArrowLink
	case proto.CanvasWireframe:
		return icon.Square
	case proto.CanvasBoard:
		return icon.Sticky
	}
	return icon.Canvas
}

// sidebar lists the canvases, by group, the open one marked and those an
// agent designs now dotted.
func (v *View) sidebar(c *ui.Context, o Options) {
	v.listColumn(c).Children(func() {
		ui.Row(c).FillWidth().Height(44).Shrink(0).PaddingX(10).Gap(6).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(theme.Border).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Children(func() {
				ui.Text(c, "Canvas").FontSize(theme.TextUI).SingleLine()
				if len(o.Spaces) > 1 && o.PickSpace != nil {
					name := o.Space
					if o.Team == "" {
						name = "Personal"
					}
					widget.SpacePicker(c, widget.Space{Team: o.Team, Name: name}, "on "+o.Host, 10.5, o.Spaces, o.PickSpace)
					return
				}
				if o.Team == "" {
					ui.Text(c, "On "+o.Host).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine().Shrink(1)
					return
				}
				ui.Row(c).Gap(5).AlignItems(ui.Center).MinWidth(0).Children(func() {
					widget.TeamMark(c, o.Team, o.Space, 11)
					ui.Text(c, o.Space+" · on "+o.Host).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine().Shrink(1)
				})
			})
			// Until the list came, a plain button: an accent one faded reads poorly.
			kind := widget.Primary
			if !o.Loaded {
				kind = widget.Secondary
			}
			b := widget.Button(c, kind, icon.Plus, "New", "").Height(28).PaddingX(10).Tooltip("New canvas")
			if !o.Loaded {
				b.Disabled(true)
			} else if b.Clicked() {
				v.creating = &createForm{open: true}
			}
		})
		ui.Row(c).FillWidth().Padding(8, 10, 4, 10).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Children(func() { widget.SearchField(c, &v.filter, "Filter canvases", icon.Search) })
		})
		ui.Scroll(c).Grow(1).FillWidth().Children(func() {
			ui.Column(c).FillWidth().Padding(6).Gap(1).Children(func() {
				if o.Loaded && len(o.Canvases) == 0 {
					ui.Text(c, "No canvases yet").FontSize(theme.TextLabel).TextColor(theme.Muted).Padding(6)
				}
				q := strings.ToLower(strings.TrimSpace(v.filter))
				var groups []string
				for _, cv := range o.Canvases {
					if !slices.Contains(groups, cv.Group) {
						groups = append(groups, cv.Group)
					}
				}
				slices.SortFunc(groups, func(a, b string) int {
					if (a == "") != (b == "") {
						if a == "" {
							return 1
						}
						return -1
					}
					return strings.Compare(a, b)
				})
				for _, g := range groups {
					if len(groups) > 1 || g != "" {
						name := g
						if name == "" {
							name = "No group"
						}
						widget.SectionLabel(c.Key("group:"+g), strings.ToUpper(name), theme.Muted).Padding(10, 6, 4, 6)
					}
					for _, cv := range o.Canvases {
						if cv.Group == g && (q == "" || strings.Contains(strings.ToLower(cv.Title), q)) {
							v.listRow(c.Key(cv.ID), o, cv)
						}
					}
				}
			})
		})
	})
}

func (v *View) listRow(c *ui.Context, o Options, cv proto.CanvasInfo) {
	row := ui.ButtonBase(c).FillWidth().Height(32).PaddingX(8).Gap(8).Radius(theme.RadiusSmall).Cursor(ui.CursorPointer).Label(cv.Title).AlignItems(ui.Center)
	switch {
	case cv.ID == v.selected:
		row.Background(theme.Selected)
	case row.Hovered():
		row.Background(theme.Hover)
	}
	if row.Clicked() {
		v.Open(o, cv.ID)
	}
	v.menu.Open(row, "canvas:"+cv.ID)
	v.menu.Show(c, row, "canvas:"+cv.ID, func(done func()) {
		if widget.MenuItem(c.Key("rename"), icon.Pencil, "Rename…", false) {
			v.Open(o, cv.ID)
			title := cv.Title
			v.renaming = &title
			done()
		}
		if widget.MenuItem(c.Key("delete"), icon.Trash, "Delete…", true) {
			x := cv
			v.deleting = &x
			done()
		}
	})
	row.Children(func() {
		ui.Icon(c, kindIcon(cv.Kind)).FontSize(13).TextColor(theme.Muted)
		ui.Text(c, cv.Title).FontSize(theme.TextUI).TextColor(theme.Text).SingleLine().Grow(1).Shrink(1).MinWidth(0)
		switch {
		case cv.Paused:
			ui.Icon(c, icon.Pause).FontSize(11).TextColor(theme.Warning).Tooltip("Agents paused")
		case cv.Agent != "" && time.Since(cv.AgentAt) < designingFor:
			widget.Dot(c, theme.Accent, false, 7)
		}
	})
}

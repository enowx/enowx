package canvasview

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// kindName is a kind of canvas's name.
func kindName(kind string) string {
	for _, k := range kinds {
		if k.id == kind {
			return k.name
		}
	}
	return "Canvas"
}

// sidebar lists the canvases, by group, the open one marked and those an
// agent designs now dotted.
func (v *View) sidebar(c *ui.Context, o Options) {
	v.listColumn(c).Children(func() {
		ui.Column(c).FillWidth().Shrink(0).Padding(theme.S3, theme.S3, theme.S1, theme.S3).Gap(theme.S2).Children(func() {
			ui.Row(c.Key("list-head")).FillWidth().MinHeight(theme.ControlHeight).Padding(0, 0, 0, theme.S3).Children(func() {
				ui.Row(c).Grow(1).MinWidth(0).Children(func() {
					widget.Running(c, "Canvas", widget.RunStyle{Weight: 600})
				})
				// Disabled until the list came: there is nothing to add it to yet.
				b := widget.IconButton(c.Key("new"), icon.Plus, "New canvas", false)
				if !o.Loaded {
					b.Disabled(true)
				} else if b.Clicked() {
					v.creating = &createForm{open: true}
				}
			})
			v.spaceLine(c, o)
			widget.Input(c.Key("filter"), &v.filter, "Filter canvases").Label("Filter canvases")
		})
		ui.Scroll(c.Key("list-rows")).Grow(1).FillWidth().MinHeight(0).Children(func() {
			ui.Column(c).FillWidth().Padding(theme.S1, theme.S3, theme.S3, theme.S3).Gap(theme.S1).Children(func() {
				if o.Loaded && len(o.Canvases) == 0 {
					ui.Text(c, "No canvases yet").FontSize(theme.TextUI).TextColor(theme.TextSoft).Padding(theme.S2, theme.S3)
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
						widget.SideLabel(c.Key("group:"+g), name)
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

// spaceLine says whose canvases these are, when they aren't simply the user's.
func (v *View) spaceLine(c *ui.Context, o Options) {
	if len(o.Spaces) > 1 && o.PickSpace != nil {
		name := o.Space
		if o.Team == "" {
			name = "Personal"
		}
		widget.SpacePicker(c, widget.Space{Team: o.Team, Name: name}, "on "+o.Host, theme.TextLabel, o.Spaces, o.PickSpace)
		return
	}
	if o.Team != "" {
		ui.Row(c.Key("space")).FillWidth().Gap(theme.S1).Padding(0, 0, 0, theme.S3).MinWidth(0).Children(func() {
			widget.TeamMark(c, o.Team, o.Space, 12)
			ui.Row(c).Grow(1).MinWidth(0).Children(func() {
				widget.Running(c, o.Space+" · on "+o.Host, widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
			})
		})
	}
}

// designsLine is what a canvas's row says under its title; only the canvas
// open is known to have so many designs.
func (v *View) designsLine(cv proto.CanvasInfo) string {
	name := kindName(cv.Kind)
	if cv.ID == v.selected && v.canvas != nil && cv.Kind == proto.CanvasMockup {
		n := len(v.canvas.Frames)
		return name + " · " + textfmt.Plural(n, "1 design", fmt.Sprintf("%d designs", n))
	}
	return name
}

func (v *View) listRow(c *ui.Context, o Options, cv proto.CanvasInfo) {
	row := widget.ListItem(c, ui.Color{}, cv.Title, v.designsLine(cv), cv.ID == v.selected, func() {
		switch {
		case cv.Paused:
			ui.Icon(c, icon.Pause).FontSize(12).TextColor(theme.Warning).Tooltip("Agents paused")
		case cv.Agent != "" && time.Since(cv.AgentAt) < designingFor:
			widget.Dot(c, theme.Accent, false, 8)
		}
	})
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
}

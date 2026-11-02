package canvasview

import (
	"fmt"
	"slices"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/canvas"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// compareState is a version compared with the one shown.
type compareState struct {
	n       int
	changes []canvas.Change
}

// versions lists the canvas's versions, the newest first: whose each is,
// what it changed; one compares with the one shown, or comes back.
func (v *View) versions(c *ui.Context, o Options) {
	h := v.history
	if h == nil {
		ui.Text(c, "Loading…").FontSize(theme.TextUI).TextColor(theme.TextSoft)
		return
	}
	if cmp := v.compare; cmp != nil {
		widget.Raise(ui.Column(c.Key("compare")).FillWidth().Shrink(0).Padding(theme.S2, theme.S3).Gap(theme.S1).Radius(theme.RadiusCard).Background(theme.Raised)).Children(func() {
			ui.Row(c.Key("head")).FillWidth().Gap(theme.S2).AlignItems(ui.Center).Children(func() {
				ui.Text(c, fmt.Sprintf("Version %d → %d", cmp.n, h.Head)).FontSize(theme.TextUI).FontWeight(500).Grow(1).Shrink(1).MinWidth(0).SingleLine()
				if widget.IconButton(c, icon.Close, "Close comparison", false).Clicked() {
					v.compare = nil
				}
			})
			if len(cmp.changes) == 0 {
				ui.Text(c.Key("same"), "The same").FontSize(theme.TextLabel).TextColor(theme.TextSoft)
			}
			for i, ch := range cmp.changes {
				col := map[string]ui.Color{"added": theme.Done, "removed": theme.Error}[ch.Kind]
				if col == (ui.Color{}) {
					col = theme.Warning
				}
				ui.Row(c.Key(i)).FillWidth().Gap(theme.S2).Children(func() {
					ui.Text(c, ch.Kind).FontSize(theme.TextLabel).TextColor(col).Width(58).Shrink(0).SingleLine()
					what := ch.Element
					if what == "" {
						what = "frame " + ch.Frame
					}
					ui.Row(c).Grow(1).MinWidth(0).Children(func() { widget.Running(c, what, widget.RunStyle{Size: theme.TextLabel}) })
				})
			}
		})
	}
	list := slices.Clone(h.Versions)
	slices.Reverse(list)
	for _, ver := range list {
		who := "You"
		if ver.Session != "" {
			who = "Agent"
			if o.AgentName != nil {
				who = o.AgentName(ver.Session)
			}
		}
		what := ver.Op
		if ver.Element != "" {
			what += " " + ver.Element
		}
		widget.Raise(ui.Row(c.Key(ver.N)).FillWidth().MinHeight(44).Shrink(0).Padding(theme.S1, theme.S2, theme.S1, theme.S3).Gap(theme.S2).Radius(theme.RadiusControl).AlignItems(ui.Center).Background(theme.Raised)).Children(func() {
			dot := theme.Muted
			if ver.N == h.Head {
				dot = theme.Accent
			}
			widget.Dot(c, dot, ver.N > h.Head, 8)
			ui.Column(c).Grow(1).MinWidth(0).Children(func() {
				widget.Running(c, fmt.Sprintf("Version %d", ver.N), widget.RunStyle{})
				widget.Running(c.Key("sub"), who+" · "+ago(ver.Time)+" · "+what, widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
			})
			if ver.N == h.Head {
				ui.Text(c, "shown").FontSize(theme.TextLabel).TextColor(theme.Now).SingleLine().Shrink(0)
				return
			}
			if widget.IconButton(c.Key("cmp"), icon.Layers, fmt.Sprintf("Compare v%d with the one shown", ver.N), v.compare != nil && v.compare.n == ver.N).Clicked() {
				v.compareWith(o, ver.N)
			}
			if widget.GhostButton(c.Key("restore"), "Restore").Label(fmt.Sprintf("Restore v%d", ver.N)).Clicked() {
				o.Restore(v.canvas.ID, ver.N, func(cv proto.Canvas, err error) {
					if err != nil {
						v.err = err.Error()
						return
					}
					v.show(o, cv)
					v.loadHistory(o)
				})
			}
		})
	}
}

func (v *View) compareWith(o Options, n int) {
	if v.canvas == nil {
		return
	}
	cur := *v.canvas
	o.Version(cur.ID, n, func(old proto.Canvas, err error) {
		if err != nil {
			v.err = err.Error()
			return
		}
		v.compare = &compareState{n: n, changes: canvas.Compare(old, cur)}
	})
}

// ShowVersions opens the versions beside the stage, version n compared
// with the one shown.
func (v *View) ShowVersions(o Options, n int) {
	v.panel = "versions"
	v.loadHistory(o)
	v.compareWith(o, n)
}

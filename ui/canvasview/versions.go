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
		ui.Text(c, "Loading…").FontSize(theme.TextLabel).TextColor(theme.Muted)
		return
	}
	if cmp := v.compare; cmp != nil {
		ui.Column(c).FillWidth().Padding(10).Gap(4).Radius(theme.RadiusSmall).Background(theme.Raised).Border(1, theme.Border).Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
				ui.Text(c, fmt.Sprintf("Version %d → %d", cmp.n, h.Head)).FontSize(theme.TextLabel).FontWeight(600).Grow(1)
				if widget.IconButton(c, icon.Close, "Close comparison", false).Clicked() {
					v.compare = nil
				}
			})
			if len(cmp.changes) == 0 {
				ui.Text(c, "The same").FontSize(theme.TextLabel).TextColor(theme.Muted)
			}
			for i, ch := range cmp.changes {
				col := map[string]ui.Color{"added": theme.Done, "removed": theme.Error}[ch.Kind]
				if col == (ui.Color{}) {
					col = theme.Warning
				}
				ui.Row(c.Key(i)).FillWidth().Gap(6).Children(func() {
					ui.Text(c, ch.Kind).FontSize(theme.TextLabel).TextColor(col).Width(58)
					what := ch.Element
					if what == "" {
						what = "frame " + ch.Frame
					}
					ui.Text(c, what).FontSize(theme.TextLabel).TextColor(theme.Text).SingleLine().Shrink(1)
				})
			}
		})
	}
	list := slices.Clone(h.Versions)
	slices.Reverse(list)
	for _, ver := range list {
		ui.Row(c.Key(ver.N)).FillWidth().Padding(6).Gap(8).Radius(theme.RadiusSmall).AlignItems(ui.Center).Children(func() {
			dot := theme.Muted
			if ver.N == h.Head {
				dot = theme.Accent
			}
			widget.Dot(c, dot, ver.N > h.Head, 7)
			ui.Column(c).Grow(1).MinWidth(0).Children(func() {
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
				ui.Text(c, fmt.Sprintf("v%d · %s", ver.N, what)).FontSize(theme.TextLabel).SingleLine()
				ui.Text(c, who+" · "+ago(ver.Time)).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine()
			})
			if ver.N == h.Head {
				ui.Text(c, "shown").FontSize(theme.TextLabel).TextColor(theme.Accent)
				return
			}
			if widget.IconButton(c.Key("cmp"), icon.Layers, fmt.Sprintf("Compare v%d with the one shown", ver.N), v.compare != nil && v.compare.n == ver.N).Clicked() {
				v.compareWith(o, ver.N)
			}
			if widget.IconButton(c.Key("restore"), icon.History, fmt.Sprintf("Restore v%d", ver.N), false).Clicked() {
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

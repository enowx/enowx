package browserview

import (
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// DrawPages shows what the session has to look at, for the panel: its dev
// servers and the other site the page may be on, each opening in the center,
// where the session's one web view shows it. This draws no web view.
func (v *View) DrawPages(c *ui.Context, o Options) {
	v.follow(o)
	ui.Column(c).Grow(1).FillWidth().MinHeight(0).Padding(0, theme.S3, theme.S3, theme.S3).Gap(theme.S2).Children(func() {
		widget.ToolHeader(c, func() {
			ui.Text(c, "Pages").FontSize(theme.TextUI).FontWeight(500).SingleLine().Shrink(0)
			ui.Row(c).Grow(1).MinWidth(0).Justify(ui.End).Children(func() {
				widget.Running(c, "Open in a tab", widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
			})
		})
		if v.err != "" {
			widget.Notice(c, nil, theme.Del, v.err, "", nil)
		}
		switch {
		case !o.Supported:
			v.note(c, o, "This device has no web view to show pages in. Open them in the browser instead.")
		case !v.loaded && v.page == nil:
			v.note(c, o, "Looking for the session's dev server…")
		case len(v.ports) == 0 && !v.showsOtherSite() && !v.stopped:
			ui.Column(c).Grow(1).FillWidth().Center().Padding(theme.S5).Gap(theme.S3).Children(func() {
				ui.Text(c, "Waiting for a dev server: start one in the session (npm run dev, say) and it shows here.").
					FontSize(theme.TextUI).TextColor(theme.TextSoft).TextAlign(ui.Center)
				if widget.Button(c, widget.Secondary, icon.Globe, "Open a page", "").Clicked() {
					v.openTab(o)
				}
			})
		default:
			widget.Scroll(c).Grow(1).FillWidth().MinHeight(0).Children(func() {
				ui.Column(c).FillWidth().Gap(2).Children(func() { v.pageRows(c, o) })
			})
		}
	})
}

// showsOtherSite reports whether the page is on a site the user or the agent
// went to, not on one of the session's ports nor the page before them.
func (v *View) showsOtherSite() bool { return v.page != nil && v.external && !v.home }

// pageRows lists the other site the page is on, then the session's ports, and
// the port shown even when it stopped: its page comes back with it.
func (v *View) pageRows(c *ui.Context, o Options) {
	front := o.Shown != nil && o.Shown()
	if v.showsOtherSite() {
		u := v.shown()
		if pageRow(c.Key("site"), theme.Done, strings.TrimPrefix(strings.TrimPrefix(u, "https://"), "http://"), "Another site", front) {
			v.openTab(o)
		}
	}
	ports := v.ports
	if v.stopped && !slices.ContainsFunc(ports, func(p proto.Port) bool { return p.Port == v.port }) {
		ports = append(slices.Clone(ports), proto.Port{Port: v.port})
	}
	for _, p := range ports {
		on := p.Port == v.port && !v.external
		dot, sub := theme.Done, p.Process
		if on && v.stopped {
			dot, sub = theme.Idle, "Stopped: the page comes back when it does"
		}
		if sub == "" {
			sub = "Listening"
		}
		if pageRow(c.Key(p.Port), dot, "localhost:"+strconv.Itoa(p.Port), sub, on && front) {
			if !on {
				v.pick(o, p)
			}
			v.openTab(o)
		}
	}
}

// pageRow is a page in the list: a dot, its address, which runs when too long,
// and what it is. It reports a click.
func pageRow(c *ui.Context, dot ui.Color, address, sub string, on bool) bool {
	b := ui.ButtonBase(c).FillWidth().MinHeight(36).Padding(theme.S1, theme.S3).Gap(theme.S2).Radius(theme.RadiusCard).Justify(ui.Start).
		Cursor(ui.CursorPointer).Label(address)
	bg := theme.Fade(theme.Selected, 0)
	switch {
	case on:
		bg = theme.Selected
	case b.Hovered():
		bg = theme.Hover
	}
	b.Background(widget.QuickTint(b, "bg", bg)).Children(func() {
		widget.Dot(c, dot, false, 8)
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			widget.Running(c, address, widget.RunStyle{Mono: true, Always: on})
			widget.Running(c.Key("sub"), sub, widget.RunStyle{Size: theme.TextLabel, Color: theme.SoftOn(on), Always: on})
		})
	})
	return b.Clicked()
}

func (v *View) openTab(o Options) {
	if o.OpenTab != nil {
		o.OpenTab()
	}
}

// Color is the color of the page's state, for its tab's dot.
func (v *View) Color() ui.Color {
	switch {
	case v.page == nil || v.stopped:
		return theme.Idle
	case v.page.Loading():
		return theme.Working
	}
	return theme.Done
}

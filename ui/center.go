package ui

import (
	"path"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// What a tool opens shows as a tab in the center, beside the session's own
// tabs, so it gets the width: a file, a file's changes, a check's output, a
// request, a page. The side panel keeps the list it was opened from. A tab is
// its ID: its kind's prefix, then what it shows.
const (
	centerDiff     = "diff:"     // a changed file's path
	centerCheck    = "check:"    // a check's name
	centerExchange = "exchange:" // a request's ID
	centerBrowser  = "browser:"  // the session's page
	centerAndroid  = "android:"  // the phone's screen
	centerActivity = "activity:" // an entry of the activity
)

// centerTabs are a session's open viewers, and the one in front; "" is the
// session's own tabs in front.
type centerTabs struct {
	tabs  []string
	front string
}

func (a *App) centerOf(id string) *centerTabs {
	if a.center == nil {
		a.center = map[string]*centerTabs{}
	}
	t := a.center[id]
	if t == nil {
		t = &centerTabs{}
		a.center[id] = t
	}
	return t
}

// openCenter brings tab to the front of session s, opening it unless open.
func (a *App) openCenter(s proto.SessionInfo, tab string) {
	s = a.parentOf(s)
	t := a.centerOf(s.ID)
	if !slices.Contains(t.tabs, tab) {
		t.tabs = append(t.tabs, tab)
	}
	t.front = tab
	a.page = ""
}

// closeCenter closes tab of session id; the tab before it comes to the front
// if it was in front, else the session's own.
func (a *App) closeCenter(id, tab string) {
	t := a.centerOf(id)
	i := slices.Index(t.tabs, tab)
	if i < 0 {
		return
	}
	t.tabs = slices.Delete(slices.Clone(t.tabs), i, i+1)
	if t.front == tab {
		t.front = ""
		if i > 0 {
			t.front = t.tabs[i-1]
		}
	}
	if p, ok := strings.CutPrefix(tab, fileTabPrefix); ok {
		delete(a.docs, id+"\x00"+p)
		a.closePage(id, p)
	}
}

// centerFront is the viewer in front of session s, "" for none.
func (a *App) centerFront(s proto.SessionInfo) string {
	if a.center == nil {
		return ""
	}
	if t := a.center[a.parentOf(s).ID]; t != nil {
		return t.front
	}
	return ""
}

// centerShows reports whether tab is in front of session id, the session the
// window shows.
func (a *App) centerShows(id, tab string) bool {
	if a.center == nil || a.page != "" {
		return false
	}
	t := a.center[id]
	return t != nil && t.front == tab
}

// centerFiles are the files session id has open.
func (a *App) centerFiles(id string) []string {
	var out []string
	if a.center != nil && a.center[id] != nil {
		for _, tab := range a.center[id].tabs {
			if p, ok := strings.CutPrefix(tab, fileTabPrefix); ok {
				out = append(out, p)
			}
		}
	}
	return out
}

// centerName is a tab's label.
func centerName(tab string) string {
	kind, what, _ := strings.Cut(tab, ":")
	switch kind + ":" {
	case fileTabPrefix:
		return path.Base(what)
	case centerDiff:
		return path.Base(what) + " · diff"
	case centerBrowser:
		return nonEmpty(what, "Browser")
	case centerAndroid:
		return nonEmpty(what, "Phone")
	}
	return what
}

// centerIcon is a tab's icon.
func centerIcon(tab string) *ui.SVG {
	kind, _, _ := strings.Cut(tab, ":")
	switch kind + ":" {
	case centerDiff:
		return icon.Git
	case centerCheck:
		return icon.Check
	case centerExchange:
		return icon.Traffic
	case centerBrowser:
		return icon.Globe
	case centerAndroid:
		return icon.Android
	case centerActivity:
		return icon.List
	}
	return icon.File
}

// centerDot is the color of a tab's status dot: what is changing moves.
func (a *App) centerDot(s proto.SessionInfo, tab string) ui.Color {
	if strings.HasPrefix(tab, centerDiff) {
		return theme.Working
	}
	return theme.Idle
}

// centerTabButton is a viewer's tab in the strip, with its ×.
func (a *App) centerTabButton(c *ui.Context, s proto.SessionInfo, tab string, front bool) {
	a.tabFace(c.Key("v:"+tab), centerName(tab), tab, front, true, a.centerDot(s, tab), false, centerIcon(tab),
		func() { a.centerOf(s.ID).front = tab },
		func() { a.closeCenter(s.ID, tab) })
}

// tabWidth is the width of every tab, whatever it says.
const tabWidth = 160

// tabFace is a tab of the strip: one width, its status dot, its icon and name,
// which runs in front or under the pointer, and an × when it can close.
func (a *App) tabFace(c *ui.Context, name, tip string, front, closable bool, dot ui.Color, hollow bool, svg *ui.SVG, pick, closeIt func()) {
	b := ui.ButtonBase(c).Width(tabWidth).Height(28).Padding(0, theme.S2).Gap(theme.S1).Radius(theme.RadiusFull).Shrink(0).Justify(ui.Start).
		Label(name).Tooltip(tip).Cursor(ui.CursorPointer)
	bg, fg := theme.Fade(theme.Selected, 0), theme.TextSoft
	switch {
	case front:
		bg, fg = theme.Selected, theme.TextStrong
	case b.Hovered():
		bg, fg = theme.Hover, theme.Text
	}
	b.Background(widget.QuickTint(b, "bg", bg)).TextColor(widget.QuickTint(b, "fg", fg))
	closed := false
	b.Children(func() {
		widget.Dot(c, dot, hollow, 8)
		ui.Icon(c, svg).FontSize(12)
		ui.Row(c).Grow(1).MinWidth(0).Children(func() {
			widget.Running(c, name, widget.RunStyle{Always: front})
		})
		if !closable {
			return
		}
		x := ui.ButtonBase(c).Size(16, 16).Shrink(0).Radius(theme.RadiusFull).Center().Cursor(ui.CursorPointer).Label("Close " + name)
		if x.Hovered() {
			x.Background(theme.Hover)
		}
		x.Children(func() { ui.Icon(c, icon.Close).FontSize(10).TextColor(theme.TextSoft) })
		closed = x.Clicked()
	})
	switch {
	case closed:
		closeIt()
	case b.Clicked():
		pick()
	}
}

// centerBody draws the viewer in front.
func (a *App) centerBody(c *ui.Context, s proto.SessionInfo, tab string) {
	kind, what, _ := strings.Cut(tab, ":")
	switch kind + ":" {
	case fileTabPrefix:
		a.docTab(c.Key("doc:"+tab), s, what)
	case centerDiff:
		a.diffTab(c.Key("diff:"+tab), s, what)
	case centerCheck:
		a.checkTab(c.Key("check:"+tab), s, what)
	case centerExchange:
		a.exchangeTab(c.Key("exchange:"+tab), s, what)
	case centerBrowser:
		a.browserTab(c.Key("browser:"+s.ID), s)
	case centerAndroid:
		a.androidTab(c.Key("android"), s)
	case centerActivity:
		a.activityEntryTab(c.Key("activity:"+tab), s, what)
	}
}

// checkTab draws a check's output, in the center.
func (a *App) checkTab(c *ui.Context, s proto.SessionInfo, name string) {
	a.checksTab(c, s)
}

// exchangeTab draws a request of the traffic, in the center.
func (a *App) exchangeTab(c *ui.Context, s proto.SessionInfo, id string) {
	a.trafficTab(c, s)
}

// activityEntryTab draws an entry of the activity, in the center.
func (a *App) activityEntryTab(c *ui.Context, s proto.SessionInfo, what string) {
	a.activityTab(c, s)
}

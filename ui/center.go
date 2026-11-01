package ui

import (
	"path"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/androidview"
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
	centerExchange = "exchange:" // a request's ID, then its name
	centerBrowser  = "browser:"  // the session's page
	centerAndroid  = "android:"  // the phone's screen
	centerGOA      = "goa:"      // a session opened from GOD of Agents' Sessions tool (goa_tabs.go)
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
	if key, ok := strings.CutPrefix(tab, centerGOA); ok {
		a.dropGOATab(key)
	}
	if what, ok := strings.CutPrefix(tab, centerExchange); ok {
		if n, ok := exchangeID(what); ok {
			a.trafficOf().ForgetExchange(n)
		}
	}
	if tab == centerAndroid+androidview.ViewScreen {
		a.androidOf().CloseScreen()
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
	case centerExchange:
		_, name, _ := strings.Cut(what, " ")
		return name
	case centerBrowser:
		return nonEmpty(what, "Browser")
	case centerAndroid:
		if what == androidview.ViewScreen {
			return "Screen"
		}
		return "Logcat"
	}
	return what
}

// centerLabel is a tab's label, by what shows now: a device's screen says whose.
func (a *App) centerLabel(tab string) string {
	if tab == centerAndroid+androidview.ViewScreen {
		if name := a.androidOf().PickedName(); name != "" {
			return name + " screen"
		}
	}
	return centerName(tab)
}

// centerIcon is a tab's icon.
func centerIcon(tab string) *ui.SVG {
	kind, _, _ := strings.Cut(tab, ":")
	switch kind + ":" {
	case centerDiff:
		return icon.Git
	case centerCheck:
		return icon.ListChecks
	case centerExchange:
		return icon.Traffic
	case centerBrowser:
		return icon.Globe
	case centerAndroid:
		return icon.Android
	}
	return icon.File
}

// centerDot is the color of a tab's status dot: what is changing moves.
func (a *App) centerDot(s proto.SessionInfo, tab string) ui.Color {
	switch {
	case strings.HasPrefix(tab, centerDiff):
		return theme.Working
	case strings.HasPrefix(tab, centerCheck):
		name, _ := strings.CutPrefix(tab, centerCheck)
		return a.checkViewOf(s.ID).Color(name)
	case strings.HasPrefix(tab, centerBrowser):
		return a.browserOf(s.ID).Color()
	case strings.HasPrefix(tab, centerExchange):
		if n, ok := exchangeID(strings.TrimPrefix(tab, centerExchange)); ok {
			return a.trafficOf().ExchangeColor(n)
		}
	case strings.HasPrefix(tab, centerAndroid):
		if a.androidOf().Online() {
			return theme.Done
		}
	}
	return theme.Idle
}

// centerTabButton is a viewer's tab in the strip, with its ×.
func (a *App) centerTabButton(c *ui.Context, s proto.SessionInfo, tab string, front bool) {
	if key, ok := strings.CutPrefix(tab, centerGOA); ok {
		a.goaTabButton(c, s, key, front)
		return
	}
	a.tabFace(c.Key("v:"+tab), a.centerLabel(tab), tab, front, true, a.centerDot(s, tab), false, centerIcon(tab),
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
		a.browserNews = false // looked at
		a.browserTab(c.Key("browser:"+s.ID), s)
	case centerAndroid:
		a.androidCenter(c.Key("android:"+what), s, what)
	case centerGOA:
		a.goaTabView(c, what)
	}
}


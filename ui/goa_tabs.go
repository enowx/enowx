package ui

import (
	"slices"
	"strings"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// A session looked at from GOD of Agents' mode, on any host the app reaches:
// a center tab of GOA's session (center.go, centerGOA) with a live terminal of
// its own connection, GOA and its conversation staying where they are. A tab
// is a view: closing it detaches, and the session runs on. The main thread
// owns these.
type goaTab struct {
	key      string // host and session
	host, id string
	agent    string
	cl       *client.Client
	term     *terminal.Terminal
	stream   *client.Stream
	err      string
	closed   bool
}

// goaClientOf is the connection to the host the Sessions tool names.
func (a *App) goaClientOf(host string) *client.Client {
	for _, h := range a.goaEveryHost() {
		if h.name == host {
			return h.cl
		}
	}
	return nil
}

// openGOATab shows session s of host in a tab, a new one unless it has one,
// bringing its agent back when it stopped, as showing a session does.
func (a *App) openGOATab(host string, cl *client.Client, s proto.SessionInfo) {
	key := host + "\x00" + s.ID
	if i := slices.IndexFunc(a.goa.tabs, func(t *goaTab) bool { return t.key == key }); i >= 0 {
		if a.goa.tabs[i].err == "" {
			a.page = ""
			a.centerOf(a.goa.board.Session).front = centerGOA + key
			return
		}
		a.closeGOATab(a.goa.tabs[i]) // an errored tab is opened again
	}
	t := &goaTab{key: key, host: host, id: s.ID, agent: s.Agent, cl: cl}
	a.goa.tabs = append(a.goa.tabs, t)
	tabs := a.centerOf(a.goa.board.Session)
	tabs.tabs, tabs.front = append(tabs.tabs, centerGOA+key), centerGOA+key
	a.page = ""
	go func() {
		if s.Exited {
			if _, err := cl.ContinueSession(s.ID); err != nil {
				a.onMain(func() { t.err = client.Explain(err) })
				return
			}
		}
		st, err := cl.Attach(s.ID)
		a.onMain(func() {
			switch {
			case t.closed:
				if st != nil {
					st.Close()
				}
			case err != nil:
				t.err = client.Explain(err)
			default:
				a.attachGOATab(t, st)
			}
		})
	}()
}

func (a *App) attachGOATab(t *goaTab, st *client.Stream) {
	term, err := terminal.New(terminal.Options{
		Conn:         &settledResize{ReadWriteCloser: st, resize: st.ResizePixels},
		ResizeByConn: true,
		Theme:        theme.Terminal(),
		DarkTheme:    theme.Terminal(),
		Font:         a.termFont(),
		OnZoom:       a.zoomTerms,
		Padding:      terminal.Padding{X: termPadX, Y: termPadY},
	})
	if err != nil {
		st.Close()
		t.err = err.Error()
		return
	}
	st.OnResize = func(cols, rows int) { a.onMain(func() { term.Resize(cols, rows) }) }
	t.term, t.stream = term, st
}

// closeGOATab closes the view alone: the session runs on.
func (a *App) closeGOATab(t *goaTab) { a.closeCenter(a.goa.board.Session, centerGOA+t.key) }

// closeGOATabOf closes the tab of a session gone from its host.
func (a *App) closeGOATabOf(id string) {
	for _, t := range slices.Clone(a.goa.tabs) {
		if t.id == id {
			a.closeGOATab(t)
		}
	}
}

// pruneGOATabs closes the tabs of sessions gone from another host, whose
// removal reaches this app only as that host's list without them. A host that
// lists nothing may just not have said yet.
func (a *App) pruneGOATabs() {
	if len(a.goa.tabs) == 0 {
		return
	}
	for _, h := range a.goaEveryHost() {
		if len(h.sessions) == 0 {
			continue
		}
		for _, t := range slices.Clone(a.goa.tabs) {
			if t.host == h.name && !slices.ContainsFunc(h.sessions, func(s proto.SessionInfo) bool { return s.ID == t.id }) {
				a.closeGOATab(t)
			}
		}
	}
}

// dropGOATab lets go of the tab of key, once it left the strip.
func (a *App) dropGOATab(key string) {
	i := slices.IndexFunc(a.goa.tabs, func(t *goaTab) bool { return t.key == key })
	if i < 0 {
		return
	}
	t := a.goa.tabs[i]
	t.closed = true
	if t.term != nil {
		t.term.Close()
	}
	if t.stream != nil {
		t.stream.Close()
	}
	a.goa.tabs = slices.Delete(slices.Clone(a.goa.tabs), i, i+1)
}

func (a *App) closeGOATabs() {
	for _, t := range slices.Clone(a.goa.tabs) {
		a.closeGOATab(t)
	}
}

// goaShownTab is the session tab in front of GOA, nil for none.
func (a *App) goaShownTab() *goaTab {
	if a.page != "" || a.center == nil {
		return nil
	}
	tabs := a.center[a.goa.board.Session]
	if tabs == nil {
		return nil
	}
	key, ok := strings.CutPrefix(tabs.front, centerGOA)
	if !ok {
		return nil
	}
	return a.goaTabOf(key)
}

// goaTabOf is the open tab of key, nil for none.
func (a *App) goaTabOf(key string) *goaTab {
	i := slices.IndexFunc(a.goa.tabs, func(t *goaTab) bool { return t.key == key })
	if i < 0 {
		return nil
	}
	return a.goa.tabs[i]
}

// goaTabSession is what the host last said of tab t's session.
func (a *App) goaTabSession(t *goaTab) (proto.SessionInfo, bool) {
	for _, h := range a.goaEveryHost() {
		if h.name != t.host {
			continue
		}
		if i := slices.IndexFunc(h.sessions, func(s proto.SessionInfo) bool { return s.ID == t.id }); i >= 0 {
			return h.sessions[i], true
		}
	}
	return proto.SessionInfo{}, false
}

// goaTabButton is a session's tab in GOA's strip: another host's says whose it
// is, and its dot shows only while the session is listed.
func (a *App) goaTabButton(c *ui.Context, g proto.SessionInfo, key string, front bool) {
	t := a.goaTabOf(key)
	if t == nil {
		return
	}
	name, dot, hollow := t.id, ui.Color{}, false
	if s, ok := a.goaTabSession(t); ok {
		st := statusOf(s)
		name, dot, hollow = a.sessionName(s), st.color, st.hollow
	}
	tip := name + " on " + t.host
	if t.host != a.host.Name {
		name += " · " + t.host
	}
	a.tabFace(c.Key("g:"+key), name, tip, front, true, dot, hollow, icon.Agent(t.agent),
		func() { a.centerOf(g.ID).front = centerGOA + key },
		func() { a.closeGOATab(t) })
}

// goaTabView is a tab's session, live.
func (a *App) goaTabView(c *ui.Context, key string) {
	t := a.goaTabOf(key)
	if t == nil {
		return
	}
	ui.Column(c).Grow(1).FillWidth().MinHeight(0).Padding(theme.S3, theme.S5, theme.S4, theme.S5).Gap(theme.S3).Children(func() {
		ui.Row(c).FillWidth().Shrink(0).PaddingX(theme.S4).Children(func() {
			widget.Running(c, "Live, on "+t.host+". Closing this tab closes the view alone; the session runs on.", widget.RunStyle{Color: theme.TextSoft})
		})
		switch {
		case t.err != "":
			widget.Notice(c, icon.Alert, theme.Error, t.err, "", nil)
		case t.term == nil:
			ui.Column(c).Grow(1).FillWidth().Center().Children(func() {
				ui.Text(c, "Connecting…").TextColor(theme.TextSoft)
			})
		default:
			terminal.View(c.Key("goa-term:"+key), t.term).Grow(1).FillWidth().Background(theme.Background)
		}
	})
}

package ui

import (
	"slices"
	"strconv"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// A session looked at from GOD of Agents' mode, on any host the app reaches:
// a tab with a live terminal of its own connection, GOA and its conversation
// staying where they are. A tab is a view: closing it detaches, and the
// session runs on. The main thread owns these.
type goaTab struct {
	key      string // host and session
	host, id string
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
	a.page, a.goa.shown = "", key
	if i := slices.IndexFunc(a.goa.tabs, func(t *goaTab) bool { return t.key == key }); i >= 0 && a.goa.tabs[i].err == "" {
		return
	} else if i >= 0 {
		a.closeGOATab(a.goa.tabs[i])
		a.goa.shown = key
	}
	t := &goaTab{key: key, host: host, id: s.ID, cl: cl}
	a.goa.tabs = append(a.goa.tabs, t)
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
func (a *App) closeGOATab(t *goaTab) {
	t.closed = true
	if t.term != nil {
		t.term.Close()
	}
	if t.stream != nil {
		t.stream.Close()
	}
	a.goa.tabs = slices.DeleteFunc(a.goa.tabs, func(x *goaTab) bool { return x == t })
	if a.goa.shown == t.key {
		a.goa.shown = ""
	}
}

func (a *App) closeGOATabs() {
	for _, t := range slices.Clone(a.goa.tabs) {
		a.closeGOATab(t)
	}
}

func (a *App) goaShownTab() *goaTab {
	if a.goa.shown == "" || a.page != "" {
		return nil
	}
	i := slices.IndexFunc(a.goa.tabs, func(t *goaTab) bool { return t.key == a.goa.shown })
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

// goaTabs is the row of open tabs, under GOA's strip.
func (a *App) goaTabs(c *ui.Context) {
	if len(a.goa.tabs) == 0 {
		return
	}
	shown := a.goaShownTab()
	ui.Row(c.Key("goa-tabs")).FillWidth().Shrink(0).Wrap().PaddingX(14).Padding(6, 14).Gap(4).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(theme.Border).Background(theme.Panel).Children(func() {
		for i, t := range a.goa.tabs {
			s, known := a.goaTabSession(t)
			name := t.id
			if known {
				name = a.sessionName(s)
			}
			ui.Row(c.Key("goa-tab:" + strconv.Itoa(i) + t.key)).Gap(2).AlignItems(ui.Center).Radius(theme.RadiusControl).
				Background(goaTabColor(t == shown)).Children(func() {
				b := ui.ButtonBase(c).Height(26).PaddingX(9).Gap(6).AlignItems(ui.Center).Radius(theme.RadiusControl).
					Cursor(ui.CursorPointer).Label(name + " on " + t.host).MaxWidth(240)
				b.Children(func() {
					if known {
						st := statusOf(s)
						widget.Dot(c, st.color, st.hollow, 7)
					}
					ui.Text(c, name).FontSize(theme.TextLabel).SingleLine().Shrink(1)
					if t.host != a.host.Name {
						ui.Text(c, t.host).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine().Shrink(2)
					}
				})
				if b.Clicked() {
					a.page, a.goa.shown = "", t.key
				}
				if widget.IconButton(c, icon.Close, "Close this tab: the session keeps running", false).Clicked() {
					a.closeGOATab(t)
				}
			})
		}
	})
}

func goaTabColor(shown bool) ui.Color {
	if shown {
		return theme.Hover
	}
	return ui.RGBA(0, 0, 0, 0)
}

// goaTabView is a tab's session, live.
func (a *App) goaTabView(c *ui.Context, t *goaTab) {
	name, state := t.id, ""
	if s, ok := a.goaTabSession(t); ok {
		name, state = a.sessionName(s), statusOf(s).label
	}
	a.viewHeader(c, func() {
		ui.Row(c).Grow(1).MinWidth(0).Gap(8).AlignItems(ui.Center).Children(func() {
			widget.Breadcrumb(c, t.host, name)
			if state != "" {
				ui.Text(c, state).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine()
			}
		})
	})
	switch {
	case t.err != "":
		widget.Notice(c, icon.Alert, theme.Error, t.err, "", nil)
	case t.term == nil:
		ui.Column(c).Grow(1).FillWidth().Center().Children(func() {
			ui.Text(c, "Connecting…").TextColor(theme.Muted)
		})
	default:
		terminal.View(c.Key("goa-term:"+t.key), t.term).Grow(1).FillWidth()
	}
}

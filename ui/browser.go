package ui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"sync"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/browserview"
)

// The Browser tool (see package browserview): the side panel lists a session's
// pages and its center tab shows the page, in one web view per session, made
// the first time it shows a page and kept while the session lasts, so switching
// tabs or sessions, or closing the tab, keeps the page.

// webViewAvailable reports, once, whether this device can show pages.
var webViewAvailable = sync.OnceValue(mygo.WebViewAvailable)

// webPage is a session's web view, as browserview uses it.
type webPage struct{ wv *mygo.WebView }

func (p webPage) Show(c *ui.Context) ui.Element { return ui.WebView(c, p.wv) }
func (p webPage) Load(url string)               { _ = p.wv.Page().LoadURL(url) }
func (p webPage) Reload()                       { p.wv.Page().Reload() }
func (p webPage) Back()                         { p.wv.Page().GoBack() }
func (p webPage) Forward()                      { p.wv.Page().GoForward() }
func (p webPage) CanBack() bool                 { return p.wv.Page().CanGoBack() }
func (p webPage) CanForward() bool              { return p.wv.Page().CanGoForward() }
func (p webPage) URL() string                   { return p.wv.Page().URL() }
func (p webPage) Loading() bool                 { return p.wv.Page().IsLoading() }
func (p webPage) ToggleDevTools()               { p.wv.Page().ToggleDevTools() }
func (p webPage) Capture() ([]byte, error)      { return p.wv.CapturePage() }
func (p webPage) Close()                        { p.wv.Destroy() }

// Pick lets the user pick an element of the page (design mode).
func (p webPage) Pick(ctx context.Context) (*browserview.Picked, error) {
	raw, err := p.wv.Page().EvalContext(ctx, browserview.PickScript)
	if err != nil || raw == nil {
		return nil, err
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	var picked browserview.Picked
	return &picked, json.Unmarshal(data, &picked)
}

// Eval runs code in the page (enowx browser).
func (p webPage) Eval(ctx context.Context, code string) (any, error) {
	return p.wv.Page().EvalContext(ctx, code)
}

// StopPick ends picking.
func (p webPage) StopPick() { go func() { _, _ = p.wv.Page().Eval(browserview.StopPickScript) }() }

func (a *App) browserOf(id string) *browserview.View {
	if a.browsers == nil {
		a.browsers = map[string]*browserview.View{}
	}
	v := a.browsers[id]
	if v == nil {
		v = browserview.New()
		a.browsers[id] = v
	}
	return v
}

// browserOptions connect session s's browser to the app.
func (a *App) browserOptions(s proto.SessionInfo) browserview.Options {
	gen := a.gen
	last := a.state.Previews[s.ID]
	return browserview.Options{
		Client:        a.client,
		Session:       s,
		Unslop:        func() { a.browserUnslop(s) },
		Unslopping:    a.browserUnslopping[s.ID],
		UnslopErr:     a.browserUnslopErr[s.ID],
		UnslopDismiss: func() { delete(a.browserUnslopErr, s.ID) },
		UnslopRun:     a.browserUnslopRun(s.ID),
		UnslopCompare: func(run proto.CanvasUnslopRun) { a.compareBrowserUnslop(s.ID, run) },
		OpenSession:   a.openSessionByID,
		AgentName:     a.sessionAgentName,
		Update:        a.onMain,
		Supported:     webViewAvailable(),
		Wanted: func() bool {
			if gen != a.gen || a.browsers[s.ID] == nil {
				return false
			}
			if a.state.BrowserViews[s.ID] {
				return true
			}
			return a.selected == s.ID && (a.panelShows(panelBrowser) || a.centerShows(s.ID, centerBrowser))
		},
		OpenTab: func() { a.openCenter(s, centerBrowser) },
		Shown:   func() bool { return a.centerShows(s.ID, centerBrowser) },
		NewPage: func(url string, done func(browserview.Page, error)) {
			win := a.win
			if win == nil {
				done(nil, errors.New("there is no window to show pages in"))
				return
			}
			go func() {
				wv, err := win.NewWebView(mygo.WebViewOptions{URL: url, Page: mygo.PageOptions{DevTools: mygo.DevToolsEnabled}})
				a.onMain(func() {
					if err != nil {
						done(nil, err)
						return
					}
					done(webPage{wv}, nil)
				})
			}()
		},
		Origin: func(p proto.Port) (string, error) {
			if a.client != nil && !a.client.Remote() && a.capturesWeb(s.ID) {
				rev, err := a.client.TrafficReverse(a.topOf(s.ID), p.Port)
				if err != nil {
					return "", err
				}
				return "http://localhost:" + strconv.Itoa(rev.Port), nil
			}
			if a.client == nil || !a.client.Remote() {
				return "http://localhost:" + strconv.Itoa(p.Port), nil
			}
			a.forwardPort(s.ID, p)
			local, ok := a.forwardAddr(p.Port)
			if !ok {
				return "", fmt.Errorf("couldn't forward :%d from %s", p.Port, a.host.Name)
			}
			return "http://" + local, nil
		},
		OpenURL:  func(url string) { go func() { _ = mygo.Shell.OpenExternal(url) }() },
		Attach:   func(png []byte) { a.attachScreenshot(s, png) },
		ToCanvas: a.pageToCanvas,
		AttachElement: func(about string, png []byte) {
			a.attachElement(s, about, png)
		},
		Port: last.Port,
		Path: last.Path,
		Remember: func(port int, path string) {
			if a.state.Previews == nil {
				a.state.Previews = map[string]preview{}
			}
			a.state.Previews[s.ID] = preview{Port: port, Path: path}
		},
	}
}

// preview is where a session's browser was, remembered between launches.
type preview struct {
	Port int    `json:"port"`
	Path string `json:"path,omitempty"`
}

// browserPages draws the Browser tool for session s: its pages.
func (a *App) browserPages(c *ui.Context, s proto.SessionInfo) {
	a.browserOf(s.ID).DrawPages(c, a.browserOptions(s))
}

// browserTab draws session s's page, in a center tab or as its own view: the
// one place its web view shows.
func (a *App) browserTab(c *ui.Context, s proto.SessionInfo) {
	a.browserOf(s.ID).Draw(c, a.browserOptions(s))
}

// browserTabBehind opens session s's page in a tab that stays behind the one in
// front: a page opened by itself doesn't take the window.
func (a *App) browserTabBehind(s proto.SessionInfo) {
	s = a.parentOf(s)
	t := a.centerOf(s.ID)
	if !slices.Contains(t.tabs, centerBrowser) {
		t.tabs = append(t.tabs, centerBrowser)
	}
}

// previewPort shows port p of session s in its browser.
func (a *App) previewPort(s proto.SessionInfo, p proto.Port) {
	a.selectSession(s)
	if !a.state.BrowserViews[s.ID] {
		a.openCenter(s, centerBrowser)
	}
	a.browserOf(s.ID).Show(a.browserOptions(s), p)
}

// attachElement pastes what an element picked in the page is into session
// s's prompt, with the path of its picture, for the user to say what to
// change; nothing is sent yet.
func (a *App) attachElement(s proto.SessionInfo, about string, png []byte) {
	path, err := saveImage(png, "element")
	if err != nil {
		a.status = err.Error()
		return
	}
	a.giveFiles(s, []string{path}, func(paths string) {
		a.typeIntoSession(s.ID, "\x1b[200~"+about+paths+"\n\x1b[201~")
	})
}

// attachScreenshot types the path of a page's screenshot into session s's
// prompt, so its agent can look at it.
func (a *App) attachScreenshot(s proto.SessionInfo, png []byte) {
	path, err := saveImage(png, "preview")
	if err != nil {
		a.status = err.Error()
		return
	}
	a.giveFiles(s, []string{path}, func(paths string) { a.typeIntoSession(s.ID, paths) })
}

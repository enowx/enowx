package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/trafficview"
)

// The side panel's Traffic tool shows the connected host's interceptor
// (ui/trafficview): one view per connection, fed by the daemon's events.

// traffic is the Traffic tool's view for the connection gen.
func (a *App) trafficOf() *trafficview.View {
	if a.trafficView == nil || a.trafficGen != a.gen {
		a.trafficView, a.trafficGen = trafficview.New(), a.gen
	}
	return a.trafficView
}

func (a *App) trafficTab(c *ui.Context, s proto.SessionInfo) {
	a.trafficOf().Draw(c, a.trafficOptions(s))
}

// exchangeTab draws a request of the traffic, in the center; what is its tab's
// ID without the kind: the exchange's ID, then its name.
func (a *App) exchangeTab(c *ui.Context, s proto.SessionInfo, what string) {
	if id, ok := exchangeID(what); ok {
		a.trafficOf().DrawExchange(c, a.trafficOptions(s), id)
	}
}

// exchangeID is the exchange a tab shows, from what its ID says after the kind.
func exchangeID(what string) (int64, bool) {
	id, _, _ := strings.Cut(what, " ")
	n, err := strconv.ParseInt(id, 10, 64)
	return n, err == nil
}

// trafficOptions connect the Traffic tool to the app, for session s.
func (a *App) trafficOptions(s proto.SessionInfo) trafficview.Options {
	top := a.parentOf(s)
	var tabs []proto.SessionInfo
	if top.ID != "" {
		tabs = append([]proto.SessionInfo{top}, a.tabsOf(top.ID)...)
	}
	return trafficview.Options{
		Host:     a.client,
		HostName: a.host.Name,
		Session:  s,
		Tabs:     tabs,
		Update:   a.onMain,
		CopyText: func(text string) { mygo.Clipboard.WriteText(text) },
		SendToAgent: func(text string) {
			a.typeIntoSession(top.ID, string(pasteStart)+text+string(pasteEnd))
		},
		SaveFile:     saveDownload,
		PickHostFile: a.pickHostFile,
		Android:      a.androidTargets(s),
		AndroidNote:  a.androidOf().Notice(),
		CatchAndroid: func(key string, on bool) {
			a.androidOf().Catch(a.androidOptions(s), key, on)
		},
		BrowserCaptured:   a.capturesWeb(top.ID),
		SetBrowserCapture: func(on bool) { a.setWebCapture(top.ID, on) },
		OpenTab: func(id string) {
			if t, ok := a.sessionByID(id); ok {
				a.selectSession(t)
			}
		},
		OpenInBrowser: a.trafficOpenInBrowser(top),
		Confirm:       a.confirm,
		OpenExchange: func(id int64, name string) {
			a.openCenter(s, centerExchange+strconv.FormatInt(id, 10)+" "+name)
		},
		Current: func() int64 {
			if what, ok := strings.CutPrefix(a.centerFront(s), centerExchange); ok {
				id, _ := exchangeID(what)
				return id
			}
			return 0
		},
		ToolShown: a.panelShows(panelTraffic),
	}
}

// saveDownload keeps data in the Downloads folder, named name (made unique),
// and returns where.
func saveDownload(name string, data []byte) (string, error) {
	path, err := downloadPath(name)
	if err != nil {
		return "", err
	}
	return path, os.WriteFile(path, data, 0o600)
}

// downloadPath is a path for name in Downloads, not taken yet.
func downloadPath(name string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, "Downloads")
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		dir = home
	}
	path := filepath.Join(dir, name)
	for i := 2; ; i++ {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return path, nil
		}
		ext := filepath.Ext(name)
		path = filepath.Join(dir, name[:len(name)-len(ext)]+"-"+strconv.Itoa(i)+ext)
	}
}

// trafficEvent applies the interceptor's events to the tool's view.
func (a *App) trafficEvent(ev proto.Event) func() {
	switch ev.Name {
	case proto.EventTrafficExchange:
		var s proto.TrafficSummary
		if json.Unmarshal(ev.Data, &s) != nil {
			return nil
		}
		return func() {
			v := a.trafficOf()
			v.Apply(s)
			a.notifyTrafficError(v, s)
		}
	case proto.EventTrafficPaused:
		var ps []proto.TrafficPaused
		if json.Unmarshal(ev.Data, &ps) != nil {
			return nil
		}
		return func() {
			v := a.trafficOf()
			before := v.PausedCount()
			v.SetPaused(ps)
			if len(ps) > before {
				a.notify("A request is paused at a breakpoint: Traffic shows it.")
			}
		}
	case proto.EventTrafficCapture:
		var st proto.TrafficCapture
		if json.Unmarshal(ev.Data, &st) != nil {
			return nil
		}
		return func() {
			v := a.trafficOf()
			if st.State == proto.CapturePaused && st.Reason != "" && v.Capture().State != proto.CapturePaused {
				a.notify("Traffic " + st.Reason + ".")
			}
			v.SetCapture(st)
		}
	case proto.EventTrafficAttached:
		var ts []proto.TrafficTarget
		if json.Unmarshal(ev.Data, &ts) != nil {
			return nil
		}
		return func() { a.trafficOf().SetTargets(ts) }
	}
	return nil
}

// capturesWeb reports whether session's Browser tool previews go through
// the interceptor; on the main thread.
func (a *App) capturesWeb(session string) bool {
	on, _ := a.webCapture.Load(a.topOf(session))
	b, _ := on.(bool)
	return b
}

// topOf is the session a tab belongs to (itself for a session), from the
// session list: on the main thread.
func (a *App) topOf(id string) string {
	for _, s := range a.sessions {
		if s.ID == id && s.Parent != "" {
			return s.Parent
		}
	}
	return id
}

// notifyTrafficError says when an app's request failed or was answered
// 5xx, if asked to in Traffic's settings, at most every few seconds.
func (a *App) notifyTrafficError(v *trafficview.View, s proto.TrafficSummary) {
	if !v.NotifiesErrors() || s.Kind == proto.TrafficTunnel || s.Source == "compose" {
		return
	}
	failed := s.State == proto.TrafficFailed && s.Error != ""
	if !failed && !(s.State == proto.TrafficDone && s.Status >= 500) {
		return
	}
	if time.Since(a.trafficNotified) < 5*time.Second {
		return
	}
	a.trafficNotified = time.Now()
	what := strconv.Itoa(s.Status)
	if failed {
		what = s.Error
	}
	a.notify(s.Method + " " + s.Host + s.Path + ": " + what + " (Traffic)")
}

// setWebCapture sends session top's previews and forwarded ports through
// the interceptor, or not. Forwards made before are made again so.
func (a *App) setWebCapture(top string, on bool) {
	a.webCapture.Store(top, on)
	a.stopForwards(top)
	for _, t := range a.tabsOf(top) {
		a.stopForwards(t.ID)
	}
}

// androidTargets are the devices Traffic can catch the traffic of.
func (a *App) androidTargets(s proto.SessionInfo) []trafficview.AndroidDevice {
	var out []trafficview.AndroidDevice
	for _, d := range a.androidOf().Devices(a.androidOptions(s)) {
		out = append(out, trafficview.AndroidDevice{Key: d.Key, Label: d.Label, Caught: d.Caught})
	}
	return out
}

// trafficOpenInBrowser opens a URL in session s's Browser tool; nil where
// there is no web view to show it.
func (a *App) trafficOpenInBrowser(s proto.SessionInfo) func(string) {
	if !webViewAvailable() {
		return nil
	}
	return func(url string) {
		if err := a.browserOf(s.ID).Open(a.browserOptions(s), url); err != nil {
			a.status = err.Error()
			return
		}
		a.openCenter(s, centerBrowser)
	}
}

// Package trafficview is the side panel's Traffic tool: the HTTP traffic
// the connected host's interceptor caught, its rules, what it is attached
// to, and requests composed by hand (docs/traffic.md).
package trafficview

import (
	"fmt"
	"slices"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// Host is what the tool asks of the host's daemon; a *client.Client.
type Host interface {
	TrafficStatus() (proto.TrafficStatus, error)
	TrafficCA() (proto.TrafficCA, error)
	TrafficAttach(proto.TrafficAttachParams) (proto.TrafficTarget, error)
	TrafficDetach(source string) error
	TrafficAttached() ([]proto.TrafficTarget, error)
	TrafficList(proto.TrafficListParams) (proto.TrafficList, error)
	TrafficGet(id int64, reveal bool) (proto.TrafficExchange, error)
	TrafficBody(proto.TrafficBodyParams) (proto.TrafficBody, error)
	TrafficMessages(proto.TrafficMessagesParams) ([]proto.TrafficMessage, error)
	TrafficPin(ids []int64, pinned bool) error
	TrafficClear(proto.TrafficClearParams) error
	TrafficRules(project string) (proto.TrafficRules, error)
	SetTrafficRules(proto.TrafficRules) (proto.TrafficRules, error)
	TrafficPaused() ([]proto.TrafficPaused, error)
	TrafficResume(proto.TrafficResumeParams) error
	TrafficSend(proto.TrafficSendParams) (proto.TrafficSummary, error)
	TrafficExport(proto.TrafficExportParams) ([]byte, error)
	TrafficImport(har []byte) (proto.TrafficList, error)
	TrafficMCP() (proto.TrafficMCP, error)
	TrafficNewCA() (proto.TrafficCA, error)
	TrafficSettings() (proto.TrafficSettings, error)
	SetTrafficSettings(proto.TrafficSettings) (proto.TrafficSettings, error)
	TrafficCapture() (proto.TrafficCapture, error)
	SetTrafficCapture(proto.TrafficCaptureParams) (proto.TrafficCapture, error)
	TrafficCapturePrefs() (proto.TrafficCapturePrefs, error)
	SetTrafficCapturePrefs(proto.TrafficCapturePrefs) (proto.TrafficCapturePrefs, error)
}

var _ Host = (*client.Client)(nil)

// Options connect the tool to the app.
type Options struct {
	Host     Host
	HostName string
	Session  proto.SessionInfo // the session selected: its traffic, its tabs
	Tabs     []proto.SessionInfo
	// Update runs a function on the main thread.
	Update func(func())
	// CopyText puts text on the clipboard; SendToAgent pastes it into the
	// session's prompt; SaveFile keeps data on this device and says where.
	CopyText    func(string)
	SendToAgent func(string)
	SaveFile    func(name string, data []byte) (string, error)
	// OpenTab shows a tab made for a target.
	OpenTab func(id string)
	// OpenInBrowser opens a URL in the session's Browser tool; nil without
	// one.
	OpenInBrowser func(url string)
	// Confirm does do once the user agrees (title, what is lost, the
	// action); nil does it at once.
	Confirm func(title, msg, action string, do func())
	// Android are the devices whose traffic can be caught; CatchAndroid
	// catches one's, or stops.
	Android      []AndroidDevice
	CatchAndroid func(key string, on bool)
	AndroidNote  string // how catching one went, and what is left to do
	// PickHostFile asks for a file of the host, for an Electron app to
	// start; nil leaves its path to type.
	PickHostFile func(title, about string, pick func(path string))
	// PickFile asks for a file of this device, for importing HAR.
	PickFile func(title string, pick func(path string))
	// BrowserCaptured says the session's Browser tool previews go through
	// the interceptor; SetBrowserCapture turns it on or off.
	BrowserCaptured   bool
	SetBrowserCapture func(on bool)
}

// AndroidDevice is a device to catch the traffic of.
type AndroidDevice struct {
	Key, Label string
	Caught     bool
}

// Pages of the tool.
const (
	pageRequests  = "Requests"
	pageRules     = "Rules"
	pageIntercept = "Intercept"
)

var pages = []string{pageRequests, pageRules, pageIntercept}

// View is the tool for one connection to a host. It belongs to the main
// thread; what it asks the host reports through Options.Update.
type View struct {
	list    []proto.TrafficSummary
	index   map[int64]int
	paused  []proto.TrafficPaused
	targets []proto.TrafficTarget
	status  proto.TrafficStatus
	loaded  bool
	loading bool
	err     string
	notice  string

	page string
	all  bool // every source's traffic, not only the session's

	// The filter, typed; applied is the one the host answered for, whose
	// matches are matched (nil without a filter).
	filter    string
	applied   string
	matched   map[int64]bool
	filterErr string
	querying  bool
	queryAt   time.Time
	poke      func() // draws again, for a filter due to be asked again
	poking    bool

	listState ui.ListState
	domains   widget.ContextMenu
	selected  int
	shown     []int // indexes into list the list shows
	rows      []int // what the list draws: shown's indexes, gaps below zero (gapRows)
	detail    *detail
	menu      widget.ContextMenu

	rules    rulesState
	settings settingsState
	compose  composeForm
	// pausedEdit is the paused exchange being changed, nil for none.
	pausedEdit *pausedEdit
	inter      interceptState
	capture    captureState
}

// New returns a tool with nothing loaded.
func New() *View {
	v := &View{index: map[int64]int{}, page: pageRequests, selected: -1}
	v.listState.FollowEnd = true
	v.listState.Key = func(row int) any {
		if row < len(v.rows) {
			if r := v.rows[row]; r >= 0 && r < len(v.shown) {
				return v.list[v.shown[r]].ID
			}
			return fmt.Sprint("gap", v.rows[row])
		}
		return row
	}
	return v
}

// Apply records an exchange told by an event.
func (v *View) Apply(s proto.TrafficSummary) {
	if s.State == "cleared" {
		v.list, v.index, v.detail, v.selected = nil, map[int64]int{}, nil, -1
		v.reload()
		return
	}
	if i, ok := v.index[s.ID]; ok {
		v.list[i] = s
	} else {
		v.index[s.ID] = len(v.list)
		v.list = append(v.list, s)
	}
	if v.detail != nil && v.detail.id == s.ID && (s.State == proto.TrafficDone || s.State == proto.TrafficFailed) && v.detail.state != s.State {
		v.detail.stale = true
	}
	if v.applied != "" && v.queryAt.IsZero() {
		v.queryAt = time.Now() // asked again soon: the new exchange may match
		if v.poke != nil && !v.poking {
			v.poking = true
			poke := v.poke
			time.AfterFunc(filterAgain+20*time.Millisecond, func() { poke() })
		}
	}
}

// SetPaused records the exchanges waiting at breakpoints.
func (v *View) SetPaused(ps []proto.TrafficPaused) { v.paused = ps }

// SetTargets records the targets attached.
func (v *View) SetTargets(ts []proto.TrafficTarget) { v.targets = ts }

// PausedCount is how many exchanges wait at breakpoints.
func (v *View) PausedCount() int { return len(v.paused) }

func (v *View) reload() { v.loaded, v.loading = false, false }

// load reads what the host has, once.
func (v *View) load(o Options) {
	if v.loaded || v.loading || o.Host == nil {
		return
	}
	v.loading = true
	h := o.Host
	go func() {
		st, err := h.TrafficStatus()
		var all []proto.TrafficSummary
		var after int64
		for err == nil {
			var l proto.TrafficList
			if l, err = h.TrafficList(proto.TrafficListParams{After: after}); err != nil {
				break
			}
			all = append(all, l.Exchanges...)
			if !l.More || len(l.Exchanges) == 0 {
				break
			}
			after = l.Exchanges[len(l.Exchanges)-1].ID
		}
		var paused []proto.TrafficPaused
		var targets []proto.TrafficTarget
		var settings proto.TrafficSettings
		settingsErr := err
		if err == nil {
			paused, _ = h.TrafficPaused()
			targets, _ = h.TrafficAttached()
			settings, settingsErr = h.TrafficSettings()
		}
		o.Update(func() {
			v.loading, v.loaded = false, true
			if err != nil {
				v.err = client.Explain(err)
				return
			}
			if st.Error != "" {
				v.err = st.Error
				return
			}
			v.err, v.status, v.paused, v.targets = "", st, paused, targets
			if settingsErr == nil {
				v.settings.set(settings)
				v.settings.loaded = true
			}
			v.list, v.index = nil, map[int64]int{}
			for _, s := range all {
				v.Apply(s)
			}
		})
	}()
}

// Draw shows the tool for o's session.
func (v *View) Draw(c *ui.Context, o Options) {
	v.load(o)
	ui.Column(c).Grow(1).FillWidth().MinHeight(0).Children(func() {
		ui.Row(c).Height(42).Shrink(0).FillWidth().Padding(0, 10, 0, 12).Gap(8).
			BorderWidth(0, 0, 1, 0).BorderColor(theme.Border).Children(func() {
			cur := slices.Index(pages, v.page)
			if i := widget.Segmented(c, pages, cur, nil); i >= 0 {
				v.page = pages[i]
			}
			ui.Spacer(c)
			v.captureBar(c, o)
			if n := len(v.paused); n > 0 {
				b := widget.Button(c, widget.Secondary, icon.Stop, plural(n, "paused"), "Exchanges waiting at a breakpoint")
				b.TextColor(theme.Warning)
				if b.Clicked() {
					v.page = pageRequests
				}
			}
			if widget.IconButton(c, icon.Send, "Send a request", v.compose.open).Clicked() {
				v.compose.open = true
			}
			if widget.IconButton(c, icon.Settings, "Traffic settings", v.settings.open).Clicked() {
				v.settings.open = true
			}
		})
		if v.notice != "" {
			ui.Row(c).FillWidth().Padding(6, 12).Gap(8).Background(theme.Sunken).Children(func() {
				ui.Text(c, v.notice).FontSize(theme.TextLabel).TextColor(theme.TextSoft).Grow(1).Shrink(1)
				if widget.IconButton(c, icon.Close, "Dismiss", false).Clicked() {
					v.notice = ""
				}
			})
		}
		switch {
		case v.err != "":
			note(c, v.err, theme.Error)
		case !v.loaded:
			note(c, "Starting the interceptor…", theme.Muted)
		case v.page == pageRules:
			v.rulesPage(c, o)
		case v.page == pageIntercept:
			v.interceptPage(c, o)
		default:
			v.requestsPage(c, o)
		}
		v.ruleSheet(c, o)
		v.composeSheet(c, o)
		v.settingsSheet(c, o)
		v.pausedSheet(c, o)
		v.interceptSheets(c, o)
	})
}

func note(c *ui.Context, text string, col ui.Color) {
	ui.Column(c).Grow(1).FillWidth().Center().Padding(24).Children(func() {
		ui.Text(c, text).FontSize(theme.TextUI).TextColor(col).TextAlign(ui.Center)
	})
}

// run runs fn off the main thread, then done with its error on it.
func run(o Options, fn func() error, done func(error)) {
	go func() {
		err := fn()
		o.Update(func() { done(err) })
	}()
}

func plural(n int, what string) string {
	if n == 1 {
		return "1 " + what
	}
	return itoa(n) + " " + what
}

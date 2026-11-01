// Package browserview is the Browser tool: in the side panel the pages a
// session has, and in a center tab what its dev server serves, in a web view
// of this device, with back, forward, reload, the address, a pick of the
// session's ports, DevTools, and a screenshot to hand the agent. On a server, the app forwards the port
// here first, so only HTTP and WebSocket traffic (hot reload) crosses the
// connection. See docs/plans/preview.md.
package browserview

import (
	"context"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/unslopview"
	"github.com/enowx/enowx/ui/widget"
)

// refreshEvery is how often a shown view asks for the session's ports.
const refreshEvery = 2 * time.Second

// Page is the web view a View shows. The app makes it, once per session,
// and the view only places it: one the view doesn't place keeps its page.
type Page interface {
	// Show places the page in the view, as ui.WebView does.
	Show(c *ui.Context) ui.Element
	Load(url string)
	Reload()
	Back()
	Forward()
	CanBack() bool
	CanForward() bool
	URL() string
	Loading() bool
	ToggleDevTools()
	// Capture is a PNG of the page.
	Capture() ([]byte, error)
	Close()
}

// Options connect a view to the app.
type Options struct {
	Client  *client.Client
	Session proto.SessionInfo
	// Update runs a function on the main thread; Wanted, called there,
	// reports whether the view still shows, else its poller stops.
	Update func(func())
	Wanted func() bool
	// OpenTab brings the page's center tab to the front; Shown reports whether
	// it is there.
	OpenTab func()
	Shown   func() bool
	// Supported says this device has a web view; without, the view offers
	// the browser.
	Supported bool
	// NewPage makes a page loading url and hands it to done on the main
	// thread, or why it couldn't.
	NewPage func(url string, done func(Page, error))
	// Origin is where port p of the session's host shows here, as
	// "http://localhost:5173": forwarded first for a server.
	Origin func(p proto.Port) (string, error)
	// OpenURL opens a URL in the browser; Attach hands a screenshot (a
	// PNG) to the session's agent.
	OpenURL func(url string)
	Attach  func(png []byte)
	// ToCanvas puts a screenshot of the page (a PNG, its URL, the page's
	// width in CSS pixels, 0 unknown) on a canvas as a design, to compare
	// designs with it; nil hides it. It is called off the main thread.
	ToCanvas func(png []byte, url string, width int)
	// AttachElement hands the agent an element picked in the page (design
	// mode): what it is, and a PNG of it. Nil hides picking.
	AttachElement func(about string, png []byte)
	// Unslop has the session's agent unslop the page shown, in the code;
	// nil hides it, Unslopping says one is starting. UnslopRun is the run
	// under way, shown over the page; AgentName names a session's agent.
	Unslop     func()
	Unslopping bool
	UnslopRun  *proto.CanvasUnslopRun
	AgentName  func(session string) string
	// UnslopErr is why the last unslop didn't start, shown over the page
	// until UnslopDismiss.
	UnslopErr     string
	UnslopDismiss func()
	// OpenSession shows a session (an unslop's reviewer).
	OpenSession func(id string)
	// UnslopCompare opens a done run's before and after.
	UnslopCompare func(run proto.CanvasUnslopRun)
	// Port and Path are where the view starts, as last left; Remember
	// keeps where it is.
	Port     int
	Path     string
	Remember func(port int, path string)
}

// View is a session's browser. It belongs to the main thread; its poller
// reports through Options.Update.
type View struct {
	// An unslop's panel, the read last marked and the page's loading as
	// last seen (unslop.go).
	panel      unslopview.Panel
	scanned    string
	wasLoading bool
	sweeping   string // the page swept as an unslop starts

	ports   []proto.Port
	loaded  bool
	err     string
	polling bool
	poke    chan struct{}

	port    int    // the port shown, 0 before one is picked
	origin  string // where it shows here
	page    Page
	making  bool   // a page is on the way
	stopped bool   // the port closed; reload when it is back
	address string // the address field, being edited
	editing bool
	going   string // an address loading, shown until the page has it
	// external: the page shows another site than the session's port, which
	// shows whatever the port does.
	external bool
	// home: the page shows HomePage, opened as no dev server ran yet, so
	// the first that starts takes its place.
	home    bool
	homed   bool // HomePage was opened: it isn't again
	started bool // Options.Port and Path were taken
	width   int  // the page's width, as a device's; 0 fits the room
	sizes   widget.ContextMenu
	portsAt widget.ContextMenu
	more    widget.ContextMenu
	picking context.CancelFunc // stops picking an element; nil when not
}

// widths are the page widths offered, as devices': 0 fits the room.
var widths = []struct {
	width int
	name  string
}{{0, "Fill the tab"}, {1280, "Desktop, 1280"}, {768, "Tablet, 768"}, {390, "Phone, 390"}}

// New returns a view with nothing open.
func New() *View { return &View{poke: make(chan struct{}, 1)} }

// Close closes the view's page: its session went.
func (v *View) Close() {
	if v.page != nil {
		v.page.Close()
		v.page = nil
	}
}

func (v *View) follow(o Options) {
	if v.polling || o.Client == nil {
		return
	}
	v.polling = true
	cl, id := o.Client, o.Session.ID
	go func() {
		for {
			got, err := cl.Ports(id)
			keep := make(chan bool, 1)
			o.Update(func() {
				if !o.Wanted() {
					v.polling = false
					keep <- false
					return
				}
				keep <- true
				v.loaded = true
				if err != nil {
					v.err = client.Explain(err)
					return
				}
				v.err = ""
				v.setPorts(o, got.Ports)
			})
			if !<-keep {
				return
			}
			select {
			case <-time.After(refreshEvery):
			case <-v.poke:
			}
		}
	}()
}

// setPorts takes the session's ports in: picks one when none is, and
// notices the one shown stop and come back.
func (v *View) setPorts(o Options, ports []proto.Port) {
	v.ports = ports
	open := v.port != 0 && slices.ContainsFunc(ports, func(p proto.Port) bool { return p.Port == v.port })
	switch {
	case v.port == 0 && len(ports) > 0 && v.external && !v.home && v.page != nil:
		// The user went elsewhere: the dev server waits in the picker.
		if origin, err := o.Origin(ports[0]); err == nil {
			v.port, v.origin = ports[0].Port, origin
		}
	case v.port == 0 && len(ports) > 0:
		p, path := ports[0], "/"
		if !v.started {
			v.started = true
			if i := slices.IndexFunc(ports, func(p proto.Port) bool { return p.Port == o.Port }); i >= 0 {
				p, path = ports[i], o.Path
			}
		}
		v.show(o, p, path)
	case v.port != 0 && !open:
		v.stopped = true
	case v.stopped && open:
		v.stopped = false
		if v.page != nil {
			v.page.Reload()
		}
	}
}

// show shows port p at path.
func (v *View) show(o Options, p proto.Port, path string) {
	origin, err := o.Origin(p)
	if err != nil {
		v.err = err.Error()
		return
	}
	v.port, v.origin, v.stopped, v.err, v.external, v.home = p.Port, origin, false, "", false, false
	target := origin + cleanPath(path)
	v.going = target
	if v.page != nil {
		v.page.Load(target)
		return
	}
	if v.making || o.NewPage == nil || !o.Supported {
		return
	}
	v.making = true
	o.NewPage(target, func(page Page, err error) {
		v.making = false
		if err != nil {
			v.err = err.Error()
			return
		}
		v.page = page
	})
}

// shown is the address the field shows: the page's, or the one it is on
// its way to.
func (v *View) shown() string {
	if v.page == nil {
		return v.going
	}
	u := v.page.URL()
	if v.going != "" && (u == "" || u == "about:blank" || v.page.Loading()) {
		return v.going
	}
	v.going = ""
	return u
}

// target is the URL what the address field says names: a path of the port
// shown ("/settings"), a URL, or a host ("google.com", "localhost:3000"),
// which gets https, or http on this machine. "" when it names none.
func (v *View) target(text string) string {
	text = strings.TrimSpace(text)
	switch {
	case text == "" || strings.ContainsAny(text, " \t"):
		return ""
	case strings.HasPrefix(text, "/"):
		if v.origin == "" {
			return ""
		}
		return v.origin + text
	case strings.Contains(text, "://"):
		if u, err := url.Parse(text); err == nil && u.Host != "" {
			return text
		}
		return ""
	}
	host := strings.ToLower(text)
	if i := strings.IndexAny(host, ":/?#"); i >= 0 {
		host = host[:i]
	}
	if host == "localhost" || strings.HasPrefix(host, "127.") || strings.HasPrefix(text, "[::1]") {
		return "http://" + text
	}
	if !strings.Contains(host, ".") {
		return ""
	}
	return "https://" + text
}

// navigate loads what the address field says, opening a page for it when
// there is none yet; a page of the session's port is remembered.
func (v *View) navigate(o Options) {
	to := v.target(v.address)
	if to == "" {
		v.err = "“" + strings.TrimSpace(v.address) + "” isn't an address: type a path of the port, a host or a URL."
		return
	}
	v.err, v.going, v.home = "", to, false
	v.external = v.origin == "" || !strings.HasPrefix(to, v.origin)
	switch {
	case v.page != nil:
		v.page.Load(to)
	case !v.making && o.NewPage != nil && o.Supported:
		v.making = true
		o.NewPage(to, func(page Page, err error) {
			v.making = false
			if err != nil {
				v.err = err.Error()
				return
			}
			v.page = page
		})
	}
	if !v.external && o.Remember != nil {
		o.Remember(v.port, strings.TrimPrefix(to, v.origin))
	}
}

// HomePage is what the Browser shows before the session runs a dev server.
const HomePage = "https://enowx.ai"

// openHome shows HomePage, once, while the session has no dev server.
func (v *View) openHome(o Options) {
	if v.homed || v.page != nil || v.making || o.NewPage == nil || !o.Supported {
		return
	}
	v.homed, v.home, v.external, v.going = true, true, true, HomePage
	v.making = true
	o.NewPage(HomePage, func(page Page, err error) {
		v.making = false
		if err != nil {
			v.err = err.Error()
			return
		}
		v.page = page
	})
}

func cleanPath(p string) string {
	if p == "" || p[0] != '/' {
		return "/" + p
	}
	return p
}

func portLabel(p int) string { return ":" + strconv.Itoa(p) }

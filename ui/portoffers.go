package ui

import (
	"encoding/json"
	"slices"
	"strconv"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/appwindow"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// When a session opens a port (a dev server an agent started), the app
// offers to open it: in the browser on this machine, forwarded here first
// for a server. The daemon tells (proto.EventPortsChanged); an offer goes
// when the user takes or dismisses it, or the port closes.

// maxPortOffers is how many offers show at once; older ones give way.
const maxPortOffers = 3

// portOffer is a port a session just opened.
type portOffer struct {
	session string
	port    proto.Port
}

// portsEvent reads a PortsChange event into what to apply on the main
// thread.
func (a *App) portsEvent(data json.RawMessage) func() {
	var ch proto.PortsChange
	if json.Unmarshal(data, &ch) != nil {
		return nil
	}
	return func() { a.portsChanged(ch) }
}

// portsChanged offers the ports a session opened and drops the offers of
// those it closed.
func (a *App) portsChanged(ch proto.PortsChange) {
	a.portOffers = slices.DeleteFunc(a.portOffers, func(o portOffer) bool {
		return o.session == ch.Session && !slices.ContainsFunc(ch.Ports, func(p proto.Port) bool { return p.Port == o.port.Port })
	})
	for _, p := range ch.Opened {
		if slices.ContainsFunc(a.portOffers, func(o portOffer) bool { return o.port.Port == p.Port }) {
			continue
		}
		// A display shows its app in a window of its own.
		if i := slices.IndexFunc(a.sessions, func(s proto.SessionInfo) bool { return s.ID == ch.Session }); i >= 0 &&
			p.Process == "display" && appwindow.Available() {
			a.openAppWindow(a.sessions[i], p, false)
			continue
		}
		// A port always previewed opens by itself in the session shown.
		if i := slices.IndexFunc(a.sessions, func(s proto.SessionInfo) bool { return s.ID == ch.Session }); i >= 0 &&
			ch.Session == a.selected && webViewAvailable() && a.alwaysPreviews(a.sessions[i], p.Port) {
			s := a.sessions[i]
			if !a.state.BrowserViews[s.ID] {
				a.showBrowserSoon()
			}
			a.browserOf(s.ID).Show(a.browserOptions(s), p)
			continue
		}
		if !a.state.QuietPorts {
			a.portOffers = append(a.portOffers, portOffer{session: ch.Session, port: p})
		}
	}
	if n := len(a.portOffers); n > maxPortOffers {
		a.portOffers = a.portOffers[n-maxPortOffers:]
	}
}

// previewKey names a project's ports always previewed.
func (a *App) previewKey(s proto.SessionInfo) string { return a.host.ID + " " + s.Project }

// alwaysPreviews reports whether port of s's project opens in the Browser
// by itself.
func (a *App) alwaysPreviews(s proto.SessionInfo, port int) bool {
	return s.Project != "" && slices.Contains(a.state.AutoPreview[a.previewKey(s)], port)
}

// setAlwaysPreview makes port of s's project open in the Browser by
// itself, or not.
func (a *App) setAlwaysPreview(s proto.SessionInfo, port int, on bool) {
	if s.Project == "" {
		return
	}
	if a.state.AutoPreview == nil {
		a.state.AutoPreview = map[string][]int{}
	}
	key := a.previewKey(s)
	ports := slices.DeleteFunc(slices.Clone(a.state.AutoPreview[key]), func(p int) bool { return p == port })
	if on {
		ports = append(ports, port)
	}
	if len(ports) == 0 {
		delete(a.state.AutoPreview, key)
		return
	}
	a.state.AutoPreview[key] = ports
}

func (a *App) dropPortOffer(port int) {
	a.portOffers = slices.DeleteFunc(a.portOffers, func(o portOffer) bool { return o.port.Port == port })
}

// openPort opens port p of the connected host in the browser, forwarding
// a server's port here first.
func (a *App) openPort(session string, p proto.Port) {
	addr := "localhost:" + strconv.Itoa(p.Port)
	if a.client != nil && a.client.Remote() {
		a.forwardPort(session, p)
		local, ok := a.forwardAddr(p.Port)
		if !ok {
			return
		}
		addr = local
	}
	url := "http://" + addr
	go func() { _ = mygo.Shell.OpenExternal(url) }()
}

// portOfferCards shows the offers, in the window's corner (cornerCards).
func (a *App) portOfferCards(c *ui.Context) {
	if len(a.portOffers) == 0 || a.client == nil {
		return
	}
	remote := a.client.Remote()
	for _, o := range a.portOffers {
		a.portOfferCard(c.Key(strconv.Itoa(o.port.Port)), o, remote)
	}
}

func (a *App) portOfferCard(c *ui.Context, o portOffer, remote bool) {
	name := o.session
	if i := slices.IndexFunc(a.sessions, func(s proto.SessionInfo) bool { return s.ID == o.session }); i >= 0 {
		name = a.sessionName(a.sessions[i])
	}
	widget.Float(ui.Column(c).FillWidth().Padding(12, 12, 12, 14).Gap(10).Radius(theme.RadiusCard).
		Background(theme.Dialog).Border(1, theme.BorderStrong)).Transition(widget.OverlayMotion).Children(func() {
		ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Start).Children(func() {
			ui.Icon(c, icon.Globe).FontSize(14).TextColor(theme.Accent).Margin(2, 0, 0, 0)
			ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
				ui.Text(c, o.port.Process+" is listening on :"+strconv.Itoa(o.port.Port)).FontSize(theme.TextUI).SingleLine().Shrink(1)
				ui.Text(c, "in "+name).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine().Shrink(1)
			})
			if widget.IconButton(c, icon.Close, "Dismiss", false).Clicked() {
				a.dropPortOffer(o.port.Port)
			}
		})
		if i := slices.IndexFunc(a.sessions, func(s proto.SessionInfo) bool { return s.ID == o.session }); i >= 0 &&
			a.sessions[i].Project != "" && webViewAvailable() {
			s := a.sessions[i]
			always := a.alwaysPreviews(s, o.port.Port)
			if widget.Checkbox(c, &always, "Always preview :"+strconv.Itoa(o.port.Port)+" in this project") {
				a.setAlwaysPreview(s, o.port.Port, always)
			}
		}
		ui.Row(c).FillWidth().Gap(8).Justify(ui.End).Children(func() {
			if widget.GhostButton(c, "Ports").Clicked() {
				if i := slices.IndexFunc(a.sessions, func(s proto.SessionInfo) bool { return s.ID == o.session }); i >= 0 {
					a.selectSession(a.sessions[i])
				}
				a.openPanel(panelPorts)
				a.dropPortOffer(o.port.Port)
			}
			label := "Open"
			if remote {
				label = "Forward & open"
			}
			kind := widget.Primary
			if webViewAvailable() {
				kind = widget.Secondary
			}
			if widget.Button(c, kind, icon.External, label, "").Clicked() {
				a.openPort(o.session, o.port)
				a.dropPortOffer(o.port.Port)
			}
			if webViewAvailable() && widget.Button(c, widget.Primary, icon.Browser, "Preview", "").Clicked() {
				if i := slices.IndexFunc(a.sessions, func(s proto.SessionInfo) bool { return s.ID == o.session }); i >= 0 {
					a.previewPort(a.sessions[i], o.port)
				}
				a.dropPortOffer(o.port.Port)
			}
		})
	})
}

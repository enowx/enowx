// Package portview is the side panel's Ports tool: the TCP ports a
// session's processes listen on, such as a dev server an agent started,
// with a way to open each in the browser. A port on a server is forwarded
// to this machine first, through the SSH connection to it.
package portview

import (
	"fmt"
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// refreshEvery is how often a shown view asks for the ports again.
const refreshEvery = 3 * time.Second

// Options connect a view to the app, which keeps the forwards: they go on
// while the view doesn't show.
type Options struct {
	Client  *client.Client
	Session proto.SessionInfo
	Host    string // the host's name
	// Update runs a function on the main thread; Wanted, called there,
	// reports whether the view still shows, else its poller stops.
	Update func(func())
	Wanted func() bool
	// Forwarded returns the local address a port of the host is forwarded
	// to, if it is; Forward starts forwarding it, and Unforward stops.
	Forwarded func(port int) (string, bool)
	Forward   func(p proto.Port)
	Unforward func(port int)
	OpenURL   func(url string)
	// Preview shows a port in the Browser tool; nil without a web view.
	Preview func(p proto.Port)
	// OpenWindow shows the app on a display port in a window of its own;
	// nil where it can't.
	OpenWindow func(p proto.Port)
	// Captured says the session's ports open here go through Traffic;
	// Capture turns it on or off (nil without Traffic).
	Captured bool
	Capture  func(on bool)
	// Offers says a card offers each port a session opens; SetOffers
	// turns them on or off, for every session (nil hides the switch).
	Offers    bool
	SetOffers func(on bool)
}

// View is one session's ports. It belongs to the main thread; its poller
// reports through Options.Update.
type View struct {
	ports   proto.SessionPorts
	loaded  bool
	err     string
	polling bool
	refresh chan struct{}
}

// New returns a view with nothing read.
func New() *View { return &View{refresh: make(chan struct{}, 1)} }

func (v *View) follow(o Options) {
	if v.polling || o.Client == nil {
		return
	}
	v.polling = true
	cl, id := o.Client, o.Session.ID
	go func() {
		for {
			ports, err := cl.Ports(id)
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
				v.err, v.ports = "", ports
			})
			if !<-keep {
				return
			}
			select {
			case <-time.After(refreshEvery):
			case <-v.refresh:
			}
		}
	}()
}

// Draw shows the ports, each with what opens it, and how they behave.
func (v *View) Draw(c *ui.Context, o Options) {
	v.follow(o)
	ui.Column(c).Grow(1).FillWidth().MinHeight(0).Padding(0, theme.S3, theme.S3, theme.S3).Gap(theme.S2).Children(func() {
		widget.ToolHeader(c, func() {
			where := "Listening on this machine"
			if o.Client != nil && o.Client.Remote() {
				where = "Listening on " + o.Host
			}
			ui.Row(c).Grow(1).MinWidth(0).Children(func() {
				widget.Running(c, where, widget.RunStyle{Weight: 500})
			})
			if widget.IconButton(c, icon.Sync, "Look again", false).Clicked() {
				select {
				case v.refresh <- struct{}{}:
				default:
				}
			}
		})
		switch {
		case v.err != "":
			widget.Notice(c, nil, theme.Del, v.err, "", nil)
			ui.Spacer(c)
		case !v.loaded:
			note(c, "Looking for ports…")
		case !v.ports.Supported:
			note(c, "This host can't tell which ports a session listens on.")
		case len(v.ports.Ports) == 0:
			note(c, "No ports open. A dev server the session starts shows here.")
		default:
			ui.Scroll(c).Grow(1).FillWidth().MinHeight(0).Children(func() {
				ui.Column(c).FillWidth().Gap(2).Children(func() {
					for _, p := range v.ports.Ports {
						v.portRow(c.Key(strconv.Itoa(p.Port)+"/"+strconv.Itoa(p.PID)), o, p)
					}
				})
			})
		}
		if o.Capture != nil {
			on := o.Captured
			if widget.ToggleRow(c, "Through Traffic", "What opens or previews these ports is recorded in Traffic.", &on) {
				o.Capture(on)
			}
		}
		if o.SetOffers != nil {
			on := o.Offers
			if widget.ToggleRow(c, "Pop up when a session opens a port", "", &on) {
				o.SetOffers(on)
			}
		}
	})
}

func note(c *ui.Context, text string) {
	ui.Column(c).Grow(1).FillWidth().Center().Padding(theme.S5).Children(func() {
		ui.Text(c, text).FontSize(theme.TextUI).TextColor(theme.TextSoft).TextAlign(ui.Center)
	})
}

// portRow is a port: the number, who listens and who can reach it, which run
// when too long, then what opens it.
func (v *View) portRow(c *ui.Context, o Options, p proto.Port) {
	remote := o.Client != nil && o.Client.Remote()
	local, forwarded := "", false
	if remote {
		local, forwarded = o.Forwarded(p.Port)
	}
	ui.Row(c).FillWidth().MinHeight(36).Padding(theme.S1, theme.S1, theme.S1, theme.S3).Gap(theme.S1).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			widget.Running(c, ":"+strconv.Itoa(p.Port), widget.RunStyle{Mono: true})
			who := fmt.Sprintf("%s · pid %d", p.Process, p.PID)
			if forwarded {
				who += " · forwarded to " + local
			}
			ui.Row(c).FillWidth().Children(func() {
				widget.Running(c.Key("who"), who+" · ", widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
				if loopback(p.Addr) {
					ui.Text(c, "only this host").FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine().Shrink(0)
				} else {
					ui.Text(c, "every address").FontSize(theme.TextLabel).TextColor(theme.Warning).SingleLine().Shrink(0).
						Tooltip("Other machines that reach this host can reach this port")
				}
			})
		})
		if o.OpenWindow != nil && p.Process == "display" && widget.IconButton(c, icon.AppWindow, "Open the app in a window", false).Clicked() {
			o.OpenWindow(p)
		}
		if o.Preview != nil && widget.IconButton(c, icon.Globe, "Preview", false).Clicked() {
			o.Preview(p)
		}
		switch {
		case !remote:
			if widget.Button(c, widget.Secondary, nil, "Open", "").Clicked() {
				o.OpenURL("http://localhost:" + strconv.Itoa(p.Port))
			}
		case forwarded:
			if widget.IconButton(c, icon.Stop, "Stop forwarding", false).Clicked() {
				o.Unforward(p.Port)
			}
			if widget.Button(c, widget.Secondary, nil, "Open", "").Clicked() {
				o.OpenURL("http://" + local)
			}
		default:
			if widget.Button(c, widget.Secondary, nil, "Forward", "").Clicked() {
				o.Forward(p)
			}
		}
	})
}

// loopback reports whether only the host itself reaches an address.
func loopback(addr string) bool {
	return addr == "::1" || addr == "localhost" || len(addr) > 4 && addr[:4] == "127."
}

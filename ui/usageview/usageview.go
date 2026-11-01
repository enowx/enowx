// Package usageview shows what enowX costs: the app on this device, and the
// daemon and sessions of the connected host.
package usageview

import (
	"fmt"
	"os"
	"slices"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/procstat"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

const every = 2 * time.Second

// Meter measures and shows usage. It belongs to the main thread; the
// poller started by Follow fills it.
type Meter struct {
	// Every, when set, says how long to wait before measuring again:
	// measuring runs ps on both ends, worth doing often only while the
	// numbers are looked at. Called on the main thread.
	Every func() time.Duration

	host    proto.DaemonStats
	app     procstat.Usage
	sampler procstat.Sampler // this app; used by the poller only
	poke    chan struct{}
	open    bool // the chip's popover
}

// Poke measures again now, as the numbers came into view.
func (u *Meter) Poke() {
	if u.poke == nil {
		return
	}
	select {
	case u.poke <- struct{}{}:
	default:
	}
}

// Host is what the views need of the connected host.
type Host struct {
	Name      string
	Local     bool   // the host is this device
	LocalName string // this device's name
	// Session labels a session ID: its name, agent and project.
	Session func(id string) (name, agent, project string)
}

// Follow measures cl's host while wanted (called on the main thread)
// reports true and cl is open. update runs a function on the main thread.
func (u *Meter) Follow(cl *client.Client, update func(func()), wanted func() bool) {
	u.host = proto.DaemonStats{}
	u.poke = make(chan struct{}, 1)
	poke := u.poke
	go func() {
		for {
			st, err := cl.Stats()
			var app procstat.Usage
			if u.sampler.Sample() == nil {
				app = u.sampler.One(os.Getpid())
			}
			stop := make(chan bool, 1)
			wait := every
			update(func() {
				if !wanted() {
					stop <- true
					return
				}
				if u.Every != nil {
					wait = u.Every()
				}
				stop <- false
				if err == nil {
					u.host = st
				}
				u.app = app
			})
			if <-stop {
				return
			}
			select {
			case <-cl.Done():
				return
			case <-poke:
			case <-time.After(wait):
			}
		}
	}()
}

// total is what enowX uses on the connected host: its daemon and sessions,
// and the app itself when that host is this device.
func (u *Meter) total(local bool) proto.Usage {
	t := u.host.Daemon
	for _, s := range u.host.Sessions {
		t.RSS, t.CPU, t.Procs = t.RSS+s.RSS, t.CPU+s.CPU, t.Procs+s.Procs
	}
	if local {
		t.RSS, t.CPU, t.Procs = t.RSS+u.app.RSS, t.CPU+u.app.CPU, t.Procs+1
	}
	return t
}

// Measured reports whether there is anything to show yet.
func (u *Meter) Measured() bool { return u.host.Supported || u.app.RSS != 0 }

// Total is what enowX uses on the connected host; local says whether that
// host is this device, so the app counts too.
func (u *Meter) Total(local bool) proto.Usage { return u.total(local) }

// Host is the connected host's own memory and CPU, if it reports them.
func (u *Meter) Host() (proto.HostUsage, bool) { return u.host.Host, u.host.Supported }

// Session is what one session uses, if it was measured.
func (u *Meter) Session(id string) (proto.Usage, bool) {
	for _, s := range u.host.Sessions {
		if s.ID == id {
			return s.Usage, true
		}
	}
	return proto.Usage{}, false
}

// Details draws the usage page's body: the host's memory and CPU, then
// enowX and every session, biggest first.
func (u *Meter) Details(c *ui.Context, h Host) {
	ui.Column(c).FillWidth().Gap(18).Children(func() {
		if u.host.Supported {
			hostMeters(c, u.host.Host, u.total(h.Local).RSS)
		} else {
			ui.Text(c, h.Name+" doesn't report its usage.").FontSize(theme.TextLabel).TextColor(theme.Muted)
		}
		ui.Column(c).FillWidth().Gap(2).Children(func() {
			header(c)
			appDetail := ""
			if !h.Local {
				appDetail = "on " + h.LocalName
			}
			if u.host.Supported {
				sessions := slices.Clone(u.host.Sessions)
				slices.SortFunc(sessions, func(x, y proto.SessionUsage) int {
					switch {
					case x.RSS > y.RSS:
						return -1
					case x.RSS < y.RSS:
						return 1
					}
					return 0
				})
				for _, su := range sessions {
					name, agentID, project := h.Session(su.ID)
					row(c.Key(su.ID), icon.Agent(agentID), name, project, su.Usage)
				}
				if len(sessions) == 0 {
					ui.Text(c, "No session is running.").FontSize(theme.TextLabel).TextColor(theme.Muted).Padding(8, 10)
				}
				row(c.Key("daemon"), icon.Server, "Daemon", "on "+h.Name, u.host.Daemon)
			}
			row(c.Key("app"), icon.Laptop, "enowX app", appDetail, proto.Usage{RSS: u.app.RSS, CPU: u.app.CPU, Procs: 1})
			widget.Divider(c)
			cells(c, func() { ui.Text(c, "enowX on "+h.Name).FontSize(theme.TextLabel).SingleLine() }, u.total(h.Local), true)
		})
		ui.Text(c, "Each session counts everything it started: language servers, MCP servers, dev servers. Updates every 2 seconds.").
			FontSize(11).TextColor(theme.Muted)
	})
}

// hostMeters is the host's memory, split into enowX, other apps and free,
// beside its CPU.
func hostMeters(c *ui.Context, h proto.HostUsage, enowx uint64) {
	tracks := []ui.Track{ui.Fr(2), ui.Fr(1)}
	if h.DiskTotal > 0 {
		tracks = append(tracks, ui.Fr(1))
	}
	ui.Grid(c).FillWidth().ColumnTracks(tracks...).Gap(12).Children(func() {
		used := ratio(float64(h.MemUsed), float64(h.MemTotal))
		card(c, "Memory", fmt.Sprintf("%s of %s", textfmt.Bytes(h.MemUsed), textfmt.Bytes(h.MemTotal)), func() {
			ours := min(float64(enowx), float64(h.MemUsed))
			share := ratio(ours, float64(h.MemTotal))
			widget.Meter(c, widget.Segment{Fraction: share, Color: theme.Accent}, widget.Segment{Fraction: used - share, Color: theme.Idle})
			ui.Row(c).FillWidth().Gap(14).Children(func() {
				widget.Legend(c, theme.Accent, "enowX "+textfmt.Bytes(uint64(ours)))
				widget.Legend(c, theme.Idle, "Other apps "+textfmt.Bytes(h.MemUsed-uint64(ours)))
				widget.Legend(c, theme.BorderStrong, "Free "+textfmt.Bytes(h.MemTotal-min(h.MemUsed, h.MemTotal)))
				ui.Spacer(c)
				if used > 0.85 {
					ui.Text(c, "Memory is getting tight").FontSize(theme.TextLabel).TextColor(theme.Warning).SingleLine()
				}
			})
		})
		card(c, "CPU", fmt.Sprintf("%s of %d cores", textfmt.CPU(h.CPU), h.CPUs), func() {
			widget.Meter(c, widget.Segment{Fraction: h.CPU / 100, Color: widget.Level(h.CPU / 100)})
		})
		if h.DiskTotal > 0 {
			disk := ratio(float64(h.DiskUsed), float64(h.DiskTotal))
			card(c, "Disk", fmt.Sprintf("%s of %s", textfmt.Bytes(h.DiskUsed), textfmt.Bytes(h.DiskTotal)), func() {
				widget.Meter(c, widget.Segment{Fraction: disk, Color: widget.Level(disk)})
				if disk > 0.9 {
					ui.Text(c, "Nearly full").FontSize(theme.TextLabel).TextColor(theme.Warning)
				}
			})
		}
	})
}

func card(c *ui.Context, title, value string, body func()) {
	widget.Card(c).Children(func() {
		ui.Row(c).FillWidth().Gap(8).Children(func() {
			ui.Text(c, title).FontSize(theme.TextLabel).TextColor(theme.TextSoft).Grow(1).Shrink(1).SingleLine()
			ui.Text(c, value).FontSize(theme.TextUI).SingleLine().Shrink(0)
		})
		body()
	})
}

func header(c *ui.Context) {
	ui.Row(c).FillWidth().Padding(0, 10, 4, 10).Gap(10).Children(func() {
		ui.Text(c, "").Grow(1)
		for _, h := range []string{"Memory", "CPU", "Processes"} {
			ui.Text(c, h).FontSize(theme.TextLabel).TextColor(theme.Muted).Width(80).TextAlign(ui.End)
		}
	})
}

func row(c *ui.Context, svg *ui.SVG, name, detail string, u proto.Usage) {
	cells(c, func() {
		ui.Icon(c, svg).FontSize(13).TextColor(theme.TextSoft)
		ui.Text(c, name).FontSize(theme.TextLabel).SingleLine().Shrink(1)
		if detail != "" {
			ui.Text(c, detail).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine().Shrink(1)
		}
	}, u, false)
}

func cells(c *ui.Context, label func(), u proto.Usage, total bool) {
	ui.Row(c).FillWidth().Padding(8, 10).Gap(10).Radius(theme.RadiusControl).Children(func() {
		ui.Row(c).Grow(1).MinWidth(0).Gap(8).Children(label)
		for _, v := range []string{textfmt.Bytes(u.RSS), textfmt.CPU(u.CPU), fmt.Sprint(u.Procs)} {
			t := ui.Text(c, v).FontSize(theme.TextLabel).Width(80).TextAlign(ui.End).SingleLine().TextColor(theme.TextSoft)
			if total {
				t.TextColor(theme.Text)
			}
		}
	})
}

func ratio(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return a / b
}

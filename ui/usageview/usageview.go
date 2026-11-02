// Package usageview shows what enowX costs: the app on this device, and the
// daemon and sessions of the connected host.
package usageview

import (
	"cmp"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/procstat"
	"github.com/enowx/enowx/proto"
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
	held    bool // Show's numbers stand
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
	// AgentName names an agent preset ID.
	AgentName func(id string) string
	// Working counts the agents at work now.
	Working int
	// Others are the other hosts the app watches.
	Others []OtherHost
	// Select shows a session.
	Select func(id string)
}

// OtherHost is another host the app watches, as it last told.
type OtherHost struct {
	Name      string
	Connected bool
	Sessions  int
	Usage     proto.HostUsage
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
				if u.held {
					return
				}
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

// Figures are the host's numbers over the page: what the machine has in use,
// and the sessions on it.
func (u *Meter) Figures(c *ui.Context, h Host) {
	var figs []widget.Fig
	tones := map[int]ui.Color{}
	if u.host.Supported {
		m := u.host.Host
		figs = append(figs, widget.Fig{Label: "Memory", Value: amountOf(m.MemUsed, m.MemTotal)})
		figs = append(figs, widget.Fig{Label: "CPU", Value: fmt.Sprintf("%.1f of %d cores", m.CPU/100*float64(m.CPUs), m.CPUs)})
		if m.DiskTotal > 0 {
			label := "Disk"
			if ratio(float64(m.DiskUsed), float64(m.DiskTotal)) > 0.9 {
				label = "Disk · Nearly full"
				tones[len(figs)] = theme.Warning
			}
			figs = append(figs, widget.Fig{Label: label, Value: amountOf(m.DiskUsed, m.DiskTotal)})
		}
	}
	figs = append(figs, widget.Fig{Label: "Sessions here", Value: strconv.Itoa(len(u.host.Sessions))}, widget.Fig{Label: "Agents at work", Value: strconv.Itoa(h.Working)})
	widget.FigsBandToned(c, figs, tones)
}

// Details draws the page under its figures: the host's memory, the app and
// daemon, the other hosts, then every session, biggest first.
func (u *Meter) Details(c *ui.Context, h Host) {
	body := ui.Scroll(c.Key("usage-body")).Grow(1).FillWidth().MinHeight(0)
	shown := body.Bounds().H
	body.Children(func() {
		column := ui.Column(c).FillWidth().Padding(theme.S4, theme.S5, theme.S5, theme.S5).Gap(theme.S4)
		column.Children(func() {
			if !u.host.Supported {
				ui.Text(c, h.Name+" doesn't report its usage.").FontSize(theme.TextUI).TextColor(theme.TextSoft)
			}
			columns := 1
			if u.host.Supported {
				columns = min(3, 2+len(h.Others))
			}
			ui.Grid(c).FillWidth().Columns(columns).Gap(theme.S3).Children(func() {
				if u.host.Supported {
					memoryCard(c, h, u.host.Host, u.total(h.Local).RSS)
				}
				u.daemonCard(c, h)
				if u.host.Supported {
					for _, o := range h.Others {
						otherHostCard(c.Key(o.Name), o)
					}
				}
			})
			if u.host.Supported {
				u.sessions(c, h, column, shown)
			}
		})
	})
}

// memoryCard is the host's memory, split into enowX, other apps and free.
func memoryCard(c *ui.Context, h Host, m proto.HostUsage, enowx uint64) {
	used := ratio(float64(m.MemUsed), float64(m.MemTotal))
	ours := min(float64(enowx), float64(m.MemUsed))
	share := ratio(ours, float64(m.MemTotal))
	widget.Card(c).Children(func() {
		widget.CardTitle(c, nil, "Memory on "+h.Name, nil)
		widget.Meter(c, widget.Segment{Fraction: share, Color: theme.Working}, widget.Segment{Fraction: used - share, Color: theme.Warning})
		widget.CardLines(c, func() {
			line(c.Key("ours"), true, "Sessions, app and daemon", textfmt.Bytes(uint64(ours)))
			line(c.Key("others"), false, "Other apps", textfmt.Bytes(m.MemUsed-uint64(ours)))
			line(c.Key("free"), false, "Free", textfmt.Bytes(m.MemTotal-min(m.MemUsed, m.MemTotal)))
		})
		if used > 0.85 {
			widget.CardNote(c, "Memory is getting tight", 0).TextColor(theme.Warning)
		}
	})
}

// daemonCard is what enowX itself takes: the app, on this device, and the daemon.
func (u *Meter) daemonCard(c *ui.Context, h Host) {
	widget.Card(c).Children(func() {
		widget.CardTitle(c, nil, "Daemon and app", nil)
		widget.CardLines(c, func() {
			app := "enowX, the app"
			if !h.Local {
				app += " on " + h.LocalName
			}
			line(c.Key("app"), true, app, textfmt.Bytes(u.app.RSS)+" · "+textfmt.CPU(u.app.CPU))
			if u.host.Supported {
				line(c.Key("daemon"), false, "enowxd, the daemon", textfmt.Bytes(u.host.Daemon.RSS)+" · "+textfmt.CPU(u.host.Daemon.CPU))
			}
		})
		widget.CardNote(c, "Each session counts everything it started: language servers, MCP servers, dev servers. Updates every 2 seconds.", 0)
	})
}

// otherHostCard is what another host told last.
func otherHostCard(c *ui.Context, o OtherHost) {
	widget.Card(c).Children(func() {
		widget.CardTitle(c, nil, o.Name, nil)
		if !o.Connected {
			widget.StatusLine(c, theme.Idle, true, "Not connected")
			return
		}
		widget.StatusLine(c, theme.Done, false, "Connected")
		m := o.Usage
		widget.CardStats(c, []widget.Stat{
			{Value: strconv.Itoa(o.Sessions), Label: "sessions"},
			{Value: amountOf(m.MemUsed, m.MemTotal), Label: "memory"},
			{Value: fmt.Sprintf("%.1f of %d", m.CPU/100*float64(m.CPUs), m.CPUs), Label: "cores"},
		})
		used := ratio(float64(m.MemUsed), float64(m.MemTotal))
		widget.Meter(c, widget.Segment{Fraction: used, Color: widget.Level(used)})
	})
}

// sessions is a card per session, biggest first.
func (u *Meter) sessions(c *ui.Context, h Host, column ui.Element, shown float32) {
	list := slices.Clone(u.host.Sessions)
	slices.SortFunc(list, func(x, y proto.SessionUsage) int { return cmp.Compare(y.RSS, x.RSS) })
	ui.Column(c).FillWidth().Gap(theme.S2).Children(func() {
		ui.Text(c, "Sessions on "+h.Name).FontSize(theme.TextUI).FontWeight(500)
		if len(list) == 0 {
			ui.Text(c, "No session is running.").FontSize(theme.TextUI).TextColor(theme.TextSoft)
			return
		}
		grid := ui.Grid(c).FillWidth().Columns(sessionColumns).Gap(theme.S3)
		grid.RowTracks(widget.RowsLeft(grid, column, shown, (len(list)+sessionColumns-1)/sessionColumns, sessionCardHeight)...).Children(func() {
			for _, su := range list {
				sessionCard(c.Key(su.ID), h, su, list[0].RSS)
			}
		})
	})
}

// The grid of sessions: four a row, each at least as tall as what it holds.
const (
	sessionColumns    = 4
	sessionCardHeight = 168
)

// sessionCard is what one session uses, its meter against the biggest's.
func sessionCard(c *ui.Context, h Host, su proto.SessionUsage, biggest uint64) {
	name, agentID, project := h.Session(su.ID)
	agentName := "Shell"
	if agentID != "" && h.AgentName != nil {
		agentName = h.AgentName(agentID)
	}
	widget.GridCard(c).Children(func() {
		widget.CardTitle(c, nil, name, nil)
		widget.CardStats(c, []widget.Stat{
			{Value: textfmt.Bytes(su.RSS), Label: "memory"},
			{Value: textfmt.CPU(su.CPU), Label: "CPU"},
			{Value: agentName, Label: "agent"},
		})
		widget.Meter(c, widget.Segment{Fraction: ratio(float64(su.RSS), float64(biggest)), Color: theme.Working})
		ui.Spacer(c) // the foot stays at the card's bottom, however tall the row
		widget.CardFoot(c, func() {
			about := fmt.Sprintf("%d %s", su.Procs, textfmt.Plural(su.Procs, "process", "processes"))
			if project != "" {
				about += " · " + project
			}
			ui.Row(c).Grow(1).MinWidth(0).Children(func() {
				widget.Running(c, about, widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
			})
			if widget.GhostButton(c, "Open").Label("Open "+name).Clicked() && h.Select != nil {
				h.Select(su.ID)
			}
		})
	})
}

// line is a row of a card's list: what it is, and how much at the right.
func line(c *ui.Context, first bool, title, value string) {
	widget.CardLine(c, first, title, "", func() {
		ui.Text(c, value).FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine()
	})
}

// amountOf says used of total, the unit once when they share it: 14.2 of 24 GB.
func amountOf(used, total uint64) string {
	trim := func(s string) (amount, unit string) {
		amount, unit, _ = strings.Cut(s, " ")
		return strings.TrimSuffix(amount, ".0"), unit
	}
	u, uu := trim(textfmt.Bytes(used))
	t, tu := trim(textfmt.Bytes(total))
	if uu == tu {
		return u + " of " + t + " " + tu
	}
	return u + " " + uu + " of " + t + " " + tu
}

func ratio(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return a / b
}

// Show puts a measurement in and keeps it from being measured over, for a
// development build's sample data.
func (u *Meter) Show(st proto.DaemonStats, app procstat.Usage) { u.host, u.app, u.held = st, app, true }

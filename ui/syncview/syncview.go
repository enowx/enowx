// Package syncview is the Sync environment dialog: it copies agents'
// skills, subagents, commands, memory files, settings and MCP servers
// between this device and a server, item by item (see core/envsync).
package syncview

import (
	"slices"
	"strings"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/hosts"
	"github.com/enowx/enowx/hostconn"
	"github.com/enowx/enowx/proto"
)

// Options configures a dialog.
type Options struct {
	Host   hosts.Host     // the server
	Remote *client.Client // the app's connection to Host
	Local  hosts.Host     // this device
	// DaemonCommand starts this device's daemon when none runs.
	DaemonCommand []string
	// Update runs a function on the main thread.
	Update func(func())
	// AgentName names agent IDs the dialog doesn't know.
	AgentName func(id string) string
	// Back leaves the page while a sync runs, which goes on.
	Back func()
}

// Dialog is one open Sync environment dialog. It belongs to the main
// thread; its goroutines report through Options.Update and drop their
// results once it is closed.
type Dialog struct {
	opts     Options
	local    *client.Client // this device's daemon, opened for the dialog
	toRemote bool           // the direction: this device → host
	closed   bool

	loading bool
	err     string
	scans   map[bool]proto.EnvScan // by toRemote: the source's scan is scans[true] for local
	rows    []row

	query     string          // filters rows by name, path or agent
	showSame  bool            // list items that are the same on both sides
	collapsed map[string]bool // by kind

	running  bool
	done     int
	total    int
	finished bool
	warnings []string
	failures []string
}

type row struct {
	item    proto.EnvItem
	status  string // statusNew, statusChanged or statusSame
	checked bool
}

const (
	statusNew     = "New"
	statusChanged = "Changed"
	statusSame    = "Same"
)

// Open opens a dialog and starts reading both sides.
func Open(opts Options) *Dialog {
	d := &Dialog{opts: opts, toRemote: true, loading: true}
	go d.load()
	return d
}

// Close closes the dialog and its connection to this device's daemon.
func (d *Dialog) Close() {
	if d.closed {
		return
	}
	d.closed = true
	if d.local != nil {
		d.local.Close()
	}
}

// Closed reports whether the dialog was closed.
func (d *Dialog) Closed() bool { return d.closed }

// load connects to this device's daemon and scans both homes.
func (d *Dialog) load() {
	update := d.opts.Update
	if d.local == nil {
		local, err := hostconn.Connect(d.opts.Local, hostconn.Options{DaemonCommand: d.opts.DaemonCommand})
		if err != nil {
			update(func() { d.loading, d.err = false, "Couldn't start this device's daemon: "+err.Error() })
			return
		}
		kept := make(chan bool, 1)
		update(func() {
			if d.closed {
				local.Close()
				kept <- false
				return
			}
			d.local = local
			kept <- true
		})
		if !<-kept {
			return
		}
	}
	localScan, lerr := d.local.EnvScan()
	remoteScan, rerr := d.opts.Remote.EnvScan()
	update(func() {
		if d.closed {
			return
		}
		d.loading = false
		switch {
		case lerr != nil:
			d.err = "Couldn't read this device: " + lerr.Error()
		case rerr != nil:
			d.err = "Couldn't read " + d.opts.Host.Name + ": " + rerr.Error()
		default:
			d.scans = map[bool]proto.EnvScan{true: localScan, false: remoteScan}
			d.buildRows()
		}
	})
}

// buildRows compares the source's items with the destination's.
func (d *Dialog) buildRows() {
	src, dst := d.scans[d.toRemote], d.scans[!d.toRemote]
	have := map[string]string{}
	for _, it := range dst.Items {
		have[it.ID] = it.Hash
	}
	d.rows = d.rows[:0]
	for _, it := range src.Items {
		r := row{item: it, status: statusNew}
		if h, ok := have[it.ID]; ok {
			r.status = statusChanged
			if h == it.Hash {
				r.status = statusSame
			}
		}
		// Secrets go only when the user ticks them.
		r.checked = r.status != statusSame && len(it.Secrets) == 0 && len(it.Warnings) == 0
		d.rows = append(d.rows, r)
	}
	slices.SortStableFunc(d.rows, func(x, y row) int {
		if n := kindRank(x.item.Kind) - kindRank(y.item.Kind); n != 0 {
			return n
		}
		return agentRank(x.item.Agent) - agentRank(y.item.Agent)
	})
	if d.collapsed == nil {
		// Long lists start folded, so every kind shows at a glance.
		d.collapsed = map[string]bool{}
		for _, k := range kinds {
			d.collapsed[k] = len(d.visibleRows(k)) > 8
		}
	}
}

// kinds orders the dialog's sections.
var kinds = []string{proto.EnvSkill, proto.EnvMCP, proto.EnvSubagent, proto.EnvCommand, proto.EnvMemory, proto.EnvSettings, proto.EnvGit}

func kindRank(kind string) int {
	if i := slices.Index(kinds, kind); i >= 0 {
		return i
	}
	return len(kinds)
}

// visibleRows are the indexes of kind's rows that the filters let through.
func (d *Dialog) visibleRows(kind string) []int {
	q := strings.ToLower(strings.TrimSpace(d.query))
	var out []int
	for i, r := range d.rows {
		if r.item.Kind != kind || (r.status == statusSame && !d.showSame && !r.checked) {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(r.item.Name+" "+r.item.Detail+" "+r.item.Agent), q) {
			continue
		}
		out = append(out, i)
	}
	return out
}

// selectable: ticking a whole section never sends secrets; those are
// ticked one by one.
func selectable(r row) bool { return len(r.item.Secrets) == 0 && len(r.item.Warnings) == 0 }

var agentOrder = []string{"claude", "codex", "opencode", "gemini", "shared"}

func agentRank(id string) int {
	if i := slices.Index(agentOrder, id); i >= 0 {
		return i
	}
	return len(agentOrder)
}

func (d *Dialog) checkedRows() []row {
	var out []row
	for _, r := range d.rows {
		if r.checked {
			out = append(out, r)
		}
	}
	return out
}

// run copies the ticked items one by one, so a large skill needs one
// frame, not the whole environment, and one failure stops nothing else.
func (d *Dialog) run() {
	rows := d.checkedRows()
	src, dst := d.local, d.opts.Remote
	if !d.toRemote {
		src, dst = d.opts.Remote, d.local
	}
	dstName := d.destName()
	d.running, d.finished, d.done, d.total = true, false, 0, len(rows)
	d.warnings, d.failures = nil, nil
	update := d.opts.Update
	go func() {
		for _, r := range rows {
			p, err := src.EnvExport(r.item.ID)
			var res proto.EnvImportResult
			if err == nil {
				res, err = dst.EnvImport(p)
			}
			update(func() {
				if d.closed {
					return
				}
				d.done++
				if err != nil {
					d.failures = append(d.failures, itemTitle(r.item)+": "+strings.TrimPrefix(err.Error(), "envsync: "))
				}
				for _, w := range res.Warnings {
					d.warnings = append(d.warnings, strings.ReplaceAll(w, "here", "on "+dstName))
				}
			})
		}
		after, err := dst.EnvScan()
		update(func() {
			if d.closed {
				return
			}
			d.running, d.finished = false, true
			if err == nil {
				d.scans[!d.toRemote] = after
				d.buildRows()
			}
		})
	}()
}

func (d *Dialog) destName() string {
	if d.toRemote {
		return d.opts.Host.Name
	}
	return d.opts.Local.Name
}

func (d *Dialog) sourceName() string {
	if d.toRemote {
		return d.opts.Local.Name
	}
	return d.opts.Host.Name
}

// ShowSame lists what is the same on both sides too.
func (d *Dialog) ShowSame(on bool) { d.showSame = on }

// Loaded is a page with both sides already read: what it shows of a host and
// this device doesn't need the daemons to draw.
func Loaded(opts Options, local, remote proto.EnvScan) *Dialog {
	d := &Dialog{opts: opts, toRemote: true}
	d.scans = map[bool]proto.EnvScan{true: local, false: remote}
	d.buildRows()
	return d
}

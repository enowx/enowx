package trafficview

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// filterDelay is how long typing pauses before the filter is asked of the
// host; filterAgain how often it is asked again as exchanges come.
const (
	filterDelay = 300 * time.Millisecond
	filterAgain = time.Second
)

// requestsPage is the list of exchanges, the filter over it, the paused
// ones on top and the one picked below.
func (v *View) requestsPage(c *ui.Context, o Options) {
	v.poke = func() { o.Update(func() {}) }
	v.queryFilter(o)
	v.computeShown(o)
	ui.Column(c).Grow(1).FillWidth().MinHeight(0).Children(func() {
		v.filterBar(c, o)
		if len(v.paused) > 0 {
			v.pausedList(c, o)
		}
		if len(v.shown) == 0 {
			msg := "Nothing caught yet. Intercept attaches a terminal, a browser or the session; what they send shows here."
			if v.filter != "" {
				msg = "Nothing matches the filter."
			}
			if v.filterErr != "" {
				msg = v.filterErr
			}
			note(c, msg, theme.Muted)
			return
		}
		grow := float32(1)
		if v.detail != nil {
			grow = 0.8
		}
		ui.Column(c.Key("list")).Grow(grow).FillWidth().MinHeight(90).Children(func() {
			v.rows = v.gapRows()
			gaps := v.gaps()
			ui.List(c.Key("rows"), &v.listState, len(v.rows), func(i int) {
				r := v.rows[i]
				if r < 0 {
					gapRow(c, gaps[-1-r])
					return
				}
				s := v.list[v.shown[r]]
				if v.row(c, s, r == v.selected).Clicked() {
					v.open(o, s.ID)
				}
			}).Grow(1).FillWidth()
		})
		if v.detail != nil {
			v.detailView(c.Key("detail"), o)
		}
	})
}

// computeShown picks what the list shows: the session's exchanges, or
// all, those the filter matched.
func (v *View) computeShown(o Options) {
	v.shown = v.shown[:0]
	for i, s := range v.list {
		if !v.all && s.Session != o.Session.ID && s.Session != o.Session.Parent {
			continue
		}
		if v.matched != nil && !v.matched[s.ID] {
			continue
		}
		v.shown = append(v.shown, i)
	}
	if v.detail != nil {
		v.selected = -1
		for row, i := range v.shown {
			if v.list[i].ID == v.detail.id {
				v.selected = row
			}
		}
	}
}

// queryFilter asks the host which exchanges the filter matches: once
// typing pauses, and again as exchanges come.
func (v *View) queryFilter(o Options) {
	if v.querying || o.Host == nil {
		return
	}
	if strings.TrimSpace(v.filter) == "" {
		v.applied, v.matched, v.filterErr = "", nil, ""
		return
	}
	due := v.filter != v.applied && time.Since(v.queryAt) > filterDelay ||
		v.filter == v.applied && !v.queryAt.IsZero() && time.Since(v.queryAt) > filterAgain
	if !due {
		return
	}
	v.querying = true
	f, h := v.filter, o.Host
	go func() {
		ids := map[int64]bool{}
		var after int64
		var err error
		for {
			var l proto.TrafficList
			if l, err = h.TrafficList(proto.TrafficListParams{Filter: f, After: after}); err != nil {
				break
			}
			for _, s := range l.Exchanges {
				ids[s.ID] = true
			}
			if !l.More || len(l.Exchanges) == 0 {
				break
			}
			after = l.Exchanges[len(l.Exchanges)-1].ID
		}
		o.Update(func() {
			v.querying, v.queryAt, v.poking = false, time.Time{}, false
			v.applied = f
			if err != nil {
				v.filterErr, v.matched = client.Explain(err), map[int64]bool{}
				return
			}
			v.filterErr, v.matched = "", ids
		})
	}()
}

func (v *View) filterBar(c *ui.Context, o Options) {
	ui.Row(c).FillWidth().Shrink(0).Padding(8, 10, 8, 12).Gap(6).BorderWidth(0, 0, 1, 0).BorderColor(theme.Border).Children(func() {
		before := v.filter
		ui.Row(c).Grow(1).MinWidth(0).Children(func() {
			widget.SearchField(c, &v.filter, "domain:example.com status:4xx type:json …", icon.Search)
		})
		if v.filter != before {
			v.queryAt = time.Now()
			time.AfterFunc(filterDelay+20*time.Millisecond, func() { o.Update(func() {}) })
		}
		scope := 0
		if v.all {
			scope = 1
		}
		if i := widget.Segmented(c, []string{"Session", "All"}, scope, nil); i >= 0 {
			v.all = i == 1
		}
		v.domainMenu(c, o)
		v.rules.load(o)
		widget.MoreMenu(c, &v.menu, "requests", func(done func()) {
			f := strings.TrimSpace(v.filter)
			saved := slices.Contains(v.rules.filters, f)
			if f != "" && !saved && widget.MenuItemTip(c, icon.Plus, "Save filter", "Keep this filter with the project", false) {
				v.rules.saveAll(o, v.rules.list, append(slices.Clone(v.rules.filters), f), nil)
				done()
			}
			if f != "" && saved && widget.MenuItem(c, icon.Trash, "Forget this filter", false) {
				v.rules.saveAll(o, v.rules.list, slices.DeleteFunc(slices.Clone(v.rules.filters), func(x string) bool { return x == f }), nil)
				done()
			}
			for _, sf := range v.rules.filters {
				if widget.MenuItem(c, icon.Search, sf, false) {
					v.filter, v.queryAt = sf, time.Now()
					time.AfterFunc(filterDelay+20*time.Millisecond, func() { o.Update(func() {}) })
					done()
				}
			}
			if len(v.rules.filters) > 0 || f != "" {
				widget.MenuSeparator(c)
			}
			if widget.MenuItem(c, icon.ArrowUp, "Export as HAR…", false) {
				v.export(o, nil)
				done()
			}
			if o.PickFile != nil && widget.MenuItem(c, icon.FolderPlus, "Import HAR…", false) {
				done()
				o.PickFile("Import a HAR file", func(path string) { v.importHAR(o, path) })
			}
			if widget.MenuItem(c, icon.Trash, "Clear", true) {
				run(o, func() error { return o.Host.TrafficClear(proto.TrafficClearParams{}) }, func(err error) {
					if err != nil {
						v.notice = client.Explain(err)
					}
				})
				done()
			}
		})
	})
}

// export saves what the list shows as HAR on this device, or ids.
func (v *View) export(o Options, ids []int64) {
	p := proto.TrafficExportParams{IDs: ids}
	if ids == nil {
		p.Filter = v.applied
		if !v.all && o.Session.ID != "" {
			id := o.Session.ID
			if o.Session.Parent != "" {
				id = o.Session.Parent
			}
			p.Filter = strings.TrimSpace(p.Filter + " session:" + id)
		}
	}
	h := o.Host
	go func() {
		har, err := h.TrafficExport(p)
		path := ""
		if err == nil && o.SaveFile != nil {
			path, err = o.SaveFile("traffic-"+time.Now().Format("20060102-150405")+".har", har)
		}
		o.Update(func() {
			if err != nil {
				v.notice = "Couldn't export: " + client.Explain(err)
				return
			}
			v.notice = "Saved as " + path + ". Secrets are masked."
		})
	}()
}

// row is one exchange in the list, lit when picked.
func (v *View) row(c *ui.Context, s proto.TrafficSummary, picked bool) ui.Element {
	b := ui.ButtonBase(c).FillWidth().Height(30).Padding(0, 12).Gap(8).Justify(ui.Start).Label(s.Method + " " + s.URL)
	switch {
	case picked:
		b.Background(theme.Selected)
	case b.Hovered():
		b.Background(theme.Fade(theme.Selected, 0.5))
	}
	return b.Children(func() {
		status, col := statusText(s)
		ui.Text(c, status).Font(theme.MonoFont).FontSize(theme.TextLabel).TextColor(col).Width(42).Shrink(0).SingleLine()
		ui.Text(c, s.Method).Font(theme.MonoFont).FontSize(theme.TextLabel).TextColor(theme.TextSoft).Width(52).Shrink(0).SingleLine()
		ui.Text(c, hostPath(s)).FontSize(theme.TextLabel).SingleLine().Grow(1).Shrink(1).MinWidth(0).Tooltip(s.URL)
		if s.Mocked {
			widget.Tag(c, "mock", theme.Accent)
		} else if s.Rule != "" {
			widget.Tag(c, "rule", theme.Accent)
		}
		if s.Kind != proto.TrafficHTTP {
			widget.Tag(c, s.Kind, theme.Muted)
		}
		ui.Text(c, sizeTime(s)).FontSize(theme.TextLabel).TextColor(theme.Muted).Shrink(0).SingleLine()
	})
}

func statusText(s proto.TrafficSummary) (string, ui.Color) {
	switch s.State {
	case proto.TrafficFailed:
		return "ERR", theme.Error
	case proto.TrafficHeld:
		return "PAUSE", theme.Warning
	case proto.TrafficPending:
		return "…", theme.Muted
	}
	switch {
	case s.Status >= 500:
		return strconv.Itoa(s.Status), theme.Error
	case s.Status >= 400:
		return strconv.Itoa(s.Status), theme.Warning
	case s.Status >= 300 || s.Status == 101:
		return strconv.Itoa(s.Status), theme.Muted
	}
	return strconv.Itoa(s.Status), theme.Done
}

func sizeTime(s proto.TrafficSummary) string {
	if s.State == proto.TrafficPending || s.State == proto.TrafficHeld {
		return ""
	}
	return textfmt.Bytes(uint64(s.ResSize)) + " · " + fmt.Sprint(s.Duration) + "ms"
}

func itoa(n int) string { return strconv.Itoa(n) }

// hostPath is where an exchange went: host, port when not the default,
// and path.
func hostPath(s proto.TrafficSummary) string {
	if u, err := url.Parse(s.URL); err == nil && u.Host != "" {
		return u.Host + s.Path
	}
	return s.Host + s.Path
}

// importHAR reads a HAR file of this device into the host's capture.
func (v *View) importHAR(o Options, path string) {
	h := o.Host
	go func() {
		data, err := os.ReadFile(path)
		var l proto.TrafficList
		if err == nil {
			l, err = h.TrafficImport(data)
		}
		o.Update(func() {
			if err != nil {
				v.notice = "Couldn't import: " + client.Explain(err)
				return
			}
			for _, s := range l.Exchanges {
				v.Apply(s)
			}
			v.all = true
			v.notice = plural(len(l.Exchanges), "exchanges") + " imported."
		})
	}()
}

// domainOf is a host's domain: its last two labels (three under a
// two-letter second level such as co.uk), the host itself for addresses.
func domainOf(host string) string {
	if net.ParseIP(host) != nil {
		return host
	}
	parts := strings.Split(strings.TrimSuffix(host, "."), ".")
	n := 2
	if len(parts) >= 3 && len(parts[len(parts)-2]) <= 3 && len(parts[len(parts)-1]) == 2 {
		n = 3
	}
	if len(parts) <= n {
		return host
	}
	return strings.Join(parts[len(parts)-n:], ".")
}

// domainMenu lists the domains of what the list could show, each with its
// count, to hide or show it (a -domain: condition in the filter).
func (v *View) domainMenu(c *ui.Context, o Options) {
	b := widget.IconButton(c.Key("domains"), icon.Globe, "Domains", false)
	v.domains.OpenBelow(b, "domains", 24)
	v.domains.Show(c, b, "domains", func(done func()) {
		counts := map[string]int{}
		for _, s := range v.list {
			if !v.all && s.Session != o.Session.ID && s.Session != o.Session.Parent {
				continue
			}
			counts[domainOf(s.Host)]++
		}
		names := make([]string, 0, len(counts))
		for d := range counts {
			names = append(names, d)
		}
		sort.Slice(names, func(i, j int) bool {
			return counts[names[i]] > counts[names[j]] || counts[names[i]] == counts[names[j]] && names[i] < names[j]
		})
		if len(names) == 0 {
			widget.PopupLabel(c, "No domains yet")
		}
		for _, d := range names {
			hide := "-domain:" + d
			hidden := strings.Contains(" "+v.filter+" ", " "+hide+" ")
			label := fmt.Sprintf("%s (%d)", d, counts[d])
			if hidden {
				label = "Show " + label
			}
			if widget.MenuItem(c, icon.Globe, label, false) {
				if hidden {
					v.filter = strings.TrimSpace(strings.Replace(" "+v.filter+" ", " "+hide+" ", " ", 1))
				} else {
					v.filter = strings.TrimSpace(v.filter + " " + hide)
				}
				v.queryAt = time.Now()
				time.AfterFunc(filterDelay+20*time.Millisecond, func() { o.Update(func() {}) })
				done()
			}
		}
	})
}

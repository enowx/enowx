package ui

import (
	"fmt"
	"slices"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/cacheclean"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// Settings' Clean cache: what the connected host's enowX folder takes, what can go of
// it, and taking it away, which is real: it says what goes and asks first.

// cacheSettings is the page's state, for the host it was read from.
type cacheSettings struct {
	host     string
	loading  bool
	err      string
	report   proto.CacheReport
	read     bool
	older    int
	off      map[string]bool // kinds picked by default the user unpicked
	on       map[string]bool // kinds not picked by default the user picked, and items
	cleaning bool
	result   *proto.CacheCleanResult
}

// cacheAges are the ages offered: what's newer stays.
var cacheAges = []int{7, cacheclean.DefaultDays, 90}

func (a *App) cacheOf() *cacheSettings {
	if a.cacheState == nil || a.cacheState.host != a.host.ID {
		a.cacheState = &cacheSettings{host: a.host.ID, older: cacheclean.DefaultDays, off: map[string]bool{}, on: map[string]bool{}}
	}
	return a.cacheState
}

// cacheRead reads what can go, off the main thread: it walks gigabytes.
func (a *App) cacheRead(cs *cacheSettings) {
	cl := a.client
	if cl == nil || cs.loading {
		return
	}
	cs.loading, cs.err = true, ""
	older := cs.older
	go func() {
		report, err := cl.CacheList(proto.CacheListParams{OlderDays: older})
		a.onMain(func() {
			cs.loading, cs.read = false, true
			if err != nil {
				cs.err = client.Explain(err)
				return
			}
			cs.report = report
		})
	}()
}

// kindPicked says kind k goes whole in a clean.
func (cs *cacheSettings) kindPicked(k proto.CacheKind) bool {
	if k.Info || k.PerItem {
		return false
	}
	if k.Default {
		return !cs.off[k.ID]
	}
	return cs.on[k.ID]
}

// cachePlan is what a clean of what's picked takes, and frees at most.
func (cs *cacheSettings) cachePlan() (proto.CacheCleanParams, int64, []string) {
	p := proto.CacheCleanParams{OlderDays: cs.older}
	var total int64
	var warnings []string
	for _, k := range cs.report.Kinds {
		if cs.kindPicked(k) {
			p.Kinds = append(p.Kinds, k.ID)
			total += k.Bytes
			if k.Warning != "" {
				warnings = append(warnings, k.Warning)
			}
			continue
		}
		if !k.PerItem {
			continue
		}
		picked := false
		for _, it := range k.Items {
			if cs.on[it.Path] && it.Blocked == "" {
				p.Items = append(p.Items, it.Path)
				total += it.Bytes
				picked = true
			}
		}
		if picked && k.Warning != "" {
			warnings = append(warnings, k.Warning)
		}
	}
	return p, total, warnings
}

// cacheClean asks, then cleans what's picked.
func (a *App) cacheClean(cs *cacheSettings) {
	p, total, warnings := cs.cachePlan()
	msg := "This frees up to " + textfmt.Bytes(uint64(total)) + " on " + a.host.Name + ". Removing is real: it can't be undone."
	for _, w := range warnings {
		msg += " " + w
	}
	a.confirm("Clean this?", msg, "Clean", func() {
		cl := a.client
		if cl == nil {
			return
		}
		cs.cleaning, cs.result = true, nil
		go func() {
			res, err := cl.CacheClean(p)
			a.onMain(func() {
				cs.cleaning = false
				if err != nil {
					cs.err = client.Explain(err)
					return
				}
				cs.result, cs.on = &res, map[string]bool{}
				a.cacheRead(cs)
			})
		}()
	})
}

func (a *App) cacheSection(c *ui.Context) {
	cs := a.cacheOf()
	if !cs.read && !cs.loading {
		a.cacheRead(cs)
	}
	_, total, _ := cs.cachePlan()
	widget.PageHeader(c, "Clean cache", "What "+a.host.Name+"'s enowX folder takes, and what can go of it", func() {
		if widget.Button(c.Key("reread"), widget.Secondary, icon.Sync, "Look again", "").Clicked() {
			a.cacheRead(cs)
		}
		clean := widget.Button(c.Key("clean"), widget.Primary, icon.Trash, "Clean…", "")
		if total == 0 || cs.cleaning {
			clean.Opacity(0.45)
		} else if clean.Clicked() {
			a.cacheClean(cs)
		}
	})
	widget.PageBody(c, true, func() {
		if cs.err != "" {
			widget.Banner(c, theme.Error, cs.err, nil)
		}
		a.cacheResult(c, cs)
		widget.FigsBand(c, []widget.Fig{
			{Label: "Its enowX folder", Value: textfmt.Bytes(uint64(cs.report.Total))},
			{Label: "Picked to clean", Value: textfmt.Bytes(uint64(total))},
		})
		ui.Row(c.Key("ages")).FillWidth().Gap(theme.S2).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Only what's older than").FontSize(theme.TextUI).TextColor(theme.TextSoft).SingleLine()
			current := slices.Index(cacheAges, cs.older)
			names := make([]string, len(cacheAges))
			for i, days := range cacheAges {
				names[i] = fmt.Sprintf("%d days", days)
			}
			if got := widget.Segmented(c.Key("age"), names, current, nil); got >= 0 && got != current {
				cs.older = cacheAges[got]
				a.cacheRead(cs)
			}
			if cs.loading {
				widget.Spinner(c, 14)
			}
		})
		for _, k := range cs.report.Kinds {
			a.cacheKindCard(c.Key(k.ID), cs, k)
		}
	})
}

// cacheResult says how the last clean went, item by item where it failed.
func (a *App) cacheResult(c *ui.Context, cs *cacheSettings) {
	res := cs.result
	if res == nil {
		return
	}
	col := theme.Done
	if len(res.Failed) > 0 {
		col = theme.Warning
	}
	widget.Banner(c, col, fmt.Sprintf("Freed %s from %d %s", textfmt.Bytes(uint64(max(res.Freed, 0))), res.Done, textfmt.Plural(res.Done, "item", "items")), nil)
	if len(res.Failed) == 0 {
		return
	}
	widget.GridCard(c.Key("failed")).Children(func() {
		widget.CardTitle(c, icon.Alert, "Not removed", nil)
		widget.CardLines(c, func() {
			for i, f := range res.Failed {
				widget.CardLine(c.Key(i), i == 0, f.Path, f.Why, nil)
			}
		})
	})
}

// cacheKindCard is a kind: its box unless it can't be cleaned or goes item by item,
// what cleaning it does, and those items.
func (a *App) cacheKindCard(c *ui.Context, cs *cacheSettings, k proto.CacheKind) {
	widget.GridCard(c).Children(func() {
		ui.Row(c).FillWidth().Gap(theme.S2).AlignItems(ui.Center).Children(func() {
			if k.Info || k.PerItem {
				ui.Text(c, k.Name).FontSize(theme.TextUI).FontWeight(500).MinWidth(0).Shrink(1)
			} else {
				picked := cs.kindPicked(k)
				if widget.Checkbox(c.Key("kind"), &picked, k.Name) {
					if k.Default {
						cs.off[k.ID] = !picked
					} else {
						cs.on[k.ID] = picked
					}
				}
			}
			ui.Spacer(c)
			ui.Text(c, cacheSize(k)).FontSize(theme.TextUI).TextColor(theme.TextSoft).SingleLine().Shrink(0)
		})
		widget.CardNote(c, k.About, 0)
		if k.Warning != "" {
			widget.CardNote(c, k.Warning, 0).TextColor(theme.Warning)
		}
		if !k.PerItem || len(k.Items) == 0 {
			return
		}
		widget.CardLines(c, func() {
			for i, it := range k.Items {
				a.cacheItemLine(c.Key(it.Path), cs, it, i == 0)
			}
		})
	})
}

// cacheSize is what a kind can free, or takes where it can't be cleaned.
func cacheSize(k proto.CacheKind) string {
	if !k.Info {
		return textfmt.Bytes(uint64(k.Bytes))
	}
	var total int64
	for _, it := range k.Items {
		total += it.Bytes
	}
	return textfmt.Bytes(uint64(total)) + ", not offered"
}

// cacheItemLine is one item of a kind picked item by item: its date and state, and its
// box unless it can't go now.
func (a *App) cacheItemLine(c *ui.Context, cs *cacheSettings, it proto.CacheItem, first bool) {
	sub := textfmt.Bytes(uint64(it.Bytes))
	if !it.When.IsZero() {
		sub += " · " + it.When.Local().Format("2 Jan 2006")
	}
	if it.Note != "" {
		sub += " · " + it.Note
	}
	if it.Blocked != "" {
		sub += " · " + it.Blocked
	}
	widget.CardLine(c, first, it.Name, sub, func() {
		if it.Blocked != "" {
			return
		}
		picked := cs.on[it.Path]
		if widget.Checkbox(c.Key("item"), &picked, "Clean "+it.Name) {
			cs.on[it.Path] = picked
		}
	})
}

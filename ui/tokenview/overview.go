package tokenview

import (
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// The parts of a count the chart and cards show, and their colors.
func partColors() []ui.Color { return []ui.Color{theme.Working, theme.Warning, theme.Done} }

// overview is the range's figures, each day's, where they went, every host's,
// and enowX's own use.
func (v *View) overview(c *ui.Context, d Data, o Options) {
	s := d.Summary
	if !d.Settings.On && s.Total.Calls == 0 {
		widget.Banner(c, theme.Accent, fmt.Sprintf("Token tracking is off on %s. Turned on, enowX reads back the last %d days of the agents' records, then keeps counting as they work: only counts, never what was said.", o.HostName, max(d.Settings.BackfillDays, 30)), nil)
	}
	v.filters(c, d, o)
	totals(c, s.Total)
	if note := totalsNote(s.Total); note != "" {
		widget.CardNote(c, note, 0)
	}
	ui.Row(c).FillWidth().Shrink(0).Gap(theme.S4).AlignItems(ui.Stretch).Children(func() {
		widget.Card(c).Grow(7).Basis(0).Children(func() {
			widget.CardTitle(c, nil, "Each day", nil)
			widget.BarChart(c, dayBars(s, v.Query(time.Now()).Since), partColors(), 160)
			ui.Row(c).FillWidth().Wrap().Gap(theme.S3).Children(func() {
				cols := partColors()
				widget.Legend(c, cols[0], "Input "+Count(s.Total.Input))
				widget.Legend(c, cols[1], "Cached "+Count(s.Total.CacheRead+s.Total.CacheWrite))
				widget.Legend(c, cols[2], "Output "+Count(s.Total.Output))
			})
		})
		widget.Card(c).Grow(5).Basis(0).Children(func() { v.whereTheyWent(c, s, o) })
	})
	ui.Grid(c).FillWidth().Columns(3).Gap(theme.S3).Children(func() {
		if s.Own.Calls > 0 {
			ownUse(c, s)
		}
		if len(d.Hosts) > 1 || (len(d.Hosts) == 1 && !d.Hosts[0].Live) {
			hosts(c, d.Hosts)
		}
		v.layers(c, d, o)
	})
}

// filters are what the numbers cover, and tracking's switch.
func (v *View) filters(c *ui.Context, d Data, o Options) {
	ui.Row(c).FillWidth().Shrink(0).Gap(theme.S2).Children(func() {
		labels := make([]string, len(ranges))
		for i, r := range ranges {
			labels[i] = r.label
		}
		if i := widget.Segmented(c.Key("range"), labels, v.Range, nil); i >= 0 && i != v.Range {
			v.Range = i
			refresh(o)
		}
		labels = make([]string, len(splits))
		for i, sp := range splits {
			labels[i] = sp.label
		}
		if i := widget.Choice(c, "Group by", labels, v.Split, 112); i >= 0 && i != v.Split {
			v.Split = i
			refresh(o)
		}
		v.projectFilter(c, o)
		if d.Loading {
			widget.Spinner(c, 14)
		}
		ui.Spacer(c)
		v.tracking(c, d, o)
	})
}

// projectFilter picks one project's use, or every project's.
func (v *View) projectFilter(c *ui.Context, o Options) {
	paths := slices.Clone(o.Projects)
	if v.Project != "" && !slices.Contains(paths, v.Project) {
		paths = append(paths, v.Project)
	}
	labels := []string{"All projects"}
	cur := 0
	for i, p := range paths {
		labels = append(labels, filepath.Base(p))
		if p == v.Project {
			cur = i + 1
		}
	}
	if i := widget.Choice(c, "Project", labels, cur, 136); i >= 0 && i != cur {
		v.Project = ""
		if i > 0 {
			v.Project = paths[i-1]
		}
		refresh(o)
	}
}

func refresh(o Options) {
	if o.Refresh != nil {
		o.Refresh()
	}
}

// totals are the range's figures.
func totals(c *ui.Context, t proto.TokenCounts) {
	cached := "0%"
	if all := t.Input + t.CacheRead + t.CacheWrite; all > 0 {
		cached = fmt.Sprintf("%.0f%%", float64(t.CacheRead+t.CacheWrite)/float64(all)*100)
	}
	widget.Figs(c, []widget.Fig{
		{Label: "Tokens", Value: Count(t.Total())},
		{Label: "Cost", Value: Dollars(t.Cost)},
		{Label: "Calls", Value: strconv.Itoa(t.Calls)},
		{Label: "Cached", Value: cached},
	})
}

// totalsNote says what the figures don't: the part of them that isn't what it
// seems. Empty when there's none.
func totalsNote(t proto.TokenCounts) string {
	var parts []string
	if t.CacheWrite > 0 {
		parts = append(parts, Count(t.CacheWrite)+" written to the cache")
	}
	if t.Reasoning > 0 {
		parts = append(parts, Count(t.Reasoning)+" of the output reasoning")
	}
	if t.Equivalent > 0 {
		parts = append(parts, Dollars(t.Equivalent)+" of the cost API-equivalent, on subscriptions")
	}
	if t.Unpriced > 0 {
		parts = append(parts, fmt.Sprintf("%d calls of models without a price", t.Unpriced))
	}
	return strings.Join(parts, "; ")
}

// dayBars are the summary's days from since on, a bar each, days without
// use included.
func dayBars(s proto.TokensSummary, since time.Time) []widget.Bar {
	byDay := map[string]proto.TokenCounts{}
	for _, d := range s.Days {
		byDay[d.Day] = d.TokenCounts
	}
	var out []widget.Bar
	end := time.Now()
	for d := since; !d.After(end); d = d.AddDate(0, 0, 1) {
		day := d.Format(time.DateOnly)
		t := byDay[day]
		out = append(out, widget.Bar{Label: d.Format("Jan 2"),
			Tip:   fmt.Sprintf("%s: %s in, %s cached, %s out, %s", d.Format("Mon Jan 2"), Count(t.Input), Count(t.CacheRead+t.CacheWrite), Count(t.Output), Dollars(t.Cost)),
			Parts: []float64{float64(t.Input), float64(t.CacheRead + t.CacheWrite), float64(t.Output)}})
	}
	return out
}

// whereTheyWent is the range's use split as asked, each against the biggest.
func (v *View) whereTheyWent(c *ui.Context, s proto.TokensSummary, o Options) {
	widget.CardTitle(c, nil, "Where they went", nil)
	if len(s.Groups) == 0 {
		widget.CardNote(c, "Nothing counted in this range.", 0)
		return
	}
	var top int64
	for _, g := range s.Groups {
		top = max(top, g.Total())
	}
	for _, g := range s.Groups {
		name := orText(g.Label, g.Key)
		if splits[v.Split].by == proto.TokensByAgent && o.Agent != nil && g.Key != "" {
			name = o.Agent(g.Key)
		}
		tip := fmt.Sprintf("%s in, %s cached, %s out, %d calls", Count(g.Input), Count(g.CacheRead+g.CacheWrite), Count(g.Output), g.Calls)
		if splits[v.Split].by == proto.TokensByProject && g.Key != "" {
			tip = g.Key + ": " + tip
		}
		ui.Column(c.Key("g:" + g.Key)).FillWidth().Shrink(0).Gap(theme.S1).Tooltip(tip).Children(func() {
			ui.Row(c).FillWidth().Height(24).Gap(theme.S2).Children(func() {
				ui.Row(c).Width(128).Shrink(0).MinWidth(0).Children(func() {
					widget.Running(c, name, widget.RunStyle{Size: theme.TextLabel})
				})
				ui.Row(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
					widget.Meter(c, widget.Segment{Fraction: float64(g.Total()) / float64(max(top, 1)), Color: theme.Working})
				})
				ui.Text(c, Count(g.Total())).FontSize(theme.TextLabel).Width(48).Shrink(0).TextAlign(ui.End).SingleLine()
			})
			ui.Row(c).FillWidth().Gap(theme.S2).Children(func() {
				ui.Text(c, "cost").FontSize(theme.TextLabel).TextColor(theme.TextSoft).Grow(1)
				cost := Dollars(g.Cost)
				if g.Equivalent > 0 && g.Equivalent >= g.Cost {
					cost = "≈" + cost
				}
				ui.Text(c, cost).FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine()
			})
		})
	}
}

// ownUse is what enowX's own judge and embedder used.
func ownUse(c *ui.Context, s proto.TokensSummary) {
	widget.Card(c).Children(func() {
		widget.CardTitle(c, nil, "Apart from the agents'", nil)
		widget.CardNote(c, "The judge (Settings → Decisions) and RAG's embedder: not in the totals above.", 0)
		widget.CardLines(c, func() {
			for i, g := range s.OwnGroups {
				name := map[string]string{proto.TokensOwnJudge: "Decision model (judge)", proto.TokensOwnRAG: "RAG's embedder"}[g.Key]
				countLine(c.Key("own:"+g.Key), i == 0, orText(name, g.Key), "", fmt.Sprintf("%d calls · %s", g.Calls, Dollars(g.Cost)))
			}
		})
	})
}

// hosts is each host's part: asked now, or as it last synced.
func hosts(c *ui.Context, list []HostTotal) {
	widget.Card(c).Children(func() {
		widget.CardTitle(c, nil, "Hosts", nil)
		widget.CardNote(c, "Every host's numbers together: those the app reaches now, the others as they last synced.", 0)
		widget.CardLines(c, func() {
			for i, h := range list {
				detail := "now"
				switch {
				case !h.Live:
					detail = "as of " + h.Updated.Local().Format("2 Jan 15:04")
				case !h.On:
					detail = "tracking off"
				}
				countLine(c.Key("h:"+h.Name), i == 0, h.Name, detail, Count(h.Total.Total())+" · "+Dollars(h.Total.Cost))
			}
		})
	})
}

// countLine is a row of a card's list, with its counts at the right.
func countLine(c *ui.Context, first bool, title, sub, value string) {
	widget.CardLine(c, first, title, sub, func() {
		ui.Text(c, value).FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine()
	})
}

// layers are tracking's layers past the agents' records, and reading the
// records back.
func (v *View) layers(c *ui.Context, d Data, o Options) {
	st := d.Settings
	widget.Card(c).Children(func() {
		widget.CardTitle(c, nil, "How agents are tracked", nil)
		widget.CardNote(c, "Agents' own records first; for those that keep none, their model API's answers through the interceptor, or the lines they print.", 0)
		on := st.Traffic
		if widget.ToggleRow(c, "Meter agents that keep no records", "Through the interceptor, for new sessions.", &on) && o.Save != nil {
			st.Traffic = on
			o.Save(st)
		}
		on = st.Screen
		if widget.ToggleRow(c, "Read the counts agents print", "On their screen, when nothing else tracks them.", &on) && o.Save != nil {
			st.Screen = on
			o.Save(st)
		}
		widget.CardNote(c, fmt.Sprintf("Records read back %d days when tracking turns on.", max(st.BackfillDays, 30)), 0)
		if d.Backfilled != "" {
			widget.CardNote(c, d.Backfilled, 0)
		}
		if st.On && o.Backfill != nil {
			widget.CardFoot(c, func() {
				if widget.GhostButton(c, "Read back now").Clicked() {
					o.Backfill()
				}
			})
		}
	})
}

func orText(s, or string) string {
	if s == "" {
		return or
	}
	return s
}

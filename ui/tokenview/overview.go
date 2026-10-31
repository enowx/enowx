package tokenview

import (
	"fmt"
	"path/filepath"
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// The parts of a count the chart and cards show, and their colors.
func partColors() []ui.Color { return []ui.Color{theme.Accent, theme.Idle, theme.Working} }

// overview is the totals, each day's, the split, every host's, and
// enowX's own use.
func (v *View) overview(c *ui.Context, d Data, o Options) {
	s := d.Summary
	if !d.Settings.On && s.Total.Calls == 0 {
		widget.Notice(c, icon.Info, theme.Accent, "Token tracking is off on "+o.HostName,
			fmt.Sprintf("Turned on, enowX reads back the last %d days of the agents' records, then keeps counting as they work: only counts, never what was said.", max(d.Settings.BackfillDays, 30)), nil)
	}
	ui.Row(c).FillWidth().Gap(10).Children(func() {
		labels := make([]string, len(ranges))
		for i, r := range ranges {
			labels[i] = r.label
		}
		if i := widget.Segmented(c.Key("range"), labels, v.Range, nil); i >= 0 && i != v.Range {
			v.Range = i
			refresh(o)
		}
		ui.Spacer(c)
		if v.Project != "" {
			ui.Text(c, "Project: "+filepath.Base(v.Project)).FontSize(12).TextColor(theme.TextSoft).SingleLine()
			if widget.IconButton(c, icon.Close, "All projects", false).Clicked() {
				v.Project = ""
				refresh(o)
			}
		}
		if d.Loading {
			widget.Spinner(c, 14)
		}
	})
	totals(c, s.Total)
	if len(s.Days) > 0 {
		widget.Card(c).Children(func() {
			ui.Row(c).FillWidth().Gap(14).Children(func() {
				ui.Text(c, "Each day").FontSize(12).TextColor(theme.TextSoft).Grow(1)
				cols := partColors()
				widget.Legend(c, cols[0], "Input")
				widget.Legend(c, cols[1], "Cached")
				widget.Legend(c, cols[2], "Output")
			})
			widget.BarChart(c, dayBars(s, v.Query(time.Now()).Since), partColors(), 120)
		})
	}
	v.splitTable(c, s, o)
	if len(d.Hosts) > 1 || (len(d.Hosts) == 1 && !d.Hosts[0].Live) {
		hosts(c, d.Hosts)
	}
	if s.Own.Calls > 0 {
		section(c, "enowX's own use", "The judge (Settings → Decisions) and RAG's embedder, apart from the agents': not in the totals above.", func() {
			for _, g := range s.OwnGroups {
				name := map[string]string{proto.TokensOwnJudge: "Decision model (judge)", proto.TokensOwnRAG: "RAG's embedder"}[g.Key]
				countsRow(c.Key("own:"+g.Key), orText(name, g.Key), "", g.TokenCounts)
			}
		})
	}
	v.layers(c, d, o)
}

func refresh(o Options) {
	if o.Refresh != nil {
		o.Refresh()
	}
}

// totals are the range's counts as cards: input, cached, written to the
// cache, output, and the cost.
func totals(c *ui.Context, t proto.TokenCounts) {
	cols := partColors()
	ui.Grid(c).FillWidth().ColumnTracks(ui.Fr(1), ui.Fr(1), ui.Fr(1), ui.Fr(1), ui.Fr(1)).Gap(10).Children(func() {
		stat(c, "Input", Count(t.Input), "not from the cache", cols[0])
		stat(c, "Cached", Count(t.CacheRead), Count(t.CacheWrite)+" written to the cache", cols[1])
		reasoning := ""
		if t.Reasoning > 0 {
			reasoning = Count(t.Reasoning) + " of it reasoning"
		}
		stat(c, "Output", Count(t.Output), reasoning, cols[2])
		stat(c, "Calls", strconv.Itoa(t.Calls), Count(t.Total())+" tokens in all", theme.TextSoft)
		note := "at the price table's rates"
		if t.Equivalent > 0 {
			note = Dollars(t.Equivalent) + " of it API-equivalent, on subscriptions"
		}
		if t.Unpriced > 0 {
			note += fmt.Sprintf("; %d calls of models without a price", t.Unpriced)
		}
		stat(c, "Cost", Dollars(t.Cost), note, theme.TextSoft)
	})
}

func stat(c *ui.Context, title, value, note string, col ui.Color) {
	widget.Card(c).Children(func() {
		ui.Row(c).Gap(6).Children(func() {
			widget.Dot(c, col, false, 6)
			ui.Text(c, title).FontSize(11.5).TextColor(theme.TextSoft).SingleLine()
		})
		ui.Text(c, value).FontSize(18).SingleLine()
		if note != "" {
			ui.Text(c, note).FontSize(10.5).TextColor(theme.Muted)
		}
	})
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

// splitTable is the range's use split as asked.
func (v *View) splitTable(c *ui.Context, s proto.TokensSummary, o Options) {
	widget.Card(c).Children(func() {
		ui.Row(c).FillWidth().Gap(10).Children(func() {
			ui.Text(c, "By").FontSize(12).TextColor(theme.TextSoft)
			labels := make([]string, len(splits))
			for i, sp := range splits {
				labels[i] = sp.label
			}
			if i := widget.Segmented(c.Key("split"), labels, v.Split, nil); i >= 0 && i != v.Split {
				v.Split = i
				refresh(o)
			}
		})
		header(c)
		for _, g := range s.Groups {
			name := orText(g.Label, g.Key)
			if splits[v.Split].by == proto.TokensByAgent && o.Agent != nil && g.Key != "" {
				name = o.Agent(g.Key)
			}
			detail := ""
			if splits[v.Split].by == proto.TokensByProject && g.Key != "" {
				detail = g.Key
			}
			countsRow(c.Key("g:"+g.Key), name, detail, g.TokenCounts)
		}
		if len(s.Groups) == 0 {
			ui.Text(c, "Nothing counted in this range.").FontSize(12).TextColor(theme.Muted).Padding(8, 10)
		}
	})
}

func header(c *ui.Context) {
	ui.Row(c).FillWidth().Padding(4, 10).Gap(10).Children(func() {
		ui.Text(c, "").Grow(1)
		for _, h := range []string{"Input", "Cached", "Output", "Calls", "Cost"} {
			ui.Text(c, h).FontSize(11).TextColor(theme.Muted).Width(76).TextAlign(ui.End)
		}
	})
}

func countsRow(c *ui.Context, name, detail string, t proto.TokenCounts) {
	ui.Row(c).FillWidth().Padding(7, 10).Gap(10).Radius(theme.RadiusControl).Children(func() {
		ui.Row(c).Grow(1).MinWidth(0).Gap(8).Children(func() {
			ui.Text(c, name).FontSize(12).SingleLine().Shrink(1)
			if detail != "" {
				ui.Text(c, detail).FontSize(11).TextColor(theme.Muted).SingleLine().Shrink(1)
			}
		})
		cost := Dollars(t.Cost)
		if t.Equivalent > 0 && t.Equivalent >= t.Cost {
			cost = "≈" + cost
		}
		for _, v := range []string{Count(t.Input), Count(t.CacheRead + t.CacheWrite), Count(t.Output), strconv.Itoa(t.Calls), cost} {
			ui.Text(c, v).FontSize(12).Width(76).TextAlign(ui.End).SingleLine().TextColor(theme.TextSoft)
		}
	})
}

// hosts is each host's part: asked now, or as it last synced.
func hosts(c *ui.Context, list []HostTotal) {
	section(c, "Hosts", "Every host's numbers together: those the app reaches now, the others as they last synced.", func() {
		for _, h := range list {
			detail := "now"
			switch {
			case !h.Live:
				detail = "synced " + h.Updated.Local().Format("Jan 2 15:04")
			case !h.On:
				detail = "tracking off"
			}
			countsRow(c.Key("h:"+h.Name), h.Name, detail, h.Total)
		}
	})
}

func section(c *ui.Context, title, note string, body func()) {
	widget.Card(c).Children(func() {
		ui.Text(c, title).FontSize(12.5)
		if note != "" {
			ui.Text(c, note).FontSize(11).TextColor(theme.Muted)
		}
		body()
	})
}

// layers are tracking's layers past the agents' records, and reading the
// records back.
func (v *View) layers(c *ui.Context, d Data, o Options) {
	st := d.Settings
	section(c, "How agents are tracked", "Agents' own records first; for those that keep none, their model API's answers through the interceptor, or the lines they print. See Coverage.", func() {
		ui.Row(c).FillWidth().Gap(10).Children(func() {
			on := st.Traffic
			if widget.Switch(c.Key("traffic"), &on, "Meter agents that keep no records") && o.Save != nil {
				st.Traffic = on
				o.Save(st)
			}
			ui.Text(c, "Meter agents that keep no records through the interceptor (new sessions)").FontSize(12).TextColor(theme.TextSoft).Grow(1).Shrink(1)
		})
		ui.Row(c).FillWidth().Gap(10).Children(func() {
			on := st.Screen
			if widget.Switch(c.Key("screen"), &on, "Read the counts agents print") && o.Save != nil {
				st.Screen = on
				o.Save(st)
			}
			ui.Text(c, "Read the counts agents print on their screen, when nothing else tracks them").FontSize(12).TextColor(theme.TextSoft).Grow(1).Shrink(1)
		})
		ui.Row(c).FillWidth().Gap(10).Children(func() {
			ui.Text(c, fmt.Sprintf("Records read back %d days when tracking turns on.", max(st.BackfillDays, 30))).FontSize(12).TextColor(theme.TextSoft).Grow(1)
			if d.Backfilled != "" {
				ui.Text(c, d.Backfilled).FontSize(11).TextColor(theme.Muted).SingleLine()
			}
			if st.On && o.Backfill != nil && widget.GhostButton(c, "Read back now").Clicked() {
				o.Backfill()
			}
		})
	})
}

func orText(s, or string) string {
	if s == "" {
		return or
	}
	return s
}

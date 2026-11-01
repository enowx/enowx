package ui

import (
	"slices"
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/automation"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// What the automations' schedules come to over the coming days: the section's
// figures and its board of the week, today first.

// automationRun is a run a schedule says is due.
type automationRun struct {
	at   time.Time
	name string
}

// automationWeek is the runs of the enabled automations, by day from today's
// start, each day's in order. A schedule this version can't read has no runs
// here rather than an error: its card still says what it is.
func automationWeek(list []proto.Automation, now time.Time) [7][]automationRun {
	var week [7][]automationRun
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	end := start.AddDate(0, 0, len(week))
	for _, au := range list {
		sc, err := automation.Parse(au.Schedule)
		if !au.Enabled || err != nil {
			continue
		}
		for at := sc.Next(start.Add(-time.Second)); at.Before(end); at = sc.Next(at) {
			for day := range week {
				if at.Before(start.AddDate(0, 0, day+1)) {
					week[day] = append(week[day], automationRun{at, au.Name})
					break
				}
			}
		}
	}
	for day := range week {
		slices.SortStableFunc(week[day], func(x, y automationRun) int { return x.at.Compare(y.at) })
	}
	return week
}

// automationFigs are the section's figures.
func automationFigs(list []proto.Automation, week [7][]automationRun) []widget.Fig {
	on, failed, runs := 0, 0, 0
	for _, au := range list {
		if au.Enabled {
			on++
		}
		if au.LastError != "" {
			failed++
		}
	}
	for _, day := range week {
		runs += len(day)
	}
	return []widget.Fig{
		{Label: "Automations", Value: strconv.Itoa(len(list))},
		{Label: "On", Value: strconv.Itoa(on)},
		{Label: "Runs this week", Value: strconv.Itoa(runs)},
		{Label: "Last run failed", Value: strconv.Itoa(failed)},
	}
}

// automationWeekBoard is the week in columns of a day each, sharing what the
// frame leaves. A day with more runs than fit, as with an hourly one, scrolls in
// its column.
func automationWeekBoard(c *ui.Context, week [7][]automationRun, now time.Time) {
	widget.Heading(c, "This week")
	ui.Row(c.Key("week")).FillWidth().Grow(1).Basis(0).MinHeight(160).AlignItems(ui.Stretch).Gap(theme.S2).Children(func() {
		for day, runs := range week {
			label, text := now.AddDate(0, 0, day).Format("Mon"), theme.Text
			if day == 0 {
				label, text = "Today", theme.Accent
			}
			ui.Column(c.Key(day)).Grow(1).Basis(0).MinWidth(0).MinHeight(0).Padding(theme.S3).Gap(theme.S2).Radius(theme.RadiusCard).Background(theme.Raised).Clip().Children(func() {
				ui.Row(c).FillWidth().Shrink(0).Children(func() {
					widget.Running(c, label, widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
				})
				ui.Scroll(c).Grow(1).FillWidth().MinHeight(0).Children(func() {
					ui.Column(c).FillWidth().Gap(theme.S2).Children(func() {
						for i, run := range runs {
							ui.Text(c.Key(i), run.at.Format("15:04")+" "+run.name).FontSize(theme.TextLabel).TextColor(text).MaxLines(3).
								FillWidth().MinWidth(0).Padding(theme.S1, theme.S2).Radius(theme.S2).Background(theme.Sunken)
						}
					})
				})
			})
		}
	})
}

// automationWhen says when a run is or was: by day when near, by date when not.
func automationWhen(t, now time.Time) string {
	t, now = t.Local(), now.Local()
	day := func(x time.Time) time.Time { return time.Date(x.Year(), x.Month(), x.Day(), 12, 0, 0, 0, x.Location()) }
	days := int(day(t).Sub(day(now)).Round(24*time.Hour) / (24 * time.Hour))
	switch {
	case days == 0:
		return "Today " + t.Format("15:04")
	case days > -7 && days < 7:
		return t.Format("Mon 15:04")
	}
	return t.Format("Jan 2 15:04")
}

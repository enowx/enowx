// Package tokenview shows the tokens agents use: every host's together,
// per day, split by agent, model, project, session, config or host, with
// costs, budgets, the price table and how each agent is tracked. It gets
// its data and acts through Options; the app fetches.
package tokenview

import (
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// Tabs of the view.
const (
	TabOverview = "Overview"
	TabBudgets  = "Budgets"
	TabPrices   = "Prices"
	TabCoverage = "Coverage"
)

var tabs = []string{TabOverview, TabBudgets, TabPrices, TabCoverage}

// Ranges a summary covers, in days back from now (0: today).
var ranges = []struct {
	label string
	days  int
}{{"Today", 0}, {"7 days", 7}, {"30 days", 30}, {"90 days", 90}, {"Year", 365}}

var splits = []struct{ label, by string }{
	{"Agent", proto.TokensByAgent}, {"Model", proto.TokensByModel}, {"Project", proto.TokensByProject},
	{"Session", proto.TokensBySession}, {"Config", proto.TokensByConfig}, {"Host", proto.TokensByHost},
}

// View is the Tokens tab's state. It belongs to the main thread.
type View struct {
	Tab     string
	Range   int    // index of ranges
	Split   int    // index of splits
	Project string // only this project's, "" all
	budget  budgetForm
	price   priceForm
}

// Query is what the view asks for now.
func (v *View) Query(now time.Time) proto.TokensQuery {
	r := ranges[min(v.Range, len(ranges)-1)]
	since := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -r.days)
	return proto.TokensQuery{Since: since, By: splits[min(v.Split, len(splits)-1)].by, Project: v.Project}
}

// HostTotal is one host's part of every host's numbers: asked now
// (Live), or as it last synced (Updated).
type HostTotal struct {
	Name    string
	Live    bool
	Updated time.Time
	On      bool
	Total   proto.TokenCounts
}

// Data is what the app fetched for the view.
type Data struct {
	Summary  proto.TokensSummary // every host's together
	Hosts    []HostTotal
	Settings proto.TokensSettings
	Budgets  []proto.TokenBudgetState
	Prices   []proto.TokenPrice
	Coverage []proto.TokenCoverage
	Loading  bool
	Err      string
	// Backfilled says what the last backfill read, "" before one.
	Backfilled string
}

// Options are what the view does through the app.
type Options struct {
	HostName string
	// Refresh asks for the data again: the query changed.
	Refresh func()
	// Save keeps the settings (tracking, layers, prices, budgets).
	Save     func(proto.TokensSettings)
	Backfill func()
	// Confirm asks before something that can't be undone.
	Confirm func(title, body, action string, fn func())
	// Agent names an agent's preset ID.
	Agent func(id string) string
}

// Draw draws the Tokens tab.
func (v *View) Draw(c *ui.Context, d Data, o Options) {
	if v.Tab == "" {
		v.Tab = TabOverview
	}
	ui.Column(c).FillWidth().Gap(16).Children(func() {
		ui.Row(c).FillWidth().Gap(12).Children(func() {
			cur := 0
			for i, t := range tabs {
				if t == v.Tab {
					cur = i
				}
			}
			if i := widget.Segmented(c.Key("tabs"), tabs, cur, nil); i >= 0 {
				v.Tab = tabs[i]
			}
			ui.Spacer(c)
			v.tracking(c, d, o)
		})
		if d.Err != "" {
			ui.Text(c, d.Err).FontSize(12).TextColor(theme.Error)
		}
		switch v.Tab {
		case TabBudgets:
			v.budgets(c, d, o)
		case TabPrices:
			v.prices(c, d, o)
		case TabCoverage:
			coverage(c, d)
		default:
			v.overview(c, d, o)
		}
	})
	v.budgetSheet(c, d, o)
	v.priceSheet(c, d, o)
}

// tracking is tracking's switch and its layers'.
func (v *View) tracking(c *ui.Context, d Data, o Options) {
	st := d.Settings
	on := st.On
	if widget.Switch(c.Key("on"), &on, "Track tokens on "+o.HostName) && o.Save != nil {
		st.On = on
		o.Save(st)
	}
	ui.Text(c, "Track on "+o.HostName).FontSize(12).TextColor(theme.TextSoft).SingleLine()
}

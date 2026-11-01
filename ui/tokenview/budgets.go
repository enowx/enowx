package tokenview

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

var (
	periods     = []string{proto.BudgetDay, proto.BudgetWeek, proto.BudgetMonth}
	periodNames = []string{"Day", "Week", "Month"}
	scopes      = []string{proto.BudgetAll, proto.BudgetAgent, proto.BudgetConfig, proto.BudgetProject}
	scopeNames  = []string{"Everything", "An agent", "A config", "A project"}
	atLimits    = []string{proto.BudgetNotify, proto.BudgetAsk, proto.BudgetStop}
	atNames     = []string{"Notify only", "Ask before new sessions", "Stop new sessions"}
)

// budgetForm is the budget being made or changed, in a sheet.
type budgetForm struct {
	open                      bool
	id                        string // "" a new one
	period, scope, at         int
	target, tokens, cost, err string
}

// budgets lists the budgets and how much of each is used.
func (v *View) budgets(c *ui.Context, d Data, o Options) {
	widget.CardNote(c, "At 80% a notification; at 100% what each budget says. Budgets are synced: the same on every host, each host counting its own use.", 0)
	if len(d.Budgets) == 0 {
		widget.EmptyState(c, icon.Gauge, "No budgets", "Cap the tokens or the cost of a day, a week or a month: for everything, an agent, a config or a project.", nil)
		return
	}
	ui.Grid(c).FillWidth().Columns(3).Gap(theme.S3).Children(func() {
		for _, b := range d.Budgets {
			v.budgetCard(c.Key(b.ID), b, d, o)
		}
	})
}

// budgetCard is a budget: how far it is, and its way to change or remove it.
func (v *View) budgetCard(c *ui.Context, b proto.TokenBudgetState, d Data, o Options) {
	widget.Card(c).Children(func() {
		widget.CardTitle(c, nil, budgetTitle(b.TokenBudget, o), nil)
		ui.Row(c).FillWidth().Shrink(0).Gap(theme.S2).AlignItems(ui.End).Children(func() {
			ui.Text(c, fmt.Sprintf("%d%%", int(b.Fraction*100))).FontSize(24).FontWeight(600).TextColor(widget.Level(b.Fraction)).SingleLine()
			ui.Row(c).Grow(1).MinWidth(0).Padding(0, 0, theme.S1, 0).Children(func() {
				widget.Running(c, budgetAgainst(b), widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
			})
		})
		widget.Meter(c, widget.Segment{Fraction: min(b.Fraction, 1), Color: widget.Level(b.Fraction)})
		widget.CardNote(c, budgetUsed(b), 2)
		ui.Row(c).FillWidth().Shrink(0).Children(func() {
			at := atNames[max(0, slices.Index(atLimits, orText(b.AtLimit, proto.BudgetNotify)))]
			widget.Chip(c, "At 100%: "+at, ui.Color{})
		})
		widget.CardFoot(c, func() {
			ui.Spacer(c)
			if widget.GhostButton(c, "Change").Clicked() {
				v.budget = formOf(b.TokenBudget)
			}
			if widget.IconButton(c, icon.Trash, "Remove", false).Clicked() && o.Confirm != nil {
				id := b.ID
				o.Confirm("Remove this budget?", "Agents go on without its limit.", "Remove", func() {
					st := d.Settings
					st.Budgets = slices.DeleteFunc(slices.Clone(st.Budgets), func(x proto.TokenBudget) bool { return x.ID == id })
					o.Save(st)
				})
			}
		})
	})
}

// budgetTitle is what a budget is for and over how long: "Everything, a week".
func budgetTitle(b proto.TokenBudget, o Options) string {
	what := "Everything"
	switch b.Scope {
	case proto.BudgetAgent:
		what = b.Target
		if o.Agent != nil {
			what = o.Agent(b.Target)
		}
	case proto.BudgetConfig:
		what = "Config " + b.Target
	case proto.BudgetProject:
		what = "Project " + b.Target
	}
	period := map[string]string{proto.BudgetDay: "a day", proto.BudgetWeek: "a week", proto.BudgetMonth: "a month"}[b.Period]
	return what + ", " + period
}

// budgetAgainst is the use against the limit that is closer to being reached.
func budgetAgainst(b proto.TokenBudgetState) string {
	tokens := float64(0)
	if b.Tokens > 0 {
		tokens = float64(b.Used.Total()) / float64(b.Tokens)
	}
	if b.Cost > 0 && b.Used.Cost/b.Cost >= tokens {
		return Dollars(b.Used.Cost) + " of " + Dollars(b.Cost)
	}
	return Count(b.Used.Total()) + " of " + Count(b.Tokens) + " tokens"
}

func budgetUsed(b proto.TokenBudgetState) string {
	return fmt.Sprintf("%s tokens and %s used since %s", Count(b.Used.Total()), Dollars(b.Used.Cost), b.Since.Local().Format("Mon Jan 2"))
}

func formOf(b proto.TokenBudget) budgetForm {
	f := budgetForm{open: true, id: b.ID, target: b.Target,
		period: max(0, slices.Index(periods, b.Period)), scope: max(0, slices.Index(scopes, b.Scope)), at: max(0, slices.Index(atLimits, orText(b.AtLimit, proto.BudgetNotify)))}
	if b.Tokens > 0 {
		f.tokens = strconv.FormatInt(b.Tokens, 10)
	}
	if b.Cost > 0 {
		f.cost = strconv.FormatFloat(b.Cost, 'f', -1, 64)
	}
	return f
}

// budgetSheet makes or changes a budget.
func (v *View) budgetSheet(c *ui.Context, d Data, o Options) {
	f := &v.budget
	if !f.open {
		return
	}
	title := "Add a budget"
	if f.id != "" {
		title = "Change the budget"
	}
	widget.FormSheet(c, &f.open, 480, title, "Tokens, a cost in dollars, or both: the first reached counts.", func() {
		ui.Grid(c).FillWidth().Columns(2).Gap(theme.S3).Children(func() {
			widget.Field(c, "Per", func() {
				if i := widget.Choice(c, "Per", periodNames, f.period, 0); i >= 0 {
					f.period = i
				}
			})
			widget.Field(c, "For", func() {
				if i := widget.Choice(c, "For", scopeNames, f.scope, 0); i >= 0 {
					f.scope = i
				}
			})
		})
		if scopes[f.scope] != proto.BudgetAll {
			hint := map[string]string{proto.BudgetAgent: "The agent's ID: claude, codex…", proto.BudgetConfig: "The config's name", proto.BudgetProject: "The project's folder"}[scopes[f.scope]]
			widget.Input(c.Key("target"), &f.target, hint).FillWidth()
		}
		ui.Grid(c).FillWidth().Columns(2).Gap(theme.S3).Children(func() {
			widget.Field(c, "Tokens", func() { widget.Input(c.Key("tokens"), &f.tokens, "20000000").FillWidth() })
			widget.Field(c, "Cost, US dollars", func() { widget.Input(c.Key("cost"), &f.cost, "75").FillWidth() })
		})
		widget.Field(c, "At 100%", func() {
			if i := widget.Choice(c, "At 100%", atNames, f.at, 0); i >= 0 {
				f.at = i
			}
		})
		if f.err != "" {
			widget.CardNote(c, f.err, 0).TextColor(theme.Error)
		}
	}, func() {
		if widget.PrimaryButton(c, nil, "Save", "").Clicked() {
			b, err := f.budget()
			if err != nil {
				f.err = err.Error()
				return
			}
			st := d.Settings
			list := slices.Clone(st.Budgets)
			if i := slices.IndexFunc(list, func(x proto.TokenBudget) bool { return x.ID == f.id && f.id != "" }); i >= 0 {
				list[i] = b
			} else {
				list = append(list, b)
			}
			st.Budgets = list
			o.Save(st)
			f.open = false
		}
	})
}

func (f *budgetForm) budget() (proto.TokenBudget, error) {
	b := proto.TokenBudget{ID: f.id, Period: periods[f.period], Scope: scopes[f.scope], AtLimit: atLimits[f.at]}
	if b.Scope != proto.BudgetAll {
		b.Target = strings.TrimSpace(f.target)
		if b.Target == "" {
			return b, fmt.Errorf("say which %s", strings.TrimPrefix(strings.ToLower(scopeNames[f.scope]), "a "))
		}
	}
	if s := strings.TrimSpace(strings.ReplaceAll(f.tokens, ",", "")); s != "" {
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil || n <= 0 {
			return b, fmt.Errorf("tokens: a whole number above 0")
		}
		b.Tokens = n
	}
	if s := strings.TrimSpace(strings.TrimPrefix(f.cost, "$")); s != "" {
		x, err := strconv.ParseFloat(s, 64)
		if err != nil || x <= 0 {
			return b, fmt.Errorf("cost: dollars above 0")
		}
		b.Cost = x
	}
	if b.Tokens == 0 && b.Cost == 0 {
		return b, fmt.Errorf("set tokens, a cost, or both")
	}
	return b, nil
}

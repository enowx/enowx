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
	ui.Row(c).FillWidth().Gap(10).Children(func() {
		ui.Text(c, "At 80% a notification; at 100% what each budget says. Budgets are synced: the same on every host, each host counting its own use.").
			FontSize(12).TextColor(theme.Muted).Grow(1).Shrink(1)
		if widget.PrimaryButton(c, icon.Plus, "Add budget", "").Clicked() {
			v.budget = budgetForm{open: true}
		}
	})
	if len(d.Budgets) == 0 {
		widget.EmptyState(c, icon.Gauge, "No budgets", "Cap the tokens or the cost of a day, a week or a month: for everything, an agent, a config or a project.", nil)
		return
	}
	for _, b := range d.Budgets {
		widget.Card(c.Key(b.ID)).Children(func() {
			ui.Row(c).FillWidth().Gap(10).Children(func() {
				ui.Text(c, budgetTitle(b.TokenBudget, o)).FontSize(12.5).Grow(1).Shrink(1).SingleLine()
				col := widget.Level(b.Fraction)
				ui.Text(c, fmt.Sprintf("%d%%", int(b.Fraction*100))).FontSize(12.5).TextColor(col)
				if widget.IconButton(c, icon.Pencil, "Change", false).Clicked() {
					v.budget = formOf(b.TokenBudget)
				}
				if widget.IconButton(c, icon.Trash, "Remove", false).Clicked() && o.Confirm != nil {
					id := b.ID
					o.Confirm("Remove this budget?", budgetTitle(b.TokenBudget, o)+": its alerts and what it does at 100% stop.", "Remove", func() {
						st := d.Settings
						st.Budgets = slices.DeleteFunc(slices.Clone(st.Budgets), func(x proto.TokenBudget) bool { return x.ID == id })
						o.Save(st)
					})
				}
			})
			widget.Meter(c, widget.Segment{Fraction: min(b.Fraction, 1), Color: widget.Level(b.Fraction)})
			ui.Text(c, budgetUsed(b)).FontSize(11).TextColor(theme.Muted)
		})
	}
}

func budgetTitle(b proto.TokenBudget, o Options) string {
	what := "everything"
	switch b.Scope {
	case proto.BudgetAgent:
		what = b.Target
		if o.Agent != nil {
			what = o.Agent(b.Target)
		}
	case proto.BudgetConfig:
		what = "config " + b.Target
	case proto.BudgetProject:
		what = "project " + b.Target
	}
	var limits []string
	if b.Tokens > 0 {
		limits = append(limits, Count(b.Tokens)+" tokens")
	}
	if b.Cost > 0 {
		limits = append(limits, Dollars(b.Cost))
	}
	period := map[string]string{proto.BudgetDay: "a day", proto.BudgetWeek: "a week", proto.BudgetMonth: "a month"}[b.Period]
	return fmt.Sprintf("%s %s for %s", strings.Join(limits, " or "), period, what)
}

func budgetUsed(b proto.TokenBudgetState) string {
	at := atNames[max(0, slices.Index(atLimits, orText(b.AtLimit, proto.BudgetNotify)))]
	return fmt.Sprintf("%s tokens and %s used since %s · at 100%%: %s", Count(b.Used.Total()), Dollars(b.Used.Cost), b.Since.Local().Format("Mon Jan 2"), strings.ToLower(at))
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
	widget.FormSheet(c, &f.open, 520, title, "Tokens, a cost in dollars, or both: the first reached counts.", func() {
		widget.Field(c, "Per", func() {
			if i := widget.Segmented(c.Key("period"), periodNames, f.period, nil); i >= 0 {
				f.period = i
			}
		})
		widget.Field(c, "For", func() {
			if i := widget.Segmented(c.Key("scope"), scopeNames, f.scope, nil); i >= 0 {
				f.scope = i
			}
			if scopes[f.scope] != proto.BudgetAll {
				hint := map[string]string{proto.BudgetAgent: "The agent's ID: claude, codex…", proto.BudgetConfig: "The config's name", proto.BudgetProject: "The project's folder"}[scopes[f.scope]]
				widget.Input(c.Key("target"), &f.target, hint).FillWidth()
			}
		})
		ui.Row(c).FillWidth().Gap(10).Children(func() {
			ui.Column(c).Grow(1).Children(func() {
				widget.Field(c, "Tokens", func() { widget.Input(c.Key("tokens"), &f.tokens, "5000000").FillWidth() })
			})
			ui.Column(c).Grow(1).Children(func() {
				widget.Field(c, "Cost, US dollars", func() { widget.Input(c.Key("cost"), &f.cost, "20").FillWidth() })
			})
		})
		widget.Field(c, "At 100%", func() {
			if i := widget.Segmented(c.Key("at"), atNames, f.at, nil); i >= 0 {
				f.at = i
			}
		})
		if f.err != "" {
			ui.Text(c, f.err).FontSize(12).TextColor(theme.Error)
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

package tokens

import (
	"sync"
	"time"

	"github.com/enowx/enowx/proto"
)

// PeriodStart is when the period of now began: the day's midnight, the
// week's Monday, the month's first, in now's time zone.
func PeriodStart(period string, now time.Time) time.Time {
	y, m, d := now.Date()
	midnight := time.Date(y, m, d, 0, 0, 0, 0, now.Location())
	switch period {
	case proto.BudgetWeek:
		back := (int(now.Weekday()) + 6) % 7 // days since Monday
		return midnight.AddDate(0, 0, -back)
	case proto.BudgetMonth:
		return time.Date(y, m, 1, 0, 0, 0, 0, now.Location())
	}
	return midnight
}

// applies reports whether call c counts against budget b: enowX's own use
// never does.
func applies(b proto.TokenBudget, c Call) bool {
	if c.Own != "" {
		return false
	}
	switch b.Scope {
	case proto.BudgetAgent:
		return c.Agent == b.Target
	case proto.BudgetConfig:
		return c.Config == b.Target
	case proto.BudgetProject:
		return c.Project == b.Target
	}
	return true
}

// Budgets is how much of each budget is used at now.
func (s *Store) Budgets(budgets []proto.TokenBudget, prices *Prices, now time.Time) []proto.TokenBudgetState {
	out := make([]proto.TokenBudgetState, 0, len(budgets))
	for _, b := range budgets {
		st := proto.TokenBudgetState{TokenBudget: b, Since: PeriodStart(b.Period, now)}
		for _, c := range s.Calls(st.Since, now.Add(time.Minute)) {
			if applies(b, c) {
				st.Used.Add(c.Counts(prices))
			}
		}
		if b.Tokens > 0 {
			st.Fraction = float64(st.Used.Total()) / float64(b.Tokens)
		}
		if b.Cost > 0 {
			st.Fraction = max(st.Fraction, st.Used.Cost/b.Cost)
		}
		switch {
		case st.Fraction >= 1:
			st.Level = 100
		case st.Fraction >= 0.8:
			st.Level = 80
		}
		out = append(out, st)
	}
	return out
}

// AtLimit is the strictest of what the budgets at 100% that a new
// session of agent in project (with config) falls under ask: BudgetStop,
// BudgetAsk, else "" (notify only, or none at its limit), and the budget.
func AtLimit(states []proto.TokenBudgetState, agent, project, config string) (string, proto.TokenBudgetState) {
	c := Call{Agent: agent, Project: project, Config: config}
	action, which := "", proto.TokenBudgetState{}
	for _, st := range states {
		if st.Level < 100 || !applies(st.TokenBudget, c) {
			continue
		}
		switch st.AtLimit {
		case proto.BudgetStop:
			return proto.BudgetStop, st
		case proto.BudgetAsk:
			action, which = proto.BudgetAsk, st
		}
	}
	return action, which
}

// Near reports whether a budget is at 80% or more: GOA and the judge
// then pick cheaper models first.
func Near(states []proto.TokenBudgetState) bool {
	for _, st := range states {
		if st.Level >= 80 {
			return true
		}
	}
	return false
}

// Alerts tells budgets crossing 80% and 100% once per period. Safe from
// any goroutine.
type Alerts struct {
	mu   sync.Mutex
	seen map[string]int // budget ID and period start → level told
}

// Crossed returns the states whose level rose since last told.
func (a *Alerts) Crossed(states []proto.TokenBudgetState) []proto.TokenBudgetState {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.seen == nil {
		a.seen = map[string]int{}
	}
	var out []proto.TokenBudgetState
	for _, st := range states {
		k := st.ID + "@" + st.Since.Format(time.RFC3339)
		if st.Level > a.seen[k] {
			a.seen[k] = st.Level
			out = append(out, st)
		}
	}
	return out
}

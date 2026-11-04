package platformagent

import (
	"context"

	"github.com/enowx/enowx/core/tokens"
)

// account adds an answer's cost to the session's, at list prices: this answer's and the
// total now. It is safe from subagents running side by side.
func (a *Agent) account(ans *answer) (cost, total float64) {
	u := ans.usage
	model := ans.model
	if model == "" {
		model = a.currentModel()
	}
	price, priced := a.prices.Cost(tokens.Call{
		Provider:   "anthropic",
		Model:      model,
		Input:      u.Input,
		CacheRead:  u.CacheRead,
		CacheWrite: u.CacheWrite,
		Output:     u.Output,
	})
	a.mu.Lock()
	a.total += price
	total = a.total
	sayUnpriced := !priced && !a.unpriced
	if !priced {
		a.unpriced = true
	}
	a.mu.Unlock()
	if sayUnpriced {
		a.notice("info", "There is no price for "+model+", so its cost isn't counted.")
	}
	return price, total
}

func (a *Agent) currentModel() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.model
}

// mayGoOn holds the budget between requests: past it, the user says. A yes raises the
// limit by another budget. The error is an interruption, ctx ending while it asked.
func (a *Agent) mayGoOn(ctx context.Context) (bool, error) {
	if !a.overBudget() {
		return true, nil
	}
	release, err := a.takeAskSlot(ctx)
	if err != nil {
		return false, err
	}
	defer release()
	// Another subagent may have asked while this one waited.
	a.mu.Lock()
	spent, limit, budget := a.total, a.limit, a.budget
	a.mu.Unlock()
	if budget <= 0 || spent < limit {
		return true, nil
	}
	yes, err := a.front.OverBudget(ctx, spent, limit)
	if err != nil {
		return false, err
	}
	if !yes {
		return false, nil
	}
	a.mu.Lock()
	for a.budget > 0 && a.limit <= a.total {
		a.limit += a.budget
	}
	a.mu.Unlock()
	return true, nil
}

func (a *Agent) overBudget() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.budget > 0 && a.total >= a.limit
}

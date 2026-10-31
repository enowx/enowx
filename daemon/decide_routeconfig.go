package daemon

import (
	"fmt"

	"github.com/enowx/enowx/core/decide"
	"github.com/enowx/enowx/core/tokens"
	"github.com/enowx/enowx/proto"
)

// cheaperConfig is the agent's model API config (another provider) whose
// model costs less than the agent's own model chosen for a prompt of
// difficulty, when the prompt isn't hard: a hard one keeps the agent's
// own. ok false when none is cheaper, or the prices aren't known.
func (s *Server) cheaperConfig(agentID, difficulty, ownModel string) (proto.Account, string, string, bool) {
	if difficulty == proto.VerdictHard || ownModel == "" {
		return proto.Account{}, "", "", false
	}
	prices := tokens.NewPrices(s.tok.settings.Get().Prices)
	unit := func(p proto.TokenPrice) float64 { return p.Input + p.Output/4 } // prompts read more than they write
	own, ok := prices.Find(decide.AgentProvider(agentID), ownModel, "")
	if !ok {
		return proto.Account{}, "", "", false
	}
	best, bestModel, bestCost := proto.Account{}, "", unit(own)
	for _, c := range s.accounts.List() {
		if c.Agent != agentID || c.Kind != proto.ConfigKindAPI || c.API == nil {
			continue
		}
		model := c.API.Model
		if difficulty == proto.VerdictEasy && c.API.SmallModel != "" {
			model = c.API.SmallModel
		}
		p, ok := prices.Find(decide.ProviderOfURL(c.API.BaseURL), model, c.ID)
		if !ok {
			continue
		}
		if cost := unit(p); cost < bestCost {
			best, bestModel, bestCost = c, model, cost
		}
	}
	if bestModel == "" {
		return proto.Account{}, "", "", false
	}
	why := fmt.Sprintf("the config %s (%s) costs less than %s for it", best.Name, bestModel, ownModel)
	return best, bestModel, why, true
}

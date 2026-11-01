package accounts

import (
	"testing"

	"github.com/enowx/enowx/core/agent"
)

// A new agent can't be added without saying how it takes a config, or why not:
// the list in Settings → Agents reads this.
func TestEveryAgentSaysHowItTakesAConfig(t *testing.T) {
	for _, info := range agent.Presets {
		way, ok := agentWays[info.ID]
		if !ok {
			t.Errorf("%s isn't in agentWays: add how it takes a config, or why it can't", info.ID)
			continue
		}
		if way.Checked == "" {
			t.Errorf("%s doesn't say how it was checked", info.ID)
		}
		if w := WayOf(info.ID); w.None() && w.Why == "" {
			t.Errorf("%s has no config and no reason", info.ID)
		}
	}
}

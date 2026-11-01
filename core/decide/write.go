package decide

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/enowx/enowx/proto"
)

// Write has a judge that writes (an agent's one-shot command, a model API
// or a local model; never a decision model, which only picks) answer user
// under system, for what the user asked to have written, such as rules.
// Agents come first, the strongest writers; the policy's local only keeps
// it to the host. It says the text and the judge's label.
func (e *Engine) Write(ctx context.Context, system, user string) (string, string, error) {
	st := e.settings.Get()
	var writers []proto.Judge
	for _, j := range e.Judges(ctx) {
		if j.Kind == proto.JudgeDecision || !j.Tested.IsZero() && !j.OK {
			continue
		}
		if st.Policy == proto.DecideLocalOnly && j.Provider != proto.ProviderLocal {
			continue
		}
		writers = append(writers, j)
	}
	rank := map[string]int{proto.JudgeAgent: 0, proto.JudgeAPI: 1, proto.JudgeLocal: 2}
	slices.SortStableFunc(writers, func(a, b proto.Judge) int { return rank[a.Kind] - rank[b.Kind] })
	if len(writers) == 0 {
		return "", "", errors.New("decide: no AI to write with: install Claude Code or Codex, or add a model in Settings → Decisions")
	}
	var errs []string
	for _, j := range writers {
		a, err := e.call(ctx, j, system, user)
		if err == nil && strings.TrimSpace(a.text) != "" {
			return strings.TrimSpace(a.text), j.Label, nil
		}
		if err == nil {
			err = errors.New("no answer")
		}
		errs = append(errs, j.Label+": "+err.Error())
		if ctx.Err() != nil {
			break
		}
	}
	return "", "", fmt.Errorf("decide: none could write: %s", strings.Join(errs, "; "))
}

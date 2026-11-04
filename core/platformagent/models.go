package platformagent

import (
	"encoding/json"
	"slices"
	"strings"
)

// modelSpec is what the API takes of a model, from its published table.
type modelSpec struct {
	window    int64
	maxTokens int64
	thinking  string // the JSON of the request's thinking field
	efforts   []string
	// fallback: refusals may go to another model server-side.
	fallback bool
}

const (
	thinkingSummarized = `{"type":"adaptive","display":"summarized"}`
	// The 4.6 models summarize by default and take no display option here.
	thinkingAdaptive = `{"type":"adaptive"}`
	// Models before 4.6 take a fixed budget.
	thinkingBudget = `{"type":"enabled","budget_tokens":16000}`

	million = 1_000_000
)

var (
	allEfforts   = []string{"low", "medium", "high", "xhigh", "max"}
	noXHigh      = []string{"low", "medium", "high", "max"}
	threeEfforts = []string{"low", "medium", "high"}

	current = modelSpec{window: million, maxTokens: 64000, thinking: thinkingSummarized, efforts: allEfforts}
)

// modelSpecs go by the model ID's beginning; the longest that matches wins.
var modelSpecs = map[string]modelSpec{
	"claude-fable-5-1":  withFallback(current),
	"claude-mythos-5-1": current,
	"claude-fable-5":    current,
	"claude-mythos-5":   current,
	"claude-opus-5-5":   withFallback(current),
	"claude-opus-5":     withFallback(current),
	"claude-opus-4-8":   current,
	"claude-opus-4-7":   current,
	"claude-sonnet-5-5": withFallback(current),
	"claude-sonnet-5":   current,
	"claude-haiku-5-5":  current,
	"claude-opus-4-6":   {window: million, maxTokens: 64000, thinking: thinkingAdaptive, efforts: noXHigh},
	"claude-sonnet-4-6": {window: million, maxTokens: 64000, thinking: thinkingAdaptive, efforts: noXHigh},
	"claude-opus-4-5":   {window: 200_000, maxTokens: 32000, thinking: thinkingBudget, efforts: threeEfforts},
	"claude-sonnet-4-5": {window: 200_000, maxTokens: 32000, thinking: thinkingBudget},
	"claude-haiku-4-5":  {window: 200_000, maxTokens: 32000, thinking: thinkingBudget},
	"claude-opus-4":     {window: 200_000, maxTokens: 32000, thinking: thinkingBudget},
	"claude-sonnet-4":   {window: 200_000, maxTokens: 32000, thinking: thinkingBudget},
}

func withFallback(s modelSpec) modelSpec {
	s.fallback = true
	return s
}

// specOf is a model's spec. An ID not known is taken for a newer model, with the
// smaller window so that compaction comes early rather than too late.
func specOf(model string) modelSpec {
	best, bestLen := modelSpec{}, -1
	for prefix, s := range modelSpecs {
		if strings.HasPrefix(model, prefix) && len(prefix) > bestLen {
			best, bestLen = s, len(prefix)
		}
	}
	if bestLen < 0 {
		s := current
		s.window = 200_000
		return s
	}
	return best
}

// outputConfig is the request's output_config, nil where the model takes no effort.
func (s modelSpec) outputConfig(effort string) json.RawMessage {
	if len(s.efforts) == 0 || effort == "" {
		return nil
	}
	if !slices.Contains(s.efforts, effort) {
		// Asked for more than the model has: its highest below.
		i := slices.Index(allEfforts, effort)
		for i > 0 && !slices.Contains(s.efforts, effort) {
			i--
			effort = allEfforts[i]
		}
	}
	return marshalStable(map[string]string{"effort": effort})
}

func validEffort(effort string) bool { return slices.Contains(allEfforts, effort) }

package tokens

import (
	_ "embed"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/enowx/enowx/proto"
)

// shippedJSON is enowX's price table, US dollars per million tokens, as
// the providers listed them (October 2026); the user's prices go over it.
//
//go:embed prices.json
var shippedJSON []byte

// Shipped is enowX's price table.
func Shipped() []proto.TokenPrice {
	var list []proto.TokenPrice
	if err := json.Unmarshal(shippedJSON, &list); err != nil {
		panic("tokens: prices.json: " + err.Error())
	}
	for i := range list {
		list[i].Shipped = true
	}
	return list
}

// Prices prices calls: the user's prices first, then the shipped ones.
type Prices struct {
	own, shipped []proto.TokenPrice
}

// NewPrices is the shipped table with own over it.
func NewPrices(own []proto.TokenPrice) *Prices { return &Prices{own: own, shipped: Shipped()} }

// All is every price, the user's first.
func (p *Prices) All() []proto.TokenPrice {
	return append(append([]proto.TokenPrice{}, p.own...), p.shipped...)
}

// Cost is what call c cost at its model's price, and whether one was
// found. A local model's is free.
func (p *Prices) Cost(c Call) (float64, bool) {
	if c.Provider == "local" {
		return 0, true
	}
	pr, ok := p.Find(c.Provider, c.Model, c.Config)
	if !ok {
		return 0, false
	}
	m := 1e-6
	return (float64(c.Input)*pr.Input + float64(c.CacheRead)*pr.CacheRead +
		float64(c.CacheWrite)*pr.CacheWrite + float64(c.Output)*pr.Output) * m, true
}

// Find is the price of model of provider in config: the user's that
// matches it most closely, else the shipped one's.
func (p *Prices) Find(provider, model, config string) (proto.TokenPrice, bool) {
	m := NormalModel(model)
	if m == "" {
		return proto.TokenPrice{}, false
	}
	for _, list := range [][]proto.TokenPrice{p.own, p.shipped} {
		best, bestLen := -1, -1
		for i, pr := range list {
			if pr.Config != "" && pr.Config != config {
				continue
			}
			if pr.Provider != "" && !pr.Shipped && provider != "" && pr.Provider != provider {
				continue
			}
			if pr.Provider == "local" && provider != "local" {
				continue
			}
			n := matchLen(strings.ToLower(pr.Model), m)
			if pr.Config != "" {
				n += 1000 // a config's own price wins
			}
			if n > bestLen {
				best, bestLen = i, n
			}
		}
		if best >= 0 {
			return list[best], true
		}
	}
	return proto.TokenPrice{}, false
}

// matchLen is how much of model pattern matches: its length when equal,
// its prefix's when a pattern ending in "*" matches, -1 otherwise.
func matchLen(pattern, model string) int {
	if prefix, ok := strings.CutSuffix(pattern, "*"); ok {
		if strings.HasPrefix(model, prefix) {
			return len(prefix)
		}
		return -1
	}
	if pattern == model {
		return len(pattern) + 1
	}
	return -1
}

var claudeDots = regexp.MustCompile(`^(claude-[a-z]+-)(\d+)\.(\d+)`)

// NormalModel is a model ID as the price table names it: lower case, its
// provider's prefix ("anthropic/", "us.anthropic.") and suffixes
// ("[1m]", "@date", ":tag") dropped, Claude's versions with dashes.
func NormalModel(model string) string {
	m := strings.ToLower(strings.TrimSpace(model))
	if i := strings.LastIndex(m, "/"); i >= 0 {
		m = m[i+1:]
	}
	for _, p := range []string{"us.anthropic.", "eu.anthropic.", "apac.anthropic.", "global.anthropic.", "anthropic."} {
		m = strings.TrimPrefix(m, p)
	}
	if i := strings.IndexAny(m, "[@:"); i > 0 {
		m = m[:i]
	}
	return claudeDots.ReplaceAllString(m, "$1$2-$3")
}

// ProviderOf guesses the provider of a model by its name, for records
// that don't say.
func ProviderOf(model string) string {
	m := NormalModel(model)
	switch {
	case strings.HasPrefix(m, "claude"):
		return "anthropic"
	case strings.HasPrefix(m, "gpt") || strings.HasPrefix(m, "o1") || strings.HasPrefix(m, "o3") || strings.HasPrefix(m, "o4") || strings.HasPrefix(m, "codex"):
		return "openai"
	case strings.HasPrefix(m, "gemini"):
		return "google"
	case strings.HasPrefix(m, "deepseek"):
		return "deepseek"
	case strings.HasPrefix(m, "grok"):
		return "xai"
	case strings.HasPrefix(m, "kimi"):
		return "moonshot"
	case strings.HasPrefix(m, "glm"):
		return "zhipu"
	case strings.HasPrefix(m, "qwen"):
		return "alibaba"
	}
	return ""
}

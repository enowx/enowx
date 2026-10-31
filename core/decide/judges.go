package decide

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/proto"
)

// agentJudges are the agents whose one-shot command can judge, and their
// cheapest models first (as their model lists name them).
var agentJudges = []struct {
	agent, bin, provider string
	models               []string // substrings of their model IDs, in the order offered
}{
	{"claude", "claude", proto.ProviderAnthropic, []string{"haiku-5-5", "sonnet-5-5"}},
	{"codex", "codex", proto.ProviderOpenAI, []string{"luna"}},
	{"gemini", "gemini", proto.ProviderGoogle, nil},
}

// apiModels are the model a provider's API judges with by default, cheap
// and fast.
var apiModels = map[string]string{
	proto.ProviderAnthropic:  "claude-haiku-5-5",
	proto.ProviderOpenAI:     "gpt-6-luna",
	proto.ProviderGoogle:     "gemini-flash-latest",
	proto.ProviderOpenRouter: "anthropic/claude-haiku-5-5",
}

// vaultProviders are the vault's platforms whose keys reach a model API.
var vaultProviders = map[string]string{
	"anthropic": proto.ProviderAnthropic, "openai": proto.ProviderOpenAI, "gemini": proto.ProviderGoogle, "openrouter": proto.ProviderOpenRouter,
}

// LocalServers are where model servers on the host listen: Ollama's, LM
// Studio's (OpenAI-compatible both).
var LocalServers = []string{"http://127.0.0.1:11434/v1", "http://127.0.0.1:1234/v1"}

// AgentProvider is the provider of agent id's models, "" for an agent
// enowX doesn't know.
func AgentProvider(id string) string {
	base := agent.Base(id)
	for _, a := range agentJudges {
		if a.agent == base {
			return a.provider
		}
	}
	return ""
}

// Found is what the host has that can judge, besides the user's own.
type Found struct {
	// LookPath finds a program; Creds are the vault's credentials (names
	// and platforms, no values); Local are model servers' addresses to try.
	LookPath func(string) (string, error)
	Creds    []proto.CredentialInfo
	Local    []string
	// LocalDecision are decision model servers' addresses to try.
	LocalDecision []string
	// AgentAPI is the model API agent is set to instead of its own
	// (core/agentapi), nil for its own: its one-shot command goes there
	// too, and the API judges directly (its key read as AgentAPICred).
	AgentAPI func(agent string) *AgentAPI
}

// AgentAPI is an agent's model API, its key left out.
type AgentAPI struct {
	BaseURL, Model, Protocol string
}

// AgentAPICred is the credential name standing for agent's API key, read
// from the agent's own settings rather than the vault.
func AgentAPICred(agent string) string { return "agentapi:" + agent }

// ProviderOfURL is the provider an API's address belongs to: a known
// service's, ProviderLocal for this host, "other" for anyone else.
func ProviderOfURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "other"
	}
	h := strings.ToLower(u.Hostname())
	switch {
	case h == "localhost" || h == "127.0.0.1" || h == "::1":
		return proto.ProviderLocal
	case strings.HasSuffix(h, "anthropic.com"):
		return proto.ProviderAnthropic
	case strings.HasSuffix(h, "openai.com"):
		return proto.ProviderOpenAI
	case strings.HasSuffix(h, "googleapis.com"):
		return proto.ProviderGoogle
	case strings.HasSuffix(h, "openrouter.ai"):
		return proto.ProviderOpenRouter
	}
	return "other"
}

// EffectiveProvider is the provider agent's models come from: its model
// API's when set to one, else its own.
func (f Found) EffectiveProvider(agent string) string {
	if f.AgentAPI != nil {
		if api := f.AgentAPI(agent); api != nil {
			return ProviderOfURL(api.BaseURL)
		}
	}
	return AgentProvider(agent)
}

// Detect lists the judges found: agents installed (their sign-in is
// tested, not assumed), the vault's model API keys, local servers' models.
func Detect(ctx context.Context, f Found) []proto.Judge {
	out := detectDecision(ctx, f) // first: made for this
	for _, a := range agentJudges {
		if f.LookPath == nil {
			break
		}
		if _, err := f.LookPath(a.bin); err != nil {
			continue
		}
		models := []string{""} // the agent's own default
		if a.models != nil {
			models = nil
			for _, want := range a.models {
				for _, m := range agent.SettingsOf(a.agent).Models {
					if strings.Contains(m.ID, want) && !slices.Contains(models, m.ID) {
						models = append(models, m.ID)
					}
				}
			}
		}
		provider := f.EffectiveProvider(a.agent)
		if f.AgentAPI != nil {
			if api := f.AgentAPI(a.agent); api != nil {
				// Set to another API: its models, not the agent's own.
				models = []string{""}
				if m := api.Model; m != "" {
					out = append(out, proto.Judge{ID: "api:" + AgentAPICred(a.agent) + ":" + m, Kind: proto.JudgeAPI, Provider: provider, Cred: AgentAPICred(a.agent),
						URL: api.BaseURL, Model: m, Protocol: api.Protocol, Label: m + " · the API " + agentName(a.agent) + " is set to"})
				}
			}
		}
		for _, m := range models {
			label := "its default model"
			for _, c := range agent.SettingsOf(a.agent).Models {
				if c.ID == m {
					label = c.Label
				}
			}
			out = append(out, proto.Judge{ID: "agent:" + a.agent + ":" + m, Kind: proto.JudgeAgent, Provider: provider, Agent: a.agent, Model: m,
				Label: strings.TrimSpace(agentName(a.agent) + " · " + label)})
		}
	}
	for _, c := range f.Creds {
		p, ok := vaultProviders[c.Platform]
		if !ok {
			continue
		}
		out = append(out, proto.Judge{ID: "api:" + c.Name + ":" + apiModels[p], Kind: proto.JudgeAPI, Provider: p, Cred: c.Name, Model: apiModels[p],
			Label: apiModels[p] + " · " + c.Name})
	}
	for _, addr := range f.Local {
		for _, m := range localModels(ctx, addr) {
			out = append(out, proto.Judge{ID: "local:" + addr + ":" + m, Kind: proto.JudgeLocal, Provider: proto.ProviderLocal, URL: addr, Model: m,
				Label: m + " · " + strings.TrimPrefix(strings.TrimPrefix(addr, "http://"), "https://")})
		}
	}
	return out
}

func agentName(id string) string {
	if p, ok := agent.Find(id); ok {
		return p.Name
	}
	return id
}

// localModels are the models of the OpenAI-compatible server at addr, at
// most 6; none when nothing answers there.
func localModels(ctx context.Context, addr string) []string {
	ctx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(addr, "/")+"/models", nil)
	if err != nil {
		return nil
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var list struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(resp.Body).Decode(&list) != nil {
		return nil
	}
	var out []string
	for _, m := range list.Data {
		if strings.Contains(strings.ToLower(m.ID), "embed") {
			continue // an embedding model can't answer
		}
		out = append(out, m.ID)
		if len(out) == 6 {
			break
		}
	}
	return out
}

// ownJudge is judge j the user added, its ID and label made.
func ownJudge(j proto.Judge) proto.Judge {
	j.Own = true
	switch j.Kind {
	case proto.JudgeDecision:
		if j.Provider == "" || strings.HasPrefix(j.URL, "http://127.0.0.1") || strings.HasPrefix(j.URL, "http://localhost") {
			j.Provider = ProviderOfURL(j.URL)
		}
		j.ID = "decision:" + j.Cred + "@" + j.URL + ":" + j.Model
	case proto.JudgeLocal:
		j.Provider = proto.ProviderLocal
		j.ID = "local:" + j.URL + ":" + j.Model
	default:
		j.ID = "api:" + j.Cred + ":" + j.Model
		if j.URL != "" {
			j.ID = "api:" + j.Cred + "@" + j.URL + ":" + j.Model
		}
	}
	if j.Label == "" {
		j.Label = j.Model
	}
	return j
}

// Ordered are judges in the order st tries them: its Order first, the
// rest after (agents, then APIs, then local, as found).
func Ordered(st proto.DecideSettings, judges []proto.Judge) []proto.Judge {
	out := slices.Clone(judges)
	rank := func(id string) int {
		if i := slices.Index(st.Order, id); i >= 0 {
			return i
		}
		return len(st.Order)
	}
	decisionFirst := func(j proto.Judge) int {
		if j.Kind == proto.JudgeDecision {
			return 0
		}
		return 1
	}
	slices.SortStableFunc(out, func(a, b proto.Judge) int {
		if d := rank(a.ID) - rank(b.ID); d != 0 {
			return d
		}
		return decisionFirst(a) - decisionFirst(b) // made for this: asked first unless ordered
	})
	return out
}

// allowed says whether judge j may judge for an agent of provider agentP
// under policy (and provider, for DecideOneProvider).
func allowed(j proto.Judge, policy, provider, agentP string) bool {
	if j.Provider == proto.ProviderLocal {
		return true // nothing leaves the host
	}
	if madeToDecide(j) && policy != proto.DecideLocalOnly {
		return true // a decision model the user set up judges for every agent
	}
	switch policy {
	case proto.DecideAnyProvider:
		return true
	case proto.DecideOneProvider:
		return j.Provider == provider
	case proto.DecideLocalOnly:
		return false
	}
	return agentP != "" && j.Provider == agentP // the session's agent's own
}

// madeToDecide says whether judge j is a decision model set up for this:
// one the user added, or a service that only decides (TypeSafe's Jev,
// Cloudflare's Clef). OpenAI's Decisions, reached with a key that is the
// user's for everything OpenAI, stays under the policy like OpenAI's chat.
func madeToDecide(j proto.Judge) bool {
	return j.Kind == proto.JudgeDecision && (j.Own || j.Provider == proto.ProviderTypeSafe || j.Provider == proto.ProviderCloudflare)
}

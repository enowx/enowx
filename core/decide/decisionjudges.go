package decide

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/enowx/enowx/proto"
)

// Decision models found on a host, asked before every other judge: the
// vault's keys of providers that serve them, and the servers that run them
// on the host.

// decisionVault are the vault's platforms whose keys reach decision
// models, and the models each serves (the fast one first).
var decisionVault = map[string]struct {
	provider, protocol string
	models             []string
}{
	"typesafe":   {proto.ProviderTypeSafe, "", []string{"jev-latest"}},
	"cloudflare": {proto.ProviderCloudflare, "", []string{"clef-flash", "clef"}},
	"openai":     {proto.ProviderOpenAI, DecisionOpenAI, []string{"gpt-6-luna"}},
}

// decisionNames are the names local servers give decision models (Ollama's
// library, Kev's).
var decisionNames = []string{"nimble", "clef", "clef-flash", "tev1", "kev"}

// LocalDecisionServers are where decision model servers listen on a host:
// Ollama's, Kev's, SGLang's (Clef), all speaking System One.
var LocalDecisionServers = []string{"http://127.0.0.1:11434", "http://127.0.0.1:8009", "http://127.0.0.1:30000"}

// DecisionPresets are the providers offered when adding a decision model,
// with their System One address ("" the user's own) and model ids.
var DecisionPresets = []proto.Judge{
	{Provider: proto.ProviderTypeSafe, Label: "TypeSafe (Jev)", URL: "https://api.typesafe.ai/v1/systemone", Model: "jev-latest"},
	{Provider: proto.ProviderCloudflare, Label: "Cloudflare Workers AI (Clef)", URL: "https://api.cloudflare.com/client/v4/accounts/{account}/ai/run/@cf/cloudflare/{model}", Model: "clef-flash"},
	{Provider: "opper", Label: "Opper (Jev, Clef, Decider, Nimble…)", URL: "https://api.opper.ai/v3/compat/v1/systemone", Model: "cloudflare:global/clef-flash"},
	{Provider: proto.ProviderOpenRouter, Label: "OpenRouter (System One)", URL: "", Model: "typesafe/jev-latest"},
	{Provider: "ollama", Label: "Ollama Cloud", URL: "https://ollama.com/v1/systemone", Model: "clef-flash"},
	{Provider: proto.ProviderOpenAI, Label: "OpenAI Decisions API", URL: "https://api.openai.com/v1/decisions", Model: "gpt-6-luna", Protocol: DecisionOpenAI},
	{Provider: proto.ProviderLocal, Label: "On this host (Ollama, Kev, SGLang)", URL: "http://127.0.0.1:11434/v1/systemone", Model: "nimble"},
	{Provider: "custom", Label: "Another System One address", URL: "", Model: ""},
}

// detectDecision lists the decision models f's host reaches.
func detectDecision(ctx context.Context, f Found) []proto.Judge {
	var out []proto.Judge
	for _, c := range f.Creds {
		d, ok := decisionVault[c.Platform]
		if !ok {
			continue
		}
		for _, m := range d.models {
			out = append(out, proto.Judge{ID: "decision:" + c.Name + ":" + m, Kind: proto.JudgeDecision, Provider: d.provider, Cred: c.Name,
				Model: m, Protocol: d.protocol, Label: m + " · decision model · " + c.Name})
		}
	}
	for _, addr := range f.LocalDecision {
		for _, m := range localDecisionModels(ctx, addr) {
			url := addr + "/v1/systemone"
			out = append(out, proto.Judge{ID: "decision:" + url + ":" + m, Kind: proto.JudgeDecision, Provider: proto.ProviderLocal, URL: url, Model: m,
				Label: m + " · decision model · " + strings.TrimPrefix(addr, "http://")})
		}
	}
	return out
}

// localDecisionModels are the decision models of the server at addr:
// Ollama's by name, a System One server's (Kev, SGLang) as it lists them.
func localDecisionModels(ctx context.Context, addr string) []string {
	ctx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
	defer cancel()
	get := func(path string, out any) bool {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, addr+path, nil)
		if err != nil {
			return false
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		return resp.StatusCode == http.StatusOK && json.NewDecoder(resp.Body).Decode(out) == nil
	}
	var tags struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if get("/api/tags", &tags) && tags.Models != nil { // Ollama
		var out []string
		for _, m := range tags.Models {
			base, _, _ := strings.Cut(m.Name, ":")
			if slices.ContainsFunc(decisionNames, func(n string) bool { return base == n || strings.HasPrefix(base, n+"-") }) {
				out = append(out, m.Name)
			}
		}
		return out
	}
	var list struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if get("/v1/models", &list) {
		var out []string
		for _, m := range list.Data {
			out = append(out, m.ID)
		}
		if len(out) == 0 && strings.HasSuffix(addr, ":8009") {
			out = []string{"kev-latest"}
		}
		return out
	}
	return nil
}

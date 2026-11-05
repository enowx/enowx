package agentapi

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// OMP reads its providers and models from models.yml in its agent folder
// (PI_CODING_AGENT_DIR, ~/.omp/agent by default), and an apiKey that names a
// variable is taken from the environment (resolveConfigValue, $envExact), so a
// session's file can name the key without holding it. A folder of the
// session's own is the only way to give it another models.yml: there is no
// flag for a second one, and --config takes a config.yml overlay alone.

// omp is OMP's way of taking an API: per session alone, its own files never
// written, as a key written there would be a key in a file.
var omp = adapter{
	protocols: []string{OpenAI, OpenAIResponses, Anthropic},
	caps:      listCaps,
	note:      "It needs the model's name. A config's sessions read only the config's models: your own ~/.omp settings, skills and sign-in don't reach them.",
}

// ompKeyVar holds the key of an OMP session's API: its models.yml names it.
const ompKeyVar = "ENOWX_OMP_API_KEY"

// ompAPIs are the protocols OMP speaks, by its names for them.
var ompAPIs = map[string]string{OpenAI: "openai-completions", OpenAIResponses: "openai-responses", Anthropic: "anthropic-messages"}

func init() {
	sessionOnly["omp"] = omp
	sessionAdapters["omp"] = ompSession
}

// ompSession: an agent folder of the session's own (PI_CODING_AGENT_DIR), with
// a models.yml holding the config's provider and models, and the model named on
// its command line. The user's ~/.omp/agent is left as it is.
func ompSession(api API, dir string) (Launch, error) {
	if api.Model == "" {
		return Launch{}, errors.New("agentapi: OMP needs the model's name")
	}
	agentDir := filepath.Join(dir, "agent")
	if err := os.MkdirAll(agentDir, 0o700); err != nil {
		return Launch{}, err
	}
	if err := os.WriteFile(filepath.Join(agentDir, "models.yml"), ompModels(api), 0o600); err != nil {
		return Launch{}, err
	}
	// A folder of its own would run OMP's setup wizard first, asking for the
	// model the session already has.
	return Launch{
		Env:  kv([2]string{"PI_CODING_AGENT_DIR", agentDir}, [2]string{"OMP_SKIP_SETUP", "1"}, [2]string{ompKeyVar, api.APIKey}),
		Args: []string{"--model", ourName + "/" + api.Model},
	}, nil
}

// ompModels is models.yml for api: one provider, enowX's, with the API's
// address, protocol and models, its key named rather than written.
func ompModels(api API) []byte {
	var b strings.Builder
	b.WriteString("providers:\n  " + ourName + ":\n")
	b.WriteString("    baseUrl: " + yamlQuote(api.BaseURL) + "\n")
	b.WriteString("    api: " + yamlQuote(ompAPIs[api.Protocol]) + "\n")
	if api.APIKey != "" {
		b.WriteString("    apiKey: " + yamlQuote(ompKeyVar) + "\n")
	} else {
		b.WriteString("    auth: none\n") // a server that asks for no key
	}
	b.WriteString("    models:\n")
	for _, m := range api.Models {
		b.WriteString("      - id: " + yamlQuote(m.ID) + "\n")
		b.WriteString("        name: " + yamlQuote(m.ID) + "\n")
		if m.Context > 0 {
			b.WriteString("        contextWindow: " + strconv.Itoa(m.Context) + "\n")
		}
		if m.Output > 0 {
			b.WriteString("        maxTokens: " + strconv.Itoa(m.Output) + "\n")
		}
		if m.Vision {
			b.WriteString("        input: [\"text\", \"image\"]\n")
		}
	}
	return []byte(b.String())
}

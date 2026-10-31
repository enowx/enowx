package agentapi

import "strconv"

// Agents that take their API from the environment: the variables go to the
// sessions enowX starts of them (envFile, LaunchEnv). Gemini CLI reads
// its own .env file the same way.

func readEnv(home, id string) (string, error) {
	data, err := readFile(home, envFile(id))
	return string(data), err
}

func setEnv(home, id string, values [][2]string) error {
	text, err := readEnv(home, id)
	if err != nil {
		return err
	}
	return writeFile(home, envFile(id), []byte(envSet(text, values)))
}

func clearEnv(home, id string, keys ...string) error {
	var values [][2]string
	for _, k := range keys {
		values = append(values, [2]string{k, ""})
	}
	return setEnv(home, id, values)
}

// protocolVar records, in an agent's variables, the protocol they are of;
// sessions don't get it.
const protocolVar = "ENOWX_API_PROTOCOL"

// Markers vars is given to find which variable holds what.
const (
	markURL   = "\x01url"
	markKey   = "\x01key"
	markModel = "\x01model"
	// Token counts found by these, as no variable holds them otherwise.
	probeContext = 970_000_001
	probeOutput  = 970_000_002
)

// envOnly is an agent that takes its API from the environment only. vars
// are its variables for api, by protocol; a variable it doesn't take is
// set "".
func envOnly(id string, protocols []string, caps Caps, vars func(api API) [][2]string, note string) adapter {
	keys := func() []string {
		out := []string{protocolVar}
		for _, p := range protocols {
			for _, kv := range vars(API{Protocol: p}) {
				out = append(out, kv[0])
			}
		}
		return out
	}
	return adapter{
		protocols: protocols,
		caps:      caps,
		files:     []string{envFile(id)},
		note:      note,
		env:       vars,
		read: func(home string) (API, bool, error) {
			text, err := readEnv(home, id)
			if err != nil {
				return API{}, false, err
			}
			p := envValue(text, protocolVar)
			if p == "" {
				return API{}, false, nil
			}
			api, m := API{Protocol: p}, Model{}
			marked := API{Protocol: p, BaseURL: markURL, APIKey: markKey, Model: markModel, Models: []Model{{ID: markModel}}}
			for _, kv := range vars(marked) {
				v := envValue(text, kv[0])
				switch kv[1] {
				case markURL:
					api.BaseURL = v
				case markKey:
					api.APIKey = v
				case markModel:
					api.Model, m.ID = v, v
				}
			}
			// Token counts are numbers: they are found by name.
			for _, kv := range vars(API{Protocol: p, Models: []Model{{ID: "m", Context: probeContext, Output: probeOutput}}, Model: "m"}) {
				switch kv[1] {
				case strconv.Itoa(probeContext):
					m.Context, _ = strconv.Atoi(envValue(text, kv[0]))
				case strconv.Itoa(probeOutput):
					m.Output, _ = strconv.Atoi(envValue(text, kv[0]))
				}
			}
			if m.ID != "" {
				api.Models = []Model{m}
			}
			return api, api.BaseURL != "", nil
		},
		write: func(home string, api API) error {
			if err := clearEnv(home, id, keys()...); err != nil {
				return err
			}
			return setEnv(home, id, append(vars(api), [2]string{protocolVar, api.Protocol}))
		},
		clear: func(home string) error { return clearEnv(home, id, keys()...) },
	}
}

var copilot = envOnly("copilot", []string{OpenAI, OpenAIResponses, Anthropic}, Caps{Context: "global", Output: "global"}, func(api API) [][2]string {
	typ, wire := "openai", ""
	switch api.Protocol {
	case Anthropic:
		typ = "anthropic"
	case OpenAIResponses:
		wire = "responses"
	}
	m := api.model(api.Model)
	return [][2]string{{"COPILOT_PROVIDER_BASE_URL", api.BaseURL}, {"COPILOT_PROVIDER_TYPE", typ}, {"COPILOT_PROVIDER_API_KEY", api.APIKey},
		{"COPILOT_PROVIDER_WIRE_API", wire}, {"COPILOT_MODEL", api.Model},
		{"COPILOT_PROVIDER_MAX_PROMPT_TOKENS", itoa(m.Context)}, {"COPILOT_PROVIDER_MAX_OUTPUT_TOKENS", itoa(m.Output)}}
}, "GitHub Copilot takes it from the environment: the sessions enowX starts of it get it.")

var openClaude = envOnly("openclaude", []string{OpenAI, Anthropic}, Caps{}, func(api API) [][2]string {
	if api.Protocol == Anthropic {
		return [][2]string{{"ANTHROPIC_BASE_URL", api.BaseURL}, {"ANTHROPIC_AUTH_TOKEN", api.APIKey}, {"ANTHROPIC_MODEL", api.Model}}
	}
	return [][2]string{{"CLAUDE_CODE_USE_OPENAI", "1"}, {"OPENAI_BASE_URL", api.BaseURL}, {"OPENAI_API_KEY", api.APIKey}, {"OPENAI_MODEL", api.Model}}
}, "OpenClaude takes it from the environment: the sessions enowX starts of it get it.")

var antigravity = func() adapter {
	a := envOnly("antigravity", []string{Gemini}, Caps{}, func(api API) [][2]string {
		return [][2]string{{"GEMINI_API_KEY", api.APIKey}, {"GOOGLE_GEMINI_BASE_URL", api.BaseURL}}
	}, "Antigravity speaks Gemini's API only, with its own models; the sessions enowX starts of it get it.")
	a.noModel = true
	return a
}()

// Gemini CLI: its .env for the key and the address, its settings for the
// API key sign-in and the model.
var geminiCLI = adapter{
	protocols: []string{Gemini},
	files:     []string{".gemini/.env", ".gemini/settings.json"},
	note:      "Gemini CLI speaks Gemini's API only; it signs in with the key instead of a Google account.",
	read: func(home string) (API, bool, error) {
		data, err := readFile(home, ".gemini/.env")
		if err != nil {
			return API{}, false, err
		}
		url := envValue(string(data), "GOOGLE_GEMINI_BASE_URL")
		if url == "" {
			return API{}, false, nil
		}
		api := API{BaseURL: url, APIKey: envValue(string(data), "GEMINI_API_KEY"), Protocol: Gemini}
		if model, _ := jsonString(home, ".gemini/settings.json", "model", "name"); model != "" {
			api.Model, api.Models = model, []Model{{ID: model}}
		}
		return api, true, nil
	},
	write: func(home string, api API) error {
		data, err := readFile(home, ".gemini/.env")
		if err != nil {
			return err
		}
		text := envSet(string(data), [][2]string{{"GOOGLE_GEMINI_BASE_URL", api.BaseURL}, {"GEMINI_API_KEY", api.APIKey}})
		if err := writeFile(home, ".gemini/.env", []byte(text)); err != nil {
			return err
		}
		edits := []jsonEdit{{path: []string{"security", "auth", "selectedType"}, value: "gemini-api-key"}}
		if api.Model != "" {
			edits = append(edits, jsonEdit{path: []string{"model", "name"}, value: api.Model})
		}
		return editJSON(home, ".gemini/settings.json", edits...)
	},
	clear: func(home string) error {
		data, err := readFile(home, ".gemini/.env")
		if err != nil {
			return err
		}
		return writeFile(home, ".gemini/.env", []byte(envSet(string(data), [][2]string{{"GOOGLE_GEMINI_BASE_URL", ""}, {"GEMINI_API_KEY", ""}})))
	},
}

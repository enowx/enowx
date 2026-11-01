package agentapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Agents that read this from their docs alone, none run here (docs/configs.md):
// a session's API by its environment, or by a file of the session's own that
// names the variable holding the key, never its value.

// sessionConfig writes agent id's settings for api under dir, its key given as
// keyRef, which the agent reads from the environment; a key-less API stays so.
// It says where the settings are.
func sessionConfig(id, keyRef string, api API, dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	if api.APIKey != "" {
		api.APIKey = keyRef
	}
	a := adapters[id]
	if err := a.write(dir, api); err != nil {
		return "", err
	}
	return filepath.Join(dir, a.files[0]), nil
}

// kiloKeyVar holds the key of a Kilo session's API: its config names it.
const kiloKeyVar = "ENOWX_KILO_API_KEY"

// kiloSession: a config in KILO_CONFIG_CONTENT, which Kilo merges over the
// user's and over a project's, where KILO_CONFIG would lose to the project's.
func kiloSession(api API, dir string) (Launch, error) {
	path, err := sessionConfig("kilo", "{env:"+kiloKeyVar+"}", api, dir)
	if err != nil {
		return Launch{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Launch{}, err
	}
	return Launch{Env: kv([2]string{"KILO_CONFIG_CONTENT", string(data)}, [2]string{kiloKeyVar, api.APIKey})}, nil
}

// continueKeyVar holds the key of a Continue session's API: its config names
// it as a secret, which its CLI reads from the environment.
const continueKeyVar = "ENOWX_CONTINUE_API_KEY"

// continueSession: a config of the session's own (--config).
func continueSession(api API, dir string) (Launch, error) {
	path, err := sessionConfig("continue", "${{ secrets."+continueKeyVar+" }}", api, dir)
	if err != nil {
		return Launch{}, err
	}
	return Launch{Env: kv([2]string{continueKeyVar, api.APIKey}), Args: []string{"--config", path}}, nil
}

// qwenSession: the variables of the protocol, and its --auth-type and --model:
// a selectedType or model.name in the user's settings would win over them.
func qwenSession(api API, _ string) (Launch, error) {
	if api.Model == "" {
		return Launch{}, errors.New("agentapi: Qwen Code needs the model's name")
	}
	prefix, auth := "OPENAI", "openai"
	if api.Protocol == Anthropic {
		prefix, auth = "ANTHROPIC", "anthropic"
	}
	return Launch{
		Env:  kv([2]string{prefix + "_API_KEY", api.APIKey}, [2]string{prefix + "_BASE_URL", api.BaseURL}, [2]string{prefix + "_MODEL", api.Model}),
		Args: []string{"--auth-type", auth, "--model", api.Model},
	}, nil
}

// kimiSession: the provider Kimi makes in memory from KIMI_MODEL_*, over its
// config's default_model. It won't start with a name and no key.
func kimiSession(api API, _ string) (Launch, error) {
	if api.Model == "" {
		return Launch{}, errors.New("agentapi: Kimi needs the model's name")
	}
	if api.APIKey == "" {
		return Launch{}, errors.New("agentapi: Kimi needs a key for a model API")
	}
	m := api.model(api.Model)
	caps := ""
	if m.Vision {
		caps = "image_in"
	}
	return Launch{Env: kv([2]string{"KIMI_MODEL_NAME", api.Model}, [2]string{"KIMI_MODEL_API_KEY", api.APIKey},
		[2]string{"KIMI_MODEL_BASE_URL", api.BaseURL}, [2]string{"KIMI_MODEL_PROVIDER_TYPE", kimiTypes[api.Protocol]},
		[2]string{"KIMI_MODEL_MAX_CONTEXT_SIZE", itoa(m.Context)}, [2]string{"KIMI_MODEL_CAPABILITIES", caps})}, nil
}

// gooseSession: the variables its docs name for a provider, and those its
// OpenAI and Anthropic providers' source reads: the docs' weren't found there,
// and either way the key goes to the config's address alone.
func gooseSession(api API, _ string) (Launch, error) {
	if api.Model == "" {
		return Launch{}, errors.New("agentapi: Goose needs the model's name")
	}
	provider, host, key := "openai", "OPENAI_BASE_URL", "OPENAI_API_KEY"
	if api.Protocol == Anthropic {
		provider, host, key = "anthropic", "ANTHROPIC_HOST", "ANTHROPIC_API_KEY"
	}
	m := api.model(api.Model)
	return Launch{Env: kv([2]string{"GOOSE_PROVIDER", provider}, [2]string{"GOOSE_MODEL", api.Model},
		[2]string{"GOOSE_PROVIDER__TYPE", provider}, [2]string{"GOOSE_PROVIDER__HOST", api.BaseURL}, [2]string{"GOOSE_PROVIDER__API_KEY", api.APIKey},
		[2]string{host, api.BaseURL}, [2]string{key, api.APIKey},
		[2]string{"GOOSE_CONTEXT_LIMIT", itoa(m.Context)}, [2]string{"GOOSE_MAX_TOKENS", itoa(m.Output)})}, nil
}

// crushKeyVar holds the key of a Crush session's API: its config names it.
const crushKeyVar = "ENOWX_CRUSH_API_KEY"

// crush is kept in variables as the other agents taking an API per session
// are; its sessions get a config made from them (crushSession).
var crush = envOnly("crush", []string{OpenAI, Anthropic}, Caps{Context: "global", Output: "global"}, func(api API) [][2]string {
	m := api.model(api.Model)
	return [][2]string{{"ENOWX_CRUSH_BASE_URL", api.BaseURL}, {crushKeyVar, api.APIKey}, {"ENOWX_CRUSH_MODEL", api.Model},
		{"ENOWX_CRUSH_CONTEXT", itoa(m.Context)}, {"ENOWX_CRUSH_OUTPUT", itoa(m.Output)}}
}, "It needs the model's name. Its sessions read only the config's settings, not your own crush.json.")

// crushSession: a global config of the session's own. CRUSH_GLOBAL_CONFIG and
// CRUSH_GLOBAL_DATA are folders, each holding a crush.json; the data one moves
// too, as a model picked in Crush is kept there and would win over the config's.
func crushSession(api API, dir string) (Launch, error) {
	if api.Model == "" {
		return Launch{}, errors.New("agentapi: Crush needs the model's name")
	}
	typ := "openai-compat" // a name it migrates, so that old versions read it too
	if api.Protocol == Anthropic {
		typ = "anthropic"
	}
	m := api.model(api.Model)
	model := map[string]any{"id": m.ID, "name": m.ID}
	if m.Context > 0 {
		model["context_window"] = m.Context
	}
	if m.Output > 0 {
		model["default_max_tokens"] = m.Output
	}
	provider := map[string]any{"name": "enowX: " + api.BaseURL, "type": typ, "base_url": api.BaseURL, "models": []any{model}}
	if api.APIKey != "" {
		provider["api_key"] = "$" + crushKeyVar
	}
	selected := map[string]any{"model": m.ID, "provider": ourName}
	data, err := json.MarshalIndent(map[string]any{
		"providers": map[string]any{ourName: provider},
		"models":    map[string]any{"large": selected, "small": selected},
	}, "", "  ")
	if err != nil {
		return Launch{}, fmt.Errorf("agentapi: Crush's config: %w", err)
	}
	conf, state := filepath.Join(dir, "crush"), filepath.Join(dir, "crush-data")
	for _, d := range []string{conf, state} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return Launch{}, err
		}
	}
	if err := os.WriteFile(filepath.Join(conf, "crush.json"), data, 0o600); err != nil {
		return Launch{}, err
	}
	return Launch{Env: kv([2]string{"CRUSH_GLOBAL_CONFIG", conf}, [2]string{"CRUSH_GLOBAL_DATA", state}, [2]string{crushKeyVar, api.APIKey})}, nil
}

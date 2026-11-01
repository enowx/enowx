package agentapi

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// Agents whose settings are TOML or YAML, and Continue's config of
// enowX's.

// Codex: a provider of its config for the API, the default model at the
// top with its effort and context window, a profile "enowx-<model>" for
// each other (codex --profile); its key from a variable the sessions enowX
// starts get, as Codex reads keys from the environment only.
const codexKeyVar = "ENOWX_CODEX_API_KEY"

var codex = adapter{
	protocols: []string{OpenAIResponses},
	files:     []string{".codex/config.toml", envFile("codex")},
	caps:      withEfforts(Caps{Multi: true, Context: "global"}, "model", "none", "minimal", "low", "medium", "high", "xhigh"),
	note:      "Codex speaks OpenAI's Responses API only: the API must serve /responses. The other models are profiles: codex --profile enowx-<model>. Its key reaches the sessions enowX starts.",
	read: func(home string) (API, bool, error) {
		data, err := readFile(home, ".codex/config.toml")
		if err != nil {
			return API{}, false, err
		}
		text := string(data)
		if tomlTop(text, "model_provider") != ourName {
			return API{}, false, nil
		}
		p := tomlTable(text, "model_providers."+ourName)
		env, _ := readEnv(home, "codex")
		api := API{BaseURL: p["base_url"], Model: tomlTop(text, "model"), APIKey: envValue(env, codexKeyVar), Protocol: OpenAIResponses}
		ctx, _ := strconv.Atoi(tomlTop(text, "model_context_window"))
		api.Models = []Model{{ID: api.Model, Effort: tomlTop(text, "model_reasoning_effort"), Context: ctx}}
		for _, name := range tomlTableNames(text, "profiles."+ourName+"-") {
			t := tomlTable(text, name)
			api.Models = append(api.Models, Model{ID: t["model"], Effort: t["model_reasoning_effort"]})
		}
		return api, true, nil
	},
	write: func(home string, api API) error {
		data, err := readFile(home, ".codex/config.toml")
		if err != nil {
			return err
		}
		def := api.model(api.Model)
		text := tomlSetTop(string(data), [][2]string{{"model_provider", ourName}, {"model", api.Model},
			{"model_reasoning_effort", def.Effort}, {"model_context_window", ""}})
		text = tomlSetTopRaw(text, "model_context_window", itoa(def.Context))
		text = tomlSetTable(text, "model_providers."+ourName, fmt.Sprintf("name = %s\nbase_url = %s\nenv_key = %s\nwire_api = \"responses\"",
			tomlQuote("enowX: "+api.BaseURL), tomlQuote(api.BaseURL), tomlQuote(codexKeyVar)))
		text = tomlRemoveTables(text, "profiles."+ourName+"-")
		for _, m := range api.Models[1:] {
			body := fmt.Sprintf("model_provider = %s\nmodel = %s", tomlQuote(ourName), tomlQuote(m.ID))
			if m.Effort != "" {
				body += "\nmodel_reasoning_effort = " + tomlQuote(m.Effort)
			}
			text = tomlSetTable(text, "profiles."+ourName+"-"+slug(m.ID), body)
		}
		if err := writeFile(home, ".codex/config.toml", []byte(text)); err != nil {
			return err
		}
		return setEnv(home, "codex", [][2]string{{codexKeyVar, api.APIKey}})
	},
	clear: func(home string) error {
		data, err := readFile(home, ".codex/config.toml")
		if err != nil {
			return err
		}
		text := string(data)
		values := [][2]string{{"model_provider", ""}}
		if tomlTop(text, "model_provider") == ourName {
			// The model and its options were the API's.
			values = append(values, [2]string{"model", ""}, [2]string{"model_reasoning_effort", ""}, [2]string{"model_context_window", ""})
		}
		text = tomlSetTable(tomlSetTop(text, values), "model_providers."+ourName, "")
		text = tomlRemoveTables(text, "profiles."+ourName+"-")
		if err := writeFile(home, ".codex/config.toml", []byte(text)); err != nil {
			return err
		}
		return clearEnv(home, "codex", codexKeyVar)
	},
}

// Kimi: a provider of its config, a model table for each model, with its
// context and images, the default's its default. Its config moved to
// ~/.kimi-code: what enowX wrote in ~/.kimi before is still found, to be
// taken out.
const (
	kimiFile    = ".kimi-code/config.toml"
	kimiOldFile = ".kimi/config.toml"
)

var kimi = adapter{
	// What its sessions take (kimiSession): one model, by two protocols.
	protocols: []string{OpenAI, Anthropic},
	files:     []string{kimiFile, kimiOldFile},
	caps:      Caps{Vision: true, Context: "model"},
	note:      "It needs the model's name and a key.",
	read: func(home string) (API, bool, error) {
		for _, file := range []string{kimiFile, kimiOldFile} {
			data, err := readFile(home, file)
			if err != nil {
				return API{}, false, err
			}
			if api, ok := kimiRead(string(data)); ok {
				return api, true, nil
			}
		}
		return API{}, false, nil
	},
	write: func(home string, api API) error {
		if api.Model == "" {
			return fmt.Errorf("agentapi: Kimi needs the model's name")
		}
		data, err := readFile(home, kimiFile)
		if err != nil {
			return err
		}
		text := tomlSetTop(string(data), [][2]string{{"default_model", ourName + "-" + slug(api.Model)}})
		text = tomlSetTable(text, "providers."+ourName, fmt.Sprintf("type = %s\nbase_url = %s\napi_key = %s", tomlQuote(kimiTypes[api.Protocol]), tomlQuote(api.BaseURL), tomlQuote(api.APIKey)))
		text = tomlRemoveTables(text, "models."+ourName+"-")
		for _, m := range api.Models {
			body := fmt.Sprintf("provider = %s\nmodel = %s\nmax_context_size = %d", tomlQuote(ourName), tomlQuote(m.ID), orDefault(m.Context, 131072))
			if m.Vision {
				body += "\ncapabilities = [\"image_in\"]"
			}
			text = tomlSetTable(text, "models."+ourName+"-"+slug(m.ID), body)
		}
		return writeFile(home, kimiFile, []byte(text))
	},
	clear: func(home string) error {
		for _, file := range []string{kimiFile, kimiOldFile} {
			data, err := readFile(home, file)
			if err != nil {
				return err
			}
			text := string(data)
			if tomlTable(text, "providers."+ourName) == nil {
				continue // nothing of enowX's: a file that isn't there stays so
			}
			if strings.HasPrefix(tomlTop(text, "default_model"), ourName+"-") {
				text = tomlSetTop(text, [][2]string{{"default_model", ""}})
			}
			text = tomlRemoveTables(tomlSetTable(text, "providers."+ourName, ""), "models."+ourName+"-")
			if err := writeFile(home, file, []byte(text)); err != nil {
				return err
			}
		}
		return nil
	},
}

// kimiRead is the API a Kimi config holds of enowX's, ok false for none.
func kimiRead(text string) (API, bool) {
	p := tomlTable(text, "providers."+ourName)
	if p == nil {
		return API{}, false
	}
	api := API{BaseURL: p["base_url"], APIKey: p["api_key"]}
	for _, types := range []map[string]string{kimiTypes, kimiOldTypes} {
		for proto, typ := range types {
			if typ == p["type"] {
				api.Protocol = proto
			}
		}
	}
	def := tomlTop(text, "default_model")
	for _, name := range tomlTableNames(text, "models."+ourName+"-") {
		t := tomlTable(text, name)
		ctx, _ := strconv.Atoi(t["max_context_size"])
		m := Model{ID: t["model"], Context: ctx, Vision: strings.Contains(tomlRawValue(text, name, "capabilities"), `"image_in"`)}
		if m.Context == 131072 {
			m.Context = 0 // enowX's stand-in
		}
		if strings.TrimPrefix(name, "models.") == def {
			api.Model = m.ID
		}
		api.Models = append(api.Models, m)
	}
	sortModels(&api)
	return api, true
}

var (
	kimiTypes = map[string]string{OpenAI: "openai", OpenAIResponses: "openai_responses", Anthropic: "anthropic", Gemini: "google-genai"}
	// kimiOldTypes are the names enowX wrote in ~/.kimi.
	kimiOldTypes = map[string]string{OpenAI: "openai_legacy", Gemini: "gemini"}
)

// Mistral Vibe: a provider of its config, a model for each model (alias
// "enowx-<model>") with its images, limits and thinking, the default's
// alias active; the key in its .env, which Vibe reads.
const vibeKeyVar = "ENOWX_VIBE_API_KEY"

var vibe = adapter{
	protocols: []string{OpenAI, OpenAIResponses, Anthropic},
	files:     []string{".vibe/config.toml", ".vibe/.env"},
	caps:      withEfforts(listCaps, "model", "off", "low", "medium", "high", "max"),
	note:      "It needs the model's name.",
	read: func(home string) (API, bool, error) {
		data, err := readFile(home, ".vibe/config.toml")
		if err != nil {
			return API{}, false, err
		}
		text := string(data)
		p := tomlArrayItem(text, "providers", "name", ourName)
		if p == nil {
			return API{}, false, nil
		}
		env, _ := readFile(home, ".vibe/.env")
		api := API{BaseURL: p["api_base"], APIKey: envValue(string(env), vibeKeyVar), Protocol: p["api_style"]}
		active := tomlTop(text, "active_model")
		for _, it := range tomlArrayItemsWhere(text, "models", "provider", ourName) {
			ctx, _ := strconv.Atoi(it["max_context_length"])
			out, _ := strconv.Atoi(it["max_output_tokens"])
			m := Model{ID: it["name"], Context: ctx, Output: out, Vision: it["supports_images"] == "true", Effort: it["thinking"]}
			if it["alias"] == active {
				api.Model = m.ID
			}
			api.Models = append(api.Models, m)
		}
		sortModels(&api)
		return api, true, nil
	},
	write: func(home string, api API) error {
		if api.Model == "" {
			return fmt.Errorf("agentapi: Mistral Vibe needs the model's name")
		}
		data, err := readFile(home, ".vibe/config.toml")
		if err != nil {
			return err
		}
		text := vibeRemove(string(data))
		text = tomlSetTop(text, [][2]string{{"active_model", ourName + "-" + slug(api.Model)}})
		var b strings.Builder
		b.WriteString(strings.TrimRight(text, "\n"))
		fmt.Fprintf(&b, "\n\n[[providers]]\nname = %s\napi_base = %s\napi_key_env_var = %s\napi_style = %s\nbackend = \"generic\"\n",
			tomlQuote(ourName), tomlQuote(api.BaseURL), tomlQuote(vibeKeyVar), tomlQuote(api.Protocol))
		for _, m := range api.Models {
			fmt.Fprintf(&b, "\n[[models]]\nname = %s\nprovider = %s\nalias = %s\n", tomlQuote(m.ID), tomlQuote(ourName), tomlQuote(ourName+"-"+slug(m.ID)))
			if m.Vision {
				b.WriteString("supports_images = true\n")
			}
			if m.Context > 0 {
				fmt.Fprintf(&b, "max_context_length = %d\n", m.Context)
			}
			if m.Output > 0 {
				fmt.Fprintf(&b, "max_output_tokens = %d\n", m.Output)
			}
			if m.Effort != "" {
				fmt.Fprintf(&b, "thinking = %s\n", tomlQuote(m.Effort))
			}
		}
		if err := writeFile(home, ".vibe/config.toml", []byte(b.String())); err != nil {
			return err
		}
		env, err := readFile(home, ".vibe/.env")
		if err != nil {
			return err
		}
		return writeFile(home, ".vibe/.env", []byte(envSet(string(env), [][2]string{{vibeKeyVar, api.APIKey}})))
	},
	clear: func(home string) error {
		data, err := readFile(home, ".vibe/config.toml")
		if err != nil {
			return err
		}
		text := vibeRemove(string(data))
		if strings.HasPrefix(tomlTop(text, "active_model"), ourName+"-") {
			text = tomlSetTop(text, [][2]string{{"active_model", ""}})
		}
		if err := writeFile(home, ".vibe/config.toml", []byte(text)); err != nil {
			return err
		}
		env, err := readFile(home, ".vibe/.env")
		if err != nil {
			return err
		}
		return writeFile(home, ".vibe/.env", []byte(envSet(string(env), [][2]string{{vibeKeyVar, ""}})))
	},
}

func vibeRemove(text string) string {
	return tomlRemoveArrayItem(tomlRemoveArrayItem(text, "providers", "name", ourName), "models", "provider", ourName)
}

// Goose: a custom provider file of enowX's with its models (context,
// images), chosen in its config with the default; one thinking effort and
// one output limit for all, in its config; its key from a variable the
// sessions enowX starts get (Goose ignores keys in its config).
const gooseKeyVar = "ENOWX_GOOSE_API_KEY"

const gooseProvider = ".config/goose/custom_providers/enowx.json"

var goose = adapter{
	protocols: []string{OpenAI, Anthropic},
	files:     []string{".config/goose/config.yaml", gooseProvider, envFile("goose")},
	// What its sessions take (gooseSession): one model, by its variables.
	caps: Caps{Context: "global", Output: "global"},
	note: "It needs the model's name.",
	read: func(home string) (API, bool, error) {
		data, err := readFile(home, ".config/goose/config.yaml")
		if err != nil || yamlTop(string(data), "GOOSE_PROVIDER") != ourName {
			return API{}, false, err
		}
		conf := string(data)
		url, _ := jsonString(home, gooseProvider, "base_url")
		engine, _ := jsonString(home, gooseProvider, "engine")
		env, _ := readEnv(home, "goose")
		api := API{BaseURL: strings.TrimSuffix(url, "/chat/completions"), Model: yamlTop(conf, "GOOSE_MODEL"), APIKey: envValue(env, gooseKeyVar),
			Effort: yamlTop(conf, "GOOSE_THINKING_EFFORT"), Protocol: OpenAI}
		if engine == "anthropic" {
			api.Protocol = Anthropic
		}
		list, _ := jsonAny(home, gooseProvider, "models")
		items, _ := list.([]any)
		out, _ := strconv.Atoi(yamlTop(conf, "GOOSE_MAX_TOKENS"))
		for _, it := range items {
			e, _ := it.(map[string]any)
			m := Model{ID: str(e["name"]), Context: num(e["context_limit"]), Vision: e["supports_vision"] == true}
			if m.Context == 128000 {
				m.Context = 0 // enowX's stand-in
			}
			if m.ID == api.Model {
				m.Output = out
			}
			api.Models = append(api.Models, m)
		}
		sortModels(&api)
		return api, true, nil
	},
	write: func(home string, api API) error {
		if api.Model == "" {
			return fmt.Errorf("agentapi: Goose needs the model's name")
		}
		engine, url := "openai", api.BaseURL
		if api.Protocol == Anthropic {
			engine = "anthropic"
		} else if !strings.HasSuffix(url, "/chat/completions") {
			url += "/chat/completions" // Goose's openai engine takes the endpoint itself
		}
		var models []any
		for _, m := range api.Models {
			models = append(models, map[string]any{"name": m.ID, "context_limit": orDefault(m.Context, 128000), "supports_vision": m.Vision})
		}
		if err := removeFile(home, gooseProvider); err != nil {
			return err
		}
		if err := editJSON(home, gooseProvider,
			jsonEdit{path: []string{"name"}, value: ourName}, jsonEdit{path: []string{"display_name"}, value: "enowX: " + api.BaseURL},
			jsonEdit{path: []string{"engine"}, value: engine}, jsonEdit{path: []string{"api_key_env"}, value: gooseKeyVar},
			jsonEdit{path: []string{"base_url"}, value: url}, jsonEdit{path: []string{"models"}, value: models}); err != nil {
			return err
		}
		data, err := readFile(home, ".config/goose/config.yaml")
		if err != nil {
			return err
		}
		conf := yamlSetTop(string(data), [][2]string{{"GOOSE_PROVIDER", ourName}, {"GOOSE_MODEL", api.Model},
			{"GOOSE_THINKING_EFFORT", api.Effort}, {"GOOSE_MAX_TOKENS", ""}})
		if out := api.model(api.Model).Output; out > 0 {
			conf = "GOOSE_MAX_TOKENS: " + strconv.Itoa(out) + "\n" + conf
		}
		if err := writeFile(home, ".config/goose/config.yaml", []byte(conf)); err != nil {
			return err
		}
		return setEnv(home, "goose", [][2]string{{gooseKeyVar, api.APIKey}})
	},
	clear: func(home string) error {
		data, err := readFile(home, ".config/goose/config.yaml")
		if err != nil {
			return err
		}
		if yamlTop(string(data), "GOOSE_PROVIDER") == ourName {
			data = []byte(yamlSetTop(string(data), [][2]string{{"GOOSE_PROVIDER", ""}, {"GOOSE_MODEL", ""}, {"GOOSE_THINKING_EFFORT", ""}, {"GOOSE_MAX_TOKENS", ""}}))
			if err := writeFile(home, ".config/goose/config.yaml", data); err != nil {
				return err
			}
		}
		if err := removeFile(home, gooseProvider); err != nil {
			return err
		}
		return clearEnv(home, "goose", gooseKeyVar)
	},
}

// Aider: its home config's OpenAI-compatible address, key, model and weak
// model, one reasoning effort; each model's limits in its model metadata.
const aiderMeta = ".aider.model.metadata.json"

var aider = adapter{
	protocols: []string{OpenAI},
	files:     []string{".aider.conf.yml", aiderMeta},
	small:     true,
	caps:      withEfforts(Caps{Context: "model", Output: "model"}, "global", "low", "medium", "high"),
	note:      "It needs the model's name; the model for quick work is its weak model. A project's own .aider.conf.yml wins over this one.",
	read: func(home string) (API, bool, error) {
		data, err := readFile(home, ".aider.conf.yml")
		if err != nil {
			return API{}, false, err
		}
		text := string(data)
		url := yamlTop(text, "openai-api-base")
		if url == "" {
			return API{}, false, nil
		}
		model, _ := strings.CutPrefix(yamlTop(text, "model"), "openai/")
		weak, _ := strings.CutPrefix(yamlTop(text, "weak-model"), "openai/")
		api := API{BaseURL: url, APIKey: yamlTop(text, "openai-api-key"), Model: model, SmallModel: weak, Effort: yamlTop(text, "reasoning-effort"), Protocol: OpenAI}
		m := Model{ID: model}
		if meta, _ := jsonAny(home, aiderMeta, "openai/"+model); meta != nil {
			mm, _ := meta.(map[string]any)
			m.Context, m.Output = num(mm["max_input_tokens"]), num(mm["max_output_tokens"])
		}
		api.Models = []Model{m}
		return api, true, nil
	},
	write: func(home string, api API) error {
		if api.Model == "" {
			return fmt.Errorf("agentapi: Aider needs the model's name")
		}
		if err := aiderClearMeta(home); err != nil {
			return err
		}
		data, err := readFile(home, ".aider.conf.yml")
		if err != nil {
			return err
		}
		weak := ""
		if api.SmallModel != "" {
			weak = "openai/" + api.SmallModel
		}
		text := yamlSetTop(string(data), [][2]string{{"openai-api-base", api.BaseURL}, {"openai-api-key", api.APIKey}, {"model", "openai/" + api.Model},
			{"weak-model", weak}, {"reasoning-effort", api.Effort}})
		if err := writeFile(home, ".aider.conf.yml", []byte(text)); err != nil {
			return err
		}
		if m := api.model(api.Model); m.Context > 0 || m.Output > 0 {
			meta := map[string]any{"litellm_provider": "openai", "mode": "chat"}
			if m.Context > 0 {
				meta["max_input_tokens"] = m.Context
			}
			if m.Output > 0 {
				meta["max_output_tokens"] = m.Output
			}
			return editJSON(home, aiderMeta, jsonEdit{path: []string{"openai/" + m.ID}, value: meta})
		}
		return nil
	},
	clear: func(home string) error {
		if err := aiderClearMeta(home); err != nil {
			return err
		}
		data, err := readFile(home, ".aider.conf.yml")
		if err != nil {
			return err
		}
		text := string(data)
		values := [][2]string{{"openai-api-base", ""}, {"openai-api-key", ""}}
		if strings.HasPrefix(yamlTop(text, "model"), "openai/") {
			values = append(values, [2]string{"model", ""}, [2]string{"weak-model", ""}, [2]string{"reasoning-effort", ""})
		}
		return writeFile(home, ".aider.conf.yml", []byte(yamlSetTop(text, values)))
	},
}

// aiderClearMeta removes the metadata of the model enowX set.
func aiderClearMeta(home string) error {
	data, err := readFile(home, ".aider.conf.yml")
	if err != nil {
		return err
	}
	model := yamlTop(string(data), "model")
	if yamlTop(string(data), "openai-api-base") == "" || model == "" {
		return nil
	}
	if meta, _ := jsonAny(home, aiderMeta, model); meta == nil {
		return nil
	}
	if err := editJSON(home, aiderMeta, jsonEdit{path: []string{model}}); err != nil {
		return err
	}
	if rest, _ := readFile(home, aiderMeta); strings.TrimSpace(string(rest)) == "{}" {
		return removeFile(home, aiderMeta)
	}
	return nil
}

// Continue: a config of enowX's, which the sessions enowX starts of it are
// given (--config), the user's own left as it is; the API as enowX set it
// beside it, to read back.
var (
	continueFile = filepath.Join(".enowx-app", "agentapi", "continue.yaml")
	continueAPI  = filepath.Join(".enowx-app", "agentapi", "continue.json")
)

var continueCLI = adapter{
	protocols: []string{OpenAI, Anthropic, Gemini},
	files:     []string{continueFile, continueAPI},
	small:     true,
	caps:      listCaps,
	note:      "It needs the model's name. The sessions enowX starts of it use it; your own config stays as it is.",
	args: func(home string) []string {
		if data, _ := readFile(home, continueFile); len(data) > 0 {
			return []string{"--config", filepath.Join(home, continueFile)}
		}
		return nil
	},
	read: func(home string) (API, bool, error) {
		data, err := readFile(home, continueAPI)
		if err != nil || len(data) == 0 {
			return API{}, false, err
		}
		var api API
		if err := json.Unmarshal(data, &api); err != nil {
			return API{}, false, err
		}
		return api, true, nil
	},
	write: func(home string, api API) error {
		if api.Model == "" {
			return fmt.Errorf("agentapi: Continue needs the model's name")
		}
		var b strings.Builder
		b.WriteString("name: enowX\nversion: 0.0.1\nschema: v1\nmodels:\n")
		models := api.Models
		if api.SmallModel != "" && !slices.ContainsFunc(models, func(m Model) bool { return m.ID == api.SmallModel }) {
			models = append(models, Model{ID: api.SmallModel})
		}
		for i, m := range models {
			roles := "[chat, edit, apply]"
			switch {
			case m.ID == api.SmallModel && i == 0:
				roles = "[chat, edit, apply, autocomplete, summarize]"
			case m.ID == api.SmallModel:
				roles = "[autocomplete, summarize]"
			case i > 0:
				roles = "[chat, edit]"
			}
			caps := "[tool_use]"
			if m.Vision {
				caps = "[tool_use, image_input]"
			}
			fmt.Fprintf(&b, "  - name: %s\n    provider: %s\n    model: %s\n    apiBase: %s\n    apiKey: %s\n    roles: %s\n    capabilities: %s\n",
				yamlQuote(m.ID), api.Protocol, yamlQuote(m.ID), yamlQuote(api.BaseURL), yamlQuote(api.APIKey), roles, caps)
			if m.Context > 0 || m.Output > 0 {
				b.WriteString("    defaultCompletionOptions:\n")
				if m.Context > 0 {
					fmt.Fprintf(&b, "      contextLength: %d\n", m.Context)
				}
				if m.Output > 0 {
					fmt.Fprintf(&b, "      maxTokens: %d\n", m.Output)
				}
			}
		}
		if err := writeFile(home, continueFile, []byte(b.String())); err != nil {
			return err
		}
		data, _ := json.Marshal(api)
		return writeFile(home, continueAPI, data)
	},
	clear: func(home string) error {
		if err := removeFile(home, continueFile); err != nil {
			return err
		}
		return removeFile(home, continueAPI)
	},
}

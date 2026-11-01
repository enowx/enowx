package agentapi

import (
	"fmt"
	"slices"
	"strings"
)

// Agents whose settings are JSON.

// Claude Code: its settings' env block for the API, the default model,
// the quick one and the context and output limits (one for every model);
// its model picker for the others; modelSettings for each one's effort.
var claudeCode = adapter{
	protocols: []string{Anthropic},
	files:     []string{".claude/settings.json"},
	small:     true,
	caps:      withEfforts(Caps{Multi: true, Context: "global", Output: "global"}, "model", "low", "medium", "high", "xhigh"),
	note:      "A key set here wins over the claude.ai sign-in, which stays saved for when it is cleared. The other models show in /model.",
	read: func(home string) (API, bool, error) {
		const f = ".claude/settings.json"
		url, err := jsonString(home, f, "env", "ANTHROPIC_BASE_URL")
		if err != nil || url == "" {
			return API{}, false, err
		}
		env := func(k string) string { s, _ := jsonString(home, f, "env", k); return s }
		api := API{BaseURL: url, APIKey: env("ANTHROPIC_AUTH_TOKEN"), Model: env("ANTHROPIC_MODEL"), SmallModel: env("ANTHROPIC_DEFAULT_HAIKU_MODEL"), Protocol: Anthropic}
		if api.APIKey == "" {
			api.APIKey = env("ANTHROPIC_API_KEY")
		}
		ids := claudeOurs(home)
		if api.Model != "" && !slices.Contains(ids, api.Model) {
			ids = append([]string{api.Model}, ids...)
		}
		for i, id := range ids {
			m := Model{ID: id}
			m.Effort, _ = jsonString(home, f, "modelSettings", id, "effortLevel")
			if i == 0 {
				m.Context, m.Output = num(env("CLAUDE_CODE_MAX_CONTEXT_TOKENS")), num(env("CLAUDE_CODE_MAX_OUTPUT_TOKENS"))
			}
			api.Models = append(api.Models, m)
		}
		return api, true, nil
	},
	write: func(home string, api API) error {
		if err := claudeClear(home); err != nil {
			return err
		}
		set := func(k, v string) jsonEdit {
			if v == "" {
				return jsonEdit{path: []string{"env", k}}
			}
			return jsonEdit{path: []string{"env", k}, value: v}
		}
		def := api.model(api.Model)
		edits := []jsonEdit{set("ANTHROPIC_BASE_URL", api.BaseURL), set("ANTHROPIC_AUTH_TOKEN", api.APIKey), set("ANTHROPIC_MODEL", api.Model),
			set("ANTHROPIC_DEFAULT_HAIKU_MODEL", api.SmallModel), set("CLAUDE_CODE_MAX_CONTEXT_TOKENS", itoa(def.Context)), set("CLAUDE_CODE_MAX_OUTPUT_TOKENS", itoa(def.Output))}
		// The picker lists the user's options, then the API's models.
		opts := claudeOthers(home)
		for _, m := range api.Models {
			opts = append(opts, map[string]any{"model": m.ID, "label": m.ID, "description": claudeMark})
			if m.Effort != "" {
				edits = append(edits, jsonEdit{path: []string{"modelSettings", m.ID, "effortLevel"}, value: m.Effort})
			}
		}
		if len(api.Models) > 1 {
			edits = append(edits, jsonEdit{path: []string{"modelPicker", "options"}, value: opts})
		}
		return editJSON(home, ".claude/settings.json", edits...)
	},
	clear: claudeClear,
}

// claudeMark marks the picker's options enowX added.
const claudeMark = "enowX: the model API set in enowX"

// claudeOurs are the models of the picker's options enowX added.
func claudeOurs(home string) []string {
	list, _ := jsonAny(home, ".claude/settings.json", "modelPicker", "options")
	items, _ := list.([]any)
	var out []string
	for _, it := range items {
		if m, _ := it.(map[string]any); str(m["description"]) == claudeMark {
			out = append(out, str(m["model"]))
		}
	}
	return out
}

// claudeOthers are the picker's options but enowX's.
func claudeOthers(home string) []any {
	list, _ := jsonAny(home, ".claude/settings.json", "modelPicker", "options")
	items, _ := list.([]any)
	var out []any
	for _, it := range items {
		if m, _ := it.(map[string]any); str(m["description"]) != claudeMark {
			out = append(out, it)
		}
	}
	return out
}

func claudeClear(home string) error {
	const f = ".claude/settings.json"
	var edits []jsonEdit
	for _, k := range []string{"ANTHROPIC_BASE_URL", "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_API_KEY", "ANTHROPIC_MODEL", "ANTHROPIC_DEFAULT_HAIKU_MODEL",
		"CLAUDE_CODE_MAX_CONTEXT_TOKENS", "CLAUDE_CODE_MAX_OUTPUT_TOKENS"} {
		edits = append(edits, jsonEdit{path: []string{"env", k}})
	}
	ours := claudeOurs(home)
	if model, _ := jsonString(home, f, "env", "ANTHROPIC_MODEL"); model != "" {
		ours = append(ours, model)
	}
	for _, id := range ours {
		edits = append(edits, jsonEdit{path: []string{"modelSettings", id, "effortLevel"}})
	}
	if others := claudeOthers(home); len(others) > 0 {
		edits = append(edits, jsonEdit{path: []string{"modelPicker", "options"}, value: others})
	} else {
		edits = append(edits, jsonEdit{path: []string{"modelPicker", "options"}})
	}
	return editJSON(home, f, edits...)
}

// OpenCode, and Kilo, which shares its configuration: a provider of
// theirs, by the AI SDK package that speaks the protocol, its models with
// their limits, images, and reasoning effort.
func openCodeLike(id, file string) adapter {
	npm := map[string]string{OpenAI: "@ai-sdk/openai-compatible", OpenAIResponses: "@ai-sdk/openai", Anthropic: "@ai-sdk/anthropic"}
	return adapter{
		protocols: []string{OpenAI, OpenAIResponses, Anthropic},
		files:     []string{file},
		small:     true,
		caps:      withEfforts(listCaps, "model", "low", "medium", "high"),
		note:      "It needs the model's name.",
		read: func(home string) (API, bool, error) {
			p, err := jsonAny(home, file, "provider", ourName)
			if err != nil || p == nil {
				return API{}, false, err
			}
			pm, _ := p.(map[string]any)
			opts, _ := pm["options"].(map[string]any)
			api := API{BaseURL: str(opts["baseURL"]), APIKey: str(opts["apiKey"])}
			for proto, n := range npm {
				if n == str(pm["npm"]) {
					api.Protocol = proto
				}
			}
			model, _ := jsonString(home, file, "model")
			api.Model, _ = strings.CutPrefix(model, ourName+"/")
			small, _ := jsonString(home, file, "small_model")
			api.SmallModel, _ = strings.CutPrefix(small, ourName+"/")
			models, _ := pm["models"].(map[string]any)
			for mid, v := range models {
				mm, _ := v.(map[string]any)
				m := Model{ID: mid}
				limit, _ := mm["limit"].(map[string]any)
				m.Context, m.Output = num(limit["context"]), num(limit["output"])
				mod, _ := mm["modalities"].(map[string]any)
				in, _ := mod["input"].([]any)
				m.Vision = slices.Contains(in, any("image"))
				mo, _ := mm["options"].(map[string]any)
				m.Effort = str(mo["reasoningEffort"])
				api.Models = append(api.Models, m)
			}
			sortModels(&api)
			return api, true, nil
		},
		write: func(home string, api API) error {
			if api.Model == "" {
				return fmt.Errorf("agentapi: %s needs the model's name", id)
			}
			models := map[string]any{}
			for _, m := range api.Models {
				mm := map[string]any{"name": m.ID, "tool_call": true}
				if m.Context > 0 || m.Output > 0 {
					// Both or neither, it says.
					mm["limit"] = map[string]any{"context": orDefault(m.Context, 128000), "output": orDefault(m.Output, 16384)}
				}
				if m.Vision {
					mm["attachment"] = true
					mm["modalities"] = map[string]any{"input": []string{"text", "image"}, "output": []string{"text"}}
				}
				if m.Effort != "" {
					mm["reasoning"] = true
					mm["options"] = map[string]any{"reasoningEffort": m.Effort}
				}
				models[m.ID] = mm
			}
			if api.SmallModel != "" && models[api.SmallModel] == nil {
				models[api.SmallModel] = map[string]any{"name": api.SmallModel, "tool_call": true}
			}
			provider := map[string]any{
				"npm":     npm[api.Protocol],
				"name":    "enowX: " + api.BaseURL,
				"options": map[string]any{"baseURL": api.BaseURL, "apiKey": api.APIKey},
				"models":  models,
			}
			small := jsonEdit{path: []string{"small_model"}}
			if api.SmallModel != "" {
				small.value = ourName + "/" + api.SmallModel
			} else if s, _ := jsonString(home, file, "small_model"); !strings.HasPrefix(s, ourName+"/") {
				small = jsonEdit{path: []string{"small_model"}, value: nil}
				if s != "" {
					small.value = s // the user's own stays
				}
			}
			edits := []jsonEdit{{path: []string{"provider", ourName}, value: provider}, {path: []string{"model"}, value: ourName + "/" + api.Model}}
			if small.value != nil || api.SmallModel == "" {
				edits = append(edits, small)
			}
			return editJSON(home, file, edits...)
		},
		clear: func(home string) error {
			edits := []jsonEdit{{path: []string{"provider", ourName}}}
			for _, k := range []string{"model", "small_model"} {
				if m, _ := jsonString(home, file, k); strings.HasPrefix(m, ourName+"/") {
					edits = append(edits, jsonEdit{path: []string{k}})
				}
			}
			return editJSON(home, file, edits...)
		},
	}
}

// sortModels puts api's default model first, then the rest by name: what
// a map of models gives back has no order.
func sortModels(api *API) {
	slices.SortFunc(api.Models, func(a, b Model) int {
		switch {
		case a.ID == api.Model:
			return -1
		case b.ID == api.Model:
			return 1
		}
		return strings.Compare(a.ID, b.ID)
	})
}

// Qwen Code: a model provider entry of its settings for each model, with
// its generation config; the key in the settings' env block under a name
// of enowX's.
const qwenKeyVar = "ENOWX_QWEN_API_KEY"

var qwenCode = adapter{
	// What its sessions take (qwenSession): one model, by its variables.
	protocols: []string{OpenAI, Anthropic},
	files:     []string{".qwen/settings.json"},
	note:      "It needs the model's name.",
	read: func(home string) (API, bool, error) {
		const f = ".qwen/settings.json"
		api := API{}
		for _, kind := range []string{"openai", "anthropic"} {
			list, err := jsonAny(home, f, "modelProviders", kind)
			if err != nil {
				return API{}, false, err
			}
			items, _ := list.([]any)
			for _, it := range items {
				e, _ := it.(map[string]any)
				if e["envKey"] != qwenKeyVar {
					continue
				}
				api.BaseURL = str(e["baseUrl"])
				api.Protocol = OpenAI
				if kind == "anthropic" {
					api.Protocol = Anthropic
				} else if e["wireApi"] == "responses" {
					api.Protocol = OpenAIResponses
				}
				m := Model{ID: str(e["id"])}
				gen, _ := e["generationConfig"].(map[string]any)
				m.Context = num(gen["contextWindowSize"])
				mod, _ := gen["modalities"].(map[string]any)
				m.Vision = mod["image"] == true
				if r, ok := gen["reasoning"].(map[string]any); ok {
					m.Effort = str(r["effort"])
				}
				sp, _ := gen["samplingParams"].(map[string]any)
				m.Output = num(sp["max_tokens"])
				api.Models = append(api.Models, m)
			}
		}
		if len(api.Models) == 0 {
			return API{}, false, nil
		}
		api.APIKey, _ = jsonString(home, f, "env", qwenKeyVar)
		api.Model, _ = jsonString(home, f, "model", "name")
		api.SmallModel, _ = jsonString(home, f, "fastModel")
		if !slices.ContainsFunc(api.Models, func(m Model) bool { return m.ID == api.SmallModel }) {
			api.SmallModel = ""
		}
		sortModels(&api)
		return api, true, nil
	},
	write: func(home string, api API) error {
		if api.Model == "" {
			return fmt.Errorf("agentapi: Qwen Code needs the model's name")
		}
		if err := qwenRemove(home); err != nil {
			return err
		}
		kind := "openai"
		if api.Protocol == Anthropic {
			kind = "anthropic"
		}
		list, err := jsonAny(home, ".qwen/settings.json", "modelProviders", kind)
		if err != nil {
			return err
		}
		items, _ := list.([]any)
		models := api.Models
		if api.SmallModel != "" && !slices.ContainsFunc(models, func(m Model) bool { return m.ID == api.SmallModel }) {
			models = append(models, Model{ID: api.SmallModel})
		}
		for _, m := range models {
			e := map[string]any{"id": m.ID, "envKey": qwenKeyVar, "baseUrl": api.BaseURL}
			if api.Protocol == OpenAIResponses {
				e["wireApi"] = "responses"
			}
			gen := map[string]any{}
			if m.Context > 0 {
				gen["contextWindowSize"] = m.Context
			}
			if m.Vision {
				gen["modalities"] = map[string]any{"image": true}
			}
			if m.Effort != "" {
				gen["reasoning"] = map[string]any{"effort": m.Effort}
			}
			if m.Output > 0 {
				gen["samplingParams"] = map[string]any{"max_tokens": m.Output}
			}
			if len(gen) > 0 {
				e["generationConfig"] = gen
			}
			items = append(items, e)
		}
		edits := []jsonEdit{
			{path: []string{"modelProviders", kind}, value: items},
			{path: []string{"env", qwenKeyVar}, value: api.APIKey},
			{path: []string{"security", "auth", "selectedType"}, value: kind},
			{path: []string{"model", "name"}, value: api.Model},
		}
		if api.SmallModel != "" {
			edits = append(edits, jsonEdit{path: []string{"fastModel"}, value: api.SmallModel})
		}
		return editJSON(home, ".qwen/settings.json", edits...)
	},
	clear: func(home string) error {
		if err := qwenRemove(home); err != nil {
			return err
		}
		return editJSON(home, ".qwen/settings.json", jsonEdit{path: []string{"env", qwenKeyVar}})
	},
}

// qwenRemove takes enowX's providers out of Qwen Code's settings, and the
// quick model when it was one of them.
func qwenRemove(home string) error {
	const f = ".qwen/settings.json"
	var edits []jsonEdit
	var ours []string
	for _, kind := range []string{"openai", "anthropic"} {
		list, err := jsonAny(home, f, "modelProviders", kind)
		if err != nil {
			return err
		}
		items, ok := list.([]any)
		if !ok {
			continue
		}
		var kept []any
		for _, it := range items {
			if e, _ := it.(map[string]any); e["envKey"] != qwenKeyVar {
				kept = append(kept, it)
			} else {
				ours = append(ours, str(e["id"]))
			}
		}
		if len(kept) == 0 {
			edits = append(edits, jsonEdit{path: []string{"modelProviders", kind}})
		} else {
			edits = append(edits, jsonEdit{path: []string{"modelProviders", kind}, value: kept})
		}
	}
	if fast, _ := jsonString(home, f, "fastModel"); fast != "" && slices.Contains(ours, fast) {
		edits = append(edits, jsonEdit{path: []string{"fastModel"}})
	}
	if len(edits) == 0 {
		return nil
	}
	return editJSON(home, f, edits...)
}

// Droid: a custom model of its settings for each model, which /model then
// offers.
var droid = adapter{
	protocols: []string{OpenAI, OpenAIResponses, Anthropic},
	files:     []string{".factory/settings.json"},
	caps:      withEfforts(listCaps, "model", "off", "low", "medium", "high"),
	note:      "They show in Droid's /model as \"enowX: <model>\"; pick one there. It needs the model's name.",
	read: func(home string) (API, bool, error) {
		list, err := jsonAny(home, ".factory/settings.json", "customModels")
		if err != nil {
			return API{}, false, err
		}
		items, _ := list.([]any)
		api := API{}
		for _, it := range items {
			e, _ := it.(map[string]any)
			if !strings.HasPrefix(str(e["displayName"]), "enowX: ") {
				continue
			}
			api.BaseURL, api.APIKey = str(e["baseUrl"]), str(e["apiKey"])
			switch e["provider"] {
			case "anthropic":
				api.Protocol = Anthropic
			case "openai":
				api.Protocol = OpenAIResponses
			default:
				api.Protocol = OpenAI
			}
			m := Model{ID: str(e["model"]), Context: num(e["maxContextLimit"]), Output: num(e["maxOutputTokens"]), Effort: str(e["reasoningEffort"])}
			m.Vision = e["noImageSupport"] == false
			api.Models = append(api.Models, m)
		}
		if len(api.Models) == 0 {
			return API{}, false, nil
		}
		api.Model = api.Models[0].ID
		return api, true, nil
	},
	write: func(home string, api API) error {
		if api.Model == "" {
			return fmt.Errorf("agentapi: Droid needs the model's name")
		}
		items, err := droidOthers(home)
		if err != nil {
			return err
		}
		provider := map[string]string{OpenAI: "generic-chat-completion-api", OpenAIResponses: "openai", Anthropic: "anthropic"}[api.Protocol]
		for _, m := range api.Models {
			e := map[string]any{"model": m.ID, "displayName": "enowX: " + m.ID, "baseUrl": api.BaseURL, "apiKey": api.APIKey, "provider": provider,
				"noImageSupport": !m.Vision}
			if m.Output > 0 {
				e["maxOutputTokens"] = m.Output
			}
			if m.Context > 0 {
				e["maxContextLimit"] = m.Context
			}
			if m.Effort != "" {
				e["reasoningEffort"] = m.Effort
			}
			items = append(items, e)
		}
		return editJSON(home, ".factory/settings.json", jsonEdit{path: []string{"customModels"}, value: items})
	},
	clear: func(home string) error {
		items, err := droidOthers(home)
		if err != nil {
			return err
		}
		if len(items) == 0 {
			return editJSON(home, ".factory/settings.json", jsonEdit{path: []string{"customModels"}})
		}
		return editJSON(home, ".factory/settings.json", jsonEdit{path: []string{"customModels"}, value: items})
	},
}

// droidOthers are Droid's custom models but enowX's.
func droidOthers(home string) ([]any, error) {
	list, err := jsonAny(home, ".factory/settings.json", "customModels")
	if err != nil {
		return nil, err
	}
	items, _ := list.([]any)
	var out []any
	for _, it := range items {
		if e, _ := it.(map[string]any); !strings.HasPrefix(str(e["displayName"]), "enowX: ") {
			out = append(out, it)
		}
	}
	return out, nil
}

// Pi: a provider of its models, with their images, limits and reasoning;
// its settings' default, and each model's thinking level.
var pi = adapter{
	protocols: []string{OpenAI, OpenAIResponses, Anthropic, Gemini},
	files:     []string{".pi/agent/models.json", ".pi/agent/settings.json"},
	caps:      withEfforts(listCaps, "model", "off", "minimal", "low", "medium", "high", "xhigh", "max"),
	note:      "It needs the model's name.",
	read: func(home string) (API, bool, error) {
		p, err := jsonAny(home, ".pi/agent/models.json", "providers", ourName)
		if err != nil || p == nil {
			return API{}, false, err
		}
		if def, _ := jsonString(home, ".pi/agent/settings.json", "defaultProvider"); def != ourName {
			return API{}, false, nil
		}
		pm, _ := p.(map[string]any)
		api := API{BaseURL: str(pm["baseUrl"]), APIKey: str(pm["apiKey"])}
		for proto, name := range piAPIs {
			if name == pm["api"] {
				api.Protocol = proto
			}
		}
		api.Model, _ = jsonString(home, ".pi/agent/settings.json", "defaultModel")
		list, _ := pm["models"].([]any)
		for _, it := range list {
			e, _ := it.(map[string]any)
			m := Model{ID: str(e["id"]), Context: num(e["contextWindow"]), Output: num(e["maxTokens"])}
			in, _ := e["input"].([]any)
			m.Vision = slices.Contains(in, any("image"))
			m.Effort, _ = jsonString(home, ".pi/agent/settings.json", "modelThinkingLevels", ourName+"/"+m.ID)
			api.Models = append(api.Models, m)
		}
		sortModels(&api)
		return api, true, nil
	},
	write: func(home string, api API) error {
		if api.Model == "" {
			return fmt.Errorf("agentapi: Pi needs the model's name")
		}
		if err := piClear(home); err != nil {
			return err
		}
		var models []any
		var edits []jsonEdit
		for _, m := range api.Models {
			e := map[string]any{"id": m.ID}
			if m.Vision {
				e["input"] = []string{"text", "image"}
			}
			if m.Context > 0 {
				e["contextWindow"] = m.Context
			}
			if m.Output > 0 {
				e["maxTokens"] = m.Output
			}
			if m.Effort != "" && m.Effort != "off" {
				e["reasoning"] = true
			}
			if m.Effort != "" {
				edits = append(edits, jsonEdit{path: []string{"modelThinkingLevels", ourName + "/" + m.ID}, value: m.Effort})
			}
			models = append(models, e)
		}
		provider := map[string]any{"baseUrl": api.BaseURL, "api": piAPIs[api.Protocol], "apiKey": api.APIKey, "models": models}
		if err := editJSON(home, ".pi/agent/models.json", jsonEdit{path: []string{"providers", ourName}, value: provider}); err != nil {
			return err
		}
		edits = append(edits, jsonEdit{path: []string{"defaultProvider"}, value: ourName}, jsonEdit{path: []string{"defaultModel"}, value: api.Model})
		return editJSON(home, ".pi/agent/settings.json", edits...)
	},
	clear: piClear,
}

func piClear(home string) error {
	p, _ := jsonAny(home, ".pi/agent/models.json", "providers", ourName)
	pm, _ := p.(map[string]any)
	list, _ := pm["models"].([]any)
	var edits []jsonEdit
	for _, it := range list {
		e, _ := it.(map[string]any)
		edits = append(edits, jsonEdit{path: []string{"modelThinkingLevels", ourName + "/" + str(e["id"])}})
	}
	if err := editJSON(home, ".pi/agent/models.json", jsonEdit{path: []string{"providers", ourName}}); err != nil {
		return err
	}
	if def, _ := jsonString(home, ".pi/agent/settings.json", "defaultProvider"); def == ourName {
		edits = append(edits, jsonEdit{path: []string{"defaultProvider"}}, jsonEdit{path: []string{"defaultModel"}})
	}
	if len(edits) == 0 {
		return nil
	}
	return editJSON(home, ".pi/agent/settings.json", edits...)
}

var piAPIs = map[string]string{OpenAI: "openai-completions", OpenAIResponses: "openai-responses", Anthropic: "anthropic-messages", Gemini: "google-generative-ai"}

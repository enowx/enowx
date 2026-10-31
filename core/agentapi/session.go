package agentapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// A model API given to one session of an agent, its own settings
// untouched: through the session's environment and command line, as each
// agent takes them (docs/configs.md). The key goes in the environment
// only; what a file must hold (OpenCode's config, Aider's model metadata)
// names the variable holding it, never its value.

// Launch is what a session gets for an API.
type Launch struct {
	Env  []string // KEY=VALUE, its key among them
	Args []string // added to the agent's command line
	// Settings, for Claude Code, are merged into its --settings (which
	// enowX's hooks already use: a second flag would replace them).
	Settings map[string]any
}

// session builds a Launch for api, files in dir (the session's own).
type sessionFunc func(api API, dir string) (Launch, error)

// sessionAdapters are the agents a config can give an API per session.
var sessionAdapters = map[string]sessionFunc{
	"claude":      claudeSession,
	"codex":       codexSession,
	"opencode":    openCodeSession,
	"copilot":     envSession(copilot),
	"openclaude":  envSession(openClaude),
	"antigravity": envSession(antigravity),
	"gemini":      geminiSession,
	"aider":       aiderSession,
}

// PerSession says agent id (a preset) can take an API per session.
func PerSession(id string) bool {
	_, ok := sessionAdapters[Base(id)]
	return ok
}

// Prepare checks api for agent id and makes it what the agent takes: its
// models normalized, its protocol the agent's default when unset.
func Prepare(id string, api API) (API, error) {
	a, ok := adapters[Base(id)]
	if !ok {
		return API{}, fmt.Errorf("agentapi: %s takes no API of the user's", id)
	}
	api.BaseURL = strings.TrimRight(strings.TrimSpace(api.BaseURL), "/")
	api.Model, api.SmallModel = strings.TrimSpace(api.Model), strings.TrimSpace(api.SmallModel)
	if api.BaseURL == "" || !(strings.HasPrefix(api.BaseURL, "http://") || strings.HasPrefix(api.BaseURL, "https://")) {
		return API{}, errors.New("agentapi: the API's address is an http:// or https:// URL")
	}
	if api.Protocol == "" {
		api.Protocol = a.protocols[0]
	}
	if !slices.Contains(a.protocols, api.Protocol) {
		return API{}, fmt.Errorf("agentapi: %s speaks %s to an API, not %s", id, strings.Join(a.protocols, " or "), api.Protocol)
	}
	normalize(&api, a)
	if err := check(api, a); err != nil {
		return API{}, fmt.Errorf("agentapi: %s: %w", id, err)
	}
	return api, nil
}

// SessionLaunch is what a session of agent id gets for api (its key in
// APIKey), files it needs made in dir.
func SessionLaunch(id string, api API, dir string) (Launch, error) {
	f, ok := sessionAdapters[Base(id)]
	if !ok {
		return Launch{}, fmt.Errorf("agentapi: %s can't take a model API per session", id)
	}
	api, err := Prepare(id, api)
	if err != nil {
		return Launch{}, err
	}
	return f(api, dir)
}

// WithSettings is args (Claude Code's) with settings merged into the JSON
// of its --settings, or a --settings of their own when it has none.
func WithSettings(args []string, settings map[string]any) []string {
	if len(settings) == 0 {
		return args
	}
	out := slices.Clone(args)
	for i := 0; i+1 < len(out); i++ {
		if out[i] != "--settings" {
			continue
		}
		var cur map[string]any
		if json.Unmarshal([]byte(out[i+1]), &cur) != nil {
			continue // a file's path: left as it is
		}
		mergeJSON(cur, settings)
		data, _ := json.Marshal(cur)
		out[i+1] = string(data)
		return out
	}
	data, _ := json.Marshal(settings)
	return append(out, "--settings", string(data))
}

// mergeJSON sets src's values in dst, objects merged key by key.
func mergeJSON(dst, src map[string]any) {
	for k, v := range src {
		if sm, ok := v.(map[string]any); ok {
			if dm, ok := dst[k].(map[string]any); ok {
				mergeJSON(dm, sm)
				continue
			}
		}
		dst[k] = v
	}
}

func kv(pairs ...[2]string) []string {
	var out []string
	for _, p := range pairs {
		if p[1] != "" {
			out = append(out, p[0]+"="+p[1])
		}
	}
	return out
}

// claudeSession: the address and models in its settings' env (which win
// over the user's own settings), the key in the session's environment.
func claudeSession(api API, _ string) (Launch, error) {
	def := api.model(api.Model)
	env := map[string]any{"ANTHROPIC_BASE_URL": api.BaseURL}
	vars := [][2]string{{"ANTHROPIC_BASE_URL", api.BaseURL}, {"ANTHROPIC_MODEL", api.Model}, {"ANTHROPIC_DEFAULT_HAIKU_MODEL", api.SmallModel},
		{"CLAUDE_CODE_MAX_CONTEXT_TOKENS", itoa(def.Context)}, {"CLAUDE_CODE_MAX_OUTPUT_TOKENS", itoa(def.Output)}}
	for _, v := range vars {
		if v[1] != "" {
			env[v[0]] = v[1]
		}
	}
	settings := map[string]any{"env": env}
	if len(api.Models) > 1 {
		var opts []any
		for _, m := range api.Models {
			opts = append(opts, map[string]any{"model": m.ID, "label": m.ID, "description": claudeMark})
		}
		settings["modelPicker"] = map[string]any{"options": opts}
	}
	ms := map[string]any{}
	for _, m := range api.Models {
		if m.Effort != "" {
			ms[m.ID] = map[string]any{"effortLevel": m.Effort}
		}
	}
	if len(ms) > 0 {
		settings["modelSettings"] = ms
	}
	return Launch{Env: kv(append(vars, [2]string{"ANTHROPIC_AUTH_TOKEN", api.APIKey})...), Settings: settings}, nil
}

// codexSession: its config overridden on the command line (-c), the key
// in the variable its provider names.
func codexSession(api API, _ string) (Launch, error) {
	c := func(k, v string) []string { return []string{"-c", k + "=" + v} }
	def := api.model(api.Model)
	p := "model_providers." + ourName + "."
	args := slices.Concat(c("model_provider", tomlQuote(ourName)), c(p+"name", tomlQuote("enowX: "+api.BaseURL)),
		c(p+"base_url", tomlQuote(api.BaseURL)), c(p+"env_key", tomlQuote(codexKeyVar)), c(p+"wire_api", tomlQuote("responses")))
	if api.Model != "" {
		args = append(args, c("model", tomlQuote(api.Model))...)
	}
	if def.Effort != "" {
		args = append(args, c("model_reasoning_effort", tomlQuote(def.Effort))...)
	}
	if def.Context > 0 {
		args = append(args, c("model_context_window", itoa(def.Context))...)
	}
	for _, m := range api.Models[min(1, len(api.Models)):] {
		prof := "profiles." + ourName + "-" + slug(m.ID) + "."
		args = append(args, c(prof+"model_provider", tomlQuote(ourName))...)
		args = append(args, c(prof+"model", tomlQuote(m.ID))...)
		if m.Effort != "" {
			args = append(args, c(prof+"model_reasoning_effort", tomlQuote(m.Effort))...)
		}
	}
	return Launch{Env: kv([2]string{codexKeyVar, api.APIKey}), Args: args}, nil
}

// openCodeKeyVar holds the key of an OpenCode session's API: its config
// names it ({env:…}), as OpenCode reads it there.
const openCodeKeyVar = "ENOWX_OPENCODE_API_KEY"

// openCodeSession: a config of the session's own, which OpenCode merges
// over the user's (OPENCODE_CONFIG), its key named from the environment.
func openCodeSession(api API, dir string) (Launch, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Launch{}, err
	}
	keyed := api
	keyed.APIKey = "{env:" + openCodeKeyVar + "}"
	if err := adapters["opencode"].write(dir, keyed); err != nil {
		return Launch{}, err
	}
	return Launch{Env: kv([2]string{"OPENCODE_CONFIG", filepath.Join(dir, adapters["opencode"].files[0])}, [2]string{openCodeKeyVar, api.APIKey})}, nil
}

// envSession is an agent that takes its API from the environment only.
func envSession(a adapter) sessionFunc {
	return func(api API, _ string) (Launch, error) {
		if a.env == nil {
			return Launch{}, errors.New("agentapi: no variables")
		}
		return Launch{Env: kv(a.env(api)...)}, nil
	}
}

// geminiSession: the variables Gemini CLI reads its key and address
// from, signing in with the key, and the model on its command line.
func geminiSession(api API, _ string) (Launch, error) {
	l := Launch{Env: kv([2]string{"GOOGLE_GEMINI_BASE_URL", api.BaseURL}, [2]string{"GEMINI_API_KEY", api.APIKey},
		[2]string{"GEMINI_DEFAULT_AUTH_TYPE", "gemini-api-key"}, [2]string{"GEMINI_MODEL", api.Model})}
	if api.Model != "" {
		l.Args = []string{"--model", api.Model}
	}
	return l, nil
}

// aiderSession: OpenAI's variables for the address and key, the models
// and their limits on its command line.
func aiderSession(api API, dir string) (Launch, error) {
	l := Launch{Env: kv([2]string{"OPENAI_API_BASE", api.BaseURL}, [2]string{"OPENAI_API_KEY", api.APIKey})}
	if api.Model == "" {
		return Launch{}, errors.New("agentapi: Aider needs the model's name")
	}
	l.Args = []string{"--model", "openai/" + api.Model}
	if api.SmallModel != "" {
		l.Args = append(l.Args, "--weak-model", "openai/"+api.SmallModel)
	}
	if api.Effort != "" {
		l.Args = append(l.Args, "--reasoning-effort", api.Effort)
	}
	if m := api.model(api.Model); m.Context > 0 || m.Output > 0 {
		meta := map[string]any{"litellm_provider": "openai", "mode": "chat"}
		if m.Context > 0 {
			meta["max_input_tokens"] = m.Context
		}
		if m.Output > 0 {
			meta["max_output_tokens"] = m.Output
		}
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return Launch{}, err
		}
		data, _ := json.Marshal(map[string]any{"openai/" + m.ID: meta})
		path := filepath.Join(dir, "aider-models.json")
		if err := os.WriteFile(path, data, 0o600); err != nil {
			return Launch{}, err
		}
		l.Args = append(l.Args, "--model-metadata-file", path)
	}
	return l, nil
}

// Package agentapi points agents at a model API of the user's choosing
// (a gateway, OpenRouter, a local server): its address, key and model,
// written into each agent's own settings the way that agent takes them,
// the files kept first so they can be put back (docs/agent-api.md).
package agentapi

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// API is a model API an agent uses instead of its own.
type API struct {
	BaseURL string
	APIKey  string
	// Model is the one the agent starts with: Models' first.
	Model string
	// Models are the API's models the agent offers, Model first, with what
	// the agent takes of each (its Caps).
	Models []Model
	// SmallModel is the model for quick work, where the agent has one
	// (Claude Code's background tasks); "" leaves it as the agent picks.
	SmallModel string
	// Effort is the reasoning effort, for agents that take one for every
	// model (Caps.Effort "global").
	Effort string
	// Protocol is how the API is spoken, for agents that speak several:
	// one of the agent's Protocols.
	Protocol string
}

// Model is one of an API's models and what the agent is told of it; zero
// values leave it to the agent.
type Model struct {
	ID      string
	Vision  bool   // it takes images
	Context int    // its context window, in tokens
	Output  int    // the most it writes at once, in tokens
	Effort  string // its reasoning effort, one of Caps.Efforts
}

// Caps are what an agent takes of an API's models.
type Caps struct {
	Multi           bool     // several models, the agent offering them
	Vision          bool     // per model
	Context, Output string   // "model", "global" (the default's), or ""
	Effort          string   // "model", "global" (API.Effort), or ""
	Efforts         []string // the efforts it takes
}

// Protocols an API speaks.
const (
	Anthropic       = "anthropic"        // Anthropic's Messages API
	OpenAI          = "openai"           // OpenAI's Chat Completions
	OpenAIResponses = "openai-responses" // OpenAI's Responses API
	Gemini          = "gemini"
)

// adapter is how one agent takes an API.
type adapter struct {
	// protocols it speaks to a custom API, the first its default.
	protocols []string
	// files it changes, under the home.
	files []string
	// read is the API it is set to, ok false when its own.
	read func(home string) (API, bool, error)
	// write sets api; clear goes back to its own, leaving the rest.
	write func(home string, api API) error
	clear func(home string) error
	// note says what else to know: a model to name, a sign-in it skips.
	note string
	// small: it has a model for quick work; noModel: it takes none, using
	// the API's own.
	small, noModel bool
	// args are what its command line gets when it starts, nil for none.
	args func(home string) []string
	// env, for an agent taking its API from the environment only, is its
	// variables for an API.
	env  func(api API) [][2]string
	caps Caps
}

// adapters are the agents enowX can point at an API, by preset ID.
var adapters = map[string]adapter{}

// Agent is what enowX knows of an agent's API.
type Agent struct {
	ID        string
	Protocols []string
	Files     []string
	Note      string
	Small     bool
	NoModel   bool
	Caps      Caps
	// Current is the API it is set to, nil for its own.
	Current *API
}

// Supported are the agents that take an API, sorted.
func Supported() []string {
	var ids []string
	for id := range adapters {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

// Of is agent id's API under home.
func Of(home, id string) (Agent, error) {
	a, ok := adapters[id]
	if !ok {
		return Agent{}, fmt.Errorf("agentapi: %s takes no API of the user's", id)
	}
	out := Agent{ID: id, Protocols: a.protocols, Files: a.files, Note: a.note, Small: a.small, NoModel: a.noModel, Caps: a.caps}
	api, set, err := a.read(home)
	if err != nil {
		return out, fmt.Errorf("agentapi: %s's settings: %w", id, err)
	}
	if set {
		out.Current = &api
	}
	return out, nil
}

// Set points agent id under home at api, its files kept first.
func Set(home, id string, api API) error {
	a, ok := adapters[id]
	if !ok {
		return fmt.Errorf("agentapi: %s takes no API of the user's", id)
	}
	api.BaseURL = strings.TrimRight(strings.TrimSpace(api.BaseURL), "/")
	api.Model, api.SmallModel, api.APIKey = strings.TrimSpace(api.Model), strings.TrimSpace(api.SmallModel), strings.TrimSpace(api.APIKey)
	if api.BaseURL == "" || !(strings.HasPrefix(api.BaseURL, "http://") || strings.HasPrefix(api.BaseURL, "https://")) {
		return errors.New("agentapi: the API's address is an http:// or https:// URL")
	}
	if api.Protocol == "" {
		api.Protocol = a.protocols[0]
	}
	normalize(&api, a)
	if err := check(api, a); err != nil {
		return fmt.Errorf("agentapi: %s: %w", id, err)
	}
	if !slices.Contains(a.protocols, api.Protocol) {
		return fmt.Errorf("agentapi: %s speaks %s to an API, not %s", id, strings.Join(a.protocols, " or "), api.Protocol)
	}
	if _, err := backup(home, id, a.files, "set to "+api.BaseURL); err != nil {
		return fmt.Errorf("agentapi: keeping %s's settings first: %w", id, err)
	}
	return a.write(home, api)
}

// Clear sets agent id back to its own API, its files kept first; the rest
// of its settings stay.
func Clear(home, id string) error {
	a, ok := adapters[id]
	if !ok {
		return fmt.Errorf("agentapi: %s takes no API of the user's", id)
	}
	if _, err := backup(home, id, a.files, "back to its own"); err != nil {
		return err
	}
	return a.clear(home)
}

// MaskKey shows a key's last four characters only.
func MaskKey(key string) string {
	if key == "" {
		return ""
	}
	if len(key) <= 8 {
		return "••••"
	}
	return "••••" + key[len(key)-4:]
}

// envFile is where enowX keeps the variables agent id's sessions get,
// under the home: its key, for agents that read it from the environment.
func envFile(id string) string { return filepath.Join(".enowx-app", "agentapi", id+".env") }

// LaunchEnv is what the environment of agent id's sessions gets, for the
// API it is set to: nil for its own.
func LaunchEnv(home, id string) []string {
	data, err := readFile(home, envFile(Base(id)))
	if err != nil || len(data) == 0 {
		return nil
	}
	var out []string
	for _, l := range strings.Split(string(data), "\n") {
		if k, v, ok := strings.Cut(strings.TrimSpace(l), "="); ok && k != "" && k != protocolVar {
			if u, err := strconv.Unquote(v); err == nil {
				v = u
			}
			out = append(out, k+"="+v)
		}
	}
	return out
}

// LaunchArgs are what agent id's command line gets for its API, nil for
// none.
func LaunchArgs(home, id string) []string {
	if a, ok := adapters[Base(id)]; ok && a.args != nil {
		return a.args(home)
	}
	return nil
}

// Base is the agent whose settings a preset shares: Claude Code's for its
// teams' preset.
func Base(id string) string {
	if id == "claude-agent-teams" {
		return "claude"
	}
	return id
}

// normalize makes api's models what a takes: Model first among them, the
// options it has no place for dropped.
func normalize(api *API, a adapter) {
	var models []Model
	for _, m := range api.Models {
		m.ID = strings.TrimSpace(m.ID)
		if m.ID != "" && !slices.ContainsFunc(models, func(x Model) bool { return x.ID == m.ID }) {
			models = append(models, m)
		}
	}
	if api.Model == "" && len(models) > 0 {
		api.Model = models[0].ID
	}
	if api.Model != "" {
		i := slices.IndexFunc(models, func(m Model) bool { return m.ID == api.Model })
		if i < 0 {
			models = append([]Model{{ID: api.Model}}, models...)
		} else {
			m := models[i]
			models = append([]Model{m}, slices.Delete(models, i, i+1)...)
		}
	}
	if !a.caps.Multi && len(models) > 1 {
		models = models[:1]
	}
	for i := range models {
		m := &models[i]
		if !a.caps.Vision {
			m.Vision = false
		}
		if a.caps.Context == "" || (a.caps.Context == "global" && i > 0) {
			m.Context = 0
		}
		if a.caps.Output == "" || (a.caps.Output == "global" && i > 0) {
			m.Output = 0
		}
		if a.caps.Effort != "model" {
			m.Effort = ""
		}
	}
	if a.caps.Effort != "global" {
		api.Effort = ""
	}
	if a.noModel {
		api.Model, models = "", nil
	}
	if !a.small {
		api.SmallModel = ""
	}
	api.Models = models
}

func check(api API, a adapter) error {
	efforts := func(e string) error {
		if e != "" && !slices.Contains(a.caps.Efforts, e) {
			return fmt.Errorf("reasoning effort %q: it takes %s", e, strings.Join(a.caps.Efforts, ", "))
		}
		return nil
	}
	if err := efforts(api.Effort); err != nil {
		return err
	}
	for _, m := range api.Models {
		if m.Context < 0 || m.Output < 0 {
			return fmt.Errorf("%s: token counts are positive", m.ID)
		}
		if err := efforts(m.Effort); err != nil {
			return fmt.Errorf("%s: %w", m.ID, err)
		}
	}
	return nil
}

// model is the model of id among api's, as the agent was told of it.
func (api API) model(id string) Model {
	for _, m := range api.Models {
		if m.ID == id {
			return m
		}
	}
	return Model{ID: id}
}

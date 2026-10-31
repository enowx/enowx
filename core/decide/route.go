package decide

import (
	"fmt"
	"slices"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/proto"
)

// Route is a model and an effort of an agent; "" keeps the agent's own.
type Route struct{ Model, Effort string }

// routes are each agent's models by how hard a prompt is, from its model
// list (core/agent/settings.go): the cheapest that does the job. Codex
// lists one model for easier tasks; harder ones keep its own default with
// more reasoning.
var routes = map[string]map[string]Route{
	"claude": {
		proto.VerdictEasy:   {"claude-haiku-5-5", "low"},
		proto.VerdictMedium: {"claude-sonnet-5-5", "medium"},
		proto.VerdictHard:   {"claude-opus-5-5", "high"},
	},
	"codex": {
		proto.VerdictEasy:   {"gpt-6-luna", "low"},
		proto.VerdictMedium: {"", "medium"},
		proto.VerdictHard:   {"", "high"},
	},
}

// efforts are the efforts by how hard a prompt is, for an agent whose
// efforts include them.
var efforts = map[string]string{proto.VerdictEasy: "low", proto.VerdictMedium: "medium", proto.VerdictHard: "high"}

// RouteAPI is the model API an agent is set to (core/agentapi): its main
// model and its small one. Its own models don't apply then.
type RouteAPI struct{ Model, SmallModel string }

// RouteFor is agent id's model and effort for a prompt of difficulty, api
// the model API it is set to (nil for its own).
func RouteFor(id, difficulty string, api *RouteAPI) Route {
	id = agent.Base(id)
	r, ok := routes[id][difficulty]
	if !ok {
		s := agent.SettingsOf(id)
		if e := efforts[difficulty]; slices.ContainsFunc(s.Efforts, func(c agent.Choice) bool { return c.ID == e }) {
			r.Effort = e
		}
	}
	if api != nil {
		r.Model = api.Model
		if difficulty == proto.VerdictEasy && api.SmallModel != "" {
			r.Model = api.SmallModel
		}
	}
	return r
}

// Switch is what to type in a running session of agent id to change to r:
// its model command, its effort command, as it has them.
func (r Route) Switch(id string) []string {
	s := agent.SettingsOf(id)
	var out []string
	if r.Model != "" && s.ModelCommand != "" {
		out = append(out, fmt.Sprintf(s.ModelCommand, r.Model))
	}
	if r.Effort != "" && s.EffortCommand != "" {
		out = append(out, fmt.Sprintf(s.EffortCommand, r.Effort))
	}
	return out
}

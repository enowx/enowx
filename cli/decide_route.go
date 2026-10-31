package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/enowx/enowx/proto"
)

// decideRoute runs `enowx decide route [--agent A] PROMPT`: a model and
// effort of the agent for the prompt, as the judge weighs how hard it is
// (this session's agent when --agent is left out).
func (r runner) decideRoute(p proto.DecideRouteParams, asJSON bool) error {
	if strings.TrimSpace(p.Prompt) == "" {
		return errors.New("usage: enowx-app decide route [--agent A] [--project DIR] [--json] PROMPT")
	}
	if p.Agent == "" {
		p.Session = os.Getenv(EnvSession)
	}
	if p.Project == "" {
		p.Project, _ = os.Getwd()
	}
	var out proto.RouteChoice
	if err := r.c.Call(proto.MethodDecideRoute, p, &out); err != nil {
		return err
	}
	if asJSON {
		data, _ := json.MarshalIndent(out, "", "  ")
		fmt.Fprintln(r.out, string(data))
		return nil
	}
	if out.Error != "" {
		fmt.Fprintf(r.out, "No answer: %s. Keep %s's own model.\n", out.Error, out.Agent)
		return nil
	}
	if out.Difficulty != "" {
		fmt.Fprintf(r.out, "%s (%.2f): %s\n", out.Difficulty, out.Confidence, out.Why)
	} else {
		fmt.Fprintln(r.out, out.Why)
	}
	model, effort := out.Model, out.Effort
	if model == "" {
		model = "its own"
	}
	if effort == "" {
		effort = "its own"
	}
	fmt.Fprintf(r.out, "model: %s · effort: %s\n", model, effort)
	if len(out.Switch) > 0 {
		fmt.Fprintf(r.out, "in a running session: %s\n", strings.Join(out.Switch, ", "))
	}
	return nil
}

// decideRouteTool is decide route as an MCP tool, for an agent picking
// models for the sessions it starts.
var decideRouteTool = mcpTool{name: "decide_route",
	description: "Which model and effort suit a prompt, for an agent you are about to spawn or prompt: the judge says easy, medium or hard, and enowX maps it to the agent's models (Claude Code: Haiku, Sonnet, Opus; Codex: GPT-6-Luna, then its own) and efforts low, medium, high. Returns JSON {model, effort, why, switch}; model or effort \"\" means the agent's own. Needs the use route on (Settings → Decisions).",
	props:       map[string]mcpProp{"prompt": {"string", "The prompt the agent will get"}, "agent": {"string", "claude, codex, … (this session's agent when left out)"}},
	required:    []string{"prompt"},
	args: func(a map[string]any) []string {
		return append(withFlag([]string{"route", "--json"}, a, "agent", "agent"), str(a, "prompt"))
	}}

package ui

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// The New session dialog's model per prompt (use route, docs/decide.md):
// with the use on, the judge is asked, when the user clicks, how hard the
// first prompt is; its model and effort are taken only when the user says
// so, and the session starts on them.

// routeForm is what the dialog knows of the judge's suggestion. It
// belongs to the main thread.
type routeForm struct {
	on      bool // the use is on on the host
	asking  bool
	asked   string // the prompt asked about
	choice  *proto.RouteChoice
	err     string
	applied bool // the session starts on choice's model and effort
}

// loadRouteOn reads whether the host's judge suggests models.
func (a *App) loadRouteOn(cl *client.Client, path string) {
	f := &a.newSession
	go func() {
		st, err := cl.DecideSettings()
		a.onMain(func() {
			if f.open && f.project.Path == path && err == nil {
				f.route.on = st.Uses[proto.DecideRoute].On
			}
		})
	}()
}

// routeField offers the judge's model and effort for the first prompt.
func (a *App) routeField(c *ui.Context) {
	f := &a.newSession
	r := &f.route
	id := agent.Base(f.agent)
	s := agent.SettingsOf(id)
	if !r.on || len(s.Models) == 0 && len(s.Efforts) == 0 {
		return
	}
	if r.choice != nil && r.choice.Agent != id {
		*r = routeForm{on: true} // another agent: its models differ
	}
	prompt := strings.TrimSpace(f.prompt)
	ui.Row(c.Key("route")).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
		switch {
		case r.asking:
			widget.Spinner(c, 12)
			ui.Text(c, "The judge weighs the prompt…").FontSize(12).TextColor(theme.Muted)
		case r.applied:
			widget.Tag(c, routeLabel(s, *r.choice), theme.Done)
			ui.Text(c, r.choice.Why).FontSize(11.5).TextColor(theme.TextSoft).Shrink(1).MaxLines(2)
			ui.Spacer(c)
			if widget.GhostButton(c.Key("undo"), "Its own model").Clicked() {
				r.applied = false
			}
		case r.err != "" && r.asked == prompt:
			ui.Text(c, r.err).FontSize(11.5).TextColor(theme.Error).Shrink(1)
		case r.choice != nil && r.asked == prompt:
			if r.choice.Model == "" && r.choice.Effort == "" {
				why := r.choice.Why
				if r.choice.Error != "" {
					why = "No answer: " + r.choice.Error
				}
				ui.Text(c, why+" Its own model is kept.").FontSize(11.5).TextColor(theme.Muted).Shrink(1)
				return
			}
			ui.Text(c, "Suggested: "+routeLabel(s, *r.choice)+". "+r.choice.Why).FontSize(11.5).TextColor(theme.TextSoft).Shrink(1).MaxLines(2)
			ui.Spacer(c)
			if widget.Button(c.Key("use"), widget.Secondary, icon.Check, "Use it", "").Height(26).Clicked() {
				r.applied = true
			}
		case prompt != "":
			if widget.GhostButton(c.Key("ask"), "Suggest a model for this prompt").Tooltip("The judge says how hard it is; the cheapest model that fits").Clicked() {
				a.askRoute(prompt)
			}
		}
	})
}

// askRoute asks the host's judge for prompt's model and effort.
func (a *App) askRoute(prompt string) {
	f := &a.newSession
	cl := a.client
	if cl == nil {
		return
	}
	f.route.asking, f.route.asked, f.route.err, f.route.choice = true, prompt, "", nil
	p := proto.DecideRouteParams{Agent: f.agent, Project: f.project.Path, Prompt: prompt}
	go func() {
		out, err := cl.DecideRoute(p)
		a.onMain(func() {
			f.route.asking = false
			if err != nil {
				f.route.err = client.Explain(err)
				return
			}
			f.route.choice = &out
		})
	}()
}

// routeLabel names r's model and effort as the agent's menus do.
func routeLabel(s agent.Settings, r proto.RouteChoice) string {
	var parts []string
	if r.Model != "" {
		name := r.Model
		if m, ok := agent.FindChoice(s.Models, r.Model); ok {
			name = m.Label
		}
		parts = append(parts, name)
	} else {
		parts = append(parts, "its own model")
	}
	if r.Effort != "" {
		parts = append(parts, r.Effort+" effort")
	}
	return strings.Join(parts, " · ")
}

// routeParams starts the session on the suggestion taken.
func (f *newSessionForm) routeParams(p *proto.SessionCreateParams) {
	if f.route.applied && f.route.choice != nil {
		p.Model, p.Effort = f.route.choice.Model, f.route.choice.Effort
	}
}

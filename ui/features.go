package ui

import (
	"fmt"
	"slices"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/features"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// Settings' Features: which of enowX's features agents are told of and
// get tools for, everywhere, on this host or in a project; and agents
// asking the user to turn one on.

// hostFeatures is the connected host's switches, as last read.
type hostFeatures struct {
	host    string
	project string // the project shown, "" for everywhere
	scope   string // where switches go: global, host, project
	st      proto.FeaturesState
	loaded  bool
	err     string
}

// featuresOf is the connected host's switches, read anew when the host
// changed.
func (a *App) featuresOf() *hostFeatures {
	if a.feats == nil || a.feats.host != a.host.ID {
		a.feats = &hostFeatures{host: a.host.ID, scope: features.ScopeGlobal}
		a.loadFeatures()
	}
	return a.feats
}

func (a *App) loadFeatures() {
	hf, cl := a.feats, a.client
	if cl == nil || hf == nil {
		return
	}
	project := ""
	if hf.scope == features.ScopeProject {
		project = hf.project
	}
	go func() {
		st, err := cl.Features(proto.FeaturesParams{Project: project})
		a.onMain(func() { a.featuresRead(hf, st, err) })
	}()
}

func (a *App) featuresRead(hf *hostFeatures, st proto.FeaturesState, err error) {
	if err != nil {
		hf.err = client.Explain(err)
		return
	}
	hf.st, hf.loaded, hf.err = st, true, ""
}

// featureOn reports whether feature id is on for the connected host, on
// until it is known.
func (a *App) featureOn(id string) bool {
	hf := a.featuresOf()
	for _, f := range hf.st.Features {
		if f.ID == id {
			return f.On
		}
	}
	return true
}

// featuresSection is Settings' Features.
func (a *App) featuresSection(c *ui.Context) {
	hf := a.featuresOf()
	widget.PageHeader(c, "Features", "What agents are told of and get tools for", nil)
	if !hf.loaded {
		widget.PageBody(c, true, func() {
			if hf.err != "" {
				widget.Notice(c, icon.Alert, theme.Error, hf.err, "", nil)
				return
			}
			ui.Row(c).Gap(theme.S2).Children(func() {
				widget.Spinner(c, 14)
				widget.CardNote(c, "Reading "+a.host.Name+"'s features…", 0)
			})
		})
		return
	}
	on := map[string]bool{}
	count := 0
	for _, f := range hf.st.Features {
		on[f.ID] = f.On
		if f.On {
			count++
		}
	}
	widget.FigsBand(c, []widget.Fig{
		{Label: "Features on", Value: fmt.Sprintf("%d of %d", count, len(hf.st.Features))},
		{Label: "Agents' guide, tokens", Value: fmt.Sprintf("~%d", hf.st.Tokens)},
		{Label: "Every feature in full", Value: fmt.Sprintf("~%d", hf.st.AllCost)},
		{Label: "Asking to turn on", Value: fmt.Sprint(len(a.featAsks))},
	})
	widget.PageBody(c, true, func() {
		if hf.err != "" {
			widget.Notice(c, icon.Alert, theme.Error, hf.err, "", nil)
		}
		widget.Cols(c, 2).Children(func() {
			widget.Card(c.Key("guide")).Children(func() {
				widget.CardTitle(c, nil, fmt.Sprintf("Agents' guide: ~%d tokens", hf.st.Tokens), func() {
					ui.Text(c, fmt.Sprintf("every feature, in full: ~%d", hf.st.AllCost)).FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine().Shrink(0)
				})
				widget.Meter(c, widget.Segment{Fraction: float64(hf.st.Tokens) / float64(max(1, hf.st.AllCost)), Color: theme.Working})
				lazy := hf.st.Lazy
				if switchRow(c, &lazy, "Load long guides when needed", "Traffic, Android, the vault and project copies are a line in the guide; the agent reads the rest (help NAME) when it needs it.") {
					a.featuresCall(func(cl *client.Client) (proto.FeaturesState, error) { return cl.SetFeaturesLazy(lazy) })
				}
			})
			widget.Card(c.Key("where")).Children(func() {
				widget.CardTitle(c, nil, "Where", nil)
				a.featuresScope(c, hf)
				widget.CardNote(c, "A feature off costs agents no tokens; the app keeps it. Changes apply to sessions started after them: running ones keep what they had until restarted.", 0)
				ui.Row(c).FillWidth().Gap(theme.S2).Wrap().Children(func() {
					ui.Text(c, "Presets").FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine()
					for _, name := range []string{"Minimal", "Web dev", "Android", "All"} {
						if widget.GhostButton(c.Key(name), name).Clicked() {
							scope, project := hf.scope, hf.project
							a.featuresCall(func(cl *client.Client) (proto.FeaturesState, error) {
								return cl.FeaturesPreset(proto.FeaturesPresetParams{Name: name, Scope: scope, Project: project})
							})
						}
					}
				})
			})
		})
		for _, g := range agent.FeatureGroups {
			ui.Column(c.Key(g)).FillWidth().Gap(theme.S2).Children(func() {
				widget.Heading(c, g)
				widget.Cols(c, 4).Children(func() {
					for _, f := range hf.st.Features {
						if f.Group == g {
							a.featureCard(c.Key(f.ID), hf, f, on)
						}
					}
				})
			})
		}
	})
}

// featuresScope picks where switches go, and which project.
func (a *App) featuresScope(c *ui.Context, hf *hostFeatures) {
	scopes := []string{"Everywhere", "This host", "A project"}
	ids := []string{features.ScopeGlobal, features.ScopeHost, features.ScopeProject}
	if i := widget.Segmented(c.Key("scope"), scopes, slices.Index(ids, hf.scope), nil); i >= 0 && ids[i] != hf.scope {
		hf.scope = ids[i]
		if hf.scope == features.ScopeProject && hf.project == "" && len(a.projects) > 0 {
			hf.project = a.projects[0].Path
			if a.project != "" {
				hf.project = a.project
			}
		}
		a.loadFeatures()
	}
	if hf.scope != features.ScopeProject {
		return
	}
	ui.Row(c.Key("projects")).FillWidth().Gap(theme.S2).Wrap().Children(func() {
		for _, p := range a.projects {
			on := p.Path == hf.project
			b := widget.Button(c.Key(p.Path), widget.Secondary, nil, p.Name, "")
			if on {
				b.Background(theme.Selected)
			}
			if b.Clicked() && !on {
				hf.project = p.Path
				a.loadFeatures()
			}
		}
	})
}

// featureCard is a feature's switch, what it is and costs, and what it
// needs that is off.
func (a *App) featureCard(c *ui.Context, hf *hostFeatures, f proto.FeatureInfo, on map[string]bool) {
	widget.Card(c).Children(func() {
		widget.CardTitle(c, nil, f.Name, func() {
			if f.Core {
				widget.Chip(c, "always on", ui.Color{})
				return
			}
			v := f.On
			if widget.Switch(c, &v, "Feature "+f.Name) {
				scope, project, id := hf.scope, hf.project, f.ID
				a.featuresCall(func(cl *client.Client) (proto.FeaturesState, error) {
					return cl.SetFeature(proto.FeatureSetParams{ID: id, On: &v, Scope: scope, Project: project})
				})
			}
		})
		widget.CardNote(c, f.About, 0)
		if missing := features.Missing(f.ID, on); f.On && len(missing) > 0 {
			ui.Row(c.Key("needs")).FillWidth().Gap(theme.S1).AlignItems(ui.Start).Children(func() {
				ui.Icon(c, icon.Alert).FontSize(12).TextColor(theme.Warning).Shrink(0)
				widget.CardNote(c, "Needs "+namesOf(missing)+", which is off", 0).TextColor(theme.Warning)
			})
		}
		widget.CardFoot(c, func() {
			if f.Tokens > 0 {
				ui.Text(c, fmt.Sprintf("~%d tokens", f.Tokens)).FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine().Shrink(0)
			}
			if f.From != "" && !f.Core {
				ui.Spacer(c)
				widget.Tag(c, "set "+map[string]string{"global": "everywhere", "host": "on this host", "project": "for the project", "session": "for a session"}[f.From], theme.Muted)
			}
		})
	})
}

func namesOf(ids []string) string {
	var out string
	for i, id := range ids {
		f, _ := agent.FindFeature(id)
		if i > 0 {
			out += ", "
		}
		out += f.Name
	}
	return out
}

// featuresCall runs fn on the connected host and shows the switches it
// answers with.
func (a *App) featuresCall(fn func(cl *client.Client) (proto.FeaturesState, error)) {
	hf, cl := a.featuresOf(), a.client
	if cl == nil {
		return
	}
	go func() {
		st, err := fn(cl)
		a.onMain(func() { a.featuresRead(hf, st, err) })
	}()
}

// featureAsk is an agent asking the user to turn a feature on, and the
// connection it came over.
type featureAsk struct {
	req   proto.FeatureRequest
	cl    *client.Client
	on    string    // its host's name
	until time.Time // past it, the daemon waits no more
}

// featureRequested queues an agent's request for the user.
func (a *App) featureRequested(cl *client.Client, host string, req proto.FeatureRequest) {
	a.featAsks = append(a.featAsks, featureAsk{req, cl, host, askUntil(hostAskLife)})
	a.notify(fmt.Sprintf("An agent on %s asks to turn on %s", host, req.ID))
}

// answerFeatureAsk answers ask and takes it off the queue.
func (a *App) answerFeatureAsk(ask featureAsk, allowed bool, scope string) {
	a.featAsks = slices.DeleteFunc(a.featAsks, func(q featureAsk) bool { return q.req.RequestID == ask.req.RequestID })
	go func() {
		if allowed {
			on := true
			_, _ = ask.cl.SetFeature(proto.FeatureSetParams{ID: ask.req.ID, On: &on, Scope: scope, Session: ask.req.Session})
		}
		_ = ask.cl.AnswerFeature(proto.FeatureAnswer{RequestID: ask.req.RequestID, Allowed: allowed, Scope: scope})
		a.onMain(a.loadFeatures)
	}()
}

// featureAskCard asks the user about the first request waiting.
func (a *App) featureAskCard(*ui.Context) *widget.Ask {
	if len(a.featAsks) == 0 {
		return nil
	}
	ask := a.featAsks[0]
	f, _ := agent.FindFeature(ask.req.ID)
	name := ask.req.ID
	if f.Name != "" {
		name = f.Name
	}
	who := "An agent"
	if s, ok := a.sessionByID(ask.req.Session); ok {
		who = a.agentName(s.Agent) + " (" + a.sessionName(s) + ")"
	}
	msg := who + " on " + ask.on + " asks to turn on " + name + "."
	if ask.req.Reason != "" {
		msg += " Why: " + ask.req.Reason + "."
	}
	if f.About != "" {
		msg += " " + f.About
	}
	answer := func(allowed bool, scope string) func() {
		return func() { a.answerFeatureAsk(ask, allowed, scope) }
	}
	buttons := []widget.AskButton{{Kind: widget.Secondary, Label: "Keep it off", Do: answer(false, "")}}
	if ask.req.Session != "" {
		buttons = append(buttons, widget.AskButton{Kind: widget.Secondary, Label: "For this session", Do: answer(true, features.ScopeSession)})
	}
	buttons = append(buttons, widget.AskButton{Kind: widget.Primary, Label: "Turn on everywhere", Do: answer(true, features.ScopeGlobal)})
	return &widget.Ask{Key: "feature:" + ask.req.RequestID, Title: "Turn on " + name + "?", About: msg, Buttons: buttons}
}

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
	sectionIntro(c, "Features", "What agents are told of and get tools for. A feature off costs them no tokens; the app keeps it. Changes apply to sessions started after them: running ones keep what they had until restarted.")
	if hf.err != "" {
		ui.Text(c, hf.err).FontSize(theme.TextLabel).TextColor(theme.Error)
	}
	if !hf.loaded {
		ui.Text(c, "Reading "+a.host.Name+"'s features…").FontSize(theme.TextLabel).TextColor(theme.Muted)
		return
	}
	widget.Card(c).FillWidth().Children(func() {
		ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Text(c, fmt.Sprintf("Agents' guide: ~%d tokens", hf.st.Tokens)).FontSize(theme.TextUI).TextColor(theme.TextStrong)
			ui.Spacer(c)
			ui.Text(c, fmt.Sprintf("every feature, in full: ~%d", hf.st.AllCost)).FontSize(theme.TextLabel).TextColor(theme.Muted)
		})
		widget.Meter(c, widget.Segment{Fraction: float64(hf.st.Tokens) / float64(max(1, hf.st.AllCost)), Color: theme.Accent})
		lazy := hf.st.Lazy
		if switchRow(c, &lazy, "Load long guides when needed", "Traffic, Android, the vault and project copies are a line in the guide; the agent reads the rest (help NAME) when it needs it.") {
			a.featuresCall(func(cl *client.Client) (proto.FeaturesState, error) { return cl.SetFeaturesLazy(lazy) })
		}
	})
	widget.Field(c, "Where", func() {
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
		if hf.scope == features.ScopeProject {
			ui.Row(c).FillWidth().Gap(6).Wrap().Children(func() {
				for _, p := range a.projects {
					on := p.Path == hf.project
					b := widget.Button(c.Key(p.Path), widget.Secondary, nil, p.Name, "")
					if on {
						b.Background(theme.Selected).Border(1, theme.AccentLine)
					}
					if b.Clicked() && !on {
						hf.project = p.Path
						a.loadFeatures()
					}
				}
			})
		}
	})
	widget.Field(c, "Presets", func() {
		ui.Row(c).FillWidth().Gap(6).Wrap().Children(func() {
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
	on := map[string]bool{}
	for _, f := range hf.st.Features {
		on[f.ID] = f.On
	}
	for _, g := range agent.FeatureGroups {
		widget.Field(c.Key(g), g, func() {
			ui.Column(c).FillWidth().Gap(4).Children(func() {
				for _, f := range hf.st.Features {
					if f.Group == g {
						a.featureRow(c.Key(f.ID), hf, f, on)
					}
				}
			})
		})
	}
}

// featureRow is a feature's switch, what it is and costs, and what it
// needs that is off.
func (a *App) featureRow(c *ui.Context, hf *hostFeatures, f proto.FeatureInfo, on map[string]bool) {
	ui.Row(c).FillWidth().Padding(10).Gap(12).Radius(theme.RadiusControl).Border(1, theme.Border).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, f.Name).FontSize(theme.TextUI).TextColor(theme.Text)
				if f.Tokens > 0 {
					ui.Text(c, fmt.Sprintf("~%d tokens", f.Tokens)).FontSize(theme.TextLabel).TextColor(theme.Muted)
				}
				if f.From != "" && !f.Core {
					widget.Tag(c, "set "+map[string]string{"global": "everywhere", "host": "on this host", "project": "for the project", "session": "for a session"}[f.From], theme.Muted)
				}
			})
			ui.Text(c, f.About).FontSize(theme.TextLabel).TextColor(theme.Muted)
			if missing := features.Missing(f.ID, on); f.On && len(missing) > 0 {
				ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
					ui.Icon(c, icon.Alert).FontSize(11).TextColor(theme.Warning)
					ui.Text(c, "Needs "+namesOf(missing)+", which is off").FontSize(theme.TextLabel).TextColor(theme.Warning)
				})
			}
		})
		if f.Core {
			ui.Text(c, "always on").FontSize(theme.TextLabel).TextColor(theme.Muted)
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

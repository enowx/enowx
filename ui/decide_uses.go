package ui

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/projectlinks"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// sureChoices are how sure the judge must be, offered.
var sureChoices = []float64{0.6, 0.75, 0.9, 0.97}

// useHooks are the hooks a use acts through, from their templates.
var useHooks = map[string]string{proto.DecideApprove: "Have the judge approve commands", proto.DecideTrim: "Trim long test and build output"}

// usesTab is each use: on, how sure, its judge, its calls a day; what it
// saved; this project's own policy.
func (a *App) usesTab(c *ui.Context, st *decideState) {
	if tokens, loops := weekSaved(st.log); tokens > 0 || loops > 0 {
		widget.Notice(c, icon.Sparkle, theme.Done, fmt.Sprintf("This week: about %s tokens kept out of agents' context, %d loops stopped", count(tokens), loops), "", nil)
	}
	for _, use := range proto.DecideUses {
		a.useCard(c.Key(use), st, use)
	}
	if p, ok := a.currentProject(); ok {
		a.projectPolicy(c, st, p)
	}
}

func (a *App) useCard(c *ui.Context, st *decideState, use string) {
	u := st.set.Uses[use]
	changed := false
	widget.Card(c).FillWidth().Padding(12, 14).Gap(10).Children(func() {
		if switchRowKey(c, "Use: "+useInfo[use][0], &u.On, useInfo[use][0], useInfo[use][1]) {
			changed = true
		}
		if !u.On {
			return
		}
		ui.Row(c).FillWidth().Gap(12).Wrap().Children(func() {
			widget.Field(c, "Acts when the judge is at least", func() {
				sure := u.MinConfidence
				if sure == 0 {
					sure = 0.75
					if use == proto.DecideApprove {
						sure = 0.9
					}
				}
				labels := make([]string, len(sureChoices))
				cur := 0
				for i, v := range sureChoices {
					labels[i] = fmt.Sprintf("%.0f%% sure", v*100)
					if v == sure {
						cur = i
					}
				}
				if i := widget.Segmented(c, labels, cur, nil); i >= 0 {
					u.MinConfidence, changed = sureChoices[i], true
				}
			})
			widget.Field(c, "Calls a day", func() {
				v := st.calls[use]
				if widget.Input(c.Key("calls"), &v, "300").Width(90).Changed() {
					st.calls[use] = v
					if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n >= 0 {
						u.DailyCalls, changed = n, true
					}
				}
			})
		})
		widget.Field(c, "Judge", func() {
			opts := []string{""}
			names := map[string]string{"": "The judges in their order, the next when one fails"}
			for _, j := range st.judges {
				opts, names[j.ID] = append(opts, j.ID), j.Label+" alone"
			}
			was := u.Judge
			hookSelect(c.Key("judge"), "Judge", &u.Judge, opts, names)
			changed = changed || was != u.Judge
		})
		if t, ok := useHooks[use]; ok {
			ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Acts through a hook before a tool runs.").FontSize(11.5).TextColor(theme.Muted)
				ui.Spacer(c)
				if widget.GhostButton(c.Key("hook"), "Add the hook…").Clicked() {
					a.startHookTemplate(t)
				}
			})
		}
	})
	if changed {
		if st.set.Uses == nil {
			st.set.Uses = map[string]proto.DecideUse{}
		}
		st.set.Uses[use] = u
		a.saveDecide(st)
	}
}

// startHookTemplate opens Settings → Hooks with template name's hook to
// save.
func (a *App) startHookTemplate(name string) {
	a.settingsSection = sectionHooks
	hs := a.hooksOf()
	for _, t := range hookTemplates {
		if t.name == name {
			hs.form.start(t.hook, hostPlace, true)
		}
	}
}

// projectPolicy is project p's own policy, the host's when unset.
func (a *App) projectPolicy(c *ui.Context, st *decideState, p proto.ProjectInfo) {
	id := projectlinks.ReadID(p.Path)
	if id == "" {
		return // a project without an ID of its own keeps the host's
	}
	cur := st.set.Projects[id].Policy
	opts := append([]string{""}, policies...)
	names := map[string]string{"": "The host's policy"}
	for k, v := range policyNames {
		names[k] = v
	}
	widget.Field(c, "In “"+p.Name+"”", func() {
		was := cur
		hookSelect(c.Key("project-policy"), "Project policy", &cur, opts, names)
		if cur == proto.DecideOneProvider {
			cur = proto.DecideSameProvider // one provider is set for the host
		}
		if was != cur {
			if st.set.Projects == nil {
				st.set.Projects = map[string]proto.DecideProject{}
			}
			if cur == "" {
				delete(st.set.Projects, id)
			} else {
				st.set.Projects[id] = proto.DecideProject{Policy: cur}
			}
			a.saveDecide(st)
		}
		ui.Text(c, "A client's project can keep to local models, say.").FontSize(11.5).TextColor(theme.Muted)
	})
}

// judgeForm adds a judge of the user's: a model API (its key from the
// vault) or a model server on the host.
type judgeForm struct {
	open                    bool
	kind, provider          string
	url, cred, model, label string
	err                     string
}

func (a *App) judgeFormSheet(c *ui.Context, st *decideState) {
	f := &st.adding
	if !f.open {
		return
	}
	widget.FormSheet(c, &f.open, 520, "Add a judge", "A small, fast model answers best: Haiku, a mini or flash model, or a 7–8B local one.", func() {
		if f.err != "" {
			widget.Notice(c, icon.Alert, theme.Error, f.err, "", nil)
		}
		cur := 0
		if f.kind == proto.JudgeAPI {
			cur = 1
		}
		if i := widget.Segmented(c, []string{"A model server on " + a.host.Name, "A model API"}, cur, nil); i >= 0 {
			f.kind = proto.JudgeLocal
			if i == 1 {
				f.kind = proto.JudgeAPI
				if f.provider == "" {
					f.provider = proto.ProviderOpenAI
				}
			}
		}
		if f.kind == proto.JudgeAPI {
			widget.Field(c, "Provider", func() {
				hookSelect(c.Key("provider"), "Provider", &f.provider, providers, providerNames)
			})
			widget.Field(c, "Key, from the vault", func() { a.ragCredSelect(c, "judge-key", &f.cred) })
			widget.Field(c, "Address (OpenAI-compatible; empty for the provider's own)", func() {
				widget.Input(c, &f.url, "https://api.example.com/v1").FillWidth().Font(theme.MonoFont)
			})
		} else {
			widget.Field(c, "Address (OpenAI-compatible)", func() {
				widget.Input(c, &f.url, "http://127.0.0.1:11434/v1").FillWidth().Font(theme.MonoFont)
			})
		}
		widget.Field(c, "Model", func() { widget.Input(c, &f.model, "qwen3:8b").FillWidth().Font(theme.MonoFont) })
		widget.Field(c, "Name (optional)", func() { widget.Input(c, &f.label, "").FillWidth() })
	}, func() {
		if widget.GhostButton(c, "Cancel").Clicked() {
			f.open = false
		}
		if widget.PrimaryButton(c, icon.Plus, "Add", "").Disabled(strings.TrimSpace(f.model) == "").Clicked() {
			j := proto.Judge{Kind: f.kind, Provider: f.provider, URL: strings.TrimSpace(f.url), Cred: f.cred, Model: strings.TrimSpace(f.model), Label: strings.TrimSpace(f.label)}
			if j.Kind == proto.JudgeAPI && j.Cred == "" && j.URL == "" {
				f.err = "Pick the vault's credential with the API's key."
				return
			}
			a.addJudge(st, j)
		}
	})
}

func (a *App) addJudge(st *decideState, j proto.Judge) {
	cl, set := a.client, st.set
	set.Own = append(set.Own, j)
	go func() {
		out, err := cl.SetDecideSettings(set)
		judges, _ := cl.DecideJudges()
		a.onMain(func() {
			if err != nil {
				st.adding.err = client.Explain(err)
				return
			}
			st.set, st.judges, st.adding = out, judges, judgeForm{}
		})
	}()
}

// decideEvent shows a decision where it acted: a refusal or a stopped loop
// asks the user whether to go on; the rest only refreshes the log.
func (a *App) decideEvent(ev proto.Event) {
	if ev.Name != proto.EventDecision {
		return
	}
	var d proto.Decision
	if json.Unmarshal(ev.Data, &d) != nil {
		return
	}
	a.onMain(func() {
		if a.decideSet.asked && a.decideSet.host == a.host.ID {
			a.decideSet.log = append([]proto.Decision{d}, a.decideSet.log...)
		}
		if d.Acted == proto.ActedPaused || d.Acted == proto.ActedRefused {
			a.decision = &d
		}
	})
}

// decisionDialog asks what to do after the judge stopped an agent or
// refused its command: go on (the judge was wrong) or leave it.
func (a *App) decisionDialog(c *ui.Context) {
	d := a.decision
	if d == nil {
		return
	}
	open := true
	who := a.sessionLabel(d.Session)
	title, body, goOn, prompt := "", "", "", ""
	if d.Acted == proto.ActedPaused {
		title, body, goOn = "enowX stopped "+who+": it seemed to loop", d.Why+" Its turn was stopped so it doesn't spend more tokens on the same thing.", "Let it go on"
		prompt = "That wasn't a loop: go on."
	} else {
		title, body, goOn = "enowX's judge refused a command in "+who, d.Why, "Let it run it"
		prompt = "The user allows the command the judge refused: run it again."
	}
	widget.Dialog(c, &open, 480, 12, func() {
		widget.DialogHeader(c, title, body, false)
		ui.Row(c).FillWidth().Gap(8).Children(func() {
			ui.Spacer(c)
			if widget.GhostButton(c, "Leave it").Clicked() {
				a.decision = nil
			}
			if widget.PrimaryButton(c, icon.Play, goOn, "").Clicked() {
				a.decision = nil
				a.overruleDecision(*d, prompt)
			}
		})
	})
	if !open {
		a.decision = nil
	}
}

// overruleDecision marks decision d wrong and tells its session's agent
// to go on.
func (a *App) overruleDecision(d proto.Decision, prompt string) {
	cl := a.client
	if cl == nil {
		return
	}
	go func() {
		_ = cl.MarkDecision(proto.DecideMark{ID: d.ID, Wrong: true, Note: "overruled from the session"})
		err := cl.Call(proto.MethodSessionInput, proto.SessionInputParams{ID: d.Session, Text: prompt, Paste: true, Enter: true}, nil)
		a.onMain(func() {
			if err != nil {
				a.status = "Couldn't tell the agent: " + client.Explain(err)
			}
		})
	}()
}

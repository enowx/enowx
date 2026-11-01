package ui

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/decide"
	"github.com/enowx/enowx/core/projectlinks"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// sureChoices are how sure the judge must be, offered.
var sureChoices = []float64{0.6, 0.75, 0.9, 0.97}

// useHooks are the hooks that replace a use's built-in one, from their
// templates.
var useHooks = map[string]string{proto.DecideApprove: "Have the judge approve commands", proto.DecideTrim: "Trim long test and build output"}

// usesTab is what the judge saved, this project's own policy, and each use:
// on, how sure, its judge, its calls a day.
func (a *App) usesTab(c *ui.Context, st *decideState) {
	tokens, loops := weekSaved(st.log)
	p, inProject := a.currentProject()
	inProject = inProject && projectlinks.ReadID(p.Path) != "" // one without an ID of its own keeps the host's
	if tokens > 0 || loops > 0 || inProject {
		ui.Column(c.Key("uses-top")).FillWidth().Shrink(0).Padding(theme.S4, theme.S5, 0, theme.S5).Gap(theme.S3).Children(func() {
			if tokens > 0 || loops > 0 {
				widget.Banner(c, theme.Done, fmt.Sprintf("This week: about %s tokens kept out of agents' context, %d loops stopped", count(tokens), loops), nil)
			}
			if inProject {
				a.projectPolicy(c, st, p)
			}
		})
	}
	widget.FillGrid(c, 3, len(proto.DecideUses), 200, func() {
		for _, use := range proto.DecideUses {
			a.useCard(c.Key(use), st, use)
		}
	})
}

// callsToday counts use's decisions in the log today.
func callsToday(log []proto.Decision, use string) int {
	n := 0
	now := time.Now()
	for _, d := range log {
		if t := d.Time.Local(); d.Use == use && t.YearDay() == now.YearDay() && t.Year() == now.Year() {
			n++
		}
	}
	return n
}

func (a *App) useCard(c *ui.Context, st *decideState, use string) {
	u := st.set.Uses[use]
	changed := false
	widget.GridCard(c).Gap(theme.S2).Padding(theme.S3, theme.S4).Children(func() {
		ui.Row(c.Key("title")).FillWidth().Shrink(0).MinHeight(24).Gap(theme.S2).Children(func() {
			ui.Row(c).Grow(1).Basis(0).MinWidth(0).Children(func() { widget.Running(c, useInfo[use][0], widget.RunStyle{Weight: 500}) })
			if t, ok := useHooks[use]; ok && u.On {
				if widget.GhostButton(c.Key("hook"), "Set your own…").Tooltip("Built in while this is on. A hook of your own replaces it, to say what it covers.").Clicked() {
					a.startHookTemplate(t)
				}
			}
			if widget.Switch(c.Key("on"), &u.On, "Use: "+useInfo[use][0]) {
				changed = true
			}
		})
		widget.CardNote(c, useShort[use], 2).Shrink(0).Tooltip(useInfo[use][1])
		if !u.On {
			return
		}
		ui.Row(c.Key("sure")).FillWidth().Shrink(0).Gap(theme.S2).Children(func() {
			today := strconv.Itoa(callsToday(st.log, use)) + " today"
			if u.DailyCalls > 0 {
				today = strconv.Itoa(callsToday(st.log, use)) + " of " + strconv.Itoa(u.DailyCalls) + " today"
			}
			ui.Row(c).Grow(1).Basis(0).MinWidth(0).Children(func() { widget.Running(c, today, widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft}) })
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
				labels[i] = fmt.Sprintf("%.0f%%", v*100)
				if v == sure {
					cur = i
				}
			}
			ui.Row(c).Shrink(0).Tooltip("Acts when the judge is at least this sure").Children(func() {
				if i := widget.Segmented(c, labels, cur, nil); i >= 0 {
					u.MinConfidence, changed = sureChoices[i], true
				}
			})
		})
		ui.Row(c.Key("who")).FillWidth().Shrink(0).Gap(theme.S2).Children(func() {
			ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
				widget.Field(c, "Judge", func() {
					opts := []string{""}
					names := map[string]string{"": "The judges in their order"}
					for _, j := range st.judges {
						opts, names[j.ID] = append(opts, j.ID), j.Label+" alone"
					}
					was := u.Judge
					hookSelect(c.Key("judge"), "Judge", &u.Judge, opts, names)
					changed = changed || was != u.Judge
				})
			})
			ui.Column(c).Width(84).Shrink(0).Children(func() {
				widget.Field(c, "Calls a day", func() {
					v := st.calls[use]
					if widget.Input(c.Key("calls"), &v, "No limit").FillWidth().Changed() {
						st.calls[use] = v
						if n, ok := parseDailyCalls(v); ok {
							u.DailyCalls, changed = n, true
						}
					}
				})
			})
		})
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
	cur := st.set.Projects[id].Policy
	opts := append([]string{""}, policies...)
	names := map[string]string{"": "The host's policy"}
	for k, v := range policyNames {
		names[k] = v
	}
	was := cur
	decidePick(c.Key("project-policy"), "In “"+p.Name+"”", 240, "A client's project can keep to local models, say.", func() {
		hookSelect(c.Key("project-policy"), "Project policy", &cur, opts, names)
	})
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
}

func (a *App) addJudge(st *decideState, j proto.Judge) {
	cl, set, key := a.client, st.set, ""
	if !st.adding.fromVault {
		key = strings.TrimSpace(st.adding.key)
	}
	set.Own = append(set.Own, j)
	go func() {
		out, err := cl.SetDecideSettings(set)
		if err == nil && key != "" {
			err = cl.SetJudgeKey(decide.OwnID(j), key)
		}
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
			a.decisions = append(a.decisions, &d)
		}
	})
}

// decisionAsk asks what to do after the judge stopped an agent or refused its
// command: go on (the judge was wrong) or leave it.
func (a *App) decisionAsk(*ui.Context) *widget.Ask {
	if len(a.decisions) == 0 {
		return nil
	}
	d := a.decisions[0]
	who := a.sessionLabel(d.Session)
	title, body, goOn, prompt := "", "", "", ""
	if d.Acted == proto.ActedPaused {
		title, body, goOn = "enowX stopped "+who+": it seemed to loop", d.Why+" Its turn was stopped so it doesn't spend more tokens on the same thing.", "Let it go on"
		prompt = "That wasn't a loop: go on."
	} else {
		title, body, goOn = "enowX's judge refused a command in "+who, d.Why, "Let it run it"
		prompt = "The user allows the command the judge refused: run it again."
	}
	done := func() { a.decisions = slices.DeleteFunc(a.decisions, func(x *proto.Decision) bool { return x == d }) }
	return &widget.Ask{Key: fmt.Sprintf("decision:%p", d), Title: title, About: body, Buttons: []widget.AskButton{
		{Kind: widget.Secondary, Label: "Leave it", Do: done},
		{Kind: widget.Primary, Icon: icon.Play, Label: goOn, Do: func() { done(); a.overruleDecision(*d, prompt) }},
	}}
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

// parseDailyCalls reads the Calls a day field: empty or "unlimited" is no
// limit (0), a number that limit.
func parseDailyCalls(v string) (int, bool) {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" || v == "unlimited" || v == "no limit" {
		return 0, true
	}
	n, err := strconv.Atoi(v)
	return n, err == nil && n >= 0
}

package ui

import (
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// Settings → Decisions: enowX's judge on the connected host
// (proto.DecideSettings): who may judge, the judges in the order tried,
// each use, the log.

const sectionDecide = "Decisions"

// decideState is the section's. It belongs to the main thread.
type decideState struct {
	host    string
	asked   bool
	loaded  bool
	set     proto.DecideSettings
	judges  []proto.Judge
	log     []proto.Decision
	err     string
	tab     int
	testing map[string]bool
	logUse  string // "" every use
	calls   map[string]string
	adding  judgeForm
}

var policyNames = map[string]string{
	proto.DecideSameProvider: "The session's agent's provider (Claude Code: Anthropic, Codex: OpenAI)",
	proto.DecideOneProvider:  "One provider I pick",
	proto.DecideAnyProvider:  "Any provider",
	proto.DecideLocalOnly:    "Local models only: nothing leaves the host",
}

var policies = []string{proto.DecideSameProvider, proto.DecideOneProvider, proto.DecideAnyProvider, proto.DecideLocalOnly}

var providerNames = map[string]string{
	proto.ProviderAnthropic: "Anthropic", proto.ProviderOpenAI: "OpenAI", proto.ProviderGoogle: "Google",
	proto.ProviderOpenRouter: "OpenRouter", proto.ProviderLocal: "Local",
	proto.ProviderTypeSafe: "TypeSafe", proto.ProviderCloudflare: "Cloudflare", "other": "Other",
}

var providers = []string{proto.ProviderAnthropic, proto.ProviderOpenAI, proto.ProviderGoogle, proto.ProviderOpenRouter}

var useInfo = map[string][2]string{
	proto.DecideState:   {"A quiet session's state", "When an agent without hooks of its own goes quiet, the judge reads its screen: one that asks you something is marked waiting, with the input sound; one stuck or failed gets the error sound."},
	proto.DecideApprove: {"Approving commands", "Before an agent's command runs: allowed without asking you, left to you, or refused. Acts through a hook: Ask the judge."},
	proto.DecideTrim:    {"Trimming long output", "A test's, build's or install's long output reaches the agent cut to what matters, saving its context; the whole stays in a file. Acts through a hook: Trim its long output."},
	proto.DecideLoop:    {"Stopping loops", "When an agent's commands keep failing alike, the judge reads its last steps; a loop stops its turn and asks you how to go on, before more tokens go."},
	proto.DecideRoute:   {"A model per prompt", "The judge says how hard a prompt is, and enowX suggests the agent's model and effort to match: the cheapest for easy ones. New session offers it; agents ask with decide route. A running session's model never changes on its own."},
	proto.DecideContext: {"RAG: context or not", "Before a prompt gets a project's search results (RAG's Context), the judge keeps only those that bear on it, or none."},
	proto.DecideMemory:  {"RAG: memories to keep", "When a session ends, its last answer is offered as a memory only when the judge finds a decision in it, written as that decision."},
	proto.DecideChat:    {"Team chat urgency", "The judge reads a team message: urgent ones sound and show a banner even when the chat is set to mentions or off; quiet ones only count as unread. A message naming you is never kept quiet."},
	proto.DecideFlaky:   {"Flaky or real test failures", "When a check fails, the judge reads its output: flaky (timing, network, order) or real, shown on the check with why; a flaky one offers Run again."},
}

func (a *App) decideOf() *decideState {
	if a.decideSet.host != a.host.ID || !a.decideSet.asked {
		a.decideSet = decideState{host: a.host.ID, asked: true, testing: map[string]bool{}, calls: map[string]string{}}
		a.loadDecide()
	}
	return &a.decideSet
}

func (a *App) loadDecide() {
	cl, host := a.client, a.host.ID
	if cl == nil {
		return
	}
	go func() {
		set, err := cl.DecideSettings()
		judges, _ := cl.DecideJudges()
		log, _ := cl.DecideLog(proto.DecideLogParams{Limit: 300})
		a.onMain(func() {
			st := &a.decideSet
			if st.host != host {
				return
			}
			st.loaded, st.set, st.judges, st.log, st.err = true, set, judges, log, ""
			if err != nil {
				st.err = client.Explain(err)
			}
			for _, u := range proto.DecideUses {
				if n := set.Uses[u].DailyCalls; n > 0 {
					st.calls[u] = strconv.Itoa(n)
				}
			}
		})
	}()
}

// saveDecide keeps the settings as st has them, then reads the judges
// again (their order, what was added).
func (a *App) saveDecide(st *decideState) {
	cl, set := a.client, st.set
	go func() {
		out, err := cl.SetDecideSettings(set)
		judges, _ := cl.DecideJudges()
		a.onMain(func() {
			if err != nil {
				a.status = "Decisions: " + client.Explain(err)
				a.loadDecide()
				return
			}
			st.set, st.judges = out, judges
		})
	}()
}

func (a *App) decideSection(c *ui.Context) {
	st := a.decideOf()
	sectionIntro(c, "Decisions", "A small, cheap model, the judge, that enowX asks instead of guessing: whether a quiet session needs you, whether an agent's command may run, what of a long output matters, whether an agent loops. Each use is off until you turn it on; with no answer, enowX does what it does without.")
	switch {
	case st.err != "":
		widget.Notice(c, icon.Alert, theme.Error, st.err, "", nil)
		return
	case !st.loaded:
		widget.Spinner(c, 14)
		return
	}
	ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
		if i := widget.Segmented(c, []string{"Judges", "Uses", "Log"}, st.tab, nil); i >= 0 && i != st.tab {
			st.tab = i
			a.loadDecide()
		}
		ui.Spacer(c)
		switch st.tab {
		case 0:
			busy := len(st.testing) > 0
			if widget.GhostButton(c, "Test all").Disabled(busy || len(st.judges) == 0).Clicked() {
				a.testJudges(st, "")
			}
			if widget.PrimaryButton(c, icon.Plus, "Add a judge", "").Clicked() {
				st.adding = judgeForm{open: true, kind: proto.JudgeLocal, url: "http://127.0.0.1:11434/v1"}
			}
		case 2:
			if widget.GhostButton(c, "Refresh").Clicked() {
				a.loadDecide()
			}
		}
	})
	switch st.tab {
	case 0:
		a.judgesTab(c, st)
	case 1:
		a.usesTab(c, st)
	default:
		a.decideLogTab(c, st)
	}
	a.judgeFormSheet(c, st)
}

// judgesTab is who may judge, and the judges in the order tried.
func (a *App) judgesTab(c *ui.Context, st *decideState) {
	widget.Field(c, "Who may judge", func() {
		before := st.set.Policy
		hookSelect(c, "Policy", &st.set.Policy, policies, policyNames)
		if st.set.Policy == proto.DecideOneProvider {
			if st.set.Provider == "" {
				st.set.Provider = proto.ProviderAnthropic
			}
			was := st.set.Provider
			hookSelect(c.Key("provider"), "Provider", &st.set.Provider, providers, providerNames)
			if was != st.set.Provider {
				a.saveDecide(st)
			}
		}
		if before != st.set.Policy && (st.set.Policy != proto.DecideOneProvider || st.set.Provider != "") {
			a.saveDecide(st)
		}
		ui.Text(c, "Local models are always allowed. A judge not allowed for a session is skipped; the next in the order is asked when one fails or takes too long.").FontSize(11.5).TextColor(theme.Muted)
	})
	if len(st.judges) == 0 {
		widget.EmptyState(c, icon.Brain, "No judge on "+a.host.Name, "Install Claude Code or Codex (their own sign-in judges), add an Anthropic, OpenAI, Gemini or OpenRouter key to the vault, or run Ollama or LM Studio.", nil)
		return
	}
	for i, j := range st.judges {
		a.judgeRow(c.Key(j.ID), st, i, j)
	}
}

func (a *App) judgeRow(c *ui.Context, st *decideState, i int, j proto.Judge) {
	off := slices.Contains(st.set.Off, j.ID)
	widget.Card(c).FillWidth().Padding(10, 12).Children(func() {
		ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Text(c, strconv.Itoa(i+1)).FontSize(11).TextColor(theme.Muted).Width(14)
			ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Text(c, j.Label).FontSize(12.5).SingleLine()
					widget.Tag(c, providerNames[j.Provider], theme.Muted)
					if !underPolicy(st.set, j) {
						widget.Tag(c, "not under this policy", theme.Warning)
					}
					switch {
					case st.testing[j.ID]:
						widget.Spinner(c, 10)
					case j.Tested.IsZero():
						widget.Tag(c, "not tested", theme.Muted)
					case j.OK:
						widget.Tag(c, judgeSpeed(j), theme.Done)
					default:
						widget.Tag(c, "failed", theme.Error)
					}
				})
				ui.Text(c, judgeHow(j)).FontSize(11).TextColor(theme.Muted).SingleLine()
				if !j.OK && j.Error != "" {
					ui.Text(c, j.Error).FontSize(11).TextColor(theme.Error).MaxLines(2)
				}
			})
			if widget.IconButton(c.Key("up"), icon.ChevronUp, "Ask it sooner", false).Disabled(i == 0).Clicked() && i > 0 {
				a.moveJudge(st, i, i-1)
			}
			if widget.IconButton(c.Key("down"), icon.ChevronDown, "Ask it later", false).Disabled(i == len(st.judges)-1).Clicked() && i < len(st.judges)-1 {
				a.moveJudge(st, i, i+1)
			}
			if widget.GhostButton(c.Key("test"), "Test").Disabled(st.testing[j.ID]).Clicked() {
				a.testJudges(st, j.ID)
			}
			on := !off
			if widget.Switch(c.Key("on"), &on, "") {
				st.set.Off = slices.DeleteFunc(slices.Clone(st.set.Off), func(id string) bool { return id == j.ID })
				if !on {
					st.set.Off = append(st.set.Off, j.ID)
				}
				a.saveDecide(st)
			}
			if j.Own && widget.IconButton(c.Key("rm"), icon.Trash, "Remove", false).Clicked() {
				a.confirm("Remove the judge "+j.Label+"?", "It is no longer asked; a use set to it alone goes back to the order.", "Remove", func() {
					st.set.Own = slices.DeleteFunc(slices.Clone(st.set.Own), func(x proto.Judge) bool { return x.ID == j.ID })
					a.saveDecide(st)
				})
			}
		})
	})
}

// underPolicy says whether judge j may judge at all under st's policy
// (the session's agent's provider: any agent's own).
func underPolicy(st proto.DecideSettings, j proto.Judge) bool {
	switch {
	case j.Provider == proto.ProviderLocal || st.Policy == proto.DecideAnyProvider:
		return true
	case st.Policy == proto.DecideLocalOnly:
		return false
	case st.Policy == proto.DecideOneProvider:
		return j.Provider == st.Provider
	}
	return j.Provider == proto.ProviderAnthropic || j.Provider == proto.ProviderOpenAI || j.Provider == proto.ProviderGoogle
}

func judgeSpeed(j proto.Judge) string {
	s := fmt.Sprintf("%.1f s", float64(j.Ms)/1000)
	if j.Ms < 1000 {
		s = fmt.Sprintf("%d ms", j.Ms)
	}
	if j.Cost > 0 {
		s += fmt.Sprintf(" · $%.4f", j.Cost)
	}
	return s
}

// judgeHow is how judge j is reached, in words.
func judgeHow(j proto.Judge) string {
	switch j.Kind {
	case proto.JudgeDecision:
		where := j.URL
		if where == "" {
			where = providerNames[j.Provider] + "'s System One"
		}
		if j.Cred != "" {
			return "Decision model · key " + j.Cred + " from the vault · " + where
		}
		return "Decision model · " + where
	case proto.JudgeAgent:
		return "Through " + j.Agent + "'s own sign-in, its tools and project left out"
	case proto.JudgeLocal:
		return "On this host: " + j.URL
	}
	where := "its API"
	if j.URL != "" {
		where = j.URL
	}
	return "Key " + j.Cred + " from the vault · " + where
}

// moveJudge moves the judge at from to to in the order.
func (a *App) moveJudge(st *decideState, from, to int) {
	ids := make([]string, len(st.judges))
	for i, j := range st.judges {
		ids[i] = j.ID
	}
	ids[from], ids[to] = ids[to], ids[from]
	st.judges[from], st.judges[to] = st.judges[to], st.judges[from]
	st.set.Order = ids
	a.saveDecide(st)
}

// testJudges asks judge id (all when "") a question it should allow.
func (a *App) testJudges(st *decideState, id string) {
	cl := a.client
	for _, j := range st.judges {
		if id == "" || j.ID == id {
			st.testing[j.ID] = true
		}
	}
	go func() {
		tested, err := cl.TestJudges(id)
		a.onMain(func() {
			for k := range st.testing {
				if id == "" || k == id {
					delete(st.testing, k)
				}
			}
			if err != nil {
				a.status = "Test: " + client.Explain(err)
				return
			}
			for _, t := range tested {
				if i := slices.IndexFunc(st.judges, func(j proto.Judge) bool { return j.ID == t.ID }); i >= 0 {
					st.judges[i] = t
				}
			}
		})
	}()
}

// decideLogTab is the latest decisions, to mark wrong.
func (a *App) decideLogTab(c *ui.Context, st *decideState) {
	// A list rather than segments: the uses don't fit in a row.
	opts, names := append([]string{""}, proto.DecideUses...), map[string]string{"": "Every use"}
	for _, u := range proto.DecideUses {
		names[u] = useInfo[u][0]
	}
	ui.Row(c).Width(260).Children(func() { hookSelect(c.Key("log-use"), "Use", &st.logUse, opts, names) })
	n := 0
	for _, d := range st.log {
		if st.logUse != "" && d.Use != st.logUse {
			continue
		}
		n++
		a.decisionRow(c.Key(d.ID), st, d)
	}
	if n == 0 {
		widget.EmptyState(c, icon.List, "No decisions yet", "They show here as the judge is asked, with why and what enowX did.", nil)
	}
}

func (a *App) decisionRow(c *ui.Context, st *decideState, d proto.Decision) {
	widget.Card(c).FillWidth().Padding(10, 12).Gap(4).Children(func() {
		ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Text(c, d.Time.Local().Format("Jan 2 15:04")).FontSize(11).TextColor(theme.Muted)
			ui.Text(c, useInfo[d.Use][0]).FontSize(12.5)
			if d.Judge == "" {
				widget.Tag(c, "no answer", theme.Muted)
			} else {
				widget.Tag(c, fmt.Sprintf("%s %.0f%%", d.Verdict, d.Confidence*100), verdictColor(d.Verdict))
			}
			if d.Acted != "" {
				widget.Tag(c, d.Acted, theme.Accent)
			}
			if d.Wrong {
				widget.Tag(c, "marked wrong", theme.Warning)
			}
			if d.Session != "" {
				ui.Text(c, a.sessionLabel(d.Session)).FontSize(11).TextColor(theme.Muted).SingleLine().Shrink(1)
			}
			ui.Spacer(c)
			if d.Judge != "" {
				label := "This was wrong"
				if d.Wrong {
					label = "Right after all"
				}
				if widget.GhostButton(c.Key("mark"), label).Clicked() {
					a.markDecision(st, d.ID, !d.Wrong)
				}
			}
		})
		why := d.Why
		if d.Judge == "" {
			why = d.Error
		}
		ui.Text(c, why).FontSize(12).TextColor(theme.TextSoft)
		meta := fmt.Sprintf("%s · %d ms", d.Judge, d.Ms)
		if d.Saved > 0 {
			meta += fmt.Sprintf(" · %d characters kept from the agent", d.Saved)
		}
		if len(d.Tried) > 0 {
			meta += fmt.Sprintf(" · %d failed first", len(d.Tried))
		}
		ui.Text(c, meta).FontSize(11).TextColor(theme.Muted).SingleLine()
	})
}

func verdictColor(v string) ui.Color {
	switch v {
	case proto.VerdictRefuse, proto.VerdictLooping, proto.VerdictFailed, proto.VerdictStuck, proto.VerdictUrgent, proto.VerdictReal:
		return theme.Error
	case proto.VerdictAsk, proto.VerdictWaiting, proto.VerdictFlaky:
		return theme.Warning
	}
	return theme.Done
}

func (a *App) markDecision(st *decideState, id string, wrong bool) {
	cl := a.client
	go func() {
		err := cl.MarkDecision(proto.DecideMark{ID: id, Wrong: wrong})
		a.onMain(func() {
			if err != nil {
				a.status = client.Explain(err)
				return
			}
			for i := range st.log {
				if st.log[i].ID == id {
					st.log[i].Wrong = wrong
				}
			}
		})
	}()
}

// weekSaved is what the judge saved in the last week: characters trimmed
// (about four a token) and loops stopped.
func weekSaved(log []proto.Decision) (tokens, loops int) {
	for _, d := range log {
		if time.Since(d.Time) > 7*24*time.Hour {
			continue
		}
		tokens += d.Saved / 4
		if d.Acted == proto.ActedPaused && !d.Wrong {
			loops++
		}
	}
	return tokens, loops
}

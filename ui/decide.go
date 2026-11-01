package ui

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
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
	move    keysMove // the typed keys going to the vault
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
	// keying is the judge whose key is being typed, keyInput the key.
	keying   *proto.Judge
	keyInput string
}

var policyNames = map[string]string{
	proto.DecideSameProvider: "The session's agent's provider",
	proto.DecideOneProvider:  "One provider I pick",
	proto.DecideAnyProvider:  "Any provider",
	proto.DecideLocalOnly:    "Local models only",
}

var policies = []string{proto.DecideSameProvider, proto.DecideOneProvider, proto.DecideAnyProvider, proto.DecideLocalOnly}

var providerNames = map[string]string{
	proto.ProviderAnthropic: "Anthropic", proto.ProviderOpenAI: "OpenAI", proto.ProviderGoogle: "Google",
	proto.ProviderOpenRouter: "OpenRouter", proto.ProviderLocal: "Local",
	proto.ProviderTypeSafe: "TypeSafe", proto.ProviderNeuralwatt: "Neuralwatt", proto.ProviderCloudflare: "Cloudflare", "other": "Other",
}

var providers = []string{proto.ProviderAnthropic, proto.ProviderOpenAI, proto.ProviderGoogle, proto.ProviderOpenRouter, proto.ProviderNeuralwatt}

var useInfo = map[string][2]string{
	proto.DecideState:   {"A quiet session's state", "When an agent without hooks of its own goes quiet, the judge reads its screen: one that asks you something is marked waiting, with the input sound; one stuck or failed gets the error sound."},
	proto.DecideApprove: {"Approving commands", "Before an agent's command runs: allowed without asking you, left to you, or refused. Runs built in, for every shell command."},
	proto.DecideTrim:    {"Trimming long output", "A test's, build's or install's long output reaches the agent cut to what matters, saving its context; the whole stays in a file. Runs built in, for test, build and install commands."},
	proto.DecideLoop:    {"Stopping loops", "When an agent's commands keep failing alike, the judge reads its last steps; a loop stops its turn and asks you how to go on, before more tokens go."},
	proto.DecideRoute:   {"A model per prompt", "The judge says how hard a prompt is, and enowX suggests the agent's model and effort to match: the cheapest for easy ones. New session offers it; agents ask with decide route. A running session's model never changes on its own."},
	proto.DecideContext: {"RAG: context or not", "Before a prompt gets a project's search results (RAG's Context), the judge keeps only those that bear on it, or none."},
	proto.DecideMemory:  {"RAG: memories to keep", "When a session ends, its last answer is offered as a memory only when the judge finds a decision in it, written as that decision."},
	proto.DecideChat:    {"Team chat urgency", "The judge reads a team message: urgent ones sound and show a banner even when the chat is set to mentions or off; quiet ones only count as unread. A message naming you is never kept quiet."},
	proto.DecideFlaky:   {"Flaky or real test failures", "When a check fails, the judge reads its output: flaky (timing, network, order) or real, shown on the check with why; a flaky one offers Run again."},
}

// useShort says what each use does on its card; useInfo's second part is its
// whole story, the card's tooltip.
var useShort = map[string]string{
	proto.DecideState:   "Reads the screen of an agent without hooks when it goes quiet.",
	proto.DecideApprove: "Before an agent's command runs: allowed, left to you, or refused.",
	proto.DecideTrim:    "A test's or build's long output reaches the agent cut to what matters.",
	proto.DecideLoop:    "When an agent's commands keep failing alike, a loop stops its turn.",
	proto.DecideRoute:   "How hard a prompt is, to suggest a model and effort.",
	proto.DecideContext: "Keeps only the search results that bear on the prompt.",
	proto.DecideMemory:  "Offers a memory only when the last answer holds a decision.",
	proto.DecideChat:    "Urgent messages sound even when the chat is set to mentions.",
	proto.DecideFlaky:   "Reads a failed check: flaky offers Run again.",
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
	widget.PageHeader(c, "Decisions", "A small model, asked instead of guessing", func() {
		if i := widget.Segmented(c, []string{"Judges", "Uses", "Log"}, st.tab, nil); i >= 0 && i != st.tab {
			st.tab = i
			a.loadDecide()
		}
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
			if widget.Button(c, widget.Secondary, icon.Sync, "Refresh", "").Clicked() {
				a.loadDecide()
			}
		}
	})
	switch {
	case st.err != "":
		widget.PageBody(c, true, func() { widget.Notice(c, icon.Alert, theme.Error, st.err, "", nil) })
		return
	case !st.loaded:
		widget.PageBody(c, true, func() { widget.Spinner(c, 14) })
		return
	}
	widget.FigsBand(c, decideFigs(st.log))
	switch st.tab {
	case 0:
		widget.PageBody(c, true, func() { a.judgesTab(c, st) })
	case 1:
		a.usesTab(c, st)
	default:
		widget.PageBody(c, true, func() { a.decideLogTab(c, st) })
	}
	a.judgeFormSheet(c, st)
	a.judgeKeySheet(c, st)
}

// decideFigs are the section's figures, all from the log.
func decideFigs(log []proto.Decision) []widget.Fig {
	acted, fell := 0, 0
	cost := 0.0
	var took []int64
	for _, d := range log {
		if d.Acted != "" {
			acted++
		}
		if len(d.Tried) > 0 {
			fell++
		}
		if d.Judge != "" {
			took = append(took, d.Ms)
		}
		cost += d.Cost
	}
	share := "–"
	if len(log) > 0 {
		share = strconv.Itoa(acted*100/len(log)) + "%"
	}
	return []widget.Fig{
		{Label: "Decisions in the log", Value: strconv.Itoa(len(log))},
		{Label: "Acted on", Value: share},
		{Label: "Typical speed", Value: typicalMs(took)},
		{Label: "Fell through a judge", Value: strconv.Itoa(fell)},
		{Label: "Cost", Value: fmt.Sprintf("$%.4f", cost)},
	}
}

// judgesTab is who may judge, the judges in the order tried, and what they
// decided of late.
func (a *App) judgesTab(c *ui.Context, st *decideState) {
	if st.move.said != "" || slices.ContainsFunc(st.judges, func(j proto.Judge) bool { return j.Own && j.HasKey }) {
		a.keysToVaultRow(c, &st.move, a.loadDecide)
	}
	ui.Row(c.Key("tools")).FillWidth().Shrink(0).Gap(theme.S3).Children(func() {
		before := st.set.Policy
		decidePick(c, "Who may judge", 240, "Local models are always allowed. A judge not allowed for a session is skipped; the next in the order is asked when one fails or takes too long.", func() {
			hookSelect(c, "Policy", &st.set.Policy, policies, policyNames)
		})
		if st.set.Policy == proto.DecideOneProvider {
			if st.set.Provider == "" {
				st.set.Provider = proto.ProviderAnthropic
			}
			was := st.set.Provider
			decidePick(c, "Provider", 160, "", func() {
				hookSelect(c.Key("provider"), "Provider", &st.set.Provider, providers, providerNames)
			})
			if was != st.set.Provider {
				a.saveDecide(st)
			}
		}
		if before != st.set.Policy && (st.set.Policy != proto.DecideOneProvider || st.set.Provider != "") {
			a.saveDecide(st)
		}
	})
	if len(st.judges) == 0 {
		widget.EmptyState(c, icon.Brain, "No judge on "+a.host.Name, "Install Claude Code or Codex (their own sign-in judges), add an Anthropic, OpenAI, Gemini or OpenRouter key to the vault, or run Ollama or LM Studio.", nil)
		return
	}
	widget.Cols(c.Key("judge-cards"), 3).Children(func() {
		for i, j := range st.judges {
			a.judgeRow(c.Key(j.ID), st, i, j)
		}
	})
	a.decideRecent(c, st)
}

// decidePick is a label and its drop-down of the given width, in a row of them.
func decidePick(c *ui.Context, label string, width float32, about string, fn func()) {
	ui.Row(c.Key("pick:" + label)).Shrink(0).Gap(theme.S2).Children(func() {
		t := ui.Text(c, label).FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine()
		if about != "" {
			t.Tooltip(about)
		}
		ui.Row(c).Width(width).Shrink(0).Children(fn)
	})
}

// decideRecent is the log in short: how many of each use, the latest.
func (a *App) decideRecent(c *ui.Context, st *decideState) {
	counts := map[string]int{}
	largest := 0
	for _, d := range st.log {
		counts[d.Use]++
		largest = max(largest, counts[d.Use])
	}
	widget.TwoParts(c, func() {
		widget.Heading(c, "Decisions by use")
		for _, u := range proto.DecideUses {
			widget.TallyRow(c, useInfo[u][0], counts[u], largest)
		}
	}, func() {
		widget.Heading(c, "Latest decisions")
		if len(st.log) == 0 {
			widget.CardNote(c, "They show here as the judge is asked.", 0)
		}
		for i, d := range st.log[:min(12, len(st.log))] {
			col, status := decisionStatus(d)
			why := d.Why
			if d.Judge == "" {
				why = d.Error
			}
			widget.RecordRow(c.Key(i), clockOrDay(d.Time), useInfo[d.Use][0]+" · "+why, col, status)
		}
	})
}

// decisionStatus is what a decision came to: its dot's color and its words.
func decisionStatus(d proto.Decision) (ui.Color, string) {
	if d.Judge == "" {
		return theme.Idle, "no answer"
	}
	return verdictColor(d.Verdict), fmt.Sprintf("%s %.0f%%", d.Verdict, d.Confidence*100)
}

func (a *App) judgeRow(c *ui.Context, st *decideState, i int, j proto.Judge) {
	off := slices.Contains(st.set.Off, j.ID)
	testing := st.testing[j.ID]
	failed := !testing && !j.Tested.IsZero() && !j.OK
	card := widget.GridCard(c)
	if failed {
		card.Background(theme.Fade(theme.Error, 0.13))
	}
	card.Children(func() {
		ui.Row(c.Key("title")).FillWidth().Shrink(0).MinHeight(24).Gap(theme.S2).Children(func() {
			ui.Row(c).Size(24, 24).Shrink(0).Radius(theme.RadiusFull).Background(theme.Sunken).Center().Children(func() {
				ui.Text(c, strconv.Itoa(i+1)).FontSize(theme.TextLabel).SingleLine()
			})
			ui.Row(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
				widget.Running(c, j.Label, widget.RunStyle{Size: theme.TextRead, Weight: 600})
			})
			on := !off
			if widget.Switch(c.Key("on"), &on, "") {
				st.set.Off = slices.DeleteFunc(slices.Clone(st.set.Off), func(id string) bool { return id == j.ID })
				if !on {
					st.set.Off = append(st.set.Off, j.ID)
				}
				a.saveDecide(st)
			}
		})
		ui.Row(c.Key("state")).FillWidth().Shrink(0).MinHeight(16).Gap(theme.S1).Children(func() {
			switch {
			case testing:
				widget.Spinner(c, 10)
				widget.Running(c, "Testing", widget.RunStyle{Size: theme.TextLabel})
			case j.Tested.IsZero():
				widget.StatusLine(c, theme.Idle, true, "Not tested")
			case j.OK:
				widget.StatusLine(c, theme.Done, false, judgeWorks(j))
			default:
				widget.StatusLine(c, theme.Error, false, "Failed")
			}
		})
		if j.HasKey || !underPolicy(st.set, j) {
			ui.Row(c.Key("tags")).FillWidth().Shrink(0).Gap(theme.S1).Wrap().Children(func() {
				if j.HasKey {
					widget.Tag(c, "key set here", theme.TextSoft)
				}
				if !underPolicy(st.set, j) {
					widget.Tag(c, "not under this policy", theme.Warning)
				}
			})
		}
		how := a.judgeHow(j)
		if failed && j.Error != "" {
			how = j.Error
		}
		widget.CardNote(c, how, 2).MinHeight(32).Shrink(0)
		answered, took := 0, []int64(nil)
		for _, d := range st.log {
			if d.Judge == j.ID {
				answered++
				took = append(took, d.Ms)
			}
		}
		last := "–"
		if !j.Tested.IsZero() && j.OK {
			last = msWords(j.Ms)
		}
		widget.CardStats(c, []widget.Stat{{Value: strconv.Itoa(answered), Label: "answered"}, {Value: typicalMs(took), Label: "typical"}, {Value: last, Label: "last test"}})
		widget.CardFoot(c, func() {
			if widget.GhostButton(c.Key("test"), "Test").Disabled(testing).Clicked() {
				a.testJudges(st, j.ID)
			}
			ui.Spacer(c)
			if needsKey(j) && widget.IconButton(c.Key("key"), icon.Key, "Its key", j.HasKey).Clicked() {
				jj := j
				st.keying, st.keyInput = &jj, ""
			}
			if j.Own && widget.IconButton(c.Key("rm"), icon.Trash, "Remove", false).Clicked() {
				a.confirm("Remove the judge "+j.Label+"?", "It is no longer asked; a use set to it alone goes back to the order.", "Remove", func() {
					st.set.Own = slices.DeleteFunc(slices.Clone(st.set.Own), func(x proto.Judge) bool { return x.ID == j.ID })
					a.saveDecide(st)
				})
			}
			if widget.IconButton(c.Key("up"), icon.ChevronUp, "Ask it sooner", false).Disabled(i == 0).Clicked() && i > 0 {
				a.moveJudge(st, i, i-1)
			}
			if widget.IconButton(c.Key("down"), icon.ChevronDown, "Ask it later", false).Disabled(i == len(st.judges)-1).Clicked() && i < len(st.judges)-1 {
				a.moveJudge(st, i, i+1)
			}
		})
	})
}

// judgeWorks says a judge's last test answered, and what a call cost when its
// service said.
func judgeWorks(j proto.Judge) string {
	if j.Cost > 0 {
		return fmt.Sprintf("Works · $%.4f a call", j.Cost)
	}
	return "Works"
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

// judgeHow is how judge j is reached, in words.
func (a *App) judgeHow(j proto.Judge) string {
	switch j.Kind {
	case proto.JudgeDecision:
		where := j.URL
		if where == "" {
			where = providerNames[j.Provider] + "'s System One"
		}
		if j.Cred != "" {
			return "Decision model, key " + j.Cred + " from the vault · " + where
		}
		return "Decision model · " + where
	case proto.JudgeAgent:
		return "Through " + a.agentName(j.Agent) + "'s own sign-in, its tools and project left out"
	case proto.JudgeLocal:
		return "On this host: " + j.URL
	}
	where := providerNames[j.Provider] + "'s API"
	if j.URL != "" {
		where = j.URL
	}
	if j.Cred == "" {
		return "Model API · " + where
	}
	return "Model API, key " + j.Cred + " from the vault · " + where
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
	ui.Row(c.Key("tools")).Width(260).Shrink(0).Children(func() { hookSelect(c.Key("log-use"), "Use", &st.logUse, opts, names) })
	shown := slices.DeleteFunc(slices.Clone(st.log), func(d proto.Decision) bool { return st.logUse != "" && d.Use != st.logUse })
	if len(shown) == 0 {
		widget.EmptyState(c, icon.List, "No decisions yet", "They show here as the judge is asked, with why and what enowX did.", nil)
		return
	}
	widget.LogGrid(c, len(shown), func() {
		for _, d := range shown {
			a.decisionRow(c.Key(d.ID), st, d)
		}
	})
}

func (a *App) decisionRow(c *ui.Context, st *decideState, d proto.Decision) {
	col, status := decisionStatus(d)
	widget.LogCard(c, useInfo[d.Use][0], false, col, status, a.decisionMeta(st, d), func() {
		if d.Acted != "" {
			widget.Tag(c, d.Acted, theme.TextSoft)
		}
		if d.Wrong {
			widget.Tag(c, "marked wrong", theme.Warning)
		}
	}, func() {
		ui.Row(c.Key("why")).FillWidth().Grow(1).MinHeight(0).Gap(theme.S2).Children(func() {
			why := d.Why
			if d.Judge == "" {
				why = d.Error
			}
			ui.Text(c, why).FontSize(theme.TextLabel).Grow(1).Basis(0).MinWidth(0).MaxLines(2)
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
	})
}

// decisionMeta says when, where, who and how long a decision took.
func (a *App) decisionMeta(st *decideState, d proto.Decision) string {
	parts := []string{clockOrDay(d.Time)}
	if d.Session != "" {
		parts = append(parts, a.sessionLabel(d.Session))
	}
	judge := "No judge"
	if d.Judge != "" {
		judge = d.Judge
		if i := slices.IndexFunc(st.judges, func(j proto.Judge) bool { return j.ID == d.Judge }); i >= 0 {
			judge = st.judges[i].Label
		}
	}
	parts = append(parts, judge)
	if d.Judge != "" {
		parts = append(parts, msWords(d.Ms))
	}
	if d.Saved > 0 {
		parts = append(parts, fmt.Sprintf("%d characters kept from the agent", d.Saved))
	}
	if len(d.Tried) > 0 {
		parts = append(parts, fmt.Sprintf("%d failed first", len(d.Tried)))
	}
	return strings.Join(parts, " · ")
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

// needsKey says whether judge j asks a service for a key: not an agent's
// own sign-in, not a server on the host.
func needsKey(j proto.Judge) bool {
	return j.Kind != proto.JudgeAgent && j.Kind != proto.JudgeLocal && j.Provider != proto.ProviderLocal
}

// judgeKeySheet takes a judge's key, typed here: kept on the host alone,
// used before its vault credential.
func (a *App) judgeKeySheet(c *ui.Context, st *decideState) {
	j := st.keying
	if j == nil {
		return
	}
	open := true
	about := "Kept on " + a.host.Name + " alone, readable by its user only, never synced or shown again; used before any vault credential."
	widget.FormSheet(c, &open, 480, "Key for "+j.Label, about, func() {
		widget.Input(c.Key("judge-key-input"), &st.keyInput, "sk-…").FillWidth().Password().AutoFocus()
		if j.HasKey {
			widget.CardNote(c, "A key is set here; a new one replaces it.", 0)
		}
	}, func() {
		if j.HasKey && widget.DangerButton(c, "Remove the key").Clicked() {
			id := j.ID
			a.confirm("Remove "+j.Label+"'s key?", "The judge falls back to its vault credential, if it has one, else it can't be asked.", "Remove", func() {
				a.saveJudgeKey(st, id, "")
			})
			st.keying = nil
		}
		if widget.PrimaryButton(c, icon.Check, "Save and test", "").Disabled(strings.TrimSpace(st.keyInput) == "").Clicked() {
			a.saveJudgeKey(st, j.ID, st.keyInput)
			st.keying = nil
		}
	})
	if !open {
		st.keying = nil
	}
}

// saveJudgeKey keeps judge id's key on the host ("" removes it), then
// tests the judge with it.
func (a *App) saveJudgeKey(st *decideState, id, key string) {
	cl := a.client
	go func() {
		err := cl.SetJudgeKey(id, strings.TrimSpace(key))
		a.onMain(func() {
			if err != nil {
				a.status = "Key: " + client.Explain(err)
				return
			}
			for i := range st.judges {
				if st.judges[i].ID == id {
					st.judges[i].HasKey = key != ""
				}
			}
			if key != "" {
				a.testJudges(st, id)
			}
		})
	}()
}

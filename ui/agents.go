package ui

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/accounts"
	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/chatview"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// Settings → Agents: a page per agent, picked in a list beside, with its
// configs (the official one, its accounts and model APIs) in the order
// they're used, what a limit does, and, for an agent that takes a model
// API only in its own settings, that API. The history of which config
// each session ran on is a tab of its own.

func (a *App) agentsSection(c *ui.Context) {
	st, apis := a.accountsOf(), a.agentAPIsOf()
	list := a.configAgents(st)
	if len(list) > 0 && !slices.ContainsFunc(list, func(ag proto.AgentInfo) bool { return ag.ID == st.agent }) {
		st.agent = list[0].ID
	}
	widget.PageHeader(c, "Agents", "Configs on "+a.host.Name, func() {
		if i := widget.Segmented(c, []string{"Configs", "History"}, st.tab, nil); i >= 0 && i != st.tab {
			st.tab = i
			a.loadAccounts()
		}
		if widget.Button(c.Key("always-allowed"), widget.Secondary, nil, "Always allowed", "").Clicked() {
			a.openAlwaysAllowed()
		}
		can := a.canAddConfig(st, list)
		if widget.PrimaryButton(c, icon.Plus, "New config", "").Disabled(!can).Clicked() && can {
			st.edit = a.newConfigForm(st.agent, proto.Account{})
		}
	})
	figs, tones := a.agentFigs(st, list)
	widget.FigsBandToned(c, figs, tones)
	if msg := cmp.Or(st.err, apis.err); msg != "" {
		ui.Column(c.Key("agents-error")).FillWidth().Shrink(0).Padding(theme.S3, theme.S5, 0, theme.S5).Children(func() {
			widget.Notice(c, icon.Alert, theme.Error, msg, "", nil)
		})
	}
	switch {
	case !st.loaded && st.err == "":
		widget.PageFill(c, func() { widget.Spinner(c, 14) })
	case st.tab == 1:
		a.accountHistory(c, st)
	case len(list) == 0:
		widget.PageFill(c, func() {
			widget.CardNote(c, "No agent here can have a config: install one to give it some.", 0)
		})
	default:
		widget.PageFill(c, func() {
			widget.MasterDetail(c, 240, func() { a.agentList(c, st, list) }, func() {
				for _, ag := range list {
					if ag.ID == st.agent {
						a.agentPage(c.Key("page:"+ag.ID), st, apis, ag)
					}
				}
			})
		})
	}
	a.configSheet(c, st)
	a.agentAPISheet(c, apis)
	a.alwaysAllowedView(c)
}

// canAddConfig says whether the agent picked can get a new config here.
func (a *App) canAddConfig(st *accountsState, list []proto.AgentInfo) bool {
	i := slices.IndexFunc(list, func(ag proto.AgentInfo) bool { return ag.ID == st.agent })
	if i < 0 || len(a.configKinds(st.agent)) == 0 {
		return false
	}
	return noConfigWhy(list[i], accounts.WayOf(st.agent)) == "" || len(st.accountsOfAgent(st.agent)) > 0
}

// agentFigs are the section's figures, each counted from what the host told:
// the official configs count, one an agent, as the page lists them. Tones are
// those that stand out.
func (a *App) agentFigs(st *accountsState, list []proto.AgentInfo) ([]widget.Fig, map[int]ui.Color) {
	agents, configs := 0, 0
	for _, ag := range list {
		if way := accounts.WayOf(ag.ID); agentRank(st, ag) == 1 && way.Base == "" {
			agents++
			configs += 1 + len(st.accountsOfAgent(ag.ID))
		}
	}
	limited := 0
	for _, s := range st.info.Accounts {
		if s.State == proto.AccountLimit || s.State == proto.AccountCredits {
			limited++
		}
	}
	y, m, d := time.Now().Date()
	fellBack := 0
	for _, e := range st.log {
		if ey, em, ed := e.Time.Local().Date(); e.Event == "fallback" && ey == y && em == m && ed == d {
			fellBack++
		}
	}
	other := 0
	for _, s := range a.sessions {
		if s.Account != "" {
			other++
		}
	}
	figs := []widget.Fig{
		{Label: "Configs", Value: strconv.Itoa(configs)},
		{Label: "Agents", Value: strconv.Itoa(agents)},
		{Label: "At their limit", Value: strconv.Itoa(limited)},
		{Label: "Fell back today", Value: strconv.Itoa(fellBack)},
		{Label: "Sessions on another config", Value: strconv.Itoa(other)},
	}
	var tones map[int]ui.Color
	if limited > 0 {
		tones = map[int]ui.Color{2: theme.Warning}
	}
	return figs, tones
}

// agentList is the list of agents beside the page, narrowed by what's typed.
func (a *App) agentList(c *ui.Context, st *accountsState, list []proto.AgentInfo) {
	widget.Input(c.Key("find"), &st.find, "Find an agent").Label("Find an agent").FillWidth().Shrink(0)
	find, group := strings.ToLower(strings.TrimSpace(st.find)), 1
	for _, ag := range list {
		if find != "" && !strings.Contains(strings.ToLower(ag.Name+" "+ag.ID), find) {
			continue
		}
		rank := agentRank(st, ag)
		if rank != group {
			group = rank
			widget.Heading(c, "Not installed")
		}
		sub := ""
		if rank == 1 {
			n := 1 + len(st.accountsOfAgent(ag.ID))
			sub = strconv.Itoa(n) + " " + textfmt.Plural(n, "config", "configs")
		}
		if widget.Pick(c.Key("ag:"+ag.ID), icon.Agent(ag.ID), ag.Name, sub, st.agent == ag.ID, nil).Clicked() {
			st.agent = ag.ID
		}
	}
}

// agentPage is an agent's configs and, where it takes a model API only in
// its own settings, that API. Its cards share the height the rest leaves, down
// to what they need; then the page scrolls.
func (a *App) agentPage(c *ui.Context, st *accountsState, apis *agentAPIState, ag proto.AgentInfo) {
	scroll := widget.Scroll(c.Key("agent-page")).Grow(1).FillWidth().MinHeight(0)
	frame := scroll.Bounds()
	scroll.Children(func() {
		ui.Column(c).FillWidth().Gap(theme.S3).Children(func() {
			way := accounts.WayOf(ag.ID)
			if chatview.OwnChat(ag.ID) {
				a.platformAgentRow(c)
			}
			above := ui.Column(c.Key("above")).FillWidth().Shrink(0).Gap(theme.S1)
			st.aboveH = above.Bounds().H
			above.Children(func() {
				widget.Heading(c, "Configs, in the order sessions use them at a limit")
				a.agentWay(c, ag, way)
			})
			if noConfigWhy(ag, way) != "" && len(st.accountsOfAgent(ag.ID)) == 0 {
				return
			}
			pr, order := configOrder(st, ag.ID)
			a.agentAccounts(c, st, ag, pr, order, frame.H)
			below := ui.Column(c.Key("below")).FillWidth().Shrink(0).Gap(theme.S3)
			st.belowH = below.Bounds().H
			below.Children(func() {
				a.agentLower(c, st, ag, pr, order, frame.W)
				a.agentOwnAPI(c, apis, ag)
			})
		})
	})
}

// agentOwnAPI is the API in an agent's own settings, for those that have one
// there or can.
func (a *App) agentOwnAPI(c *ui.Context, apis *agentAPIState, ag proto.AgentInfo) {
	info, ok := apiInfoOf(apis, ag.ID)
	if !ok || info.PerSession && info.Current == nil && len(info.Backups) == 0 {
		return
	}
	title, about := "A model API in its own settings", ag.Name+" takes a model API only in its own settings: enowX writes it there, after keeping them as they were, and every session of it uses it. Restore puts them back."
	if info.PerSession {
		title, about = "Its own settings", "enowX wrote a model API into "+ag.Name+"'s settings before configs: it becomes a config once its key is in the vault (the app asks), and the settings go back to how you had them. Their backups stay here."
	}
	ui.Column(c.Key("own-api")).FillWidth().Shrink(0).Gap(theme.S1).Children(func() {
		widget.Heading(c, title)
		widget.CardNote(c, about, 0)
		ui.Box(c).Height(theme.S1)
		a.agentAPIRow(c.Key("api"), apis, info)
	})
}

// configAgents are every agent, installed ones first: each says how it takes a
// config, or why it can't (accounts.WayOf).
func (a *App) configAgents(st *accountsState) []proto.AgentInfo {
	var out []proto.AgentInfo
	for _, ag := range a.agents {
		if _, ok := agent.Find(ag.ID); !ok || ag.ID == agent.Shell {
			continue
		}
		out = append(out, ag)
	}
	slices.SortStableFunc(out, func(x, y proto.AgentInfo) int {
		return cmp.Compare(agentRank(st, y), agentRank(st, x))
	})
	return out
}

// agentRank puts first what can have a config now: installed, or with some.
func agentRank(st *accountsState, ag proto.AgentInfo) int {
	if ag.Installed || len(st.accountsOfAgent(ag.ID)) > 0 {
		return 1
	}
	return 0
}

func apiInfoOf(apis *agentAPIState, id string) (proto.AgentAPIInfo, bool) {
	i := slices.IndexFunc(apis.list, func(x proto.AgentAPIInfo) bool { return x.Agent == id })
	if i < 0 {
		return proto.AgentAPIInfo{}, false
	}
	return apis.list[i], true
}

// agentWay says what an agent takes, what it doesn't and how that was checked.
func (a *App) agentWay(c *ui.Context, ag proto.AgentInfo, way accounts.Way) {
	line := ag.Name + " takes " + way.Says() + "."
	if way.Why != "" {
		line += " " + way.Why
	}
	if checked := wayChecked[way.Checked]; checked != "" {
		line += " " + checked
	}
	widget.CardNote(c, line, 0)
	if why := noConfigWhy(ag, way); why != "" {
		ui.Text(c, why).FontSize(theme.TextLabel).TextColor(theme.Warning)
	} else if way.Account && ag.SignIn == "" {
		ui.Text(c, "It has no sign-in command: Sign in starts it in the account's own home, and it asks there.").FontSize(theme.TextLabel).TextColor(theme.Warning)
	}
}

// wayChecked says how a Way was checked.
var wayChecked = map[string]string{
	"run":  "Run here.",
	"help": "From its --help and docs; not run with a config.",
	"docs": "Read from its docs; not run.",
}

// noConfigWhy is why ag has no New config on this host, "" when it has.
func noConfigWhy(ag proto.AgentInfo, way accounts.Way) string {
	switch {
	case way.Base != "":
		return "Pick the config in " + agentNameOf(way.Base) + "'s page."
	case way.None():
		return "It can't have a config."
	case !ag.Installed:
		return "It isn't installed on this host: install it to give it a config."
	}
	return ""
}

func agentNameOf(id string) string {
	if p, ok := agent.Find(id); ok {
		return p.Name
	}
	return id
}

// configKinds are the kinds of config agent id can have, as the form
// offers them.
func (a *App) configKinds(id string) []string {
	var out []string
	if accounts.Supported(agent.Base(id)) {
		out = append(out, kindAccount)
	}
	if info, ok := apiInfoOf(a.agentAPIsOf(), id); ok && info.PerSession {
		out = append(out, kindAPI)
	}
	if accounts.WayOf(id).Key {
		out = append(out, kindKey)
	}
	return out
}

// The kinds of config, as the form names them.
const (
	kindAPI     = "Model API"
	kindAccount = "Account"
	kindKey     = "API key"
)

// configDetail is what config c is, in a line.
func configDetail(c proto.Account) string {
	switch {
	case c.Kind == proto.ConfigKindAPI && c.API != nil:
		d := c.API.BaseURL
		if c.API.Model != "" {
			d += " · " + c.API.Model
		}
		if n := len(c.API.Models); n > 1 {
			d += fmt.Sprintf(" +%d", n-1)
		}
		if c.API.KeyCred != "" {
			d += " · key " + c.API.KeyCred
		}
		return d
	case c.Cred != "":
		return "The vault's key " + c.Cred + ", lent to its sessions"
	}
	return "Its own sign-in, on each host it is signed in on"
}

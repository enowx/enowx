package ui

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/accounts"
	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// Settings → Agents: a page per agent, picked in a list beside, with its
// configs (the official one, its accounts and model APIs) in the order
// they're used, what a limit does, and, for an agent that takes a model
// API only in its own settings, that API. The history of which config
// each session ran on is a tab of its own.

func (a *App) agentsSection(c *ui.Context) {
	settingsColumn(c, "Agents", "", nil, func() { a.agentsBody(c) })
}

func (a *App) agentsBody(c *ui.Context) {
	st, apis := a.accountsOf(), a.agentAPIsOf()
	sectionIntro(c, "Agents", "Each agent's configs on "+a.host.Name+". The official one is the agent as installed, never changed by enowX. The others are another account (signed in once on each host) or a model API (a gateway, OpenRouter, a local server, its key from the vault). A session picks one at New session and keeps it; at a usage limit it can go on as the next in the order.")
	ui.Row(c).FillWidth().Children(func() {
		if i := widget.Segmented(c, []string{"Configs", "History"}, st.tab, nil); i >= 0 && i != st.tab {
			st.tab = i
			a.loadAccounts()
		}
	})
	switch {
	case st.err != "":
		widget.Notice(c, icon.Alert, theme.Error, st.err, "", nil)
	case apis.err != "":
		widget.Notice(c, icon.Alert, theme.Error, apis.err, "", nil)
	case !st.loaded:
		widget.Spinner(c, 14)
	}
	if st.tab == 1 {
		a.accountHistory(c, st)
		return
	}
	list := a.configAgents(st)
	if len(list) > 0 && !slices.ContainsFunc(list, func(ag proto.AgentInfo) bool { return ag.ID == st.agent }) {
		st.agent = list[0].ID
	}
	ui.Row(c).FillWidth().Gap(14).AlignItems(ui.Start).Children(func() {
		ui.Column(c).Width(184).Shrink(0).Gap(2).Children(func() {
			widget.Input(c.Key("find"), &st.find, "Find an agent").Label("Find an agent").FillWidth()
			find, group := strings.ToLower(strings.TrimSpace(st.find)), 1
			for _, ag := range list {
				if find != "" && !strings.Contains(strings.ToLower(ag.Name+" "+ag.ID), find) {
					continue
				}
				if rank := agentRank(st, ag); rank != group {
					group = rank
					ui.Text(c, "Not installed").FontSize(theme.TextLabel).FontWeight(600).TextColor(theme.Muted).Padding(8, 8, 2, 8)
				}
				a.agentListItem(c.Key("ag:"+ag.ID), st, ag)
			}
		})
		ui.Column(c).Grow(1).MinWidth(0).Gap(12).Children(func() {
			for _, ag := range list {
				if ag.ID == st.agent {
					a.agentPage(c.Key("page:"+ag.ID), st, apis, ag)
				}
			}
		})
	})
	a.configSheet(c, st)
	a.agentAPISheet(c, apis)
}

// agentListItem is an agent in the list beside its page.
func (a *App) agentListItem(c *ui.Context, st *accountsState, ag proto.AgentInfo) {
	on := st.agent == ag.ID
	b := ui.ButtonBase(c).FillWidth().Height(36).PaddingX(8).Gap(8).Radius(theme.RadiusControl).AlignItems(ui.Center).Cursor(ui.CursorPointer).Label(ag.Name)
	switch {
	case on:
		b.Background(theme.Selected)
	case b.Hovered():
		b.Background(theme.Hover)
	}
	b.Children(func() {
		agentTile(c, ag.ID, 20)
		ui.Text(c, ag.Name).FontSize(theme.TextUI).SingleLine().Grow(1).Shrink(1)
		if n := len(st.accountsOfAgent(ag.ID)); n > 0 {
			ui.Text(c, strconv.Itoa(n)).FontSize(theme.TextLabel).TextColor(theme.Muted)
		}
	})
	if widget.Pressable(b).Clicked() {
		st.agent = ag.ID
	}
}

// agentPage is an agent's configs and, where it takes a model API only in
// its own settings, that API.
func (a *App) agentPage(c *ui.Context, st *accountsState, apis *agentAPIState, ag proto.AgentInfo) {
	way := accounts.WayOf(ag.ID)
	a.agentWay(c, ag, way)
	if why := noConfigWhy(ag, way); why != "" && len(st.accountsOfAgent(ag.ID)) == 0 {
		return
	}
	a.agentAccounts(c, st, ag)
	info, ok := apiInfoOf(apis, ag.ID)
	if !ok || info.PerSession && info.Current == nil && len(info.Backups) == 0 {
		return
	}
	ui.Column(c).FillWidth().Gap(6).Children(func() {
		title, about := "A model API in its own settings", ag.Name+" takes a model API only in its own settings: enowX writes it there, after keeping them as they were, and every session of it uses it. Restore puts them back."
		if info.PerSession {
			title, about = "Its own settings", "enowX wrote a model API into "+ag.Name+"'s settings before configs: it becomes a config once its key is in the vault (the app asks), and the settings go back to how you had them. Their backups stay here."
		}
		ui.Text(c, title).FontSize(theme.TextUI).TextColor(theme.TextSoft)
		ui.Text(c, about).FontSize(theme.TextLabel).TextColor(theme.Muted)
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
	ui.Column(c).FillWidth().Gap(4).Children(func() {
		ui.Text(c, ag.Name+" takes "+way.Says()).FontSize(theme.TextUI).TextColor(theme.TextSoft)
		if way.Why != "" {
			ui.Text(c, way.Why).FontSize(theme.TextLabel).TextColor(theme.Muted)
		}
		if why := noConfigWhy(ag, way); why != "" {
			ui.Text(c, why).FontSize(theme.TextLabel).TextColor(theme.Warning)
		} else if way.Account && ag.SignIn == "" {
			ui.Text(c, "It has no sign-in command: Sign in starts it in the account's own home, and it asks there.").FontSize(theme.TextLabel).TextColor(theme.Warning)
		}
		ui.Text(c, wayChecked[way.Checked]).FontSize(theme.TextLabel).TextColor(theme.Muted)
	})
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
	if info, ok := apiInfoOf(a.agentAPIsOf(), id); ok && info.PerSession {
		out = append(out, kindAPI)
	}
	if accounts.Supported(agent.Base(id)) {
		out = append(out, kindAccount)
	}
	if accounts.WayOf(id).Key {
		out = append(out, kindKey)
	}
	return out
}

// The kinds of config, as the form names them.
const (
	kindAPI     = "Model API"
	kindAccount = "Another account"
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

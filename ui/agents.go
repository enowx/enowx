package ui

import (
	"fmt"
	"slices"
	"strconv"

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
	list := a.configAgents(st, apis)
	if st.loaded && len(list) == 0 {
		ui.Text(c, "No agent that can have configs is installed on "+a.host.Name+".").FontSize(12).TextColor(theme.Muted)
	}
	if len(list) > 0 && !slices.ContainsFunc(list, func(ag proto.AgentInfo) bool { return ag.ID == st.agent }) {
		st.agent = list[0].ID
	}
	ui.Row(c).FillWidth().Gap(14).AlignItems(ui.Start).Children(func() {
		ui.Column(c).Width(184).Shrink(0).Gap(2).Children(func() {
			for _, ag := range list {
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
		ui.Text(c, ag.Name).FontSize(12.5).SingleLine().Grow(1).Shrink(1)
		if n := len(st.accountsOfAgent(ag.ID)); n > 0 {
			ui.Text(c, strconv.Itoa(n)).FontSize(11).TextColor(theme.Muted)
		}
	})
	if widget.Pressable(b).Clicked() {
		st.agent = ag.ID
	}
}

// agentPage is an agent's configs and, where it takes a model API only in
// its own settings, that API.
func (a *App) agentPage(c *ui.Context, st *accountsState, apis *agentAPIState, ag proto.AgentInfo) {
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
		ui.Text(c, title).FontSize(12.5).TextColor(theme.TextSoft)
		ui.Text(c, about).FontSize(11.5).TextColor(theme.Muted)
		a.agentAPIRow(c.Key("api"), apis, info)
	})
}

// configAgents are the agents a page shows: those installed that can
// have a config (another account, an API key, a model API), and any with
// configs.
func (a *App) configAgents(st *accountsState, apis *agentAPIState) []proto.AgentInfo {
	var out []proto.AgentInfo
	for _, ag := range a.agents {
		if agent.Base(ag.ID) != ag.ID || ag.ID == agent.Shell || ag.Command == "" && !accounts.Supported(ag.ID) {
			continue
		}
		_, api := apiInfoOf(apis, ag.ID)
		if ag.Installed && (accounts.Supported(ag.ID) || api) || len(st.accountsOfAgent(ag.ID)) > 0 {
			out = append(out, ag)
		}
	}
	return out
}

func apiInfoOf(apis *agentAPIState, id string) (proto.AgentAPIInfo, bool) {
	i := slices.IndexFunc(apis.list, func(x proto.AgentAPIInfo) bool { return x.Agent == id })
	if i < 0 {
		return proto.AgentAPIInfo{}, false
	}
	return apis.list[i], true
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
	return append(out, kindKey)
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

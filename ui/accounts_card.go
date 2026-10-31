package ui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/accounts"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// An agent's configs in Settings → Agents: the official one and its own
// in the order they're used, each its kind, badge, state and what it has
// left, moved up or down, made the default, kept to some projects,
// changed, duplicated or removed; what a limit does; a new config.

// accountStatus is account name of agentID's state, as the host told.
func (st *accountsState) accountStatus(agentID, name string) (proto.AccountStatus, bool) {
	i := slices.IndexFunc(st.info.Accounts, func(s proto.AccountStatus) bool { return s.Agent == agentID && s.Name == name })
	if i < 0 {
		return proto.AccountStatus{}, false
	}
	return st.info.Accounts[i], true
}

func (a *App) agentAccounts(c *ui.Context, st *accountsState, ag proto.AgentInfo) {
	pr := st.info.Prefs[ag.ID]
	var names []string
	for _, acc := range st.accountsOfAgent(ag.ID) {
		names = append(names, acc.Name)
	}
	order := accounts.Ordered(pr, names)
	widget.Card(c).FillWidth().Children(func() {
		ui.Column(c).FillWidth().Gap(8).Children(func() {
			ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
				agentTile(c, ag.ID, 24)
				ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
					ui.Text(c, ag.Name).FontSize(13)
					ui.Text(c, "Configs, in the order sessions use them at a limit").FontSize(11).TextColor(theme.Muted).SingleLine()
				})
				if widget.Button(c, widget.Secondary, icon.Plus, "New config", "").Disabled(len(a.configKinds(ag.ID)) == 0).Clicked() {
					st.edit = a.newConfigForm(ag.ID, proto.Account{})
				}
			})
			for i, name := range order {
				a.accountLine(c.Key("acc:"+name), st, ag, pr, order, i, name)
			}
			if len(order) > 1 {
				a.fallbackControls(c, st, ag, pr)
			}
		})
	})
}

// accountLine is one account in the order: its place, name, state and
// what it has left, and what to do with it.
func (a *App) accountLine(c *ui.Context, st *accountsState, ag proto.AgentInfo, pr proto.AccountPrefs, order []string, i int, name string) {
	acc, isOther := proto.Account{}, name != ""
	if isOther {
		for _, x := range st.accountsOfAgent(ag.ID) {
			if x.Name == name {
				acc = x
			}
		}
	}
	status, _ := st.accountStatus(ag.ID, name)
	ui.Column(c).FillWidth().Gap(4).Padding(6, 8).Radius(theme.RadiusControl).Background(theme.Sunken).Children(func() {
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, fmt.Sprint(i+1)).Font(theme.MonoFont).FontSize(11).TextColor(theme.Muted).Width(14)
			label, detail := name, configDetail(acc)
			if !isOther {
				label, detail = "Official", ag.Name+" as installed: its own sign-in and settings, never changed by enowX"
			}
			ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					if acc.Color != "" {
						widget.BadgeTag(c, label, acc.Color)
					} else {
						ui.Text(c, label).FontSize(12.5)
					}
					if isOther {
						widget.Tag(c, configKindLabel(acc), ui.Color{})
					}
					if pr.Default == name && (name != "" || pr.Default == "") && len(order) > 1 {
						widget.Tag(c, "new sessions", theme.Accent)
					}
					accountStateTag(c, status)
				})
				ui.Text(c, detail).FontSize(11).TextColor(theme.Muted).SingleLine()
				if cr := status.Credits; cr != nil {
					ui.Text(c, creditLine(*cr)).FontSize(11).TextColor(theme.TextSoft).SingleLine()
				}
				if only := pr.Only[name]; len(only) > 0 {
					ui.Text(c, "Only for "+strings.Join(only, ", ")).FontSize(11).TextColor(theme.TextSoft).SingleLine()
				}
			})
			if widget.IconButton(c.Key("up"), icon.ChevronUp, "Use it sooner", false).Disabled(i == 0).Clicked() && i > 0 {
				next := slices.Clone(order)
				next[i-1], next[i] = next[i], next[i-1]
				pr.Order = next
				a.saveAccountPrefs(st, ag.ID, pr)
			}
			if widget.IconButton(c.Key("down"), icon.ChevronDown, "Use it later", false).Disabled(i == len(order)-1).Clicked() && i < len(order)-1 {
				next := slices.Clone(order)
				next[i+1], next[i] = next[i], next[i+1]
				pr.Order = next
				a.saveAccountPrefs(st, ag.ID, pr)
			}
			widget.MoreMenu(c, &a.menu, "account:"+ag.ID+":"+name, func(done func()) {
				if isOther && widget.MenuItem(c, icon.Pencil, "Edit…", false) {
					st.edit = a.newConfigForm(ag.ID, acc)
					done()
				}
				if isOther && widget.MenuItem(c, icon.Copy, "Duplicate…", false) {
					dup := acc
					dup.ID, dup.Name = "", acc.Name+" copy"
					st.edit = a.newConfigForm(ag.ID, dup)
					done()
				}
				if widget.MenuItem(c, icon.Check, "Use for new sessions", false) {
					pr.Default = name
					a.saveAccountPrefs(st, ag.ID, pr)
					done()
				}
				if isOther && widget.MenuItem(c, icon.Folder, "Only for some projects…", false) {
					v := strings.Join(pr.Only[name], ", ")
					st.only[ag.ID+"\x00"+name] = &v
					done()
				}
				if isOther && acc.Cred == "" && acc.Kind != proto.ConfigKindAPI && widget.MenuItem(c, icon.Account, "Sign in on "+a.host.Name, false) {
					a.signInAccount(ag.ID, acc)
					done()
				}
				if isOther && widget.MenuItem(c, icon.Trash, "Remove", true) {
					what := "It goes from every host of yours; sessions using it keep running until they end. The official config stays as it is."
					if acc.Kind == proto.ConfigKindAccount && acc.Cred == "" {
						what = "Its sign-in on " + a.host.Name + " goes with it, and the config from every host of yours; sessions using it keep running until they end."
					}
					a.confirm("Remove the config "+name+"?", what, "Remove", func() { a.removeAccount(st, acc) })
					done()
				}
			})
		})
		if v := st.only[ag.ID+"\x00"+name]; v != nil {
			ui.Row(c).FillWidth().Gap(6).Children(func() {
				in := widget.Input(c, v, "acme, ~/code/client-site").Grow(1).Font(theme.MonoFont).AutoFocus()
				if widget.GhostButton(c, "Cancel").Clicked() {
					delete(st.only, ag.ID+"\x00"+name)
				}
				if widget.Button(c, widget.Secondary, nil, "Keep", "").Clicked() || in.Submitted() {
					if pr.Only == nil {
						pr.Only = map[string][]string{}
					}
					pr.Only[name] = splitList(*v)
					if len(pr.Only[name]) == 0 {
						delete(pr.Only, name)
					}
					delete(st.only, ag.ID+"\x00"+name)
					a.saveAccountPrefs(st, ag.ID, pr)
				}
			})
		}
	})
}

// accountStateTag says an account's limit, if it hit one.
func accountStateTag(c *ui.Context, s proto.AccountStatus) {
	switch s.State {
	case proto.AccountLimit:
		label := "at its limit"
		if !s.Until.IsZero() {
			label += " until " + s.Until.Local().Format("15:04")
			if time.Until(s.Until) > 20*time.Hour {
				label = "at its limit until " + s.Until.Local().Format("Jan 2 15:04")
			}
		}
		widget.Tag(c, label, theme.Warning)
	case proto.AccountCredits:
		widget.Tag(c, "out of credits", theme.Error)
	case proto.AccountAuth:
		widget.Tag(c, "signed out", theme.Error)
	}
}

// creditLine is what an account has left, in a line.
func creditLine(cr proto.CreditAccount) string {
	if cr.Error != "" {
		return cr.Error
	}
	var parts []string
	if cr.Plan != "" {
		parts = append(parts, cr.Plan)
	}
	for _, w := range cr.Windows {
		p := fmt.Sprintf("%s %.0f%% used", w.Label, w.Used)
		if !w.ResetsAt.IsZero() {
			p += ", resets " + w.ResetsAt.Local().Format("Mon 15:04")
		}
		parts = append(parts, p)
	}
	return strings.Join(parts, " · ")
}

// fallbackControls set what an account at its limit does.
func (a *App) fallbackControls(c *ui.Context, st *accountsState, ag proto.AgentInfo, pr proto.AccountPrefs) {
	ui.Column(c).FillWidth().Gap(6).Padding(4, 0, 0, 0).Children(func() {
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "At a limit").FontSize(12).TextColor(theme.TextSoft).Width(110)
			modes := []string{proto.FallbackOff, proto.FallbackAsk, proto.FallbackAuto}
			cur := max(0, slices.Index(modes, pr.Fallback))
			if i := widget.Segmented(c, []string{"Stop and tell me", "Ask me first", "Go on as the next"}, cur, nil); i >= 0 && i != cur {
				pr.Fallback = modes[i]
				a.saveAccountPrefs(st, ag.ID, pr)
			}
		})
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Then").FontSize(12).TextColor(theme.TextSoft).Width(110)
			options, names := []string{""}, map[string]string{"": "nothing more"}
			for _, other := range a.agents {
				if other.Installed && other.ID != ag.ID && other.Command != "" && other.ID != "shell" {
					options = append(options, other.ID)
					names[other.ID] = "go on with " + other.Name
				}
			}
			to := pr.ToAgent
			hookSelect(c.Key("then:"+ag.ID), "Then", &to, options, names)
			if to != pr.ToAgent {
				pr.ToAgent = to
				a.saveAccountPrefs(st, ag.ID, pr)
			}
		})
		back := pr.Return
		if switchRowKey(c, "return:"+ag.ID, &back, "Back to the first once its limit resets", "Between turns, the session goes back to the account first in the order.") {
			pr.Return = back
			a.saveAccountPrefs(st, ag.ID, pr)
		}
		ui.Text(c, ag.Name+"'s terms are for your own accounts: don't share one, or use several to get around limits its plan doesn't allow. enowX only switches between the accounts you add here.").
			FontSize(11).TextColor(theme.Warning)
	})
}

func (a *App) saveAccountPrefs(st *accountsState, agentID string, pr proto.AccountPrefs) {
	cl := a.client
	if cl == nil {
		return
	}
	if st.info.Prefs == nil {
		st.info.Prefs = map[string]proto.AccountPrefs{}
	}
	st.info.Prefs[agentID] = pr // shown at once
	go func() {
		info, err := cl.SetAccountPrefs(agentID, pr)
		a.onMain(func() {
			if err != nil {
				st.err = client.Explain(err)
				return
			}
			st.info = info
		})
	}()
}

// accountHistory is which account each session used, when, and why.
func (a *App) accountHistory(c *ui.Context, st *accountsState) {
	if len(st.log) == 0 {
		ui.Text(c, "Nothing yet: sessions started, switched and fallen back show here.").FontSize(12).TextColor(theme.Muted)
		return
	}
	for i, e := range st.log[:min(len(st.log), 200)] {
		ui.Row(c.Key(i)).FillWidth().Gap(10).Padding(6, 8).Radius(theme.RadiusControl).Background(theme.Sunken).AlignItems(ui.Center).Children(func() {
			ui.Text(c, e.Time.Local().Format("Jan 2 15:04")).Font(theme.MonoFont).FontSize(11).TextColor(theme.Muted).Width(96)
			col := theme.TextSoft
			switch e.Event {
			case "fallback", "limit":
				col = theme.Warning
			case "back":
				col = theme.Done
			}
			ui.Text(c, e.Event).FontSize(11.5).TextColor(col).Width(64)
			name := e.Account
			if name == "" {
				name = "Official"
			}
			ui.Text(c, a.agentName(e.Agent)+" · "+name).FontSize(12).SingleLine().Width(200)
			ui.Text(c, e.Reason).FontSize(11).TextColor(theme.Muted).SingleLine().Grow(1).Shrink(1)
			if e.Session != "" {
				ui.Text(c, a.hookSessionName(e.Session)).FontSize(11).TextColor(theme.Muted).SingleLine().MaxWidth(160)
			}
		})
	}
}

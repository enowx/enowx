package ui

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/accounts"
	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/textfmt"
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

// configOrder is agent id's preferences and its configs' names in the order
// they're used, "" the official one.
func configOrder(st *accountsState, id string) (proto.AccountPrefs, []string) {
	pr := st.info.Prefs[id]
	var names []string
	for _, acc := range st.accountsOfAgent(id) {
		names = append(names, acc.Name)
	}
	return pr, accounts.Ordered(pr, names)
}

// configCardMin is what a config's card needs before its list of sessions.
const configCardMin = 272

// agentAccounts are the configs' cards, two a row, sharing the height of the
// page that isn't its other parts. Each row keeps the height it's given whatever
// its cards hold.
func (a *App) agentAccounts(c *ui.Context, st *accountsState, ag proto.AgentInfo, pr proto.AccountPrefs, order []string, height float32) {
	rows := (len(order) + 1) / 2
	row := float32(configCardMin)
	if height > 0 {
		room := height - st.aboveH - st.belowH - 2*theme.S3 - float32(rows-1)*theme.S3
		row = max(row, room/float32(rows))
	}
	tracks := make([]ui.Track, rows)
	for i := range tracks {
		tracks[i] = ui.Fixed(row)
	}
	widget.Cols(c.Key("configs"), 2).Shrink(0).RowTracks(tracks...).Children(func() {
		for i, name := range order {
			a.accountLine(c.Key("acc:"+name), st, ag, pr, order, i, name)
		}
	})
}

// agentLower is what follows the cards: what a limit does, with more than one
// config, and what happened last. Side by side only where the limit's choices
// fit in half of the page.
func (a *App) agentLower(c *ui.Context, st *accountsState, ag proto.AgentInfo, pr proto.AccountPrefs, order []string, width float32) {
	cols := 1
	if len(order) > 1 && width >= 820 {
		cols = 2
	}
	widget.Cols(c.Key("lower"), cols).Children(func() {
		if len(order) > 1 {
			a.fallbackControls(c, st, ag, pr)
		}
		a.agentLatest(c, st, ag)
	})
}

// accountLine is one config's card: its place, name, state and what it has
// left, the sessions on it, and what to do with it.
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
	label, detail := name, configDetail(acc)
	if !isOther {
		label, detail = "Default", "The agent as installed"
	}
	sessions := a.sessionsOnConfig(ag.ID, acc.ID)
	widget.GridCard(c).Children(func() {
		ui.Row(c.Key("title")).FillWidth().Shrink(0).Height(24).Gap(theme.S2).Children(func() {
			ui.Box(c).Size(24, 24).Shrink(0).Radius(theme.RadiusFull).Background(theme.Sunken).Center().Children(func() {
				ui.Text(c, strconv.Itoa(i+1)).FontSize(theme.TextLabel)
			})
			if hex, ok := widget.ParseHex(acc.Color); ok {
				widget.Dot(c, hex, false, 8)
			}
			ui.Row(c).Grow(1).MinWidth(0).Children(func() {
				widget.Running(c, label, widget.RunStyle{Size: theme.TextRead, Weight: 600})
			})
		})
		ui.Row(c.Key("tags")).FillWidth().Shrink(0).Wrap().Gap(theme.S1).Children(func() {
			if isOther {
				widget.Chip(c, configKindName(acc), theme.Accent)
			} else {
				widget.Chip(c, "Official", theme.Accent)
			}
			if pr.Default == name && len(order) > 1 {
				widget.Chip(c, "New sessions", theme.Accent)
			}
			accountStateTag(c, status)
		})
		ui.Row(c.Key("detail")).FillWidth().Shrink(0).Children(func() {
			widget.Running(c, detail, widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
		})
		accountCredits(c, status, acc)
		if only := pr.Only[name]; len(only) > 0 {
			ui.Row(c.Key("only")).FillWidth().Shrink(0).Children(func() {
				widget.Running(c, "Only for "+strings.Join(only, ", "), widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
			})
		}
		configSessions(c, sessions)
		a.onlyEditor(c, st, ag, pr, name)
		ui.Row(c.Key("foot")).FillWidth().Shrink(0).Gap(theme.S2).Children(func() {
			ui.Row(c).Grow(1).MinWidth(0).Children(func() {
				widget.Running(c, "Used by "+textfmt.Plural(len(sessions), "session", "sessions"), widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
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
			a.accountMenu(c, st, ag, pr, name, acc, isOther)
		})
	})
}

// accountMenu is what else can be done with a config.
func (a *App) accountMenu(c *ui.Context, st *accountsState, ag proto.AgentInfo, pr proto.AccountPrefs, name string, acc proto.Account, isOther bool) {
	widget.MoreMenu(c, &a.menu, "account:"+ag.ID+":"+name, func(done func()) {
		if isOther && widget.MenuItem(c, icon.Pencil, "Change…", false) {
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
		if isOther && widget.MenuItem(c, icon.Trash, "Remove…", true) {
			what := "It goes from every host of yours; sessions using it keep running until they end. The official config stays as it is."
			if acc.Kind == proto.ConfigKindAccount && acc.Cred == "" {
				what = "Its sign-in on " + a.host.Name + " goes with it, and the config from every host of yours; sessions using it keep running until they end."
			}
			a.confirm("Remove the config "+name+"?", what, "Remove", func() { a.removeAccount(st, acc) })
			done()
		}
	})
}

// onlyEditor keeps a config to the projects typed, while it's being set.
func (a *App) onlyEditor(c *ui.Context, st *accountsState, ag proto.AgentInfo, pr proto.AccountPrefs, name string) {
	key := ag.ID + "\x00" + name
	v := st.only[key]
	if v == nil {
		return
	}
	ui.Row(c.Key("only-edit")).FillWidth().Shrink(0).Gap(theme.S1).Children(func() {
		in := widget.Input(c, v, "enowx, enowx-web").Grow(1).Basis(0).MinWidth(0).AutoFocus()
		if widget.GhostButton(c, "Cancel").Clicked() {
			delete(st.only, key)
		}
		if widget.Button(c, widget.Secondary, nil, "Save", "").Clicked() || in.Submitted() {
			if pr.Only == nil {
				pr.Only = map[string][]string{}
			}
			pr.Only[name] = splitList(*v)
			if len(pr.Only[name]) == 0 {
				delete(pr.Only, name)
			}
			delete(st.only, key)
			a.saveAccountPrefs(st, ag.ID, pr)
		}
	})
}

// accountCredits is what a config has left: its line and a meter of its fullest
// window, or its reading's error; a model API's limit isn't known.
func accountCredits(c *ui.Context, s proto.AccountStatus, acc proto.Account) {
	line, col := "", theme.TextSoft
	switch {
	case s.Credits != nil && s.Credits.Error != "":
		line, col = s.Credits.Error, theme.Warning
	case s.Credits != nil:
		line = creditLine(*s.Credits)
	case acc.Kind == proto.ConfigKindAPI:
		line = "No limit known"
	}
	if line == "" {
		return
	}
	ui.Row(c.Key("credits")).FillWidth().Shrink(0).Children(func() {
		widget.Running(c, line, widget.RunStyle{Size: theme.TextLabel, Color: col})
	})
	if s.Credits == nil || s.Credits.Error != "" || len(s.Credits.Windows) == 0 {
		return
	}
	used := 0.0
	for _, w := range s.Credits.Windows {
		used = max(used, w.Used)
	}
	fill := theme.Working
	if used >= 100 {
		fill = theme.Warning
	}
	widget.Meter(c.Key("meter"), widget.Segment{Fraction: used / 100, Color: fill})
}

// sessionsOnConfig are the names of the sessions of agent id on config (an ID,
// "" the official one).
func (a *App) sessionsOnConfig(id, config string) []string {
	var out []string
	for _, s := range a.sessions {
		if agent.Base(s.Agent) == agent.Base(id) && s.Account == config {
			out = append(out, s.Name)
		}
	}
	return out
}

// configSessions lists a card's sessions in what the card leaves for them: those
// that fit, and how many more.
func configSessions(c *ui.Context, names []string) {
	const rowHeight = 32
	list := ui.Column(c.Key("sessions")).Grow(1).Basis(0).FillWidth().MinHeight(0).Clip()
	fits := 5
	if h := list.Bounds().H; h > 0 {
		fits = max(1, int(h/rowHeight))
	}
	shown, more := names, 0
	if len(names) > fits {
		shown, more = names[:fits-1], len(names)-fits+1
	}
	list.Children(func() {
		if len(names) == 0 {
			widget.CardNote(c, "No session now.", 1)
			return
		}
		for i, name := range shown {
			line := ui.Row(c.Key(i)).FillWidth().Shrink(0).Height(rowHeight)
			if i > 0 {
				line.BorderWidth(0.5, 0, 0, 0).BorderColor(theme.Border)
			}
			line.Children(func() { widget.Running(c, name, widget.RunStyle{Size: theme.TextLabel}) })
		}
		if more > 0 {
			line := ui.Row(c.Key("more")).FillWidth().Shrink(0).Height(rowHeight)
			if len(shown) > 0 {
				line.BorderWidth(0.5, 0, 0, 0).BorderColor(theme.Border)
			}
			line.Children(func() {
				widget.Running(c, "and "+strconv.Itoa(more)+" more", widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
			})
		}
	})
}

// configKindName is a config's kind, as its card's chip and the form name it.
func configKindName(c proto.Account) string {
	switch {
	case c.Kind == proto.ConfigKindAPI:
		return kindAPI
	case c.Cred != "":
		return kindKey
	}
	return kindAccount
}

// accountStateTag says an account's limit, if it hit one.
func accountStateTag(c *ui.Context, s proto.AccountStatus) {
	switch s.State {
	case proto.AccountLimit:
		label := "At its limit"
		if !s.Until.IsZero() {
			label += " until " + s.Until.Local().Format("15:04")
			if time.Until(s.Until) > 20*time.Hour {
				label = "At its limit until " + s.Until.Local().Format("Jan 2 15:04")
			}
		}
		widget.Chip(c, label, theme.Warning)
	case proto.AccountCredits:
		widget.Chip(c, "Out of credits", theme.Error)
	case proto.AccountAuth:
		widget.Chip(c, "Signed out", theme.Error)
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
		p := fmt.Sprintf("%s %.0f%%", w.Short(), w.Used)
		if !w.ResetsAt.IsZero() {
			at := w.ResetsAt.Local()
			if y, m, d := time.Now().Date(); at.Year() == y && at.Month() == m && at.Day() == d {
				p += ", resets " + at.Format("15:04")
			} else {
				p += ", resets " + at.Format("Mon 15:04")
			}
		}
		parts = append(parts, p)
	}
	return strings.Join(parts, " · ")
}

// fallbackControls set what an account at its limit does.
func (a *App) fallbackControls(c *ui.Context, st *accountsState, ag proto.AgentInfo, pr proto.AccountPrefs) {
	widget.Card(c.Key("at-a-limit")).Children(func() {
		widget.CardTitle(c, nil, "At a limit", nil)
		modes := []string{proto.FallbackAuto, proto.FallbackAsk, proto.FallbackOff}
		cur := slices.Index(modes, pr.Fallback)
		if cur < 0 {
			cur = slices.Index(modes, proto.FallbackOff) // not set yet stops
		}
		if i := widget.Segmented(c, []string{"Go on as the next", "Ask me first", "Stop and tell me"}, cur, nil); i >= 0 && i != cur {
			pr.Fallback = modes[i]
			a.saveAccountPrefs(st, ag.ID, pr)
		}
		ui.Row(c.Key("then")).FillWidth().Shrink(0).Gap(theme.S2).Children(func() {
			ui.Text(c, "Then").FontSize(theme.TextUI).TextColor(theme.TextSoft).Shrink(0)
			options, names := []string{""}, map[string]string{"": "nothing more"}
			for _, other := range a.agents {
				if other.Installed && other.ID != ag.ID && other.Command != "" && other.ID != "shell" {
					options = append(options, other.ID)
					names[other.ID] = "go on with " + other.Name
				}
			}
			to := pr.ToAgent
			ui.Row(c).Grow(1).Basis(0).MinWidth(0).Children(func() { hookSelect(c.Key("then:"+ag.ID), "Then", &to, options, names) })
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
		widget.CardNote(c, ag.Name+"'s terms are for your own accounts: don't share one, or use several to get around limits its plan doesn't allow. enowX only switches between the accounts you add here.", 0)
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

// accountEventTitle says what an entry of the history did, and its color.
func accountEventTitle(e proto.AccountLogEntry) (string, ui.Color) {
	name := e.Account
	if name == "" {
		name = "Default"
	}
	switch e.Event {
	case "fallback":
		return "Fell back to " + name, theme.Warning
	case "limit":
		return name + " reached its limit", theme.Warning
	case "back":
		return "Back on " + name, theme.Done
	case "switch":
		return "Switched to " + name, theme.Text
	case "start":
		return "Started on " + name, theme.Text
	}
	return e.Event + " · " + name, theme.Text
}

// agentLatest is the last of an agent's history.
func (a *App) agentLatest(c *ui.Context, st *accountsState, ag proto.AgentInfo) {
	ui.Column(c.Key("latest")).MinWidth(0).Gap(theme.S1).Children(func() {
		widget.Heading(c, "Latest of "+ag.Name)
		n := 0
		for _, e := range st.log {
			if e.Agent != agent.Base(ag.ID) {
				continue
			}
			if n == 3 {
				return
			}
			title, col := accountEventTitle(e)
			what := title
			if e.Session != "" {
				what += " · " + a.hookSessionName(e.Session)
			}
			if e.Reason != "" {
				what += " · " + e.Reason
			}
			ui.Row(c.Key(n)).FillWidth().Shrink(0).Height(32).Gap(theme.S2).BorderWidth(0.5, 0, 0, 0).BorderColor(theme.Border).Children(func() {
				at := e.Time.Local().Format("Jan 2")
				if y, m, d := time.Now().Date(); e.Time.Local().Year() == y && e.Time.Local().Month() == m && e.Time.Local().Day() == d {
					at = e.Time.Local().Format("15:04")
				}
				ui.Text(c, at).FontSize(theme.TextLabel).TextColor(theme.TextSoft).Width(44).Shrink(0).SingleLine()
				ui.Row(c).Grow(1).MinWidth(0).Children(func() { widget.Running(c, what, widget.RunStyle{Size: theme.TextLabel, Color: col}) })
			})
			n++
		}
		if n == 0 {
			widget.CardNote(c, "Nothing yet.", 1)
		}
	})
}

// accountHistory is which config each session used, when, and why.
func (a *App) accountHistory(c *ui.Context, st *accountsState) {
	if len(st.log) == 0 {
		widget.PageFill(c, func() {
			widget.EmptyState(c, icon.History, "Nothing yet", "Sessions started, switched and fallen back show here.", nil)
		})
		return
	}
	widget.PageBody(c, true, func() {
		widget.Cols(c.Key("history"), 2).Children(func() {
			for i, e := range st.log[:min(len(st.log), 200)] {
				title, col := accountEventTitle(e)
				ui.Column(c.Key(i)).MinWidth(0).Padding(theme.S3, theme.S4).Gap(2).Radius(theme.RadiusCard).Background(theme.Raised).Children(func() {
					ui.Row(c).FillWidth().Gap(theme.S2).Children(func() {
						ui.Row(c).Grow(1).MinWidth(0).Children(func() { widget.Running(c, title, widget.RunStyle{Weight: 500, Color: col}) })
						ui.Text(c, e.Time.Local().Format("Jan 2 15:04")).FontSize(theme.TextLabel).TextColor(theme.TextSoft).Shrink(0).SingleLine()
					})
					what := a.agentName(e.Agent)
					if e.Session != "" {
						what += " · " + a.hookSessionName(e.Session)
					}
					if e.Reason != "" {
						what += " · " + e.Reason
					}
					widget.Running(c.Key("sub"), what, widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
				})
			}
		})
	})
}

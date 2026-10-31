package ui

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/accounts"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// A session's account at its limit: asked first, a dialog to go on as the
// next account (or agent); switched by itself, a line saying so; and a
// session's menu switching its account by hand.

// accountEvent takes a fallback the daemon told.
func (a *App) accountEvent(ev proto.Event) {
	if ev.Name != proto.EventAccountFallback {
		return
	}
	var f proto.AccountFallback
	if json.Unmarshal(ev.Data, &f) != nil {
		return
	}
	a.onMain(func() {
		switch {
		case f.Ask:
			a.acctAsk = &f
		case f.Back:
			a.notify(fmt.Sprintf("%s is back on %s, its limit reset", a.sessionLabel(f.Session), accountTitle(f.To)))
		case f.Switched:
			to := accountTitle(f.To)
			if f.ToAgent != "" {
				to = a.agentName(f.ToAgent)
			}
			a.notify(fmt.Sprintf("%s: %s %s, went on as %s", a.sessionLabel(f.Session), accountTitle(f.From), limitText(f.Reason), to))
		default:
			a.notify(fmt.Sprintf("%s: %s %s", a.sessionLabel(f.Session), accountTitle(f.From), limitText(f.Reason)))
		}
		if a.accts.asked {
			a.loadAccounts()
		}
	})
}

func accountTitle(name string) string {
	if name == "" {
		return "the official config"
	}
	return name
}

func limitText(reason string) string {
	switch reason {
	case proto.AccountCredits:
		return "ran out of credits"
	case proto.AccountAuth:
		return "was signed out"
	}
	return "hit its usage limit"
}

func (a *App) sessionLabel(id string) string { return a.hookSessionName(id) }

// accountAskDialog asks to go on as the next account.
func (a *App) accountAskDialog(c *ui.Context) {
	f := a.acctAsk
	if f == nil {
		return
	}
	open := true
	widget.Dialog(c, &open, 460, 12, func() {
		what := accountTitle(f.From) + " " + limitText(f.Reason)
		if !f.Until.IsZero() {
			what += ", until " + f.Until.Local().Format("Mon 15:04")
		}
		widget.DialogHeader(c, a.sessionLabel(f.Session)+": "+what, f.Line, false)
		to := accountTitle(f.To)
		if f.ToAgent != "" {
			to = a.agentName(f.ToAgent) + " (a new conversation, told where the task stands)"
		}
		ui.Text(c, "Go on as "+to+"? The session picks up its conversation and is told to go on.").FontSize(12.5).TextColor(theme.TextSoft)
		ui.Row(c).FillWidth().Gap(8).Children(func() {
			ui.Spacer(c)
			if widget.GhostButton(c, "Not now").Clicked() {
				a.acctAsk = nil
			}
			if widget.PrimaryButton(c, icon.Play, "Switch and go on", "").Clicked() {
				a.acctAsk = nil
				a.switchSessionAccount(f.Session, f.ToID, f.ToAgent, true)
			}
		})
	})
	if !open {
		a.acctAsk = nil
	}
}

// switchSessionAccount has session id go on as account (an ID, "" its
// agent's own) or agent.
func (a *App) switchSessionAccount(id, account, agentID string, goOn bool) {
	cl := a.client
	if cl == nil {
		return
	}
	go func() {
		info, err := cl.SwitchAccount(proto.AccountSwitchParams{Session: id, Account: account, Agent: agentID, Continue: goOn})
		a.onMain(func() {
			if err != nil {
				a.status = "Couldn't switch the config: " + client.Explain(err)
				return
			}
			a.upsert(info)
			a.notify(a.sessionLabel(id) + " switched its config at " + time.Now().Format("15:04"))
		})
	}()
}

// switchAccountItems are a session's menu items moving it to another of
// its agent's accounts.
func (a *App) switchAccountItems(c *ui.Context, s proto.SessionInfo, done func()) {
	st := a.accountsOf()
	others := st.accountsOfAgent(s.Agent)
	if len(others) == 0 || s.Parent != "" {
		return
	}
	if s.Account != "" && widget.MenuItemTip(c, icon.Account, "Switch to the official config", "Its conversation picked up as the agent as installed", false) {
		a.switchSessionAccount(s.ID, "", "", false)
		done()
	}
	for _, acc := range others {
		if acc.ID == s.Account {
			continue
		}
		if widget.MenuItemTip(c.Key("switch:"+acc.ID), icon.Account, "Switch to "+acc.Name, "Its conversation picked up on the config "+acc.Name+" ("+configKindLabel(acc)+")", false) {
			a.switchSessionAccount(s.ID, acc.ID, "", false)
			done()
		}
	}
}

// defaultAccountFor is the account a new session of agentID in project
// starts on, by the preferences: its ID, "" the agent's own.
func (a *App) defaultAccountFor(agentID, project string) string {
	st := a.accountsOf()
	name := accounts.Default(st.info.Prefs[agentID], project)
	for _, acc := range st.accountsOfAgent(agentID) {
		if acc.Name == name {
			return acc.ID
		}
	}
	return ""
}

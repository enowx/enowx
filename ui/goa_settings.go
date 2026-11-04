package ui

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// goaSettingsForm is what GOD of Agents runs on, being changed.
type goaSettingsForm struct {
	open          bool
	agent, config string // the picks: an agent's ID, a config's ID ("" the official one)
	checked       string // the picks the check below is of
	check         proto.GOASwitchResult
	checkErr      string
	checking      bool
	restarting    bool
	err           string
}

func goaSettingsKey(agent, config string) string { return agent + "\x00" + config }

// openGOASettings opens the panel on what GOA runs on now.
func (a *App) openGOASettings() {
	s, ok := a.goaSession()
	if !ok {
		return
	}
	a.goa.settings = &goaSettingsForm{open: true, agent: s.Agent, config: s.Account}
	a.goa.settings.checked = goaSettingsKey(s.Agent, s.Account)
	a.accountsOf()
	a.loadGOAAgents("This host's enowX is too old for GOD of Agents: update it.")
}

// goaSettingsPick picks an agent: its preferred config, or the one GOA is on with it now.
func (a *App) goaSettingsPick(f *goaSettingsForm, s proto.SessionInfo, agent string) {
	if f.agent == agent {
		return
	}
	f.agent, f.config = agent, a.defaultAccountFor(agent, "")
	if agent == s.Agent {
		f.config = s.Account
	}
	f.err = ""
}

// goaSettingsCheck asks the host what the picks would do, once for each pick: what can't
// be done shows before anything is pressed.
func (a *App) goaSettingsCheck(f *goaSettingsForm) {
	key := goaSettingsKey(f.agent, f.config)
	cl := a.client
	if f.checked == key || cl == nil {
		return
	}
	f.checked, f.checking, f.checkErr, f.check = key, true, "", proto.GOASwitchResult{}
	p := goaSwitchParamsOf(f)
	p.Check = true
	go func() {
		var res proto.GOASwitchResult
		err := cl.Call(proto.MethodGOASwitch, p, &res)
		a.onMain(func() {
			if f.checked != key {
				return // another pick since
			}
			f.checking, f.check, f.checkErr = false, res, goaSwitchErrorText(err)
		})
	}()
}

func goaSwitchParamsOf(f *goaSettingsForm) proto.GOASwitchParams {
	config := f.config
	if config == "" {
		config = proto.GOAConfigOfficial
	}
	return proto.GOASwitchParams{Agent: f.agent, Config: config}
}

// goaSwitchErrorText says err to the user: GOA is as it was, which an older daemon can't
// say by itself.
func goaSwitchErrorText(err error) string {
	switch {
	case err == nil:
		return ""
	case client.IsUnknownMethod(err):
		return "This host's enowX is too old to change this: update it. GOD of Agents goes on as it was."
	}
	_, text, found := strings.Cut(err.Error(), "goa: ") // the daemon's marks, on what it says
	if !found {
		text = err.Error()
	}
	return goaSentence(text)
}

// goaSwitchRun asks the host to make the change; what needs a yes comes back undone, and
// is asked here, so it is the host's word on whether GOA is at work that counts.
func (a *App) goaSwitchRun(f *goaSettingsForm, s proto.SessionInfo, yes bool) {
	p := goaSwitchParamsOf(f)
	p.Force, p.NewConversation = yes, yes
	var res proto.GOASwitchResult
	f.restarting, f.err = true, ""
	a.goaCallQuiet(proto.MethodGOASwitch, p, &res, func(err error) {
		f.restarting = false
		if err != nil {
			f.err = goaSwitchErrorText(err)
			return
		}
		if !res.Restarted && !res.Same {
			a.goaSwitchAsk(f, s, res)
			return
		}
		f.open = false
		if res.Restarted {
			a.upsert(res.Session)
			a.goa.board.Session, a.goa.board.Agent, a.goa.board.Level = res.Session.ID, res.Agent, res.Level
			a.goa.chatSet = "" // its chat, where the new agent has one
			a.notify(a.goaSwitchDone(res))
		}
	})
}

// goaSwitchAsk asks for the yes res lacks, saying what would be lost.
func (a *App) goaSwitchAsk(f *goaSettingsForm, s proto.SessionInfo, res proto.GOASwitchResult) {
	title, msg := "Restart GOD of Agents while it works?", "What it is doing now is cut off. It goes on with the same conversation on the new config."
	if res.Conversation == proto.GOAConversationNew {
		title = "Start a new conversation on " + a.agentName(res.Agent) + "?"
		msg = "The conversation with " + a.agentName(s.Agent) + " doesn't carry over. The board, GOA's collection, its rules and waiting lines stay, and workers keep running."
		if res.AtWork {
			msg = "GOD of Agents is at work, which this cuts off. " + msg
		}
	}
	a.confirm(title, msg, "Restart it", func() { a.goaSwitchRun(f, s, true) })
}

func (a *App) goaSwitchDone(res proto.GOASwitchResult) string {
	on := a.agentName(res.Agent) + " on " + accountTitle(res.Config)
	switch res.Conversation {
	case proto.GOAConversationNew:
		return "GOD of Agents runs on " + on + ", in a new conversation"
	case proto.GOAConversationSame:
		return "GOD of Agents runs on " + on + ", in the same conversation"
	}
	return "GOD of Agents runs on " + on
}

// goaSettingsDialog is the panel of GOA's header's Settings.
func (a *App) goaSettingsDialog(c *ui.Context) {
	f := a.goa.settings
	if f == nil {
		return
	}
	s, ok := a.goaSession()
	if !f.open || !ok {
		a.goa.settings = nil
		return
	}
	a.goaSettingsCheck(f)
	changed := goaSettingsKey(f.agent, f.config) != goaSettingsKey(s.Agent, s.Account)
	widget.FormSheet(c, &f.open, 520, "Settings of GOD of Agents", "What it runs on. Its board, collection, rules and workers stay whatever you pick.", func() {
		if a.goa.err != "" {
			widget.Notice(c, icon.Alert, theme.Error, a.goa.err, "", nil)
			return
		}
		a.goaSettingsNow(c, s)
		widget.Field(c, "Agent", func() { a.goaSettingsAgents(c, f, s) })
		widget.Field(c, "Config", func() { a.goaSettingsConfigs(c, f) })
		if changed {
			a.goaSettingsOutcome(c, f)
		}
		if f.err != "" {
			widget.Notice(c, icon.Alert, theme.Error, f.err, "", nil)
		}
	}, func() {
		label := "Restart GOA"
		if f.restarting {
			label = "Restarting…"
		}
		b := widget.PrimaryButton(c, nil, label, "")
		if !changed || f.restarting || f.checking || f.checkErr != "" || a.goa.err != "" {
			b.Disabled(true)
		} else if b.Clicked() {
			a.goaSwitchRun(f, s, false)
		}
	})
}

// goaSettingsNow is what GOA runs on now.
func (a *App) goaSettingsNow(c *ui.Context, s proto.SessionInfo) {
	widget.Field(c, "Running now", func() {
		ui.Row(c.Key("now")).FillWidth().MinWidth(0).Gap(theme.S2).Children(func() {
			ui.Row(c).Grow(1).MinWidth(0).Children(func() {
				widget.Running(c, a.agentName(s.Agent)+" · "+accountTitle(goaConfigName(s)), widget.RunStyle{Weight: 600})
			})
			widget.Legend(c, goaLevelColor(a.goa.board.Level), goaSentence(a.goa.board.Level))
		})
	})
}

func goaConfigName(s proto.SessionInfo) string {
	if s.Config != nil {
		return s.Config.Name
	}
	return ""
}

// goaSettingsAgents lists the agents GOA is offered on, each with its level.
func (a *App) goaSettingsAgents(c *ui.Context, f *goaSettingsForm, s proto.SessionInfo) {
	if a.goa.agents == nil {
		ui.Text(c, "Reading the agents…").FontSize(theme.TextUI).TextColor(theme.TextSoft)
		return
	}
	ui.Column(c).FillWidth().Gap(theme.S1).Children(func() {
		for _, ag := range a.goa.agents {
			if ag.Installed && goaAgentRow(c.Key("settings-agent:"+ag.Agent), ag, f.agent == ag.Agent) {
				a.goaSettingsPick(f, s, ag.Agent)
			}
		}
	})
}

// goaSettingsConfigs lists the configs of the agent picked, the official one first.
func (a *App) goaSettingsConfigs(c *ui.Context, f *goaSettingsForm) {
	st := a.accountsOf()
	ui.Column(c).FillWidth().Gap(theme.S1).Children(func() {
		if widget.ListItem(c.Key("settings-official"), ui.Color{}, "Official", "The agent as installed", f.config == "", nil).Clicked() {
			f.config, f.err = "", ""
		}
		if st.err != "" {
			widget.Notice(c, icon.Alert, theme.Error, st.err, "", nil)
		}
		for _, acc := range st.accountsOfAgent(f.agent) {
			if widget.ListItem(c.Key("settings-config:"+acc.ID), ui.Color{}, acc.Name, configKindLabel(acc), f.config == acc.ID, nil).Clicked() {
				f.config, f.err = acc.ID, ""
			}
		}
	})
}

// goaSettingsOutcome says what the picks do, or why they can't be done.
func (a *App) goaSettingsOutcome(c *ui.Context, f *goaSettingsForm) {
	switch {
	case f.checkErr != "":
		widget.Notice(c, icon.Alert, theme.Error, f.checkErr, "", nil)
	case f.checking:
		ui.Text(c, "Checking…").FontSize(theme.TextUI).TextColor(theme.TextSoft)
	default:
		text := "It restarts and goes on with the same conversation."
		switch f.check.Conversation {
		case proto.GOAConversationNew:
			text = "It restarts on a new conversation: the one it is on doesn't carry over to another agent."
		case proto.GOAConversationNone:
			text = "It restarts. It has no conversation yet to go on with."
		}
		ui.Text(c, text).FontSize(theme.TextUI).TextColor(theme.TextSoft)
		if f.check.Note != "" {
			ui.Text(c.Key("note"), f.check.Note).FontSize(theme.TextLabel).TextColor(theme.TextSoft)
		}
	}
}

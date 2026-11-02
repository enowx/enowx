package ui

import (
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// agentStartCard is what new sessions of the agent chosen above start on, kept on the host:
// what one session saves as its agent's default doesn't reach them once this is set.
func (a *App) agentStartCard(c *ui.Context) {
	id := agent.Base(a.defaultAgent())
	set := agent.SettingsOf(id)
	if len(set.Models) == 0 && len(set.Efforts) == 0 {
		return
	}
	hs := a.hostSettingsOf()
	cur := hs.agentStart[id]
	widget.Card(c.Key("agent-start")).Children(func() {
		widget.CardTitle(c, nil, "New sessions start on", nil)
		row := func(name string, list []agent.Choice, value string, pick func(*proto.AgentStart, string)) {
			if len(list) == 0 {
				return
			}
			options, at := []string{"The agent's own"}, 0
			for i, ch := range list {
				options = append(options, ch.Label)
				if ch.ID == value {
					at = i + 1
				}
			}
			widget.Field(c.Key(name), name, func() {
				if i := widget.Choice(c, name, options, at, 0); i >= 0 {
					next := cur
					if i == 0 {
						pick(&next, "")
					} else {
						pick(&next, list[i-1].ID)
					}
					a.setAgentStart(id, next)
				}
			})
		}
		row("Model", set.Models, cur.Model, func(s *proto.AgentStart, v string) { s.Model = v })
		row("Effort", set.Efforts, cur.Effort, func(s *proto.AgentStart, v string) { s.Effort = v })
		if len(set.ModeFlag) > 0 {
			row("Permission mode", set.Modes, cur.Mode, func(s *proto.AgentStart, v string) { s.Mode = v })
		}
		ui.Text(c, "Set here, a new session isn't changed by what another saved as its agent's default. The agent's own takes whatever its settings say by then.").
			FontSize(theme.TextLabel).TextColor(theme.TextSoft)
	})
}

// setAgentStart keeps agent id's choice on the connected host, with its other settings.
func (a *App) setAgentStart(id string, start proto.AgentStart) {
	hs, cl := a.hostSettingsOf(), a.client
	if cl == nil {
		return
	}
	if hs.agentStart == nil {
		hs.agentStart = map[string]proto.AgentStart{}
	}
	hs.agentStart[id] = start
	go func() {
		st, err := cl.Settings()
		if err == nil {
			if st.AgentStart == nil {
				st.AgentStart = map[string]proto.AgentStart{}
			}
			st.AgentStart[id] = start
			_, err = cl.SetSettings(st)
		}
		if err != nil {
			a.onMain(func() { a.status = client.Explain(err) })
		}
	}()
}

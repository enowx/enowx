package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// Limits on agents: how many may work at once on a host (the daemon
// queues the others) and how long one session's agent may work (its
// budget, past which the daemon interrupts it).

// budgetChoices are the budgets offered, in minutes; 0 is none.
var budgetChoices = []int{0, 15, 30, 60, 120}

// agentChoices are the limits of agents at once offered; 0 is none.
var agentChoices = []int{0, 1, 2, 3, 4, 6}

// budgetForm sets a session's budget.
type budgetForm struct {
	open    bool
	session proto.SessionInfo
	minutes string
}

// agentsForm sets the host's limit of agents at once.
type agentsForm struct {
	open    bool
	loaded  bool
	current int
	pick    int
}

func (a *App) askBudget(s proto.SessionInfo) {
	m := ""
	if s.Budget > 0 {
		m = strconv.FormatInt(s.Budget/60, 10)
	}
	a.budget = &budgetForm{open: true, session: s, minutes: m}
}

// budgetDialog sets how long the session's agent may work.
func (a *App) budgetDialog(c *ui.Context) {
	f := a.budget
	if f == nil {
		return
	}
	s := f.session
	save := false
	worked := "It has worked " + workTime(s.Worked) + " so far."
	widget.FormSheet(c, &f.open, 480, "Time budget of this session",
		"How long “"+a.sessionName(s)+"” may work, counting only while it works. Past it, enowX interrupts it. "+worked, func() {
			widget.Field(c, "Stop after", func() {
				options, names := []string{}, map[string]string{f.minutes: f.minutes + " minutes"}
				for _, m := range budgetChoices {
					id, label := "", "No limit"
					if m > 0 {
						id, label = strconv.Itoa(m), workTime(int64(m)*60)
					}
					options, names[id] = append(options, id), label
				}
				hookSelect(c, "Stop after", &f.minutes, options, names)
			})
			widget.Field(c, "Minutes", func() {
				if widget.Input(c, &f.minutes, "No limit").FillWidth().Submitted() {
					save = true
				}
			})
		}, func() {
			if widget.PrimaryButton(c, nil, "Save", "").Clicked() {
				save = true
			}
		})
	if save {
		minutes, err := strconv.Atoi(strings.TrimSpace(f.minutes))
		if strings.TrimSpace(f.minutes) == "" {
			minutes, err = 0, nil
		}
		if err != nil || minutes < 0 {
			a.status = "The budget is a number of minutes."
		} else if cl := a.client; cl != nil {
			f.open = false
			go func() {
				if err := cl.SetBudget(s.ID, time.Duration(minutes)*time.Minute); err != nil {
					a.onMain(func() { a.status = err.Error() })
				}
			}()
		}
	}
	if !f.open {
		a.budget = nil
	}
}

// askAgentLimit opens the host's limit of agents at once, reading it.
func (a *App) askAgentLimit() {
	cl := a.client
	if cl == nil {
		return
	}
	f := &agentsForm{open: true}
	a.agentLimit = f
	go func() {
		st, err := cl.Settings()
		a.onMain(func() {
			if err != nil {
				a.status = err.Error()
				f.open = false
				return
			}
			f.loaded, f.current, f.pick = true, st.MaxAgents, st.MaxAgents
		})
	}()
}

// agentLimitDialog sets how many agents may work at once on the host.
func (a *App) agentLimitDialog(c *ui.Context) {
	f := a.agentLimit
	if f == nil {
		return
	}
	save := false
	widget.FormSheet(c, &f.open, 480, "Agents at once on "+a.host.Name,
		"More wait in a queue, and start as others finish.", func() {
			if !f.loaded {
				ui.Text(c, "Reading…").FontSize(theme.TextUI).TextColor(theme.TextSoft)
				return
			}
			labels := make([]string, len(agentChoices))
			cur := 0
			for i, n := range agentChoices {
				labels[i] = strconv.Itoa(n)
				if n == f.pick {
					cur = i
				}
			}
			labels[0] = "No limit"
			if i := widget.Segmented(c.Key("limit"), labels, cur, nil); i >= 0 {
				f.pick = agentChoices[i]
			}
		}, func() {
			if f.loaded && widget.PrimaryButton(c, nil, "Save", "").Clicked() {
				save = true
			}
		})
	if save {
		f.open = false
		pick := f.pick
		if cl := a.client; cl != nil {
			go func() {
				// The other settings stay as they are.
				st, err := cl.Settings()
				if err == nil {
					st.MaxAgents = pick
					_, err = cl.SetSettings(st)
				}
				a.onMain(func() {
					if err != nil {
						a.status = err.Error()
						return
					}
					if pick == 0 {
						a.notify("Agents on " + a.host.Name + " work without a limit")
					} else {
						a.notify(fmt.Sprintf("At most %d %s work at once on %s", pick, textfmt.Plural(pick, "agent", "agents"), a.host.Name))
					}
				})
			}()
		}
	}
	if !f.open {
		a.agentLimit = nil
	}
}

// workTime writes seconds of work: 45s, 12m, 1h 30m.
func workTime(seconds int64) string {
	d := time.Duration(seconds) * time.Second
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", seconds)
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d%time.Hour == 0:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
}

// budgetFact says how far a session is into its budget, "" without one.
func budgetFact(s proto.SessionInfo) (string, ui.Color) {
	if s.Budget <= 0 {
		return "", theme.Muted
	}
	text := "worked " + workTime(s.Worked) + " of " + workTime(s.Budget)
	if s.OverBudget {
		return text + " · budget reached", theme.Warning
	}
	return text, theme.Muted
}

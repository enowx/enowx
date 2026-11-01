package ui

import (
	"github.com/enowx/enowx/core/hosts"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// GOA's tools in the side panel: Sessions, every session of every host
// with the task GOA gave it; Tasks, the board, each prompt's model and
// effort with why (the user can change them), and what a watched GOA did
// itself, to revert.

const (
	panelSessions = "goa-sessions"
	panelTasks    = "goa-tasks"
)

// goaScroll is a tool's body: a padded column that scrolls.
func goaScroll(c *ui.Context, gap float32, body func()) {
	ui.Scroll(c).Grow(1).FillWidth().MinHeight(0).Children(func() {
		ui.Column(c).FillWidth().Padding(10).Gap(gap).Children(body)
	})
}

// goaHostSessions are a host's sessions as the Sessions tool lists them.
type goaHostSessions struct {
	name, state string
	cl          *client.Client
	sessions    []proto.SessionInfo
}

// goaEveryHost is every host's sessions: the one shown first, then those
// watched, by name.
func (a *App) goaEveryHost() []goaHostSessions {
	var out []goaHostSessions
	if a.client != nil {
		out = append(out, goaHostSessions{name: a.host.Name, state: hostOnline, cl: a.client, sessions: a.sessions})
	}
	var watched []goaHostSessions
	for _, w := range a.watches {
		watched = append(watched, goaHostSessions{name: w.host.Name, cl: w.cl, state: w.status.state, sessions: w.status.sessions})
	}
	slices.SortFunc(watched, func(x, y goaHostSessions) int { return strings.Compare(x.name, y.name) })
	return append(out, watched...)
}

// goaListed says whether session s is one the Sessions tool lists: the
// user's, not a tab, nor the app's own agents, nor GOA itself.
func goaListed(s proto.SessionInfo) bool {
	return s.Parent == "" && s.NotesFor == "" && s.CanvasFor == "" && !s.GOA
}

// sessionsTab is the Sessions tool.
func (a *App) sessionsTab(c *ui.Context) {
	goaScroll(c, 10, func() {
		for _, h := range a.goaEveryHost() {
			list := slices.DeleteFunc(slices.Clone(h.sessions), func(s proto.SessionInfo) bool { return !goaListed(s) })
			ui.Row(c).FillWidth().Gap(8).PaddingX(4).Children(func() {
				col := theme.Done
				if h.state != hostOnline {
					col = theme.Idle
				}
				widget.Dot(c, col, h.state != hostOnline, 6)
				widget.SectionLabel(c, h.name, theme.Muted)
				ui.Spacer(c)
				ui.Text(c, plural(len(list), "session")).FontSize(theme.TextLabel).TextColor(theme.Idle)
			})
			if len(list) == 0 {
				ui.Text(c, "No sessions.").FontSize(theme.TextLabel).TextColor(theme.Muted).PaddingX(4)
				continue
			}
			for _, s := range list {
				a.goaSessionRow(c.Key("goa-s:"+s.ID), h.name, s)
			}
		}
	})
}

// goaSessionRow is a session: its state, name and agent, and its task.
// Clicking it opens it in a tab; its details are a button beside it.
func (a *App) goaSessionRow(c *ui.Context, host string, s proto.SessionInfo) {
	ui.Row(c).FillWidth().Gap(2).AlignItems(ui.Center).Children(func() {
		a.goaSessionButton(c, host, s)
		if widget.IconButton(c, icon.More, "Details of "+a.sessionName(s), false).Clicked() {
			a.goa.detail = &goaDetail{open: true, host: host, session: s}
		}
	})
}

func (a *App) goaSessionButton(c *ui.Context, host string, s proto.SessionInfo) {
	st := statusOf(s)
	t := a.goaTaskOf(s.ID)
	row := ui.ButtonBase(c).Grow(1).MinWidth(0).Padding(7, 8).Gap(8).Radius(theme.RadiusControl).Cursor(ui.CursorPointer).Label(a.sessionName(s)).
		Tooltip("Click to open it in a tab here; drag it into a chat or terminal to point an agent at it").
		Drag(widget.TextDrag{Text: a.sessionPointer(host, s)})
	if row.Hovered() {
		row.Background(theme.Hover)
	}
	row.Children(func() {
		widget.Dot(c, st.color, st.hollow, 7)
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Row(c).FillWidth().Gap(6).Children(func() {
				ui.Text(c, a.sessionName(s)).FontSize(theme.TextUI).SingleLine().Shrink(1)
				ui.Text(c, a.agentName(s.Agent)+" · "+st.label).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine().Shrink(2)
			})
			switch {
			case t != nil && t.Urgent != "" && t.Status == proto.GOATaskWaiting:
				ui.Text(c, "Needs you: "+t.Asks).FontSize(theme.TextLabel).TextColor(theme.Warning).SingleLine()
			case t != nil:
				ui.Text(c, t.Title+" · "+t.Status).FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine()
			}
			a.sessionFacts(c, s)
		})
	})
	if row.Clicked() {
		if cl := a.goaClientOf(host); cl != nil {
			a.openGOATab(host, cl, s)
		} else {
			a.goa.detail = &goaDetail{open: true, host: host, session: s}
		}
	}
}

// sessionPointer is session s on host as text an agent reads: enowX's
// guide says what it points at.
func (a *App) sessionPointer(host string, s proto.SessionInfo) string {
	name := strings.Join(strings.Fields(strings.NewReplacer("[", "(", "]", ")").Replace(a.sessionName(s))), " ")
	return "[session " + s.ID + ": " + name + ", " + a.agentName(s.Agent) + " on " + host + "]"
}

// goaDetail is a session's details, from the Sessions tool.
type goaDetail struct {
	open    bool
	host    string
	session proto.SessionInfo
}

func (a *App) goaDetailDialog(c *ui.Context) {
	d := a.goa.detail
	if d == nil {
		return
	}
	if !d.open {
		a.goa.detail = nil
		return
	}
	s := d.session
	t := a.goaTaskOf(s.ID)
	cl := a.goaClientOf(d.host)
	widget.DetailSheet(c, &d.open, 520, a.sessionName(s), a.agentName(s.Agent)+" on "+d.host+" · "+statusOf(s).label, func() {
		if cl == nil {
			widget.Notice(c, icon.Alert, theme.Warning, d.host+" isn't connected, so this session can't open here now: this list is what it last said. enowX keeps trying, and it opens once the host is back; or go to the host.", "", nil)
		}
		a.sessionFacts(c, s)
		if t == nil {
			ui.Text(c, "GOD of Agents gave this session no task.").FontSize(theme.TextLabel).TextColor(theme.Muted)
			return
		}
		a.goaTaskBody(c, *t)
	}, func() {
		if t != nil && !goaSettled(*t) && widget.GhostButton(c, "Change model").Clicked() {
			d.open = false
			a.openGOAOverride(*t)
		}
		if cl == nil {
			if h, ok := a.watchedHost(d.host); ok && widget.PrimaryButton(c, nil, "Go to "+d.host, "").Clicked() {
				d.open = false
				a.openSession = s.ID
				a.leaveGOA()
				a.connect(h)
			}
		} else {
			if widget.PrimaryButton(c, nil, "Open in a tab", "").Tooltip("Shows it beside GOD of Agents; closing the tab leaves it running").Clicked() {
				d.open = false
				a.openGOATab(d.host, cl, s)
			}
		}
	})
}

// goaOpenTask is a task's way to its session.
func (a *App) goaOpenTask(c *ui.Context, t proto.GOATask) {
	host := nonEmpty(a.goaHostName(t.Host), a.host.Name)
	s, ok := a.sessionOn(host, t.Session)
	if ok && widget.GhostButton(c.Key("open"), "Open session").Clicked() {
		if cl := a.goaClientOf(host); cl != nil {
			a.openGOATab(host, cl, s)
		} else {
			a.goa.detail = &goaDetail{open: true, host: host, session: s}
		}
	}
}

// watchedHost is the watched host named.
func (a *App) watchedHost(name string) (hosts.Host, bool) {
	for _, w := range a.watches {
		if w.host.Name == name {
			return w.host, true
		}
	}
	return hosts.Host{}, false
}

// tasksTab is the Tasks tool: the board, newest first, then what a
// watched GOA did itself.
func (a *App) tasksTab(c *ui.Context) {
	goaScroll(c, 8, func() {
		b := a.goa.board
		ui.Row(c).FillWidth().Gap(8).PaddingX(4).Children(func() {
			widget.SectionLabel(c, "Tasks", theme.Muted)
			if b.Level != "" {
				widget.Tag(c, b.Level, goaLevelColor(b.Level))
			}
			ui.Spacer(c)
			ui.Text(c, plural(len(b.Tasks), "task")).FontSize(theme.TextLabel).TextColor(theme.Idle)
		})
		if len(b.Tasks) == 0 {
			ui.Text(c, "Tasks GOD of Agents hands to sessions show here, with the model each runs on and why.").FontSize(theme.TextLabel).TextColor(theme.Muted).PaddingX(4)
		}
		for i := len(b.Tasks) - 1; i >= 0; i-- {
			t := b.Tasks[i]
			widget.Card(c.Key("goa-t:" + t.ID)).FillWidth().Padding(10).Gap(6).Children(func() {
				a.goaTaskBody(c, t)
				if !goaSettled(t) {
					ui.Row(c).FillWidth().Children(func() {
						ui.Spacer(c)
						if widget.GhostButton(c, "Change model").Clicked() {
							a.openGOAOverride(t)
						}
						a.goaOpenTask(c, t)
					})
				}
			})
		}
		if len(a.goa.flags) > 0 {
			a.goaFlagsList(c)
		}
	})
}

// goaStatusColor is a task's status as a color.
func goaStatusColor(t proto.GOATask) ui.Color {
	switch {
	case t.Urgent != "" && t.Status == proto.GOATaskWaiting, t.Status == proto.GOATaskStalled:
		return theme.Warning
	case t.Status == proto.GOATaskWorking:
		return theme.Working
	case t.Status == proto.GOATaskTurnEnded:
		return theme.Accent
	case t.Status == proto.GOATaskDone:
		return theme.Done
	case t.Status == proto.GOATaskFailed:
		return theme.Error
	}
	return theme.Idle
}

// goaTaskBody is a task: title and status, where it went, its model and
// effort and why, what it asks, its result.
func (a *App) goaTaskBody(c *ui.Context, t proto.GOATask) {
	ui.Row(c).FillWidth().Gap(8).Children(func() {
		ui.Text(c, t.Title).FontSize(theme.TextUI).FontWeight(600).SingleLine().Shrink(1)
		ui.Spacer(c)
		widget.Tag(c, t.Status, goaStatusColor(t))
	})
	where := a.agentName(t.Agent) + " · "
	if t.SessionName != "" {
		where += t.SessionName + " "
	}
	where += shortID(t.Session)
	if t.Host != "" {
		where += " on " + a.goaHostName(t.Host)
	}
	ui.Text(c, where+" · "+textfmt.Age(t.Created, time.Now())).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine()
	s, shown := a.sessionOn(a.goaHostName(t.Host), t.Session)
	model := taskRanOn(t, s, shown)
	if t.Config != "" {
		model += " · " + t.Config
	}
	ui.Row(c).FillWidth().Gap(6).Children(func() {
		ui.Icon(c, icon.Brain).FontSize(11).TextColor(theme.Accent)
		ui.Text(c, model).FontSize(theme.TextLabel).Font(theme.MonoFont).TextColor(theme.TextSoft).SingleLine().Shrink(1).Tooltip(strings.Join(t.Ran, " → "))
	})
	projectLine(c, taskProject(t), t.Dir, t.Branch, a.goaHostName(t.Host))
	if t.Why != "" {
		ui.Text(c, t.Why).FontSize(theme.TextLabel).TextColor(theme.Muted)
	}
	switch {
	case t.Urgent != "" && t.Status == proto.GOATaskWaiting:
		ui.Text(c, "Waits on you ("+t.Urgent+"): "+t.Asks+". Answer it in its session.").FontSize(theme.TextLabel).TextColor(theme.Warning)
	case t.Asks != "" && t.Status == proto.GOATaskWaiting:
		ui.Text(c, "Asks: "+t.Asks+" (GOD of Agents answers)").FontSize(theme.TextLabel).TextColor(theme.TextSoft)
	}
	switch {
	case t.Result != "":
		ui.Text(c, textfmt.FirstLine(t.Result, 240)).FontSize(theme.TextLabel).TextColor(theme.Text)
	case t.Said != "":
		ui.Text(c, "Last said: "+textfmt.FirstLine(t.Said, 240)).FontSize(theme.TextLabel).TextColor(theme.TextSoft)
	}
}

// goaHostName is a host as tasks name it (a daemon's ID, or a name) as
// the app names it.
func (a *App) goaHostName(host string) string {
	for _, h := range a.reachable() {
		if h.info.ID == host || h.host.ID == host {
			return h.host.Name
		}
	}
	return host
}

// taskRanOn is what a task really runs on: what its agent said, a session on
// another host read live; else what was asked, marked so; else unknown.
func taskRanOn(t proto.GOATask, s proto.SessionInfo, shown bool) string {
	switch n := len(t.Ran); {
	case n > 0:
		return textfmt.ShortModel(t.Ran[n-1])
	case shown && s.Model != "":
		return textfmt.ShortModel(s.Model) + " · " + nonEmpty(s.Effort, "unknown")
	case t.Model != "":
		return textfmt.ShortModel(t.Model) + " · " + nonEmpty(t.Effort, "unknown") + " (asked)"
	}
	return "unknown"
}

// taskProject is a task's project, or what to say for none: "" with a session
// place known is no project, and with none known it can't be said.
func taskProject(t proto.GOATask) string {
	if t.Project == "" && t.Dir == "" {
		return "unknown"
	}
	return t.Project
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// goaFlagsList is what a watched GOA did itself, files revertable.
func (a *App) goaFlagsList(c *ui.Context) {
	widget.SectionLabel(c, "Done by GOD of Agents itself", theme.Warning)
	for i := len(a.goa.flags) - 1; i >= 0; i-- {
		f := a.goa.flags[i]
		title, detail := "Ran "+f.Command, ""
		if f.File != "" {
			title, detail = strings.ToUpper(f.Change[:1])+f.Change[1:]+" "+f.File, filepath.Base(f.Project)
		}
		widget.Row(c.Key("goa-f:"+f.ID), title, detail, textfmt.Age(f.Time, time.Now()), func() {
			switch {
			case f.Reverted:
				widget.Tag(c, "reverted", theme.Done)
			case f.Revertable && widget.GhostButton(c, "Revert").Clicked():
				a.confirm("Put "+f.File+" back?", "It goes back to how it was when GOD of Agents started, in "+f.Project+". What changed in it since is lost.", "Revert", func() {
					var out proto.GOAFlag
					a.goaCall(proto.MethodGOARevert, proto.GOARevertParams{ID: f.ID}, &out, nil)
				})
			}
		})
	}
}

// goaOverrideForm is the user's model and effort for a task.
type goaOverrideForm struct {
	open          bool
	task          proto.GOATask
	model, effort string
}

func (a *App) openGOAOverride(t proto.GOATask) {
	a.goa.override = &goaOverrideForm{open: true, task: t, model: t.Model, effort: t.Effort}
}

func (a *App) goaOverrideDialog(c *ui.Context) {
	f := a.goa.override
	if f == nil {
		return
	}
	if !f.open {
		a.goa.override = nil
		return
	}
	set := agent.SettingsOf(f.task.Agent)
	save := false
	why := "The judge's pick: " + orOwn(f.task.Model) + " · " + orOwn(f.task.Effort) + ". " + f.task.Why
	widget.FormSheet(c, &f.open, 480, "Model for “"+f.task.Title+"”", why, func() {
		widget.Field(c, "Model", func() {
			widget.Input(c, &f.model, orOwn(f.task.Model)).FillWidth()
			ui.Row(c).FillWidth().Gap(4).Wrap().Children(func() {
				for _, m := range set.Models {
					if m.ID == "default" {
						continue
					}
					if widget.Button(c.Key("m:"+m.ID), widget.Secondary, nil, m.Label, "").Clicked() {
						f.model = m.ID
					}
				}
			})
		})
		if len(set.Efforts) > 0 {
			widget.Field(c, "Effort", func() {
				var labels []string
				current := -1
				for i, e := range set.Efforts {
					labels = append(labels, e.ID)
					if e.ID == f.effort {
						current = i
					}
				}
				if i := widget.Segmented(c.Key("effort"), labels, current, nil); i >= 0 {
					f.effort = labels[i]
				}
			})
		}
	}, func() {
		if widget.PrimaryButton(c, nil, "Switch", "").Tooltip("Switches its session now and keeps it on the board").Clicked() {
			save = true
		}
	})
	if !save {
		return
	}
	f.open = false
	var out proto.GOATask
	a.goaCall(proto.MethodGOAOverride, proto.GOAOverrideParams{ID: f.task.ID, Model: strings.TrimSpace(f.model), Effort: f.effort}, &out, nil)
}

// orOwn says what is asked of a task: "" keeps the agent's own.
func orOwn(s string) string {
	if s == "" {
		return "its own"
	}
	return s
}

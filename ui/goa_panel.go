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

// goaScroll is a tool's body: a column that scrolls, in the panel's narrow
// width.
func goaScroll(c *ui.Context, body func()) {
	ui.Scroll(c).Grow(1).FillWidth().MinHeight(0).Children(func() {
		ui.Column(c).FillWidth().Padding(0, theme.S2, theme.S2, theme.S2).Gap(theme.S2).Children(body)
	})
}

// goaRowFace is how a row of these tools looks: a raised, narrow card.
func goaRowFace(e ui.Element) ui.Element {
	return widget.Raise(e.FillWidth().Shrink(0).MinHeight(52).Padding(theme.S2, theme.S3).Gap(theme.S2).Radius(theme.RadiusCard).Background(theme.Raised))
}

// goaSideStrip is the panel's top with GOA's tools in it: the choice of them.
func (a *App) goaSideStrip(c *ui.Context, front string) {
	ui.Row(c).Height(52).Shrink(0).FillWidth().PaddingX(theme.S2).Gap(theme.S2).Children(func() {
		current := 0
		if front == panelTasks {
			current = 1
		}
		switch widget.Segmented(c.Key("goa-side"), []string{"Sessions", "Tasks"}, current, nil) {
		case 0:
			a.state.Panel.Tab = panelSessions
		case 1:
			a.state.Panel.Tab = panelTasks
		}
	})
}

// foldTool closes the tool in the panel: GOD of Agents' own tools come back
// in its mode, where they are what the panel holds when no tool is asked for;
// otherwise the panel folds.
func (a *App) foldTool() {
	if !a.goaOn() || a.panelTab() == panelSessions || a.panelTab() == panelTasks {
		a.state.Panel.Open = false
		return
	}
	a.state.Panel.Tab = nonEmpty(a.state.Panel.Last["goa"], panelSessions)
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

// sessionsTab is the Sessions tool: narrow rows, what needs the user on top.
// A host's name heads its rows only where it says something the rows don't:
// several hosts, or one with nothing to list.
func (a *App) sessionsTab(c *ui.Context) {
	goaScroll(c, func() {
		every := a.goaEveryHost()
		for _, h := range every {
			list := slices.DeleteFunc(slices.Clone(h.sessions), func(s proto.SessionInfo) bool { return !goaListed(s) })
			slices.SortStableFunc(list, func(x, y proto.SessionInfo) int { return goaRank(x) - goaRank(y) })
			if len(every) > 1 || len(list) == 0 {
				goaHostHead(c.Key("goa-host:"+h.name), h, len(list))
			}
			if len(list) == 0 {
				ui.Text(c, "No sessions.").FontSize(theme.TextUI).TextColor(theme.TextSoft).PaddingX(theme.S3)
			}
			for _, s := range list {
				a.goaSessionRow(c.Key("goa-s:"+s.ID), h.name, s)
			}
		}
	})
}

// goaHostHead names a host above its sessions, its dot hollow while it can't
// be reached.
func goaHostHead(c *ui.Context, h goaHostSessions, n int) {
	ui.Row(c).FillWidth().Shrink(0).Height(24).Gap(theme.S2).PaddingX(theme.S3).Children(func() {
		col := theme.Done
		if h.state != hostOnline {
			col = theme.Idle
		}
		widget.Dot(c, col, h.state != hostOnline, 8)
		ui.Row(c).Grow(1).MinWidth(0).Children(func() { widget.Running(c, h.name, widget.RunStyle{Weight: 500}) })
		ui.Text(c, plural(n, "session")).FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine().Shrink(0)
	})
}

// goaRank orders the Sessions tool: waiting, working, failed, done, the rest.
func goaRank(s proto.SessionInfo) int {
	switch statusOf(s).color {
	case theme.Warning:
		return 0
	case theme.Working:
		return 1
	case theme.Error:
		return 2
	case theme.Done:
		return 3
	}
	return 4
}

// goaSessionRow is a session: its state, name and what it's on. Clicking it
// opens it in a tab; its details are the button beside it.
func (a *App) goaSessionRow(c *ui.Context, host string, s proto.SessionInfo) {
	goaRowFace(ui.Row(c)).Padding(theme.S1, theme.S1, theme.S1, theme.S3).Children(func() {
		a.goaSessionButton(c, host, s)
		if widget.IconButton(c, icon.Info, "Details of "+a.sessionName(s), false).Clicked() {
			a.goa.detail = &goaDetail{open: true, host: host, session: s}
		}
	})
}

// goaSessionLine is a row's second line and its color: what it needs of the
// user, its task, else where it runs and how it is.
func (a *App) goaSessionLine(host string, s proto.SessionInfo) (string, ui.Color) {
	t := a.goaTaskOf(s.ID)
	switch {
	case t != nil && t.Urgent != "" && t.Status == proto.GOATaskWaiting:
		return "Needs you: " + t.Asks, theme.Warning
	case t != nil:
		return t.Title + " · " + t.Status, theme.TextSoft
	}
	return host + " · " + statusOf(s).label, theme.TextSoft
}

func (a *App) goaSessionButton(c *ui.Context, host string, s proto.SessionInfo) {
	st := statusOf(s)
	row := ui.ButtonBase(c).Grow(1).MinWidth(0).Gap(theme.S2).Justify(ui.Start).Cursor(ui.CursorPointer).Label(a.sessionName(s)).
		Tooltip("Click to open it in a tab here; drag it into a chat or terminal to point an agent at it").
		Drag(widget.TextDrag{Text: a.sessionPointer(host, s)})
	line, col := a.goaSessionLine(host, s)
	row.Children(func() {
		widget.Dot(c, st.color, st.hollow, 8)
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			widget.Running(c, a.sessionName(s), widget.RunStyle{})
			widget.Running(c.Key("line"), line, widget.RunStyle{Size: theme.TextLabel, Color: col})
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
	about := "A session GOA handed a task"
	if t == nil {
		about = "GOD of Agents gave this session no task."
	}
	widget.DetailSheet(c, &d.open, 520, a.sessionName(s), about, func() {
		if cl == nil {
			widget.Notice(c, icon.Alert, theme.Warning, d.host+" isn't connected, so this session can't open here now: this list is what it last said. enowX keeps trying, and it opens once the host is back; or go to the host.", "", nil)
		}
		widget.KV(c, "Host", d.host, false)
		widget.KV(c, "Agent", a.agentName(s.Agent), false)
		if t == nil {
			widget.KV(c, "State", statusOf(s).label, false)
			a.sessionFacts(c, s)
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

// watchedHost is the watched host named.
func (a *App) watchedHost(name string) (hosts.Host, bool) {
	for _, w := range a.watches {
		if w.host.Name == name {
			return w.host, true
		}
	}
	return hosts.Host{}, false
}

// tasksTab is the Tasks tool: the board, newest first, then what a watched
// GOA did itself.
func (a *App) tasksTab(c *ui.Context) {
	goaScroll(c, func() {
		b := a.goa.board
		if b.Level != "" {
			ui.Row(c).FillWidth().Shrink(0).Height(24).Gap(theme.S2).PaddingX(theme.S3).Children(func() {
				ui.Row(c).Grow(1).MinWidth(0).Children(func() {
					widget.Running(c, plural(len(b.Tasks), "task"), widget.RunStyle{Color: theme.TextSoft})
				})
				widget.Tag(c, b.Level, goaLevelColor(b.Level))
			})
		}
		if len(b.Tasks) == 0 {
			ui.Text(c, "Tasks GOD of Agents hands to sessions show here, with the model each runs on and why.").FontSize(theme.TextUI).TextColor(theme.TextSoft).PaddingX(theme.S3)
		}
		for i := len(b.Tasks) - 1; i >= 0; i-- {
			a.goaTaskRow(c.Key("goa-t:"+b.Tasks[i].ID), b.Tasks[i])
		}
		if len(a.goa.flags) > 0 {
			a.goaFlagsList(c)
		}
	})
}

// goaTaskRow is a task in the panel's width: what it is and what it runs on
// (clicked, the whole of it), its state and the way to change its model.
func (a *App) goaTaskRow(c *ui.Context, t proto.GOATask) {
	host := nonEmpty(a.goaHostName(t.Host), a.host.Name)
	s, shown := a.sessionOn(host, t.Session)
	goaRowFace(ui.Row(c)).Children(func() {
		name := nonEmpty(t.SessionName, shortID(t.Session))
		m := ui.ButtonBase(c.Key("task")).Grow(1).MinWidth(0).Justify(ui.Start).AlignItems(ui.Start).Cursor(ui.CursorPointer).Label(t.Title).Tooltip("The whole of this task")
		m.Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Children(func() {
				widget.Running(c, t.Title, widget.RunStyle{})
				widget.Running(c.Key("on"), name+" · "+taskRanOn(t, s, shown), widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
			})
		})
		if m.Clicked() {
			if !shown {
				s = proto.SessionInfo{ID: t.Session, Name: t.SessionName, Agent: t.Agent}
			}
			a.goa.detail = &goaDetail{open: true, host: host, session: s}
		}
		ui.Column(c).Shrink(0).Gap(theme.S1).AlignItems(ui.End).Children(func() {
			goaTaskState(c, t)
			if !goaSettled(t) && widget.Button(c, widget.Secondary, nil, "Change model", "").Clicked() {
				a.openGOAOverride(t)
			}
		})
	})
}

// goaTaskState is a task's state: its dot and word.
func goaTaskState(c *ui.Context, t proto.GOATask) {
	label := nonEmpty(strings.ToUpper(t.Status[:min(1, len(t.Status))])+t.Status[min(1, len(t.Status)):], "Unknown")
	if t.Urgent != "" && t.Status == proto.GOATaskWaiting {
		label = "Waiting on you"
	}
	widget.Legend(c, goaStatusColor(t), label)
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

// goaTaskBody is the whole of a task: what it is, its state, its model and
// effort and why, where it runs, what it asks, its result.
func (a *App) goaTaskBody(c *ui.Context, t proto.GOATask) {
	widget.KV(c, "Task", t.Title, false)
	widget.KV(c, "State", t.Status+" · "+textfmt.Age(t.Created, time.Now()), false)
	s, shown := a.sessionOn(a.goaHostName(t.Host), t.Session)
	model := taskRanOn(t, s, shown)
	if t.Config != "" {
		model += " · " + t.Config
	}
	widget.KV(c, "Model", model, false)
	if t.Why != "" {
		widget.KV(c, "Why", t.Why, false)
	}
	projectLine(c, taskProject(t), t.Dir, t.Branch, a.goaHostName(t.Host))
	switch {
	case t.Urgent != "" && t.Status == proto.GOATaskWaiting:
		widget.Notice(c, icon.Alert, theme.Warning, "Waits on you ("+t.Urgent+"): "+t.Asks, "Answer it in its session.", nil)
	case t.Asks != "" && t.Status == proto.GOATaskWaiting:
		widget.KV(c, "Asks", t.Asks+" (GOD of Agents answers)", false)
	}
	switch {
	case t.Result != "":
		widget.KV(c, "Result", textfmt.FirstLine(t.Result, 240), false)
	case t.Said != "":
		widget.KV(c, "Last said", textfmt.FirstLine(t.Said, 240), false)
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
	ui.Text(c, "Done by GOD of Agents itself").FontSize(theme.TextUI).TextColor(theme.TextSoft).Padding(theme.S2, theme.S3, 0, theme.S3)
	for i := len(a.goa.flags) - 1; i >= 0; i-- {
		f := a.goa.flags[i]
		title, detail := "Ran "+f.Command, textfmt.Age(f.Time, time.Now())
		if f.File != "" {
			title, detail = f.File, strings.ToUpper(f.Change[:1])+f.Change[1:]+" in "+filepath.Base(f.Project)+" · "+detail
		}
		goaRowFace(ui.Row(c.Key("goa-f:" + f.ID))).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Children(func() {
				widget.Running(c, title, widget.RunStyle{Mono: f.File != ""})
				widget.Running(c.Key("d"), detail, widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
			})
			switch {
			case f.Reverted:
				widget.Tag(c, "reverted", theme.Done)
			case f.Revertable && widget.Button(c, widget.Secondary, nil, "Revert", "").Clicked():
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

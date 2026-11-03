package ui

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// New session's first field picks what to work in: a new session (the
// default), or one the project has, to go on with: a session running or
// hibernated, or a past conversation of its agents, resumed where it ran.
// What the dialog's prompt says goes to it.

// sessionChoice is a session of the project the dialog can go on with.
type sessionChoice struct {
	key     string
	agent   string
	name    string
	where   string // its branch, or "project folder"
	status  string
	when    time.Time
	session *proto.SessionInfo  // one the host runs (or hibernated)
	conv    *proto.Conversation // a past conversation, to resume
}

// sessionChoices are the project's sessions, the latest first: the host's
// sessions, then its agents' past conversations.
func (a *App) sessionChoices() []sessionChoice {
	f := &a.newSession
	var out []sessionChoice
	for _, s := range a.projectSessions(f.project.Path) {
		if s.NotesFor != "" || s.CanvasFor != "" {
			continue
		}
		s := s
		where := "project folder"
		if s.Worktree && s.Branch != "" {
			where = s.Branch
		}
		out = append(out, sessionChoice{key: "s:" + s.ID, agent: s.Agent, name: a.sessionName(s), where: where,
			status: statusOf(s).label, when: s.Created, session: &s})
	}
	slices.SortStableFunc(out, func(x, y sessionChoice) int { return y.when.Compare(x.when) })
	var past []sessionChoice
	for _, cv := range f.convs {
		cv := cv
		where := "project folder"
		if cv.Dir != f.project.Path && cv.Branch != "" {
			where = cv.Branch
		}
		name := cv.Title
		if name == "" {
			name = a.agentName(cv.Agent) + " conversation"
		}
		past = append(past, sessionChoice{key: "c:" + cv.Agent + ":" + cv.ID, agent: cv.Agent, name: name, where: where,
			status: "past conversation", when: cv.Updated, conv: &cv})
	}
	slices.SortStableFunc(past, func(x, y sessionChoice) int { return y.when.Compare(x.when) })
	return append(out, past...)
}

// loadConversations reads the project's past conversations, for the
// dialog's choices.
func (a *App) loadConversations(cl *client.Client, path string) {
	go func() {
		list, err := cl.History(path)
		a.onMain(func() {
			if f := &a.newSession; f.open && f.project.Path == path && err == nil {
				f.convs = list
			}
		})
	}()
}

// sessionField is the choice of session: a button showing it, which opens
// the project's sessions to search and pick from.
func (a *App) sessionField(c *ui.Context) {
	f := &a.newSession
	choices := a.sessionChoices()
	if len(choices) == 0 {
		return
	}
	widget.Field(c, "Session", func() {
		label := "New session"
		if f.cont != nil {
			label = f.cont.name
		}
		b := widget.Trigger(ui.ButtonBase(c.Key("session-pick")), f.contOpen).Label("Session: " + label)
		b.Children(func() {
			ui.Row(c).Grow(1).MinWidth(0).Gap(theme.S2).Children(func() {
				ui.Row(c).MinWidth(0).Shrink(1).Children(func() { widget.Running(c, label, widget.RunStyle{}) })
				sub := textfmt.Plural(len(choices), "1 in this project", strconv.Itoa(len(choices))+" in this project")
				if f.cont != nil {
					sub = f.cont.where + " · " + f.cont.status
				}
				ui.Row(c).MinWidth(0).Shrink(100).Children(func() {
					widget.Running(c.Key("sub"), sub, widget.RunStyle{Color: theme.TextSoft})
				})
			})
			ui.Icon(c, icon.ChevronDown).FontSize(12).TextColor(theme.TextSoft)
		})
		if b.Clicked() {
			f.contOpen = !f.contOpen
		}
		if !f.contOpen {
			return
		}
		ui.PopoverBase(c, b, &f.contOpen, func(panel ui.Element) {
			widget.Popup(panel.AttachTo(b, ui.AnchorTopLeft, ui.AnchorBottomLeft).Margin(theme.S1, 0, 0, 0).Width(592)).Gap(theme.S1)
			widget.SearchField(c, &f.contFilter, "Find a session by name or branch", icon.Search)
			widget.Scroll(c).FillWidth().MaxHeight(280).Children(func() {
				ui.Column(c).FillWidth().Children(func() {
					if a.choiceRow(c, nil) {
						f.cont, f.contOpen = nil, false
					}
					q := strings.ToLower(strings.TrimSpace(f.contFilter))
					for i := range choices {
						ch := choices[i]
						if q != "" && !strings.Contains(strings.ToLower(ch.name+" "+ch.where+" "+ch.agent), q) {
							continue
						}
						if a.choiceRow(c, &ch) {
							f.cont, f.contOpen = &ch, false
						}
					}
				})
			})
		})
	})
}

// choiceRow is a row of the session list, nil the new session's; it
// reports a click.
func (a *App) choiceRow(c *ui.Context, ch *sessionChoice) bool {
	f := &a.newSession
	key, name, svg, sub := "new", "New session", icon.Plus, "Its own, with the fields below"
	if ch != nil {
		key, name, svg = ch.key, ch.name, icon.Agent(ch.agent)
		sub = a.agentName(ch.agent) + " · " + ch.where + " · " + ch.status
		if !ch.when.IsZero() {
			sub += " · " + textfmt.Age(ch.when, time.Now())
		}
	}
	picked := ch == nil && f.cont == nil || ch != nil && f.cont != nil && f.cont.key == ch.key
	row := ui.ButtonBase(c.Key(key)).FillWidth().MinHeight(44).Padding(theme.S1, theme.S3).Gap(theme.S2).Radius(theme.RadiusCard).Justify(ui.Start).Cursor(ui.CursorPointer).Label(name)
	bg := theme.Fade(theme.Selected, 0)
	switch {
	case picked:
		bg = theme.Selected
	case row.Hovered():
		bg = theme.Hover
	}
	row.Background(widget.QuickTint(row, "bg", bg)).Children(func() {
		ui.Icon(c, svg).FontSize(14).TextColor(theme.SoftOn(picked))
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			widget.Running(c, name, widget.RunStyle{Always: picked})
			widget.Running(c.Key("sub"), sub, widget.RunStyle{Size: theme.TextLabel, Color: theme.SoftOn(picked)})
		})
	})
	return widget.Pressable(row).Clicked()
}

// goOn goes on with the session picked: shows the one running, wakes the
// one hibernated, or resumes the past conversation where it ran; the
// dialog's prompt goes to it.
func (a *App) goOn(ch sessionChoice) {
	f := &a.newSession
	cl := a.client
	if cl == nil {
		f.err = "Not connected."
		return
	}
	prompt := strings.TrimSpace(f.prompt)
	if ch.conv != nil {
		p := proto.SessionCreateParams{Project: f.project.Path, Agent: ch.conv.Agent, Resume: ch.conv.ID, BranchMode: proto.BranchNone,
			SkipPermissions: a.skipPermissions, Prompt: prompt, Cols: 120, Rows: 36}
		if ch.conv.Dir != f.project.Path && ch.conv.Branch != "" {
			p.BranchMode, p.Branch = proto.BranchExisting, ch.conv.Branch
		}
		a.createFromForm(cl, p)
		return
	}
	s := *ch.session
	f.busy, f.err = true, ""
	go func() {
		var err error
		if s.Hibernated {
			s, err = cl.WakeSession(s.ID)
		}
		if err == nil && prompt != "" {
			err = cl.Input(s.ID, prompt, true, true)
		}
		a.onMain(func() {
			f.busy = false
			if err != nil {
				f.err = client.Explain(err)
				return
			}
			f.open = false
			a.upsert(s)
			a.selectSession(s)
		})
	}()
}

// continuingCard says where the session picked runs, in place of the new
// session's fields.
func (a *App) continuingCard(c *ui.Context, ch sessionChoice) {
	what := "Opens it"
	switch {
	case ch.conv != nil:
		what = "Resumes it in a new session, in " + ch.where
	case ch.session != nil && ch.session.Hibernated:
		what = "Wakes it up, in " + ch.where
	case ch.session != nil:
		what = "Opens it, in " + ch.where
	}
	widget.Notice(c, icon.Agent(ch.agent), ui.Color{}, a.agentName(ch.agent)+" · "+ch.status, what+"; the message below, if any, is sent to it.", nil)
}

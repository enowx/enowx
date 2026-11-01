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
		label, svg := "New session", icon.Plus
		if f.cont != nil {
			label, svg = f.cont.name, icon.Agent(f.cont.agent)
		}
		b := ui.ButtonBase(c.Key("session-pick")).FillWidth().Height(34).PaddingX(10).Gap(8).Radius(theme.RadiusControl).
			Background(theme.Sunken).Border(1, theme.BorderStrong).Cursor(ui.CursorPointer).Label("Session: " + label)
		b.Children(func() {
			ui.Icon(c, svg).FontSize(14).TextColor(theme.TextSoft)
			ui.Text(c, label).FontSize(theme.TextUI).TextColor(theme.Text).SingleLine().Grow(1).Shrink(1).MinWidth(0)
			if f.cont != nil {
				ui.Text(c, f.cont.where+" · "+f.cont.status).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine()
			} else {
				ui.Text(c, textfmt.Plural(len(choices), "1 in this project", strconv.Itoa(len(choices))+" in this project")).FontSize(theme.TextLabel).TextColor(theme.Muted)
			}
			ui.Icon(c, icon.ChevronDown).FontSize(12).TextColor(theme.Muted)
		})
		if widget.Pressable(b).Clicked() {
			f.contOpen = !f.contOpen
		}
		if !f.contOpen {
			return
		}
		ui.Column(c.Key("session-list")).FillWidth().Gap(4).Padding(6).Radius(theme.RadiusControl).Background(theme.Panel).
			Border(1, theme.Border).Children(func() {
			widget.SearchField(c, &f.contFilter, "Find a session by name or branch", nil)
			ui.Scroll(c).FillWidth().MaxHeight(240).Children(func() {
				ui.Column(c).FillWidth().Gap(2).Children(func() {
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
	key, name, svg, sub := "new", "New session", icon.Plus, "its own, with the fields below"
	if ch != nil {
		key, name, svg = ch.key, ch.name, icon.Agent(ch.agent)
		sub = a.agentName(ch.agent) + " · " + ch.where + " · " + ch.status
		if !ch.when.IsZero() {
			sub += " · " + textfmt.Age(ch.when, time.Now())
		}
	}
	picked := ch == nil && f.cont == nil || ch != nil && f.cont != nil && f.cont.key == ch.key
	row := ui.ButtonBase(c.Key(key)).FillWidth().Padding(6, 8).Gap(8).Radius(theme.RadiusSmall).Cursor(ui.CursorPointer).Label(name)
	switch {
	case picked:
		row.Background(theme.Selected)
	case row.Hovered():
		row.Background(theme.Hover)
	}
	row.Children(func() {
		ui.Icon(c, svg).FontSize(14).TextColor(theme.TextSoft)
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Children(func() {
			ui.Text(c, name).FontSize(theme.TextUI).TextColor(theme.Text).SingleLine()
			ui.Text(c, sub).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine()
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
	widget.Notice(c, icon.Agent(ch.agent), ui.Color{}, a.agentName(ch.agent)+" · "+ch.status, what+"; the prompt below, if any, is sent to it.", nil)
}

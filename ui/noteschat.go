package ui

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/chatview"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/notesview"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// The Notes page's agent: a session of the agent the user picks for a
// note, on notes only (SessionCreateParams.NotesFor), drawn beside the
// page as a chat, or a terminal for agents without one. What it writes
// comes back as proposals the page shows.

// notesAgents are the agents the page offers: those installed.
func (a *App) notesAgents() []proto.AgentInfo {
	var out []proto.AgentInfo
	for _, ag := range a.agents {
		if ag.Command != "" && ag.Installed {
			out = append(out, ag)
		}
	}
	return out
}

// notesAgentOf is the agent note id talks to: the one picked for it last,
// else Claude Code when installed, else the first.
func (a *App) notesAgentOf(id string) string {
	agents := a.notesAgents()
	installed := func(id string) bool {
		return slices.ContainsFunc(agents, func(ag proto.AgentInfo) bool { return ag.ID == id })
	}
	// The note's own pick, the same on every host, else this device's.
	if hn := a.noteList; hn != nil {
		if i := slices.IndexFunc(hn.list, func(n proto.NoteInfo) bool { return n.ID == id }); i >= 0 && installed(hn.list[i].Agent) {
			return hn.list[i].Agent
		}
	}
	if pick := a.state.NotesAgents[id]; installed(pick) {
		return pick
	}
	if slices.ContainsFunc(agents, func(ag proto.AgentInfo) bool { return ag.ID == "claude" }) {
		return "claude"
	}
	if len(agents) > 0 {
		return agents[0].ID
	}
	return ""
}

// notesSession is note id's session of agent, the newest.
func (a *App) notesSession(id, agentID string) (proto.SessionInfo, bool) {
	for i := len(a.sessions) - 1; i >= 0; i-- {
		if s := a.sessions[i]; s.NotesFor == id && s.Agent == agentID && !a.removed[s.ID] {
			return s, true
		}
	}
	return proto.SessionInfo{}, false
}

// notesProposals is the proposals waiting, with their agents' names and
// the notes whose agents made them.
func (a *App) notesProposals() []notesview.Proposal {
	var out []notesview.Proposal
	for _, p := range a.noteList.proposals {
		pr := notesview.Proposal{NoteProposal: p, Agent: "An agent"}
		if s, ok := a.sessionByID(p.Session); ok {
			pr.Agent, pr.AgentID, pr.For = a.agentName(s.Agent), s.Agent, s.NotesFor
		}
		out = append(out, pr)
	}
	return out
}

// proposalsEvent takes the proposals as they changed.
func (a *App) proposalsEvent(data json.RawMessage) func() {
	var list []proto.NoteProposal
	if json.Unmarshal(data, &list) != nil {
		return nil
	}
	return func() {
		if hn := a.noteList; hn != nil && hn.host == a.host.ID {
			hn.proposals = list
		}
	}
}

// notesAssistant is the agent's panel for note n.
func (a *App) notesAssistant(c *ui.Context, n proto.NoteInfo) {
	agentID := a.notesAgentOf(n.ID)
	s, ok := a.notesSession(n.ID, agentID)
	ui.Row(c).FillWidth().Height(52).Shrink(0).Padding(0, theme.S2, 0, theme.S4).Gap(theme.S2).Children(func() {
		ui.Row(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
			widget.Running(c, "Note agent", widget.RunStyle{Size: theme.TextRead, Weight: 600})
		})
		ui.Row(c).Width(148).Shrink(0).Children(func() {
			pick := ui.ButtonBase(c).Label("Agent").Tooltip("The agent this page talks to")
			widget.Trigger(pick, false)
			pick.Children(func() {
				ui.Icon(c, icon.Agent(agentID)).FontSize(14).Shrink(0)
				ui.Row(c).Grow(1).MinWidth(0).Children(func() { widget.Running(c, a.agentName(agentID), widget.RunStyle{}) })
				ui.Icon(c, icon.ChevronDown).FontSize(12).TextColor(theme.TextSoft).Shrink(0)
			})
			a.menu.OpenBelow(pick, "notes-agent", theme.ControlHeight)
			a.menu.Show(c, pick, "notes-agent", func(done func()) {
				for _, ag := range a.notesAgents() {
					if widget.MenuItem(c.Key(ag.ID), icon.Agent(ag.ID), ag.Name, false) {
						if a.state.NotesAgents == nil {
							a.state.NotesAgents = map[string]string{}
						}
						a.state.NotesAgents[n.ID] = ag.ID
						if cl := a.client; cl != nil {
							id, agent := n.ID, ag.ID
							go func() { _, _ = cl.SetNoteAgent(id, agent) }() // older daemons: kept here alone
						}
						done()
					}
				}
			})
		})
		if widget.IconButton(c, icon.Close, "Close the note agent", false).Clicked() {
			a.notes.SetAssistant(false)
		}
	})
	if ok {
		st := statusOf(s)
		ui.Row(c).FillWidth().Height(theme.ControlHeight).Shrink(0).Padding(0, theme.S2, 0, theme.S4).Gap(theme.S2).Children(func() {
			widget.Dot(c, st.color, st.hollow, 8)
			ui.Row(c).Grow(1).MinWidth(0).Children(func() {
				widget.Running(c, st.label, widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
			})
			if widget.IconButton(c, icon.Plus, "New conversation", false).Clicked() {
				a.endNotesSession(s)
			}
		})
	}
	a.notesContextRow(c, n)
	if agentID == "" {
		ui.Text(c, "No agent is installed on "+a.host.Name+".").FontSize(theme.TextUI).TextColor(theme.TextSoft).Padding(theme.S2, theme.S4)
		return
	}
	if !ok {
		a.notesStart(c, n, agentID)
		return
	}
	shown := func() bool { return a.page == pageNotes && a.notes.AssistantOpen() && a.notes.SelectedNote() == n.ID }
	switch p := a.panes[s.ID]; {
	case chatview.Supported(s):
		ui.Column(c).Grow(1).FillWidth().MinHeight(0).Children(func() { a.chatViewWhile(c.Key("chat:"+s.ID), s, shown) })
	case p != nil:
		terminal.View(c.Key("term:"+s.ID), p.term).Grow(1).FillWidth()
	default:
		ui.Text(c, "Connecting…").FontSize(theme.TextUI).TextColor(theme.TextSoft).Padding(theme.S2, theme.S4)
	}
}

// notesSuggestion is an ask the panel offers before a conversation: its
// card's title and line, and what it asks.
type notesSuggestion struct {
	svg          *ui.SVG
	title, about string
	ask          string
}

var notesSuggestions = []notesSuggestion{
	{icon.List, "Tidy up", "Group it, tasks as to-dos", "Tidy this page: group it, make tasks to-dos"},
	{icon.Target, "Make a plan", "Goal, steps, next step", "Turn this into a plan: goal, steps, risks, next step"},
	{icon.Brain, "Brainstorm", "Pitch, MVP, open questions", "Brainstorm this as a new project: pitch, MVP, open questions"},
	{icon.File, "Summarize", "A few lines at the top", "Summarize this page in a few lines at the top"},
}

// notesStart is the panel before a conversation: the chat, empty, its
// composer starting the agent on the first message, with what the agent
// is for and asks to start from above it.
func (a *App) notesStart(c *ui.Context, n proto.NoteInfo, agentID string) {
	title := strings.TrimSpace(n.Title)
	if title == "" {
		title = "Untitled"
	}
	ch := a.chatOf("notes-new:" + n.ID + ":" + agentID)
	ch.View(c, chatview.Options{
		Agent:     agentID,
		AgentName: a.agentName(agentID),
		Fresh:     true,
		Send:      func(text string, _ []string) { a.notesAsk(n, agentID, text) },
		Set: func(setting, id string) {
			switch setting {
			case "model":
				a.notesModel = setPick(a.notesModel, agentID, id)
			case "effort":
				a.notesEffort = setPick(a.notesEffort, agentID, id)
			}
		},
		Empty: func(c *ui.Context) { a.notesIntro(c, n, title, agentID) },
	})
}

func setPick(m map[string]string, key, id string) map[string]string {
	if m == nil {
		m = map[string]string{}
	}
	m[key] = id
	return m
}

// notesIntro is what the agent is for, and asks to start from.
func (a *App) notesIntro(c *ui.Context, n proto.NoteInfo, title, agentID string) {
	tip := "It works on your notes only: no project, no files, no commands. What it writes shows above the page for you to accept or reject."
	if agentID != "claude" {
		tip = "It runs in an empty folder and is asked to keep to your notes (Claude Code is held to them). What it writes shows above the page for you to accept or reject."
	}
	ui.Column(c).FillWidth().Gap(theme.S3).Padding(0, 0, theme.S2, 0).Children(func() {
		ui.Text(c, "Ask about this page. Changes come back as proposals, for you to accept.").FontSize(theme.TextUI).TextColor(theme.TextSoft).Tooltip(tip)
		ui.Column(c).FillWidth().Gap(theme.S2).Children(func() {
			for row := 0; row < len(notesSuggestions); row += 2 {
				ui.Row(c.Key(row)).FillWidth().Gap(theme.S2).AlignItems(ui.Stretch).Children(func() {
					for _, sug := range notesSuggestions[row:min(row+2, len(notesSuggestions))] {
						a.suggestionCard(c.Key(sug.title), n, agentID, sug)
					}
				})
			}
		})
		a.earlierChats(c, n.ID)
	})
}

// suggestionCard is an ask to start from, half the panel wide.
func (a *App) suggestionCard(c *ui.Context, n proto.NoteInfo, agentID string, sug notesSuggestion) {
	card := widget.Raise(ui.ButtonBase(c).Grow(1).Basis(0).MinWidth(0).Padding(theme.S3).Gap(theme.S1).Radius(theme.RadiusCard).
		Background(theme.Raised).Cursor(ui.CursorPointer).Label(sug.ask).Column().AlignItems(ui.Start).Justify(ui.Start))
	if card.Hovered() {
		card.Background(theme.Hover)
	}
	card.Children(func() {
		ui.Icon(c, sug.svg).FontSize(14).TextColor(theme.TextSoft)
		ui.Text(c, sug.title).FontSize(theme.TextUI).FontWeight(500).TextColor(theme.TextStrong)
		ui.Text(c, sug.about).FontSize(theme.TextLabel).TextColor(theme.TextSoft)
	})
	if card.Clicked() {
		a.notesAsk(n, agentID, sug.ask)
	}
}

// notesAsk starts note n's agent on a question about it.
func (a *App) notesAsk(n proto.NoteInfo, agentID, question string) {
	cl := a.client
	if cl == nil {
		return
	}
	// Which page it is goes in its guide (agent.NotesArgs): the chat
	// shows the question alone.
	params := proto.SessionCreateParams{Agent: agentID, NotesFor: n.ID, Prompt: question, SkipUnheard: a.skipPermissions, Cols: 100, Rows: 40,
		Model: a.notesModel[agentID], Effort: a.notesEffort[agentID]}
	go func() {
		info, err := cl.CreateSession(params)
		a.onMain(func() {
			if err != nil {
				a.status = strings.TrimPrefix(err.Error(), proto.MethodSessionCreate+": ")
				return
			}
			a.upsert(info)
			if ch := a.chatOf(info.ID); ch != nil {
				ch.ScrollToEnd()
			}
		})
	}()
}

// endNotesSession ends a note's conversation, so the next question starts
// another.
func (a *App) endNotesSession(s proto.SessionInfo) {
	cl := a.client
	if cl == nil {
		return
	}
	a.remove(s.ID)
	go func() {
		if err := cl.KillSession(s.ID); err != nil {
			a.onMain(func() { a.status = err.Error() })
		}
	}()
}

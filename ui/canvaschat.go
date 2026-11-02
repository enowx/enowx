package ui

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/canvas"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/chatview"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// The Canvas page's agent: a session of the agent picked for a canvas
// (SessionCreateParams.CanvasFor), drawn beside the page as a chat, as the
// Notes page's is. Each message says where the user is (the design shown,
// the element selected); what the agent changes shows live on the canvas,
// and in review mode the user keeps or undoes each message's changes.

// hostCanvasChat is a canvas's conversation with its agent, as last read.
type hostCanvasChat struct {
	canvas  string
	log     proto.CanvasChatLog
	loaded  bool
	history bool // its messages listed, with their versions
	// viewing is the unslop region agent shown in place of the
	// conversation, "" for none (canvaschat_region.go).
	viewing string
}

// canvasChatOf is canvas id's conversation, read anew when another canvas
// opened; the chat event keeps it current.
func (a *App) canvasChatOf(id string) *hostCanvasChat {
	if a.canvasChat == nil || a.canvasChat.canvas != id {
		hc, cl := &hostCanvasChat{canvas: id}, a.client
		a.canvasChat = hc
		if cl != nil {
			go func() {
				log, err := cl.CanvasChatLog(id)
				a.onMain(func() {
					if err == nil {
						hc.log, hc.loaded = log, true
					}
				})
			}()
		}
	}
	return a.canvasChat
}

// canvasChatEvent takes a canvas's conversation as it changed.
func (a *App) canvasChatEvent(data json.RawMessage) func() {
	var log proto.CanvasChatLog
	if json.Unmarshal(data, &log) != nil {
		return nil
	}
	return func() {
		if hc := a.canvasChat; hc != nil && hc.canvas == log.ID {
			hc.log, hc.loaded = log, true
		}
	}
}

// canvasAgentOf is the agent canvas chat talks to: the one picked for it,
// installed here, else the Notes page's default.
func (a *App) canvasAgentOf(log proto.CanvasChatLog) string {
	if slices.ContainsFunc(a.notesAgents(), func(ag proto.AgentInfo) bool { return ag.ID == log.Agent }) {
		return log.Agent
	}
	return a.notesAgentOf("")
}

// canvasSession is canvas id's session of agent, the newest.
func (a *App) canvasSession(id, agentID string) (proto.SessionInfo, bool) {
	for i := len(a.sessions) - 1; i >= 0; i-- {
		// A fanned-out unslop's region agents aren't its conversation.
		if s := a.sessions[i]; s.CanvasFor == id && s.Agent == agentID && !a.removed[s.ID] && !strings.HasPrefix(s.Group, proto.UnslopGroup) {
			return s, true
		}
	}
	return proto.SessionInfo{}, false
}

// canvasAgentPanel is the Canvas page's agent, beside the workspace, as
// wide as the user drags it.
func (a *App) canvasAgentPanel(c *ui.Context) {
	if a.page != pageCanvas || a.client == nil || !a.canvases.AssistantOpen() {
		return
	}
	cv := a.canvases.Canvas()
	if cv == nil {
		return
	}
	width := a.state.CanvasChatWidth
	if width <= 0 {
		width = 320
	}
	// The canvas keeps the most of a narrow window.
	if winW, _ := c.Size(); winW > 0 {
		width = min(width, max(320, winW*0.36))
	}
	edge := ui.Box(c.Key("canvas-agent-edge")).Width(6).FillHeight().Shrink(0).Cursor(ui.CursorResizeEW).Label("Resize the agent's panel")
	if dx, _, ok := edge.Dragged(); ok && dx != 0 {
		a.state.CanvasChatWidth = min(max(width-dx, 320), 760)
	}
	widget.Rim(ui.Column(c.Key("canvas-agent")).Width(width).MinWidth(minPanelWidth).Shrink(1).FillHeight().Radius(theme.RadiusCard).Clip().Background(theme.Popover)).
		Transition(widget.PanelMotion(24)).Children(func() { a.canvasAssistant(c, *cv) })
}

// canvasAssistant is the agent's panel for canvas cv.
func (a *App) canvasAssistant(c *ui.Context, cv proto.Canvas) {
	hc := a.canvasChatOf(cv.ID)
	if rs, ok := a.regionAgent(cv, hc); ok {
		a.canvasRegionView(c, cv, hc, rs)
		return
	}
	agentID := a.canvasAgentOf(hc.log)
	s, ok := a.canvasSession(cv.ID, agentID)
	a.canvasChatHeader(c, cv, hc, agentID, s, ok)
	if agentID == "" {
		ui.Text(c, "No agent is installed on "+a.host.Name+".").FontSize(theme.TextUI).TextColor(theme.TextSoft).Padding(theme.S4)
		return
	}
	if hc.history {
		a.canvasChatHistory(c, cv, hc)
		return
	}
	a.canvasReview(c, cv, hc, s, ok)
	if !ok {
		a.canvasStart(c, cv, agentID)
		return
	}
	shown := func() bool {
		return a.page == pageCanvas && a.canvases.AssistantOpen() && a.canvases.Selected() == cv.ID
	}
	switch p := a.panes[s.ID]; {
	case chatview.Supported(s):
		ui.Column(c).Grow(1).FillWidth().MinHeight(0).Children(func() {
			a.chatViewSending(c.Key("chat:"+s.ID), s, shown, chatTweaks{
				send:     func(text string, files []string) { a.canvasSend(cv.ID, s, text, files) },
				paste:    func(ch *chatview.Chat, text string) bool { return a.canvasPaste(cv.ID, ch, text) },
				showUser: canvas.WithoutContext,
			})
		})
	case p != nil:
		terminal.View(c.Key("term:"+s.ID), p.term).Grow(1).FillWidth()
	default:
		ui.Text(c, "Connecting…").FontSize(theme.TextUI).TextColor(theme.TextSoft).Padding(theme.S4)
	}
}

// canvasChatHeader is the agent picked, the mode, attaching a picture,
// the history and a new conversation.
func (a *App) canvasChatHeader(c *ui.Context, cv proto.Canvas, hc *hostCanvasChat, agentID string, s proto.SessionInfo, ok bool) {
	cl := a.client
	ui.Row(c.Key("agent-head")).FillWidth().Height(52).Shrink(0).Padding(0, theme.S2, 0, theme.S3).Gap(theme.S1).AlignItems(ui.Center).Clip().Children(func() {
		pick := ui.ButtonBase(c.Key("pick")).Height(theme.ControlHeight).Shrink(1).MinWidth(0).PaddingX(theme.S2).Gap(theme.S1).Radius(theme.RadiusFull).Cursor(ui.CursorPointer).Label("Agent").Tooltip("The agent this canvas talks to")
		if pick.Hovered() {
			pick.Background(theme.Hover)
		}
		pick.Children(func() {
			ui.Icon(c, icon.Agent(agentID)).FontSize(14)
			ui.Row(c).Shrink(1).MinWidth(0).Children(func() {
				widget.Running(c, a.agentName(agentID), widget.RunStyle{Weight: 500})
			})
			ui.Icon(c, icon.ChevronDown).FontSize(12).TextColor(theme.TextSoft)
		})
		a.menu.OpenBelow(pick, "canvas-agent", theme.ControlHeight)
		a.menu.Show(c, pick, "canvas-agent", func(done func()) {
			for _, ag := range a.notesAgents() {
				if widget.MenuItem(c.Key(ag.ID), icon.Agent(ag.ID), ag.Name, false) {
					hc.log.Agent = ag.ID
					go func() { _, _ = cl.CanvasChat(proto.CanvasChatParams{ID: cv.ID, Agent: ag.ID}) }()
					done()
				}
			}
		})
		ui.Spacer(c)
		review := hc.log.Mode != proto.CanvasChatDirect
		label := "Review each message's changes: on"
		if !review {
			label = "Changes apply directly: click to review each message's instead"
		}
		if widget.IconButton(c.Key("review"), icon.History, label, review).Clicked() {
			mode := proto.CanvasChatDirect
			if !review {
				mode = proto.CanvasChatReview
			}
			hc.log.Mode = mode
			go func() { _, _ = cl.CanvasChat(proto.CanvasChatParams{ID: cv.ID, Mode: mode}) }()
		}
		if widget.IconButton(c.Key("attach"), icon.Paperclip, "Attach a picture (or paste one)", false).Clicked() {
			a.canvasAttachFile(cv.ID, s, ok)
		}
		if widget.IconButton(c.Key("list"), icon.List, fmt.Sprintf("Messages and their versions (%d)", len(hc.log.Messages)), hc.history).Clicked() {
			hc.history = !hc.history
		}
		if ok {
			st := statusOf(s)
			widget.Dot(c.Key("state"), st.color, st.hollow, 8)
			if widget.IconButton(c.Key("new"), icon.Plus, "New conversation", false).Clicked() {
				a.endNotesSession(s)
			}
		}
		if widget.IconButton(c.Key("close"), icon.Close, "Close the agent", false).Clicked() {
			a.canvases.CloseAssistant()
		}
	})
}

// canvasSuggestions are asks the panel offers before a conversation.
var canvasSuggestions = []notesSuggestion{
	{icon.Layers, "Variations", "Five takes on this design", "Make 5 variations of this design, each a design of its own with a line on what it tries"},
	{icon.Laptop, "Responsive", "Tablet and mobile frames", "Make tablet and mobile designs of this one, laid out for each"},
	{icon.Target, "Critique", "What to fix, by priority", "Critique this design: hierarchy, spacing, contrast, copy; then fix the top three"},
	{icon.Palette, "Restyle", "A dark theme of it", "Make a dark theme variant of this design as a new design"},
}

// canvasStart is the panel before a conversation: the chat, empty, its
// composer starting the agent on the first message.
func (a *App) canvasStart(c *ui.Context, cv proto.Canvas, agentID string) {
	ch := a.chatOf("canvas-new:" + cv.ID + ":" + agentID)
	ch.View(c, chatview.Options{
		Agent:     agentID,
		AgentName: a.agentName(agentID),
		Fresh:     true,
		Send:      func(text string, files []string) { a.canvasAsk(cv, agentID, text, files) },
		Paste:     func(text string) bool { return a.canvasPaste(cv.ID, ch, text) },
		Set: func(setting, id string) {
			switch setting {
			case "model":
				a.notesModel = setPick(a.notesModel, agentID, id)
			case "effort":
				a.notesEffort = setPick(a.notesEffort, agentID, id)
			}
		},
		Empty: func(c *ui.Context) { a.canvasIntro(c, cv, agentID) },
	})
}

func (a *App) canvasIntro(c *ui.Context, cv proto.Canvas, agentID string) {
	ui.Column(c).FillWidth().Gap(theme.S4).Padding(theme.S5, 0, theme.S2, 0).Children(func() {
		ui.Column(c.Key("hello")).FillWidth().Center().Gap(theme.S2).Children(func() {
			ui.Row(c).Size(48, 48).Radius(theme.RadiusCard).Center().Background(theme.AccentSoft).Children(func() {
				ui.Icon(c, icon.Agent(agentID)).FontSize(24).TextColor(theme.Now)
			})
			ui.Text(c, "Design with "+a.agentName(agentID)).FontSize(theme.TextTitle).FontWeight(600).TextColor(theme.TextStrong).TextAlign(ui.Center)
			ui.Text(c, "On “"+cv.Title+"”: it sees the design you show and the element you select").FontSize(theme.TextUI).TextColor(theme.TextSoft).TextAlign(ui.Center)
			tip := "It changes the canvas with its tools only, live; it reads the projects the canvas belongs to, and writes no files and runs no commands."
			ui.Row(c.Key("only")).MaxWidth(280).Height(24).PaddingX(theme.S2).Gap(theme.S1).Radius(theme.RadiusFull).Background(theme.Sunken).AlignItems(ui.Center).Tooltip(tip).Children(func() {
				ui.Icon(c, icon.Lock).FontSize(12).TextColor(theme.TextSoft)
				ui.Row(c).Shrink(1).MinWidth(0).Children(func() {
					widget.Running(c, "Canvas only · you keep or undo each change", widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
				})
			})
		})
		ui.Column(c.Key("asks")).FillWidth().Gap(theme.S2).Children(func() {
			for row := 0; row < len(canvasSuggestions); row += 2 {
				ui.Row(c.Key(row)).FillWidth().Gap(theme.S2).AlignItems(ui.Stretch).Children(func() {
					for _, sug := range canvasSuggestions[row:min(row+2, len(canvasSuggestions))] {
						card := ui.ButtonBase(c.Key(sug.title)).Grow(1).Basis(0).MinWidth(0).Padding(theme.S3).Gap(theme.S1).Radius(theme.RadiusCard).
							Background(theme.Raised).Cursor(ui.CursorPointer).Label(sug.ask).Column().AlignItems(ui.Start)
						if card.Hovered() {
							card.Background(theme.Hover)
						}
						widget.Raise(card).Children(func() {
							ui.Icon(c, sug.svg).FontSize(14).TextColor(theme.Now)
							ui.Text(c, sug.title).FontSize(theme.TextUI).FontWeight(500).TextColor(theme.TextStrong)
							ui.Text(c, sug.about).FontSize(theme.TextLabel).TextColor(theme.TextSoft)
						})
						if card.Clicked() {
							a.canvasAsk(cv, agentID, sug.ask, nil)
						}
					}
				})
			}
		})
	})
}

// canvasContext is the line a message starts with: the canvas, the design
// shown, the element selected.
func (a *App) canvasContext(cv proto.Canvas) string {
	frame, element := a.canvases.Selection()
	return canvas.ChatContext(cv, frame, element)
}

// withFiles is a message with the pictures attached to it, named.
func withFiles(text string, files []string) string {
	if len(files) == 0 {
		return text
	}
	return text + "\n(attached: " + strings.Join(files, " ") + ")"
}

// canvasAsk starts canvas cv's agent on a first message.
func (a *App) canvasAsk(cv proto.Canvas, agentID, text string, files []string) {
	cl := a.client
	if cl == nil {
		return
	}
	params := proto.SessionCreateParams{Agent: agentID, CanvasFor: cv.ID, Prompt: a.canvasContext(cv) + withFiles(text, files), SkipUnheard: a.skipPermissions, Cols: 100, Rows: 40,
		Model: a.notesModel[agentID], Effort: a.notesEffort[agentID]}
	go func() {
		info, err := cl.CreateSession(params)
		if err == nil {
			_, _ = cl.CanvasChat(proto.CanvasChatParams{ID: cv.ID, Text: text, Files: files, Session: info.ID, Agent: agentID})
		}
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

// canvasSend sends a message to canvas id's agent in session s, where the
// user is now said first, and records it with the canvas's version.
func (a *App) canvasSend(id string, s proto.SessionInfo, text string, files []string) {
	cv := a.canvases.Canvas()
	if cv == nil || cv.ID != id {
		a.sendWithFiles(s.ID, text, files)
		return
	}
	// Pasted, so that the context's line break doesn't send it early.
	a.pasteToSession(s.ID, a.canvasContext(*cv)+withFiles(text, files))
	if cl := a.client; cl != nil {
		go func() { _, _ = cl.CanvasChat(proto.CanvasChatParams{ID: id, Text: text, Files: files, Session: s.ID}) }()
	}
}

// canvasTell has canvas cv's agent do text: in its conversation when there
// is one, else starting one.
func (a *App) canvasTell(cv proto.Canvas, text string) {
	agentID := a.canvasAgentOf(a.canvasChatOf(cv.ID).log)
	if s, ok := a.canvasSession(cv.ID, agentID); ok {
		a.canvasSend(cv.ID, s, text, nil)
		return
	}
	a.canvasAsk(cv, agentID, text, nil)
}

// canvasDraft puts text in canvas cv's agent's composer.
func (a *App) canvasDraft(cv proto.Canvas, text string) {
	agentID := a.canvasAgentOf(a.canvasChatOf(cv.ID).log)
	key := "canvas-new:" + cv.ID + ":" + agentID
	if s, ok := a.canvasSession(cv.ID, agentID); ok {
		key = s.ID
	}
	a.chatOf(key).SetDraft(text)
}

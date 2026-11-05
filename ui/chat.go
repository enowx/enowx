package ui

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/chatview"
	"github.com/enowx/enowx/ui/tokenview"
)

// The chats of sessions shown as a chat (see package chatview), and which
// sessions show as one, remembered between launches.

func (a *App) chatOf(id string) *chatview.Chat {
	if a.chats == nil {
		a.chats = map[string]*chatview.Chat{}
	}
	ch := a.chats[id]
	if ch == nil {
		ch = chatview.New()
		a.chats[id] = ch
	}
	return ch
}

func (a *App) toggleChat(s proto.SessionInfo) {
	if a.state.Chats == nil {
		a.state.Chats = map[string]bool{}
	}
	if a.showsChat(s.ID) {
		if chatview.OwnChat(s.Agent) {
			a.state.Chats[s.ID] = false // opening on its chat, it remembers leaving it
		} else {
			delete(a.state.Chats, s.ID)
		}
		return
	}
	a.state.Chats[s.ID] = true
	ch := a.chatOf(s.ID)
	ch.ScrollToEnd()
	ch.FocusComposer()
}

// showsChat says session id shows as a chat: as the user last chose, and an agent whose
// chat is its main face from the start.
func (a *App) showsChat(id string) bool {
	if on, chose := a.state.Chats[id]; chose {
		return on
	}
	s, ok := a.sessionByID(id)
	return ok && chatview.OwnChat(s.Agent)
}

// chatView draws the chat in place of the terminal.
func (a *App) chatView(c *ui.Context, s proto.SessionInfo) {
	a.chatViewWhile(c, s, func() bool { return a.showsChat(s.ID) })
}

// chatViewWhile draws session s's chat, following its conversation while
// shown says it shows.
func (a *App) chatViewWhile(c *ui.Context, s proto.SessionInfo, shown func() bool) {
	a.chatViewSending(c, s, shown, chatTweaks{})
}

// chatOnScreen says a chat last drawn at drawn is still in sight. The session in chat mode
// isn't enough: a session left selected-away stays in that mode, and went on polling every
// 700 ms, which kept all its conversation in the daemon. A chat following asks about every
// 5 s, each asking draws a frame, so one on screen has always been drawn lately.
func chatOnScreen(drawn, now time.Time) bool { return now.Sub(drawn) < 15*time.Second }

// chatTweaks change a chat for a page's agent: where the composer's
// messages go, pastes, and how the user's messages read. Nil ones are a
// session's own.
type chatTweaks struct {
	send     func(text string, files []string)
	paste    func(ch *chatview.Chat, text string) bool
	showUser func(string) string
}

// chatViewSending is chatViewWhile as tw changes it.
func (a *App) chatViewSending(c *ui.Context, s proto.SessionInfo, shown func() bool, tw chatTweaks) {
	send, paste := tw.send, tw.paste
	if send == nil {
		send = func(text string, files []string) { a.sendWithFiles(s.ID, text, files) }
	}
	var deliver func(string, string, []string, func(error))
	if tw.send == nil {
		deliver = func(key, text string, files []string, done func(error)) {
			a.deliverToSession(s, key, text, files, done)
		}
	}
	spent, spentTip := "", ""
	if t, ok := a.sessionTokens(s.ID); ok {
		spent, spentTip = tokenview.Spent(t), tokenview.SpentTip(t)
	}
	ch := a.chatOf(s.ID)
	if a.chatDrawn == nil {
		a.chatDrawn = map[string]time.Time{}
	}
	a.chatDrawn[s.ID] = time.Now()
	if cl := a.client; cl != nil {
		gen := a.gen
		ch.Follow(cl, s.ID, a.onMain, func() bool {
			return gen == a.gen && shown() && a.chats[s.ID] == ch && chatOnScreen(a.chatDrawn[s.ID], time.Now())
		})
	}
	column := float32(0)
	if goa, ok := a.goaSession(); ok && a.goaOn() && goa.ID == s.ID {
		column = 2000 // GOA's chat runs the frame's width, its list taking the side
	}
	opts := chatview.Options{
		ColumnWidth: column,
		Agent:       s.Agent,
		Dir:         s.Dir,
		AgentName:   a.agentName(s.Agent),
		Working:     isWorking(s),
		Send:        send,
		Deliver:     deliver,
		Run:         s.Run,
		Interrupt:   func() { a.interruptSession(s) },
		Set:         func(setting, id string) { a.setAgentSetting(s, setting, id) },
		Paste: func(text string) bool {
			if paste != nil {
				return paste(ch, text)
			}
			return a.chatPaste(s, ch, text)
		},
		OpenLink: func(link string) bool { return a.openLink(s.ID, link) },
		Edit: func(back int, original, text string, files []string, done func(error)) {
			a.editMessage(s.ID, back, original, text, files, done)
		},
		Ended:       s.Exited,
		ShowUser:    tw.showUser,
		FirstPrompt: s.Prompt,
		Spent:       spent,
		SpentTip:    spentTip,
		Waiting:     s.State == proto.StateWaiting,
		Answer:      func(keys string) { a.answerAgent(s, keys) },
		ShowTerminal: func() {
			if a.showsChat(s.ID) {
				a.toggleChat(s)
			}
		},
		AttachFiles: func() { a.pickChatFiles(s, ch) },
		Drop:        func(paths []string) { a.attachToChat(s, ch, paths) },
		OpenChanges: func(file string) {
			a.openCenter(s, centerDiff+a.diffOf(s.ID).PathOf(file))
		},
	}
	if chatview.OwnChat(s.Agent) {
		a.platformChat(&opts, s, ch)
	}
	ch.View(c, opts)
}

// setAgentSetting asks session s's daemon for a setting of its agent: the model and effort
// for that session alone (session.settings), the mode by cycling its mode key.
func (a *App) setAgentSetting(s proto.SessionInfo, setting, id string) {
	cl := a.client
	if cl == nil {
		return
	}
	switch setting {
	case "model", "effort":
		p := proto.SessionSettingsParams{ID: s.ID, Model: id}
		if setting == "effort" {
			p = proto.SessionSettingsParams{ID: s.ID, Effort: id}
		}
		go func() {
			if err := cl.SwitchSettings(p); err != nil {
				a.onMain(func() { a.status = client.Explain(err) })
			}
		}()
	case "mode":
		go func() {
			if err := cl.SwitchMode(s.ID, id); err != nil {
				a.onMain(func() { a.status = err.Error() })
			}
		}()
	}
}

// interruptSession stops what session s's agent is doing, with its own
// key, through the daemon: a pane not attached yet would drop it.
func (a *App) interruptSession(s proto.SessionInfo) {
	key := "\x03"
	if preset, ok := agent.Find(s.Agent); ok {
		key = preset.InterruptKey()
	}
	cl := a.client
	if cl == nil {
		return
	}
	go func() {
		if err := cl.Input(s.ID, key, false, false); err != nil {
			a.onMain(func() { a.status = err.Error() })
		}
	}()
}

// answerAgent presses keys in session s's agent, answering what it asks
// (a tool to allow, a question), through the daemon.
func (a *App) answerAgent(s proto.SessionInfo, keys string) {
	cl := a.client
	if cl == nil {
		return
	}
	go func() {
		if err := cl.Input(s.ID, keys, false, false); err != nil {
			a.onMain(func() { a.status = "Couldn't answer the agent: " + err.Error() })
		}
	}()
}

// typeIntoSession types raw keys into the session.
func (a *App) typeIntoSession(id, keys string) {
	a.withPane(id, func(p *pane) {
		go func() { _, _ = p.stream.Write([]byte(keys)) }()
	}, func() {})
}

// editMessage sends text in place of one of the user's messages: the
// daemon takes the agent back to before it, then the message goes as any
// other does.
func (a *App) editMessage(id string, back int, original, text string, files []string, done func(error)) {
	cl := a.client
	if cl == nil {
		done(errors.New("not connected to the host"))
		return
	}
	go func() {
		err := cl.Rewind(id, back, original)
		a.onMain(func() {
			if err != nil {
				done(fmt.Errorf("couldn't go back to that message: %s", client.Explain(err)))
				return
			}
			a.sendWithFiles(id, text, files)
			done(nil)
		})
	}()
}

// sendWithFiles sends a chat message with the paths of the files attached
// to it, as a paste: agents take the images in a paste as images.
func (a *App) sendWithFiles(id, text string, files []string) {
	if len(files) == 0 {
		a.sendToSession(id, text)
		return
	}
	msg := strings.Join(files, " ")
	if strings.TrimSpace(text) != "" {
		msg = text + " " + msg
	}
	a.pasteToSession(id, msg)
}

// deliverToSession is sendWithFiles saying on the main thread how it went.
func (a *App) deliverToSession(s proto.SessionInfo, key, text string, files []string, done func(error)) {
	cl := a.client
	if cl == nil {
		done(errors.New("not connected to the host"))
		return
	}
	msg := text
	if len(files) > 0 {
		msg = strings.Join(files, " ")
		if strings.TrimSpace(text) != "" {
			msg = text + " " + msg
		}
	}
	go func() {
		result, err := cl.Deliver(proto.DeliveryParams{ID: s.ID, Run: s.Run, Key: key, Text: msg})
		if client.IsUnknownMethod(err) {
			err = errors.New("reconnect to update this host before sending chat messages")
		} else if err != nil {
			err = fmt.Errorf("couldn't reach the session: %s", client.Explain(err))
		} else if result.Interrupted {
			err = chatview.ErrDeliveryInterrupted
		}
		a.onMain(func() { done(err) })
	}()
}

// inputThroughDaemon types msg into session id, then Enter, for a session whose
// terminal isn't attached (yet): the chat doesn't wait for it. done runs on the main
// thread; nil puts a failure in the status line.
func (a *App) inputThroughDaemon(id, msg string, paste bool, done func(error)) {
	if done == nil {
		done = func(err error) {
			if err != nil {
				a.status = err.Error()
			}
		}
	}
	cl := a.client
	if cl == nil {
		done(errors.New("not connected to the host"))
		return
	}
	go func() {
		err := cl.Input(id, msg, paste, true)
		if err != nil {
			err = fmt.Errorf("couldn't reach the session: %s", client.Explain(err))
		}
		a.onMain(func() { done(err) })
	}()
}

// sendToSession types text into the session, then Enter. Text of several
// lines goes as a paste, so its line breaks don't send it early.
func (a *App) sendToSession(id, text string) {
	if strings.Contains(text, "\n") {
		a.pasteToSession(id, text)
		return
	}
	a.withPane(id, func(p *pane) { a.writeThenEnter(p, text) }, func() { a.inputThroughDaemon(id, text, false, nil) })
}

// pasteToSession sends text as a paste (bracketed), then Enter.
func (a *App) pasteToSession(id, text string) {
	a.withPane(id, func(p *pane) { a.writeThenEnter(p, "\x1b[200~"+text+"\x1b[201~") }, func() { a.inputThroughDaemon(id, text, true, nil) })
}

// writeThenEnter writes text to p's session, then Enter, off the main
// thread.
func (a *App) writeThenEnter(p *pane, text string) {
	go func() {
		_, _ = p.stream.Write([]byte(text))
		// Agents that read a burst of input as a paste would take an Enter
		// sent with it as a newline; a short gap keeps it a keypress.
		time.Sleep(50 * time.Millisecond)
		_, _ = p.stream.Write([]byte("\r"))
	}()
}

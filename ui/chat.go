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
	if a.state.Chats[s.ID] {
		delete(a.state.Chats, s.ID)
		return
	}
	a.state.Chats[s.ID] = true
	ch := a.chatOf(s.ID)
	ch.ScrollToEnd()
	ch.FocusComposer()
}

func (a *App) showsChat(id string) bool { return a.state.Chats[id] }

// chatView draws the chat in place of the terminal.
func (a *App) chatView(c *ui.Context, s proto.SessionInfo) {
	a.chatViewWhile(c, s, func() bool { return a.showsChat(s.ID) })
}

// chatViewWhile draws session s's chat, following its conversation while
// shown says it shows.
func (a *App) chatViewWhile(c *ui.Context, s proto.SessionInfo, shown func() bool) {
	a.chatViewSending(c, s, shown, chatTweaks{})
}

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
	spent, spentTip := "", ""
	if t, ok := a.sessionTokens(s.ID); ok {
		spent, spentTip = tokenview.Spent(t), tokenview.SpentTip(t)
	}
	ch := a.chatOf(s.ID)
	if cl := a.client; cl != nil {
		gen := a.gen
		ch.Follow(cl, s.ID, a.onMain, func() bool {
			return gen == a.gen && shown() && a.chats[s.ID] == ch
		})
	}
	ch.View(c, chatview.Options{
		Agent:     s.Agent,
		AgentName: a.agentName(s.Agent),
		Working:   isWorking(s),
		Send:      send,
		Interrupt: func() { a.interruptSession(s) },
		Set:       func(setting, id string) { a.setAgentSetting(s, setting, id) },
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
			a.openPanel(panelChanges)
			a.diffOf(s.ID).OpenFile(file)
		},
	})
}

// setAgentSetting asks session s's agent for a setting, as its preset
// says: a command typed in for the model and effort, the daemon cycling
// the agent's mode key for the mode.
func (a *App) setAgentSetting(s proto.SessionInfo, setting, id string) {
	set := agent.SettingsOf(s.Agent)
	switch setting {
	case "model":
		a.sendToSession(s.ID, fmt.Sprintf(set.ModelCommand, id))
	case "effort":
		a.sendToSession(s.ID, fmt.Sprintf(set.EffortCommand, id))
	case "mode":
		cl := a.client
		if cl == nil {
			return
		}
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
	if p := a.panes[id]; p != nil {
		go func() { _, _ = p.stream.Write([]byte(keys)) }()
	}
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

// sendToSession types text into the session, then Enter. Text of several
// lines goes as a paste, so its line breaks don't send it early.
func (a *App) sendToSession(id, text string) {
	p := a.panes[id]
	if p == nil {
		a.status = "The session isn't attached yet; try again in a moment."
		return
	}
	if strings.Contains(text, "\n") {
		a.pasteToSession(id, text)
		return
	}
	a.writeThenEnter(p, text)
}

// pasteToSession sends text as a paste (bracketed), then Enter.
func (a *App) pasteToSession(id, text string) {
	p := a.panes[id]
	if p == nil {
		a.status = "The session isn't attached yet; try again in a moment."
		return
	}
	a.writeThenEnter(p, "\x1b[200~"+text+"\x1b[201~")
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

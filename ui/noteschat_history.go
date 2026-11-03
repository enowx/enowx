package ui

import (
	"encoding/json"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// A note's conversations with its agent are kept with the note, on every
// host: before a conversation starts here, the panel shows what was said
// lately, here or elsewhere.

// noteChats are notes' conversations as last read, by note, the main
// thread's.
type noteChats struct {
	host    string
	logs    map[string]proto.NoteChatLog
	loading map[string]bool
}

// noteChatLog is note id's conversations, read the first time asked.
func (a *App) noteChatLog(id string) proto.NoteChatLog {
	nc := &a.noteChatsRead
	if nc.host != a.host.ID || nc.logs == nil {
		*nc = noteChats{host: a.host.ID, logs: map[string]proto.NoteChatLog{}, loading: map[string]bool{}}
	}
	if _, ok := nc.logs[id]; !ok && !nc.loading[id] && a.client != nil {
		nc.loading[id] = true
		cl, host := a.client, a.host.ID
		go func() {
			log, err := cl.NoteChatLog(id)
			a.onMain(func() {
				if nc.host != host {
					return
				}
				nc.loading[id] = false
				if err == nil {
					nc.logs[id] = log
				} else {
					nc.logs[id] = proto.NoteChatLog{Note: id} // a daemon of before
				}
			})
		}()
	}
	return nc.logs[id]
}

// noteChatEvent takes a note's conversations as a turn was kept.
func (a *App) noteChatEvent(data json.RawMessage) func() {
	var log proto.NoteChatLog
	if json.Unmarshal(data, &log) != nil || log.Note == "" {
		return nil
	}
	return func() {
		if nc := &a.noteChatsRead; nc.logs != nil && nc.host == a.host.ID {
			nc.logs[log.Note] = log
		}
	}
}

// earlierChats shows the last of what was said with note id's agent.
func (a *App) earlierChats(c *ui.Context, id string) {
	msgs := a.noteChatLog(id).Messages
	if len(msgs) == 0 {
		return
	}
	msgs = msgs[max(0, len(msgs)-6):]
	ui.Column(c).FillWidth().Gap(theme.S2).Children(func() {
		ui.Text(c, "Lately, with this note's agent").FontSize(theme.TextLabel).FontWeight(500).TextColor(theme.TextSoft)
		for i, m := range msgs {
			ui.Column(c.Key(i)).FillWidth().Gap(theme.S1).Padding(theme.S2, theme.S3).Radius(theme.RadiusCard).Background(theme.Sunken).Children(func() {
				who := "You"
				if m.Role == proto.TranscriptAssistant {
					who = a.agentName(m.Agent)
				}
				meta := who + " · " + textfmt.Age(m.Time, time.Now())
				if m.Host != "" {
					meta += " · " + m.Host
				}
				widget.Running(c, meta, widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
				ui.Text(c, m.Text).FontSize(theme.TextUI).TextColor(theme.Text).MaxLines(4)
			})
		}
	})
}

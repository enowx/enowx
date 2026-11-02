package chatview

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// A message sent shows at once, as the last turn of the list, until the agent records
// it: a poll and the agent's own file away, more on a slow connection. Its row is
// keyed as the real one will be, so that one takes its place without a jump.

// outgoingLost is how long a message may go unrecorded with the agent free to take
// it before it counts as not taken.
const outgoingLost = 45 * time.Second

// outgoing is a message on its way. It belongs to the main thread.
type outgoing struct {
	text   string
	files  []string
	at     time.Time // sent, or last seen with the agent busy
	after  int       // an entry from this index on may be it
	failed string    // why it didn't go, "" while on its way
}

// shown is the message as the user's entry will read: its files' paths after its text.
func (m *outgoing) shown() string {
	if len(m.files) == 0 {
		return m.text
	}
	paths := strings.Join(m.files, " ")
	if strings.TrimSpace(m.text) == "" {
		return paths
	}
	return m.text + " " + paths
}

// outgoingSend puts a message in the list now and starts it on its way. A command to
// the agent isn't one: agents don't record it as a message, and nothing would replace
// its bubble.
func (ch *Chat) outgoingSend(o Options, text string, files []string) {
	if strings.HasPrefix(strings.TrimSpace(text), "/") {
		o.Send(text, files)
		return
	}
	m := &outgoing{text: text, files: files, after: ch.base + len(ch.entries)}
	ch.out = append(ch.out, m)
	ch.outgoingGo(o, m)
}

// outgoingGo sends m, or sends it again.
func (ch *Chat) outgoingGo(o Options, m *outgoing) {
	m.failed, m.at = "", time.Now()
	if o.Deliver == nil {
		o.Send(m.text, m.files)
		return
	}
	o.Deliver(m.text, m.files, func(err error) {
		if err != nil {
			m.failed = err.Error()
		}
	})
}

// outgoingBack takes m out of the list and puts it in the composer, to change.
func (ch *Chat) outgoingBack(m *outgoing) {
	ch.outgoingDrop(m)
	if strings.TrimSpace(ch.input) != "" {
		ch.input = strings.TrimRight(ch.input, "\n") + "\n"
	}
	ch.input, ch.focus, ch.caretEnd = ch.input+m.text, true, true
	for _, path := range m.files {
		ch.nextAttach++
		ch.attachments = append(ch.attachments, &attachment{id: ch.nextAttach, name: filepath.Base(path), path: path})
	}
}

func (ch *Chat) outgoingDrop(m *outgoing) {
	for i, x := range ch.out {
		if x == m {
			ch.out = append(ch.out[:i:i], ch.out[i+1:]...)
			return
		}
	}
}

// outgoingSettle takes out the messages the agent has recorded, oldest first.
func (ch *Chat) outgoingSettle() {
	if len(ch.out) == 0 {
		return
	}
	for i := range ch.entries {
		e := &ch.entries[i]
		if e.Kind != proto.TranscriptUser {
			continue
		}
		text := e.Text
		if ch.showUser != nil {
			text = ch.showUser(text)
		}
		for j, m := range ch.out {
			if e.Index < m.after || !sameMessage(text, m.text) {
				continue
			}
			ch.out = append(ch.out[:j:j], ch.out[j+1:]...)
			// That entry is taken: the same words sent again need one of their own.
			for _, later := range ch.out {
				later.after = max(later.after, e.Index+1)
			}
			break
		}
	}
}

// sameMessage says an entry reading got is message want: agents add a picture's
// reference or a reminder of their own, and may keep a long paste apart.
func sameMessage(got, want string) bool {
	want = strings.Join(strings.Fields(want), " ")
	if want == "" {
		return true // files alone
	}
	got = strings.Join(strings.Fields(got), " ")
	if r := []rune(want); len(r) > 200 {
		want = string(r[:200])
	}
	return strings.Contains(got, want)
}

// outgoingLook marks what the agent hasn't taken although free to. While it is busy
// or asks, a message only waits its turn.
func (ch *Chat) outgoingLook(o Options, now time.Time) {
	for _, m := range ch.out {
		switch {
		case m.failed != "":
		case o.Working || o.Waiting:
			m.at = now
		case now.Sub(m.at) > outgoingLost:
			m.failed = "The agent hasn't taken this message."
		}
	}
}

// outgoingFlying says a message is on its way.
func (ch *Chat) outgoingFlying() bool {
	for _, m := range ch.out {
		if m.failed == "" {
			return true
		}
	}
	return false
}

// withOutgoing is turns with the messages on their way after them, and the first
// prompt where the agent has recorded nothing yet.
func (ch *Chat) withOutgoing(turns []turn, firstPrompt string) []turn {
	if len(ch.out) == 0 {
		return turns
	}
	out := turns[:len(turns):len(turns)]
	if len(turns) == 0 && strings.TrimSpace(firstPrompt) != "" {
		out = append(out, turn{start: -1, prompt: &proto.TranscriptEntry{Kind: proto.TranscriptUser, Index: -1, Text: firstPrompt}})
	}
	next := ch.base + len(ch.entries)
	for i, m := range ch.out {
		e := proto.TranscriptEntry{Kind: proto.TranscriptUser, Index: next + i, Text: m.shown(), Time: m.at}
		out = append(out, turn{start: e.Index, prompt: &e, out: m})
	}
	return out
}

// runningTurn is which of turns is under way, -1 for none: the agent's last when
// it works, a message waiting its turn after it, or else the message just sent.
func runningTurn(turns []turn, working bool) int {
	recorded := 0
	for recorded < len(turns) && turns[recorded].out == nil {
		recorded++
	}
	if working && recorded > 0 {
		return recorded - 1
	}
	for i := recorded; i < len(turns); i++ {
		if turns[i].out.failed == "" {
			return i
		}
	}
	return -1
}

// outgoingNote says under a message what it waits for, or why it didn't go, with
// what can be done about it.
func (ch *Chat) outgoingNote(c *ui.Context, m *outgoing, running bool) {
	switch {
	case m.failed != "":
		ui.Row(c.Key("outgoing")).FillWidth().Justify(ui.End).Gap(theme.S2).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icon.Alert).FontSize(13).TextColor(theme.Del).Shrink(0)
			ui.Row(c).MinWidth(0).Shrink(1).Children(func() {
				widget.Running(c, m.failed, widget.RunStyle{Color: theme.TextSoft})
			})
			if widget.Button(c.Key("retry"), widget.Secondary, nil, "Retry", "").Clicked() {
				ch.outgoingGo(ch.sending, m)
			}
			if widget.Button(c.Key("edit"), widget.Secondary, nil, "Edit", "").Clicked() {
				ch.outgoingBack(m)
			}
		})
	case !running:
		ui.Row(c.Key("outgoing")).FillWidth().Justify(ui.End).Children(func() {
			ui.Text(c, "Waiting for the agent").FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine()
		})
	}
}

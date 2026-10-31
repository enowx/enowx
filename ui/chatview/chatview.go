// Package chatview shows a session as a chat read from the transcript its
// agent records. The terminal keeps running behind the chat: the composer
// types into it, and approvals or menus still show there.
//
// A long conversation stays light: the daemon sends its last turns and
// older ones as the user scrolls up to them; the conversation is a virtual
// list, which builds only the turns in view; and the chat draws again only
// when something changed.
package chatview

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/transcript"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

const (
	// pollEvery is how often a shown chat asks for news.
	pollEvery = 700 * time.Millisecond
	// page is about how many entries the chat asks for at once: the last
	// ones when it opens, older ones as the user scrolls up to them.
	page = 300
	// checkEvery is how often a poller with no news asks the main thread
	// whether the chat is still wanted, which costs a frame.
	checkEvery = 5 * time.Second
)

// columnWidth is how wide the conversation runs, centered: lines a reader
// can follow, however wide the window.
const columnWidth = 760

// Supported reports whether the session's agent could show as a chat:
// its daemon reads the conversations of some agents only.
func Supported(s proto.SessionInfo) bool { return transcript.Supports(s.Agent) }

// Chat is one session's chat. It belongs to the main thread; its poller
// reports through the update function given to Follow.
type Chat struct {
	attachments []*attachment // going with the next message
	nextAttach  int
	viewing     *attachment // shown large, in a modal
	compact     bool        // the composer is narrow: settings as icons

	// entries are the conversation's from index base on: older ones load
	// as the user scrolls up to them.
	entries   []proto.TranscriptEntry
	base      int
	rev       int64
	supported bool
	reason    string
	loaded    bool
	polling   bool
	// What the entries add up to, worked out as they change rather than
	// in every frame.
	turns          []turn
	added, removed int
	latestTool     *proto.TranscriptEntry
	asked          time.Time
	// animateFrom is the first entry that came after the chat opened: only
	// turns from there on animate in.
	animateFrom int

	list        ui.ListState
	olderAsked  bool          // older entries are on the way
	older       chan struct{} // asks the poller for older entries
	scrollToEnd bool
	width       float32 // the conversation's, last laid out

	// The agent's settings as the daemon last read them.
	model, mode, effort string
	// The context the conversation fills, of the model's window.
	contextUsed, contextWindow int64

	input string
	// showUser is Options.ShowUser, as the last frame gave it.
	showUser func(string) string
	// The agent's slash commands, once its daemon has read them, and the
	// slash menu: the command highlighted, and the input it was closed on.
	commands     []proto.AgentCommand
	commandsRead bool
	slashPick    int
	slashClosed  string
	// focus gives the composer the keyboard focus on the next frame, after
	// anything that took it away: a send, a menu, a button.
	focus bool
	open  map[int]bool // turns, thoughts and tool calls opened or closed, by entry index
	// ask is what the agent waits on the user for, as of this frame;
	// answered the asks answered here, by entry, until it goes on.
	ask       asking
	answered  map[int]int
	ownAnswer string // the user's own answer to the agent's question
	// openChanges shows a file's changes in the Git panel (Options).
	openChanges func(file string)
	// The search within the chat: shown, its words (found the words the
	// last match was looked for with), the match shown.
	finding, focusFind bool
	find, found        string
	findAt             int
	// histAt is the message of the user's the composer shows again, from
	// the latest (0), with ↑ and ↓; -1 none.
	histAt int
	links  widget.LinkOpener // Options.OpenLink, as the last frame gave it
	// editable says the user's messages offer to edit them, as of the last
	// frame; editing is the one being edited in the composer.
	editable bool
	editing  *proto.TranscriptEntry
	editBusy bool   // the agent is going back to before it
	editErr  string // why it couldn't
	menu     string // the composer menu open: "model", "mode", "effort" or ""
	// pending are settings asked for and not yet read back, shown meanwhile.
	pending map[string]pendingSet
}

// New returns an empty chat.
func New() *Chat {
	return &Chat{open: map[int]bool{}, answered: map[int]int{}, histAt: -1, pending: map[string]pendingSet{}, older: make(chan struct{}, 1)}
}

// pendingSet is a setting asked for, and when.
type pendingSet struct {
	id string
	at time.Time
}

// settleAfter is how long a setting asked for shows before what the agent
// reports takes over again, should the agent not have taken it.
const settleAfter = 15 * time.Second

// SetDraft puts text in the composer, to finish and send, and focuses it.
func (ch *Chat) SetDraft(text string) { ch.input, ch.focus = text, true }

// ScrollToEnd scrolls to the newest entry on the next frame.
func (ch *Chat) ScrollToEnd() { ch.scrollToEnd = true }

// FocusComposer gives the composer the keyboard focus on the next frame.
func (ch *Chat) FocusComposer() { ch.focus = true }

// Follow polls session id's transcript on cl, unless it already does,
// until wanted (called on the main thread) reports false. update runs a
// function on the main thread; the poller calls it with news, and else
// only every checkEvery to ask wanted, so a quiet chat costs few frames.
func (ch *Chat) Follow(cl *client.Client, id string, update func(func()), wanted func() bool) {
	if ch.polling {
		return
	}
	ch.polling = true
	since, commandsRead := ch.rev, ch.commandsRead
	go func() {
		var last proto.Transcript // what the main thread was last told
		first, checked := true, time.Now()
		for {
			if !commandsRead {
				if list, err := cl.Commands(id); err == nil && !list.Loading {
					commandsRead = true
					update(func() { ch.commands, ch.commandsRead = list.Commands, true })
				} else if err != nil {
					commandsRead = true // an older daemon: no list
				}
			}
			// Tail also bounds the answer when the transcript starts over.
			p := proto.TranscriptParams{ID: id, Since: since, Tail: page}
			t, err := cl.TranscriptPage(p)
			if err == nil {
				since = t.Rev
			}
			if first || err != nil || news(last, t) || time.Since(checked) > checkEvery {
				first, last, checked = false, t, time.Now()
				if !ch.deliver(update, wanted, func() {
					if err != nil {
						ch.reason, ch.loaded = err.Error(), true
						return
					}
					ch.apply(t)
				}) {
					return
				}
			}
			select {
			case <-time.After(pollEvery):
			case <-ch.older:
				ch.fetchOlder(cl, id, update, wanted)
			}
		}
	}()
}

// deliver runs fn on the main thread unless the chat is no longer wanted,
// and reports whether the poller should go on.
func (ch *Chat) deliver(update func(func()), wanted func() bool, fn func()) bool {
	keep := make(chan bool, 1)
	update(func() {
		if !wanted() {
			ch.polling = false
			keep <- false
			return
		}
		keep <- true
		fn()
	})
	return <-keep
}

// fetchOlder asks for the entries before the oldest the chat has, on the
// poller's goroutine.
func (ch *Chat) fetchOlder(cl *client.Client, id string, update func(func()), wanted func() bool) {
	before := make(chan int, 1)
	update(func() { before <- ch.base })
	b := <-before
	if b <= 0 {
		return
	}
	t, err := cl.TranscriptPage(proto.TranscriptParams{ID: id, Before: b, Tail: page})
	ch.deliver(update, wanted, func() {
		ch.olderAsked = false
		if err == nil {
			ch.prepend(t.Entries)
		}
	})
}

// news reports whether t says anything the chat doesn't show yet.
func news(last, t proto.Transcript) bool {
	return len(t.Entries) > 0 || t.Reset || t.Supported != last.Supported || t.Reason != last.Reason ||
		t.Model != last.Model || t.Mode != last.Mode || t.Effort != last.Effort ||
		t.ContextTokens != last.ContextTokens
}

func (ch *Chat) apply(t proto.Transcript) {
	first := !ch.loaded
	ch.loaded, ch.supported, ch.reason = true, t.Supported, t.Reason
	if t.Reset {
		ch.entries, ch.base, ch.open = nil, 0, map[int]bool{}
		first = true
	}
	if len(ch.entries) == 0 && len(t.Entries) > 0 {
		ch.base = t.Entries[0].Index
	}
	grew := false
	for _, e := range t.Entries {
		switch i := e.Index - ch.base; {
		case i < 0:
			// An entry older than those shown: it shows once they load.
		case i < len(ch.entries):
			ch.entries[i] = e
		case i == len(ch.entries):
			ch.entries = append(ch.entries, e)
			grew = true
		}
	}
	ch.rev = t.Rev
	if first {
		ch.animateFrom = ch.base + len(ch.entries)
		ch.scrollToEnd = true
	}
	for key, v := range map[string]string{"model": t.Model, "mode": t.Mode, "effort": t.Effort} {
		if p, ok := ch.pending[key]; ok && (strings.Contains(strings.ToLower(v), strings.ToLower(p.id)) || time.Since(p.at) > settleAfter) {
			delete(ch.pending, key)
		}
	}
	ch.model, ch.mode, ch.effort = t.Model, t.Mode, t.Effort
	if t.ContextTokens > 0 {
		ch.contextUsed, ch.contextWindow = t.ContextTokens, t.ContextWindow
	}
	if grew || len(t.Entries) > 0 || first {
		ch.reckon()
	}
}

// prepend adds older entries in front of those the chat has.
func (ch *Chat) prepend(older []proto.TranscriptEntry) {
	var keep []proto.TranscriptEntry
	for _, e := range older {
		if e.Index < ch.base {
			keep = append(keep, e)
		}
	}
	if len(keep) == 0 || keep[len(keep)-1].Index != ch.base-1 {
		return // not what comes right before: leave the chat as it is
	}
	ch.entries = append(keep, ch.entries...)
	ch.base = keep[0].Index
	ch.reckon()
}

// reckon works out what the entries add up to: the turns, and what the
// chips over the composer show.
func (ch *Chat) reckon() {
	ch.turns = splitTurns(ch.entries)
	ch.added, ch.removed, ch.latestTool, ch.asked = 0, 0, nil, time.Time{}
	for i := range ch.entries {
		e := &ch.entries[i]
		ch.added, ch.removed = ch.added+e.Added, ch.removed+e.Removed
		switch e.Kind {
		case proto.TranscriptTool:
			ch.latestTool = e
		case proto.TranscriptUser:
			ch.asked, ch.latestTool = e.Time, nil
		}
	}
}

// Options are what the chat needs of its session.
type Options struct {
	Agent     string // preset ID
	AgentName string
	Working   bool // the agent is busy
	// Send sends a message, with the paths of the files attached to it.
	Send      func(text string, files []string)
	Interrupt func()
	// Set changes a setting ("model", "mode" or "effort") to the agent's
	// value id.
	Set func(setting, id string)
	// Paste sees a paste into the composer, with the clipboard's text ("" for
	// an image or files), and reports whether it took it: the app attaches
	// an image or files with Attach.
	Paste func(text string) bool
	// OpenLink opens a link of the agent's answers the app knows (a file in
	// the session's folder, its dev server), reporting whether it did; the
	// system opens the rest.
	OpenLink widget.LinkOpener
	// Edit sends text, with files, in place of one of the user's messages
	// sent before (back of them from the end, 1 for the last; original is
	// its text): the agent goes back to before it first. It calls done, on
	// the main thread, once the message went or with why it couldn't go
	// back. Nil, or an agent that can't go back, hides the pencil on the
	// user's messages.
	Edit func(back int, original, text string, files []string, done func(error))
	// Ended says the session's program isn't running: nothing it said can
	// be edited.
	Ended bool
	// Fresh says there is no session yet: the composer's first message
	// starts it (Send), the settings picked before go with it (Set).
	Fresh bool
	// Empty, when set, is what a conversation without turns shows.
	Empty func(c *ui.Context)
	// ShowUser, when set, is how the user's messages read: what the app
	// added to them (where the user is) left out.
	ShowUser func(text string) string
	// Spent is the session's tokens and cost so far, shown beside its
	// context; SpentTip says them in full.
	Spent, SpentTip string
	// FirstPrompt is the message the session started with, shown as the
	// user's until the agent records the conversation.
	FirstPrompt string
	// Waiting: the agent waits on the user, to allow a tool or answer its
	// question. Answer types keys into its terminal; ShowTerminal shows
	// that terminal, to answer there.
	Waiting      bool
	Answer       func(keys string)
	ShowTerminal func()
	// OpenChanges shows file's changes in the session's Git panel.
	OpenChanges func(file string)
	// AttachFiles asks for files to attach to the next message; Drop
	// attaches files dropped on the chat. Nil hides or ignores them.
	AttachFiles func()
	Drop        func(paths []string)
}

// View draws the chat: the conversation, a list of its turns in a
// centered column that follows its end, and the composer under it.
func (ch *Chat) View(c *ui.Context, o Options) {
	ch.links = o.OpenLink
	ch.showUser = o.ShowUser
	ch.openChanges = o.OpenChanges
	ch.editable = o.Edit != nil && agent.CanRewind(o.Agent) && !o.Ended
	ch.ask = asking{waiting: o.Waiting && !o.Ended, answer: o.Answer, terminal: o.ShowTerminal}
	if p, ok := agent.Find(o.Agent); ok {
		ch.ask.keys = p.Approve
	}
	if !ch.ask.waiting {
		clear(ch.answered)
	}
	col := ui.Column(c).Grow(1).FillWidth().MinHeight(0)
	if o.Drop != nil {
		if files := col.DroppedFiles(); len(files) > 0 {
			o.Drop(files)
		}
	}
	col.Children(func() {
		switch {
		case len(ch.turns) == 0 && strings.TrimSpace(o.FirstPrompt) != "" && (!ch.loaded || ch.supported):
			ch.firstPrompt(c, o)
		case len(ch.turns) == 0 && o.Empty != nil && (o.Fresh || ch.loaded && ch.supported):
			ui.Scroll(c).Grow(1).FillWidth().Children(func() {
				ui.Row(c).FillWidth().Justify(ui.Center).Padding(22, 24, 12, 24).Children(func() {
					ui.Column(c).Grow(1).MaxWidth(columnWidth).Children(func() { o.Empty(c) })
				})
			})
		case !ch.loaded && !o.Fresh:
			ch.notice(c, "Reading the conversation…")
		case len(ch.turns) == 0 && !o.Fresh:
			ch.notice(c, ch.reason)
		case len(ch.turns) > 0:
			ch.conversation(c, o)
		default:
			ui.Box(c).Grow(1)
		}
		if ch.supported || !ch.loaded || o.Fresh {
			ch.composer(c, o)
		}
	})
}

// firstPrompt is the message the session started with, the agent at work
// on it, until the agent records the conversation.
func (ch *Chat) firstPrompt(c *ui.Context, o Options) {
	editable := ch.editable
	ch.editable = false // nothing recorded to go back to yet
	ui.Column(c).Grow(1).FillWidth().MinHeight(0).Padding(22, 24, 12, 24).Children(func() {
		ui.Row(c).FillWidth().Justify(ui.Center).Children(func() {
			ui.Column(c).Grow(1).MaxWidth(columnWidth).Children(func() {
				first := proto.TranscriptEntry{Kind: proto.TranscriptUser, Text: o.FirstPrompt}
				ch.turnTimeline(c, turn{prompt: &first}, true)
			})
		})
	})
	ch.editable = editable
}

func (ch *Chat) notice(c *ui.Context, text string) {
	ui.Column(c).Grow(1).FillWidth().Center().Children(func() {
		ui.Text(c, text).TextColor(theme.Muted)
	})
}

// conversation is the virtual list of turns, with a row on top that loads
// older ones as it comes into view.
func (ch *Chat) conversation(c *ui.Context, o Options) {
	more := 0
	if ch.base > 0 {
		more = 1
	}
	turns := ch.turns
	if ch.editing != nil {
		// What the edit replaces goes from view at once: the message, in
		// the composer now, and what came after.
		for len(turns) > 0 && turns[len(turns)-1].start >= ch.editing.Index {
			turns = turns[:len(turns)-1]
		}
	}
	ch.list.Key = func(i int) any {
		if i < more {
			return "older"
		}
		return turns[i-more].start
	}
	ch.list.FollowEnd = true
	ch.list.FollowAnimation = &ui.Smooth
	if ch.scrollToEnd {
		ch.list.FollowAnimation = nil
		ch.list.ScrollToEnd()
		ch.scrollToEnd = false
	}
	ch.toolbar(c, o, turns, more)
	area := ui.Column(c.Key("conversation")).Grow(1).FillWidth().MinHeight(0)
	if w := area.Bounds().W; w > 0 {
		ch.width = w
	}
	area.Children(func() {
		ch.conversationList(c, turns, more, o)
		if ch.width >= columnWidth+2*(outlineWidth+16) {
			ch.outline(c, turns, more)
		}
		ch.toLatest(c, o.Working)
	})
}

// conversationList is the turns' virtual list, centered at the reading
// column's width.
func (ch *Chat) conversationList(c *ui.Context, turns []turn, more int, o Options) {
	ui.List(c, &ch.list, len(turns)+more, func(i int) {
		ui.Row(c).FillWidth().Justify(ui.Center).Children(func() {
			ui.Column(c).Grow(1).MaxWidth(columnWidth).Children(func() {
				if i < more {
					ch.olderRow(c)
					return
				}
				t := turns[i-more]
				ch.turn(c, t, o.Working && i-more == len(turns)-1, t.start >= ch.animateFrom)
			})
		})
	}).Grow(1).FillWidth().MinHeight(0).Padding(22, 24, 12, 24).Gap(16)
}

// olderRow stands for the turns not loaded yet; showing it loads them.
func (ch *Chat) olderRow(c *ui.Context) {
	if !ch.olderAsked {
		ch.olderAsked = true
		select {
		case ch.older <- struct{}{}:
		default:
		}
	}
	ui.Row(c).FillWidth().Justify(ui.Center).Gap(8).Padding(4).Children(func() {
		widget.Spinner(c, 12)
		ui.Text(c, "Loading earlier messages…").FontSize(12).TextColor(theme.Muted)
	})
}

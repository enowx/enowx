package ui

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// Watching and taking over shared sessions (docs/cloud.md, Hosts and
// sessions). On the host, its user sees who watches a session and who has
// control, is asked before a member types in, and takes it back. A
// visitor watches a shared session read-only and asks to take it over.

// hostPresence is the members connected to the connected host, as last
// told, and their asks. It belongs to the main thread.
type hostPresence struct {
	host     string
	visitors []proto.Visitor
	asks     []proto.TakeoverAsk // waiting for the user's answer
}

func (a *App) presenceOf() *hostPresence {
	if a.presence == nil || a.presence.host != a.host.ID {
		a.presence = &hostPresence{host: a.host.ID}
		hp, cl := a.presence, a.client
		if cl != nil {
			go func() {
				if list, err := cl.HostVisitors(); err == nil {
					a.onMain(func() { hp.visitors = list })
				}
			}()
		}
	}
	return a.presence
}

// presenceEvent takes who visits, and members asking to take over.
func (a *App) presenceEvent(ev proto.Event) func() {
	switch ev.Name {
	case proto.EventHostVisitors:
		var list []proto.Visitor
		if json.Unmarshal(ev.Data, &list) != nil {
			return nil
		}
		return func() { a.presenceOf().visitors = list }
	case proto.EventTakeoverAsked:
		var ask proto.TakeoverAsk
		if json.Unmarshal(ev.Data, &ask) != nil {
			return nil
		}
		return func() {
			hp := a.presenceOf()
			hp.asks = append(hp.asks, ask)
			a.notify(visitorName(ask.Visitor) + " asks to take over “" + nonEmpty(ask.Name, ask.Session) + "”")
		}
	}
	return nil
}

func visitorName(v proto.Visitor) string {
	if v.Name != "" {
		return v.Name
	}
	return "@" + v.GitHub
}

func nonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// takeoverAsk asks the host's user, one ask at a time.
func (a *App) takeoverAsk(*ui.Context) *widget.Ask {
	if a.presence == nil || len(a.presence.asks) == 0 {
		return nil
	}
	hp := a.presence
	ask := hp.asks[0]
	who := visitorName(ask.Visitor)
	answer := func(allow bool) func() {
		return func() {
			hp.asks = hp.asks[1:]
			cl := a.client
			go func() {
				if err := cl.AnswerTakeover(ask.Session, ask.Visitor.Account, allow); err != nil {
					a.onMain(func() { a.status = client.Explain(err) })
				}
			}()
		}
	}
	return &widget.Ask{Key: "takeover:" + ask.Session + ":" + ask.Visitor.Account, Icon: icon.Eye, Title: who + " asks to take over",
		About: who + " (" + ask.Visitor.Role + " of " + a.spaceName(ask.Visitor.Team) + ") watches “" + nonEmpty(ask.Name, ask.Session) +
			"” and asks to type into it, as you would. You see what they do, and take it back any time.",
		Buttons: []widget.AskButton{
			{Kind: widget.Secondary, Label: "Not now", Do: answer(false)},
			{Kind: widget.Primary, Label: "Let them type", Do: answer(true)},
		}}
}

// sessionPresence shows, in a session's header, the members watching it
// and the one in control, whom the user takes it back from.
func (a *App) sessionPresence(c *ui.Context, s proto.SessionInfo) {
	if a.client == nil || len(a.spacesOf().list) == 0 {
		return
	}
	for _, v := range a.presenceOf().visitors {
		if !slices.Contains(v.Watching, s.ID) {
			continue
		}
		controls := slices.Contains(v.Controls, s.ID)
		label := visitorName(v) + " watching"
		if controls {
			label = visitorName(v) + " has control"
		}
		ui.Row(c.Key("watch:"+v.Account)).Gap(5).Height(22).Padding(0, 8, 0, 6).Radius(11).Background(theme.Fade(widget.TeamColor(v.Team), 0.16)).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icon.Eye).FontSize(11).TextColor(theme.TextSoft)
			ui.Text(c, label).FontSize(11).TextColor(theme.TextSoft).SingleLine()
		})
		if controls && widget.GhostButton(c, "Take back").Clicked() {
			cl, account := a.client, v.Account
			go func() { _ = cl.EndTakeover(s.ID, account) }()
		}
	}
}

// A visitor's watching of a shared session: its terminal, read-only until
// the host's user lets them take over.
type watchPane struct {
	session  string
	term     *terminal.Terminal
	stream   *client.Stream
	asked    bool
	controls bool
	err      string
}

// watchSession opens a shared session's terminal for the visit v.
func (a *App) watchSession(v *teamVisit, s proto.SessionInfo) {
	a.stopWatching(v)
	w := &watchPane{session: s.ID}
	v.watch = w
	cl := v.cl
	go func() {
		st, err := cl.Attach(s.ID)
		a.onMain(func() {
			if v.watch != w {
				if st != nil {
					st.Close()
				}
				return
			}
			if err != nil {
				w.err = client.Explain(err)
				return
			}
			t, err := terminal.New(terminal.Options{Conn: st, ResizeByConn: true, Theme: theme.Terminal(), DarkTheme: theme.Terminal(),
				Font: a.termFont(), OnZoom: a.zoomTerms, Padding: terminal.Padding{X: termPadX, Y: termPadY}})
			if err != nil {
				st.Close()
				w.err = err.Error()
				return
			}
			st.OnResize = func(cols, rows int) { a.onMain(func() { t.Resize(cols, rows) }) }
			w.term, w.stream = t, st
		})
	}()
}

func (a *App) stopWatching(v *teamVisit) {
	if v == nil || v.watch == nil {
		return
	}
	if v.watch.term != nil {
		v.watch.term.Close()
	}
	if v.watch.stream != nil {
		v.watch.stream.Close()
	}
	v.watch = nil
}

// visitEvent takes what the visited host tells the visitor: answers to
// their asks to take over.
func (a *App) visitEvent(v *teamVisit, ev proto.Event) {
	if ev.Name != proto.EventTakeoverAnswered {
		return
	}
	var ans proto.TakeoverAnswer
	if json.Unmarshal(ev.Data, &ans) != nil {
		return
	}
	a.onMain(func() {
		w := v.watch
		if w == nil || w.session != ans.Session {
			return
		}
		w.asked, w.controls = false, ans.Allow
		switch {
		case ans.Allow:
			a.notify("You have control of the session: what you type reaches it")
		case w.controls:
		default:
			a.notify(v.host.Owner + " keeps control of the session")
		}
	})
}

// watchView is the session a visitor watches, its controls above it.
func (a *App) watchView(c *ui.Context, v *teamVisit) {
	w := v.watch
	ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Padding(8, 14).BorderWidth(0, 0, 1, 0).BorderColor(theme.Border).Children(func() {
		ui.Icon(c, icon.Eye).FontSize(13).TextColor(theme.TextSoft)
		name := w.session
		for _, s := range v.sessions {
			if s.ID == w.session {
				name = nonEmptyName(s)
			}
		}
		ui.Text(c, name).FontSize(12.5).Grow(1).Shrink(1).MinWidth(0).SingleLine()
		own := false
		for _, s := range v.sessions {
			if s.ID == w.session && s.Member != nil && s.Member.Account == v.me.Account {
				own = true
			}
		}
		switch {
		case own:
			widget.Tag(c, "your session", theme.Done)
		case w.controls:
			widget.Tag(c, "you have control", theme.Done)
			if widget.GhostButton(c, "Give back").Clicked() {
				w.controls = false
				cl, id := v.cl, w.session
				go func() { _ = cl.EndTakeover(id, "") }()
			}
		case w.asked:
			widget.Tag(c, "asked "+v.host.Owner, theme.TextSoft)
		case v.me.Role != "viewer":
			if widget.Button(c, widget.Secondary, nil, "Ask to take over", "").Clicked() {
				w.asked = true
				cl, id := v.cl, w.session
				go func() {
					if err := cl.AskTakeover(id); err != nil {
						a.onMain(func() { w.asked, w.err = false, client.Explain(err) })
					}
				}()
			}
		default:
			widget.Tag(c, "watching", theme.TextSoft)
		}
		if widget.IconButton(c, icon.Close, "Stop watching", false).Clicked() {
			a.stopWatching(v)
		}
	})
	if w.err != "" {
		widget.Notice(c, icon.Alert, theme.Error, strings.TrimSpace(w.err), "", nil)
	}
	if w.term == nil {
		ui.Column(c).Grow(1).FillWidth().Center().Children(func() {
			ui.Text(c, "Connecting…").TextColor(theme.Muted)
		})
		return
	}
	terminal.View(c.Key("watch:"+w.session), w.term).Grow(1).FillWidth()
}

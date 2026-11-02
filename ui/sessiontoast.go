package ui

import (
	"slices"
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// What a sound is about: a card in the window's top right, above the team's
// messages, naming the kind, the session and its host, with a short message.
// A click opens the session. Cards stack and each leaves on its own.

const (
	sessionToastLife = 10 * time.Second // unless hovered
	sessionToastMax  = 4                // shown at once, the oldest going first
	toastNotice      = "notice"         // a card's kind: not a session's (toastAction)
)

// sessionToast is a notification's card. It belongs to the main thread.
type sessionToast struct {
	key        int
	kind       string
	host, id   string
	name, text string
	shown      time.Time
	// open is what a click does on a card about no session (toastAction).
	open func()
}

// toastSession shows a card for session id of host; a newer one of the same
// session and kind replaces it. An empty id is about no session.
func (a *App) toastSession(kind, host, id, name, text string) {
	if text == "" {
		text = toastSays(kind)
	}
	a.sessionToasts = slices.DeleteFunc(a.sessionToasts, func(t *sessionToast) bool {
		return id != "" && t.id == id && t.host == host && t.kind == kind
	})
	a.sessionToastKey++
	a.sessionToasts = append(a.sessionToasts, &sessionToast{key: a.sessionToastKey, kind: kind, host: host, id: id, name: name, text: text, shown: time.Now()})
	if over := len(a.sessionToasts) - sessionToastMax; over > 0 {
		a.sessionToasts = slices.Delete(a.sessionToasts, 0, over)
	}
}

// toastAction shows a quiet card about something other than a session, which
// the user may go to: what agents ask to show them comes this way, never as a
// move. A newer one of the same id replaces it.
func (a *App) toastAction(id, name, text string, open func()) {
	a.toastSession(toastNotice, a.host.Name, id, name, text)
	a.sessionToasts[len(a.sessionToasts)-1].open = open
}

// toastChange shows the card for the change soundKind found in session info.
func (a *App) toastChange(kind, host string, info proto.SessionInfo) {
	text := info.Note
	if kind == proto.NotifyError {
		text = "It ended with exit code " + strconv.Itoa(info.ExitCode)
	}
	a.toastSession(kind, host, info.ID, a.sessionName(info), text)
}

// toastAsked shows the card for what an agent or the daemon notified.
func (a *App) toastAsked(p proto.NotifySoundParams, host string) {
	name := p.Session
	if s, ok := a.sessionOn(host, p.Session); ok {
		name = a.sessionName(s)
	}
	a.toastSession(p.Kind, host, p.Session, name, p.Text)
}

// sessionOn is session id of the host named, the one shown or one watched.
func (a *App) sessionOn(host, id string) (proto.SessionInfo, bool) {
	for _, h := range a.goaEveryHost() {
		if h.name != host {
			continue
		}
		if i := slices.IndexFunc(h.sessions, func(s proto.SessionInfo) bool { return s.ID == id }); i >= 0 {
			return h.sessions[i], true
		}
	}
	return proto.SessionInfo{}, false
}

func toastSays(kind string) string {
	switch kind {
	case proto.NotifyInput:
		return "It's waiting for you"
	case proto.NotifyError:
		return "Something failed"
	}
	return "It finished its turn"
}

func toastKind(kind string) (*ui.SVG, ui.Color, string) {
	switch kind {
	case toastNotice:
		return icon.Canvas, theme.Accent, "Look"
	case proto.NotifyInput:
		return icon.Alert, theme.Warning, "Needs you"
	case proto.NotifyError:
		return icon.Alert, theme.Error, "Failed"
	}
	return icon.Check, theme.Done, "Done"
}

// openToast opens a card's session: in a tab in GOD of Agents' mode, on the
// host shown as any session, on a watched host by going there.
func (a *App) openToast(t *sessionToast) {
	a.sessionToasts = slices.DeleteFunc(a.sessionToasts, func(x *sessionToast) bool { return x == t })
	if t.open != nil {
		t.open()
		return
	}
	s, ok := a.sessionOn(t.host, t.id)
	if !ok {
		a.status = "That session is gone from " + t.host
		return
	}
	switch {
	case a.goaOn():
		if cl := a.goaClientOf(t.host); cl != nil {
			a.openGOATab(t.host, cl, s)
		}
	case t.host == a.host.Name:
		a.selectSession(s)
	default:
		for _, w := range a.watches {
			if w.host.Name == t.host {
				a.openSession = s.ID
				a.connect(w.host)
				return
			}
		}
	}
}

// sessionToastsView draws the cards, dropping those whose time is up.
func (a *App) sessionToastsView(c *ui.Context, now time.Time) {
	a.sessionToasts = slices.DeleteFunc(a.sessionToasts, func(t *sessionToast) bool {
		return now.Sub(t.shown) > sessionToastLife
	})
	for _, t := range slices.Clone(a.sessionToasts) {
		a.sessionToastCard(c.Key("session-toast:"+strconv.Itoa(t.key)), t, now)
	}
}

func (a *App) sessionToastCard(c *ui.Context, t *sessionToast, now time.Time) {
	svg, col, label := toastKind(t.kind)
	card := widget.Float(ui.Row(c).FillWidth().Padding(theme.S2, theme.S2, theme.S2, theme.S3).Gap(theme.S2).AlignItems(ui.Start).Radius(theme.RadiusCard).
		Background(theme.Dialog).Border(1, theme.BorderStrong)).Transition(widget.OverlayMotion)
	if card.Hovered() {
		t.shown = now // read: it waits
	} else {
		c.After(sessionToastLife - now.Sub(t.shown) + 50*time.Millisecond)
	}
	card.Children(func() {
		ui.Icon(c, svg).FontSize(15).TextColor(col)
		body := ui.ButtonBase(c.Key("open")).Grow(1).MinWidth(0).Cursor(ui.CursorPointer).Label("Open " + t.name + " on " + t.host)
		body.Children(func() {
			ui.Column(c).FillWidth().Gap(2).Children(func() {
				ui.Row(c).FillWidth().Gap(theme.S1).AlignItems(ui.Center).Children(func() {
					ui.Text(c, label).FontSize(theme.TextUI).FontWeight(600).TextColor(col).SingleLine().Shrink(0)
					ui.Row(c).Grow(1).MinWidth(0).Children(func() { widget.Running(c, t.name, widget.RunStyle{}) })
				})
				if t.host != "" {
					widget.Running(c.Key("host"), t.host, widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
				}
				ui.Text(c, t.text).FontSize(theme.TextUI).TextColor(theme.TextSoft).MaxLines(3)
			})
		})
		if body.Clicked() && t.id != "" {
			a.openToast(t)
		}
		if widget.IconButton(c.Key("close"), icon.Close, "Dismiss", false).Clicked() {
			a.sessionToasts = slices.DeleteFunc(a.sessionToasts, func(x *sessionToast) bool { return x == t })
		}
	})
}

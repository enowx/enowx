package ui

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/cloud/api"
	"github.com/enowx/enowx/ui/theme"
)

// Who of the user's teams is here, and who types in a team's chat: while
// the app runs it tells the cloud the user is here (api.PathTeamPresence),
// often while a team's chat is open, and that they type as its draft
// changes. The cloud keeps it a minute at most, never what is typed.

const (
	hereEvery     = 20 * time.Second // the user's teams, as the app runs
	hereChatEvery = 2 * time.Second  // the team whose chat is open
	typingEvery   = 3 * time.Second  // while the user types
)

// teamsHere is what the cloud last told of the connected host's teams'
// presence. It belongs to the main thread.
type teamsHere struct {
	host   string
	byTeam map[string]api.Presence
	asked  map[string]time.Time
	busy   map[string]bool
	draft  string    // the chat's draft as last seen, to tell typing
	typed  time.Time // when the user's typing was last told
}

func (a *App) hereOf() *teamsHere {
	if a.here == nil || a.here.host != a.host.ID {
		a.here = &teamsHere{host: a.host.ID, byTeam: map[string]api.Presence{}, asked: map[string]time.Time{}, busy: map[string]bool{}}
	}
	return a.here
}

// keepHere tells the cloud the user is here in each of their teams, and
// reads who else is, as often as what shows needs.
func (a *App) keepHere(c *ui.Context) {
	if a.client == nil || !a.cloudOf().st.SignedIn {
		return
	}
	th := a.hereOf()
	chat := ""
	if a.state.Panel.Open && a.state.Panel.Tab == panelChat {
		chat = a.chatTeam()
		hc := a.chatsOf()
		if hc.draft != th.draft {
			th.draft = hc.draft
			if hc.draft != "" && hc.editing == nil && time.Since(th.typed) > typingEvery {
				th.typed = time.Now()
				a.tellHere(chat, true)
			}
		}
	}
	for _, sp := range a.spacesOf().list {
		every := hereEvery
		if sp.Team == chat {
			every = hereChatEvery
		}
		if time.Since(th.asked[sp.Team]) > every {
			a.tellHere(sp.Team, false)
		}
	}
	if chat != "" {
		c.After(hereChatEvery)
	} else {
		c.After(hereEvery)
	}
}

// tellHere tells the cloud the user is here in team, typing or not, and
// keeps what it answers of the others.
func (a *App) tellHere(team string, typing bool) {
	th, cl := a.hereOf(), a.client
	if team == "" || cl == nil || th.busy[team] && !typing {
		return
	}
	th.asked[team], th.busy[team] = time.Now(), true
	go func() {
		var p api.Presence
		err := cl.CloudCall("POST", teamPath(api.PathTeamPresence, team), api.PresenceParams{Typing: typing}, &p)
		a.onMain(func() {
			th.busy[team] = false
			if err == nil {
				th.byTeam[team] = p
			} else if time.Since(th.asked[team]) > time.Minute {
				delete(th.byTeam, team) // an older cloud, or none: no one shown
			}
		})
	}()
}

// stoppedTyping tells the cloud at once that the user's message went.
func (a *App) stoppedTyping(team string) {
	th := a.hereOf()
	th.typed, th.draft = time.Time{}, ""
	th.busy[team] = false
	a.tellHere(team, false)
}

// online says account is here in team now, as the cloud last told.
func (a *App) online(team, account string) bool {
	for _, x := range a.hereOf().byTeam[team].Online {
		if x == account {
			return true
		}
	}
	return false
}

// typingLine names who of the team types in its chat, the user aside;
// its room is kept when no one does, so the chat doesn't jump.
func (a *App) typingLine(c *ui.Context, team string) {
	me := a.cloudOf().st.AccountID
	var names []string
	for _, acc := range a.hereOf().byTeam[team].Typing {
		if acc == me {
			continue
		}
		name := "Someone"
		for _, m := range a.teamMembers(team) {
			if m.Account.ID == acc {
				name = memberTitle(m.Account.Name, m.Account.GitHub)
			}
		}
		names = append(names, name)
	}
	text := ""
	switch len(names) {
	case 0:
	case 1:
		text = names[0] + " is typing…"
	case 2:
		text = names[0] + " and " + names[1] + " are typing…"
	default:
		text = fmt.Sprintf("%s and %d others are typing…", names[0], len(names)-1)
	}
	ui.Row(c).FillWidth().Height(18).Shrink(0).PaddingX(14).AlignItems(ui.Center).Children(func() {
		if text != "" {
			ui.Text(c, text).FontSize(theme.TextLabel).Italic().TextColor(theme.Muted).SingleLine()
		}
	})
}

// onlineDot marks an avatar of size whose member is here now.
func onlineDot(c *ui.Context, size float32) {
	d := max(size*0.34, 7)
	ui.Box(c).Absolute().Right(-1).Bottom(-1).Size(d, d).Radius(d/2).Background(theme.Done).Border(1.5, theme.Panel).
		Tooltip("Online").PassThrough()
}

// boolOrder sorts true after false.
func boolOrder(b bool) int {
	if b {
		return 1
	}
	return 0
}

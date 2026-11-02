package ui

import (
	"testing"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
)

// GOD of Agents mode says so, with the project shown or hidden, and leaving it goes
// back to the usual words.
func TestDiscordSaysGOA(t *testing.T) {
	a := &App{client: &client.Client{}}
	a.host.ID = "local"
	a.goa.host, a.goa.board.Session = "local", "g1"
	a.state.GOA = true
	a.sessions = []proto.SessionInfo{{ID: "g1", GOA: true}, {ID: "s1"}}
	for _, hide := range []bool{false, true} {
		a.state.Discord.HideProject = hide
		act := a.discordActivity()
		if act.Details != goaPresence || act.State != "2 sessions" {
			t.Errorf("hide %v: %q, %q", hide, act.Details, act.State)
		}
	}
	a.state.GOA = false
	if act := a.discordActivity(); act.Details == goaPresence {
		t.Errorf("after leaving: %q", act.Details)
	}
}

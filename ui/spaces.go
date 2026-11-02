package ui

import (
	"encoding/json"
	"slices"
	"time"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/widget"
)

// Spaces (docs/cloud.md, Personal and team): the account's menu at the title
// bar's right (railaccount.go), and a picker over Notes' and Canvas's lists, say whose notes and canvases the Notes and
// Canvas pages show, the user's own or a team's. The connected host keeps each team's apart and lists them all,
// each marked with its team; the pages show the chosen space's, and what
// is made there is that space's. The menu offers them once the host is
// signed in to the cloud as a member of a team.

// hostSpaces is the connected host's teams' spaces, as last listed. It
// belongs to the main thread.
type hostSpaces struct {
	host string
	list []proto.Space
	// client is the connection it was last read from: one read each, however many
	// frames draw it. events counts the changes taken, newer than any read begun
	// before them.
	client *client.Client
	events int
}

// How long a failed read of the spaces waits before the next, doubling.
const (
	spacesRetryFirst = time.Second
	spacesRetryMost  = 30 * time.Second
)

// spacesOf is the host's spaces. Drawing calls it from the first frame, before
// there is a connection, so it reads only for a list it has just made;
// spacesConnected reads for every connection after.
func (a *App) spacesOf() *hostSpaces {
	if a.spaceList == nil || a.spaceList.host != a.host.ID {
		a.spaceList = &hostSpaces{host: a.host.ID}
		a.readSpaces(a.spaceList, a.client)
	}
	return a.spaceList
}

// spacesConnected reads the spaces from the new connection c: the app's start, a
// reconnect, a restarted daemon. The list stays as it is until the read answers.
func (a *App) spacesConnected(c *client.Client) { a.readSpaces(a.spacesOf(), c) }

// readSpaces reads hs from cl, once per connection, until it answers or cl closes.
func (a *App) readSpaces(hs *hostSpaces, cl *client.Client) {
	if cl == nil || hs.client == cl {
		return
	}
	hs.client = cl
	events := hs.events
	go retrySpaces(cl.Spaces, cl.Done(), spacesRetryFirst, spacesRetryMost, func(list []proto.Space) {
		a.onMain(func() {
			if hs.client == cl && hs.events == events {
				hs.list = list
			}
		})
	})
}

// retrySpaces calls read until it answers, giving got its list, waiting first and
// then twice as long each time up to most; closing done ends it.
func retrySpaces(read func() ([]proto.Space, error), done <-chan struct{}, first, most time.Duration, got func([]proto.Space)) {
	for wait := first; ; wait = min(wait*2, most) {
		list, err := read()
		if err == nil {
			got(list)
			return
		}
		select {
		case <-done:
			return
		case <-time.After(wait):
		}
	}
}

// spacesEvent takes the host's spaces as they changed.
func (a *App) spacesEvent(data json.RawMessage) func() {
	var list []proto.Space
	if json.Unmarshal(data, &list) != nil {
		return nil
	}
	return func() {
		if hs := a.spaceList; hs != nil && hs.host == a.host.ID {
			hs.list = list
			hs.events++
		}
	}
}

// space is the team whose space the pages show, "" for the user's own:
// the one picked while the host has it.
func (a *App) space() string {
	team := a.state.Space
	if team == "" || !slices.ContainsFunc(a.spacesOf().list, func(sp proto.Space) bool { return sp.Team == team }) {
		return ""
	}
	return team
}

// spaceName is team's name as the host knows it, "" for the user's own.
func (a *App) spaceName(team string) string {
	if team == "" {
		return ""
	}
	for _, sp := range a.spacesOf().list {
		if sp.Team == team {
			return sp.Name
		}
	}
	return "a team"
}

// spaceChoices are the spaces a page may show: the user's own, then
// their teams'.
func (a *App) spaceChoices() []widget.Space {
	list := a.spacesOf().list
	if len(list) == 0 {
		return nil
	}
	out := []widget.Space{{Name: "Personal"}}
	for _, sp := range list {
		out = append(out, widget.Space{Team: sp.Team, Name: sp.Name})
	}
	return out
}

// showSpace makes the pages show team's space, as opening one of its
// items does.
func (a *App) showSpace(team string) { a.state.Space = team }

// openNote shows note id on the Notes page, in team's space.
func (a *App) openNote(id, team string) {
	a.showSpace(team)
	a.page = pageNotes
	a.notes.Select(a.notesOptions(), id)
}

// notesInSpace are the notes of the space shown.
func (a *App) notesInSpace(list []proto.NoteInfo) []proto.NoteInfo {
	team := a.space()
	out := make([]proto.NoteInfo, 0, len(list))
	for _, n := range list {
		if n.Team == team {
			out = append(out, n)
		}
	}
	return out
}

// groupsInSpace are the groups of the space shown.
func (a *App) groupsInSpace(list []proto.NoteGroup) []proto.NoteGroup {
	team := a.space()
	out := make([]proto.NoteGroup, 0, len(list))
	for _, g := range list {
		if g.Team == team {
			out = append(out, g)
		}
	}
	return out
}

// canvasesInSpace are the canvases of the space shown.
func (a *App) canvasesInSpace(list []proto.CanvasInfo) []proto.CanvasInfo {
	team := a.space()
	out := make([]proto.CanvasInfo, 0, len(list))
	for _, cv := range list {
		if cv.Team == team {
			out = append(out, cv)
		}
	}
	return out
}

// notesSyncState is how the Notes page's space syncs: the user's own with
// their hosts, a team's with the cloud.
func (a *App) notesSyncState() (string, bool) {
	team := a.space()
	if team == "" {
		return a.syncSummary()
	}
	for _, sp := range a.spacesOf().list {
		if sp.Team != team {
			continue
		}
		switch {
		case sp.SyncErr != "":
			return sp.Name + ": " + sp.SyncErr, true
		case sp.Synced.IsZero():
			return "Not synced with " + sp.Name + " yet: click to sync now", false
		}
		return "Synced with " + sp.Name + " " + textfmt.Age(sp.Synced, time.Now()) + ": click to sync now", false
	}
	return "", false
}

// syncSpaceNow syncs team's space with the cloud now; the spaces event
// says how it went.
func (a *App) syncSpaceNow(team string) {
	cl := a.client
	if cl == nil {
		return
	}
	go func() {
		if _, err := cl.SyncSpace(team); err != nil {
			a.onMain(func() { a.status = client.Explain(err) })
		}
	}()
}

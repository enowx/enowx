package server

import (
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/enowx/enowx/cloud/api"
)

// A team's presence (api.PathTeamPresence): the cloud keeps when each
// member's app last said it was here, and until when they type, in memory
// only, so it is gone as the server restarts and never kept as history.

const (
	presenceFor = 75 * time.Second // here since the app last said so
	typingFor   = 6 * time.Second  // typing since the app last said so
)

type presenceEntry struct{ seen, typing time.Time }

// presences is every team's presence; mu guards teams.
type presences struct {
	mu    sync.Mutex
	teams map[string]map[string]presenceEntry // team → account → entry
}

// see records account's presence in team, and answers the team's: those
// gone quiet are forgotten.
func (p *presences) see(team, account string, typing bool, now time.Time) api.Presence {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.teams == nil {
		p.teams = map[string]map[string]presenceEntry{}
	}
	for t, members := range p.teams {
		for a, e := range members {
			if now.Sub(e.seen) > presenceFor {
				delete(members, a)
			}
		}
		if len(members) == 0 {
			delete(p.teams, t)
		}
	}
	members := p.teams[team]
	if members == nil {
		members = map[string]presenceEntry{}
		p.teams[team] = members
	}
	e := presenceEntry{seen: now}
	if typing {
		e.typing = now.Add(typingFor)
	}
	members[account] = e
	out := api.Presence{Online: []string{}, Typing: []string{}}
	for a, e := range members {
		out.Online = append(out.Online, a)
		if e.typing.After(now) {
			out.Typing = append(out.Typing, a)
		}
	}
	slices.Sort(out.Online)
	slices.Sort(out.Typing)
	return out
}

// forget drops team's presence, as the team is deleted.
func (p *presences) forget(team string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.teams, team)
}

// presence takes a member's presence and answers the team's; one not a
// member is told the team isn't there.
func (s *Server) presence(w http.ResponseWriter, r *http.Request) {
	var p api.PresenceParams
	if !s.decode(w, r, &p) {
		return
	}
	team, me := r.PathValue("team"), asker(r).Account.ID
	role, err := s.db.Role(r.Context(), team, me)
	if err != nil {
		s.answer(w, nil, err)
		return
	}
	// A viewer doesn't write in the chat, so isn't shown typing.
	s.reply(w, s.present.see(team, me, p.Typing && role != api.RoleViewer, time.Now()))
}

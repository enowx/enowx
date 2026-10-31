package daemon

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/enowx/enowx/cloud/api"
	cloudclient "github.com/enowx/enowx/cloud/client"
	"github.com/enowx/enowx/proto"
)

// A team the host's account is no longer in (removed, or the team deleted
// by its owner) leaves the host at its next sync: its copy of the space,
// what the host shares with it, and the visitors of it the host let in. A
// team deleted is also noted (spaces-gone.json) until the user saw it, so
// the app says why it left the rail.

// spacesGone guards spaces-gone.json.
var spacesGone sync.Mutex

func (s *Server) spacesGoneFile() string { return filepath.Join(s.dataDir, "spaces-gone.json") }

// teamDeleted says the cloud answered err: the team was deleted.
func teamDeleted(err error) bool {
	var e *cloudclient.Error
	return errors.As(err, &e) && e.Message == api.ErrTeamDeleted
}

// dropSpace takes team sp's space off the host, deleted saying its owner
// deleted the team; the caller holds s.teamSync.run.
func (s *Server) dropSpace(sp proto.Space, deleted bool) {
	os.RemoveAll(filepath.Join(s.teamsDir(), sp.Team))
	s.teams.mu.Lock()
	delete(s.teams.notes, sp.Team)
	delete(s.teams.canvases, sp.Team)
	s.teams.mu.Unlock()
	shared := s.unshareTeam(sp.Team)
	if deleted {
		s.noteSpaceGone(proto.SpaceGone{Team: sp.Team, Name: sp.Name, At: time.Now().UTC().Truncate(time.Second), Shared: shared})
	}
	s.spacesChanged()
	s.teamProjectsChanged()
	s.teamKitChanged()
	s.broadcast(proto.EventTeamHostsChanged, s.teamHosts())
}

// unshareTeam stops sharing the host with team, closing the team listener
// when it was the last, and lets the team's visitors go. It says whether
// anything was shared with it.
func (s *Server) unshareTeam(team string) bool {
	f := s.readShare()
	shared := len(f.Teams[team]) > 0
	if _, ok := f.Teams[team]; ok || f.Sessions[team] {
		delete(f.Teams, team)
		delete(f.Sessions, team)
		data, _ := json.MarshalIndent(f, "", "  ")
		if err := writeTeamFile(s.hostShareFile(), data); err != nil {
			s.logger().Warn("teams: could not stop sharing with a team gone", "team", team, "err", err)
		}
		s.startSharing()
	}
	s.enforceScopes() // the team's visitors: its space is gone
	return shared
}

// dropDeletedHere takes off the host the space of team, which this host's
// user just deleted: no notice, they know.
func (s *Server) dropDeletedHere(team string) {
	s.teamSync.run.Lock()
	defer s.teamSync.run.Unlock()
	if sp, ok := s.spaceByTeam(team); ok {
		s.dropSpace(sp, false)
	}
	s.spaceGoneSeen(team) // a sync that ran first noted it
}

// spacesGoneList is the teams deleted, not seen yet.
func (s *Server) spacesGoneList() []proto.SpaceGone {
	spacesGone.Lock()
	defer spacesGone.Unlock()
	return s.readSpacesGone()
}

func (s *Server) readSpacesGone() []proto.SpaceGone {
	out := []proto.SpaceGone{}
	if data, err := os.ReadFile(s.spacesGoneFile()); err == nil {
		_ = json.Unmarshal(data, &out)
	}
	return out
}

// noteSpaceGone keeps g's notice, and tells the apps.
func (s *Server) noteSpaceGone(g proto.SpaceGone) {
	spacesGone.Lock()
	list := slices.DeleteFunc(s.readSpacesGone(), func(x proto.SpaceGone) bool { return x.Team == g.Team })
	list = append(list, g)
	s.writeSpacesGone(list)
	spacesGone.Unlock()
	s.broadcast(proto.EventSpacesGone, list)
}

// spaceGoneSeen forgets team's notice.
func (s *Server) spaceGoneSeen(team string) {
	spacesGone.Lock()
	old := s.readSpacesGone()
	list := slices.DeleteFunc(slices.Clone(old), func(x proto.SpaceGone) bool { return x.Team == team })
	changed := len(list) != len(old)
	if changed {
		s.writeSpacesGone(list)
	}
	spacesGone.Unlock()
	if changed {
		s.broadcast(proto.EventSpacesGone, list)
	}
}

// writeSpacesGone keeps list; the caller holds spacesGone.
func (s *Server) writeSpacesGone(list []proto.SpaceGone) {
	if len(list) == 0 {
		os.Remove(s.spacesGoneFile())
		return
	}
	data, _ := json.Marshal(list)
	if err := writeTeamFile(s.spacesGoneFile(), data); err != nil {
		s.logger().Warn("teams: could not keep a deleted team's notice", "err", err)
	}
}

// deletedTeamPath is the team a DELETE of path deletes, "" when it isn't
// one (api.PathTeam).
func deletedTeamPath(path string) string {
	team, ok := strings.CutPrefix(path, strings.TrimSuffix(api.PathTeam, "{team}"))
	if !ok || !teamPattern.MatchString(team) {
		return ""
	}
	return team
}

var spacesGoneHandlers = map[string]handler{
	proto.MethodSpacesGone: bare(func(c *conn) (any, error) {
		if c.remote {
			return nil, errors.New("only the host's own connections read its notices")
		}
		return c.srv.spacesGoneList(), nil
	}),
	proto.MethodSpaceGoneSeen: with(func(c *conn, p proto.SpaceRef) (any, error) {
		if c.remote {
			return nil, errors.New("only the host's own connections read its notices")
		}
		c.srv.spaceGoneSeen(p.Team)
		return ok, nil
	}),
}

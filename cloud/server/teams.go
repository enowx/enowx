package server

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/enowx/enowx/cloud/api"
	"github.com/enowx/enowx/cloud/store"
)

// answer replies with v, or the status err calls for: a team not the
// asker's is not found, one their role doesn't allow is forbidden.
func (s *Server) answer(w http.ResponseWriter, v any, err error) {
	switch {
	case errors.Is(err, store.ErrTeamDeleted):
		s.fail(w, http.StatusNotFound, api.ErrTeamDeleted)
	case errors.Is(err, store.ErrNotFound):
		s.fail(w, http.StatusNotFound, "not found")
	case errors.Is(err, store.ErrForbidden):
		s.fail(w, http.StatusForbidden, "your role in the team doesn't allow it")
	case errors.Is(err, store.ErrExpired):
		s.fail(w, http.StatusNotFound, "this invite expired, was used, or never was")
	case err != nil && errors.Unwrap(err) != nil:
		// The database's own error, wrapped: logged, not shown.
		s.log.Error("store", "err", err)
		s.fail(w, http.StatusInternalServerError, "something went wrong on the server")
	case err != nil && strings.HasPrefix(err.Error(), "store: "):
		s.fail(w, http.StatusBadRequest, strings.TrimPrefix(err.Error(), "store: "))
	case err != nil:
		s.log.Error("server", "err", err)
		s.fail(w, http.StatusInternalServerError, "something went wrong on the server")
	case v == nil:
		w.WriteHeader(http.StatusNoContent)
	default:
		s.reply(w, v)
	}
}

func (s *Server) createTeam(w http.ResponseWriter, r *http.Request) {
	var p api.TeamParams
	if !s.decode(w, r, &p) {
		return
	}
	name := ""
	if p.Name != nil {
		name = *p.Name
	}
	t, err := s.db.CreateTeam(r.Context(), asker(r).Account.ID, name)
	s.answer(w, t, err)
}

func (s *Server) teams(w http.ResponseWriter, r *http.Request) {
	s.joinDefault(r.Context(), asker(r).Account.ID) // those signed in before it was set too
	list, err := s.db.Teams(r.Context(), asker(r).Account.ID)
	s.answer(w, list, err)
}

func (s *Server) team(w http.ResponseWriter, r *http.Request) {
	t, err := s.db.Team(r.Context(), r.PathValue("team"), asker(r).Account.ID)
	s.answer(w, t, err)
}

func (s *Server) updateTeam(w http.ResponseWriter, r *http.Request) {
	var p api.TeamParams
	if !s.decode(w, r, &p) {
		return
	}
	s.answer(w, nil, s.db.UpdateTeam(r.Context(), r.PathValue("team"), asker(r).Account.ID, p))
}

func (s *Server) deleteTeam(w http.ResponseWriter, r *http.Request) {
	team := r.PathValue("team")
	blobs, _ := s.db.TeamBlobs(r.Context(), team)
	err := s.db.DeleteTeam(r.Context(), team, asker(r).Account.ID)
	if err == nil {
		s.present.forget(team)
	}
	if err == nil && s.cfg.Blobs != nil {
		// The team's files go with it.
		for _, id := range blobs {
			if err := s.cfg.Blobs.Delete(r.Context(), blobKey(team, id)); err != nil {
				s.log.Warn("blobs: a deleted team's file stays", "team", team, "blob", id, "err", err)
			}
		}
	}
	s.answer(w, nil, err)
}

func (s *Server) setRole(w http.ResponseWriter, r *http.Request) {
	var p api.RoleParams
	if !s.decode(w, r, &p) {
		return
	}
	s.answer(w, nil, s.db.SetRole(r.Context(), r.PathValue("team"), asker(r).Account.ID, r.PathValue("account"), p.Role))
}

func (s *Server) removeMember(w http.ResponseWriter, r *http.Request) {
	s.answer(w, nil, s.db.RemoveMember(r.Context(), r.PathValue("team"), asker(r).Account.ID, r.PathValue("account")))
}

func (s *Server) createInvite(w http.ResponseWriter, r *http.Request) {
	var p api.InviteParams
	if !s.decode(w, r, &p) {
		return
	}
	in, err := s.db.CreateInvite(r.Context(), r.PathValue("team"), asker(r).Account.ID, p)
	if err == nil && s.cfg.URL != "" {
		// The token after #: it stays in the browser, out of logs.
		in.URL = strings.TrimSuffix(s.cfg.URL, "/") + "/invite#" + in.Token
	}
	s.answer(w, in, err)
}

func (s *Server) invites(w http.ResponseWriter, r *http.Request) {
	list, err := s.db.Invites(r.Context(), r.PathValue("team"), asker(r).Account.ID)
	s.answer(w, list, err)
}

func (s *Server) deleteInvite(w http.ResponseWriter, r *http.Request) {
	s.answer(w, nil, s.db.DeleteInvite(r.Context(), r.PathValue("team"), asker(r).Account.ID, r.PathValue("invite")))
}

func (s *Server) join(w http.ResponseWriter, r *http.Request) {
	var p api.JoinParams
	if !s.decode(w, r, &p) {
		return
	}
	// An invite's link as pasted works as well as its token.
	t, err := s.db.AcceptInvite(r.Context(), inviteToken(p.Token), asker(r).Account.ID)
	s.answer(w, t, err)
}

// joinDefault makes accID a member of the default team, when there is one
// and they never were.
func (s *Server) joinDefault(ctx context.Context, accID string) {
	if s.cfg.DefaultTeam == "" {
		return
	}
	r := s.defaultRole(ctx, accID)
	if joined, err := s.db.JoinDefaultTeam(ctx, s.cfg.DefaultTeam, accID, r); err != nil {
		s.log.Warn("default team", "err", err)
	} else if joined {
		s.log.Info("joined the default team", "account", accID, "role", r)
	} else if r == api.RoleMember {
		// A viewer who has since contributed.
		if raised, err := s.db.RaiseDefaultViewer(ctx, s.cfg.DefaultTeam, accID); err != nil {
			s.log.Warn("default team", "err", err)
		} else if raised {
			s.log.Info("a contributor became a member of the default team", "account", accID)
		}
	}
}

// inviteToken is an invite's token from what was pasted: the token, or
// its link.
func inviteToken(s string) string {
	if _, after, ok := strings.Cut(s, "#"); ok {
		s = after
	}
	return strings.TrimSpace(s)
}

func (s *Server) peekInvite(w http.ResponseWriter, r *http.Request) {
	var p api.JoinParams
	if !s.decode(w, r, &p) {
		return
	}
	peek, err := s.db.PeekInvite(r.Context(), inviteToken(p.Token))
	s.answer(w, peek, err)
}

func (s *Server) teamKeys(w http.ResponseWriter, r *http.Request) {
	me := asker(r)
	keys, err := s.db.TeamKeys(r.Context(), r.PathValue("team"), me.Account.ID, me.Device.ID)
	s.answer(w, keys, err)
}

func (s *Server) teamDevices(w http.ResponseWriter, r *http.Request) {
	list, err := s.db.TeamDevices(r.Context(), r.PathValue("team"), asker(r).Account.ID)
	s.answer(w, list, err)
}

func (s *Server) shareKey(w http.ResponseWriter, r *http.Request) {
	epoch, err := strconv.Atoi(r.PathValue("epoch"))
	if err != nil {
		s.fail(w, http.StatusBadRequest, "an epoch is a number")
		return
	}
	var p api.ShareKeyParams
	if !s.decode(w, r, &p) {
		return
	}
	me := asker(r)
	s.answer(w, nil, s.db.ShareTeamKey(r.Context(), r.PathValue("team"), me.Account.ID, me.Device.ID, epoch, p.Keys, p.Check))
}

func (s *Server) dropKey(w http.ResponseWriter, r *http.Request) {
	epoch, err := strconv.Atoi(r.PathValue("epoch"))
	if err != nil {
		s.fail(w, http.StatusBadRequest, "an epoch is a number")
		return
	}
	me := asker(r)
	s.answer(w, nil, s.db.DropTeamKey(r.Context(), r.PathValue("team"), me.Account.ID, me.Device.ID, epoch))
}

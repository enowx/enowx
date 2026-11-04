package server

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/enowx/enowx/cloud/api"
)

// A default team's contributors: the people GitHub lists as contributors
// of the team's repository (Config.DefaultTeamRepo) join it as members,
// not viewers, and a viewer who becomes one is made a member, by their
// GitHub account's ID, not its login (a login can change hands). The list
// is read from GitHub's public API with the server's OAuth app, kept a
// while, and never makes anyone less than they are.

var contributorsFresh = 10 * time.Minute

// contributors is the repository's contributors as last read. Handlers on
// net/http's goroutines share it under mu.
type contributors struct {
	mu   sync.Mutex
	ids  map[int64]bool
	read time.Time
	err  error
}

// isContributor says GitHub user id contributed to the default team's
// repository, as last read; reading it again when stale.
func (s *Server) isContributor(ctx context.Context, id int64) bool {
	if s.cfg.DefaultTeamRepo == "" || id == 0 {
		return false
	}
	c := &s.contribs
	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Since(c.read) > contributorsFresh {
		ids, err := s.readContributors(ctx)
		c.read, c.err = time.Now(), err
		if err == nil {
			c.ids = ids
			s.keepContributors(ctx, "repository", slices.Collect(maps.Keys(ids))...)
		} else {
			s.log.Warn("contributors", "repo", s.cfg.DefaultTeamRepo, "err", err)
		}
	}
	return c.ids[id]
}

// keepContributors records who the Cloud found to have contributed, for the library's
// badge. It reads nothing from GitHub itself; failing leaves them to the next finding.
func (s *Server) keepContributors(ctx context.Context, source string, ids ...int64) {
	if err := s.db.MarkGitHubContributors(ctx, source, ids...); err != nil {
		s.log.Warn("contributors: keep", "err", err)
	}
}

// readContributors lists every contributor of the repository, a page of a
// hundred at a time.
func (s *Server) readContributors(ctx context.Context) (map[int64]bool, error) {
	repo := strings.Trim(s.cfg.DefaultTeamRepo, "/")
	if strings.Count(repo, "/") != 1 {
		return nil, fmt.Errorf("the default team's repository is owner/name, not %q", repo)
	}
	ids := map[int64]bool{}
	for page := 1; page <= 20; page++ {
		req, _ := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/repos/%s/contributors?per_page=100&page=%d", s.githubAPI(), repo, page), nil)
		req.Header.Set("Accept", "application/vnd.github+json")
		if s.cfg.GitHubClientID != "" && s.cfg.GitHubClientSecret != "" {
			// The OAuth app's own limit, not an address's.
			req.SetBasicAuth(s.cfg.GitHubClientID, s.cfg.GitHubClientSecret)
		}
		var list []struct {
			ID int64 `json:"id"`
		}
		if err := githubJSON(req, &list); err != nil {
			return nil, err
		}
		for _, u := range list {
			ids[u.ID] = true
		}
		if len(list) < 100 {
			break
		}
	}
	return ids, nil
}

// defaultRole is the role account accID joins the default team with, or
// is raised to: a member for a contributor, else the configured one.
func (s *Server) defaultRole(ctx context.Context, accID string) string {
	r := s.cfg.DefaultRole
	if r == "" {
		r = api.RoleViewer
	}
	if r == api.RoleViewer {
		if id, err := s.db.GitHubID(ctx, accID); err == nil && s.isContributor(ctx, id) {
			return api.RoleMember
		}
	}
	return r
}

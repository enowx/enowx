package vault

import (
	"slices"
	"time"

	"github.com/enowx/enowx/proto"
)

// AgentKey says what an agent running on a credential of platform has for a session of
// project, from a host's index of the vault (no values): pick is the credential to ask
// for, "" when none or when have (the session's, names or IDs) or the app's "in every
// session" brings one; has says there is one either way. The app still decides whether
// the host may have it.
func AgentKey(list []proto.CredentialInfo, platform, project string, have []string, now time.Time) (pick string, has bool) {
	rank := -1
	for _, c := range list {
		if c.Platform != platform {
			continue
		}
		if c.Auto || slices.Contains(have, c.Name) || slices.Contains(have, c.ID) {
			return "", true
		}
		if c.Revoked || !c.Expires.IsZero() && c.Expires.Before(now) {
			continue
		}
		r := 2
		switch {
		case len(c.Projects) > 0:
			if !slices.ContainsFunc(c.Projects, func(p string) bool { return sameProject(p, project) }) {
				continue // kept to other projects
			}
			r = 0
		case c.Default:
			r = 1
		}
		if rank < 0 || r < rank {
			pick, rank = c.Name, r
		}
	}
	return pick, pick != ""
}

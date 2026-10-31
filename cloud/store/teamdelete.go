package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/enowx/enowx/cloud/api"
)

// A team deleted by its owner goes with all it holds; its members are kept
// as its former ones for a while (deleted_teams), so their hosts, asking
// for it, learn it was deleted rather than that they were removed.

// ErrTeamDeleted is ErrNotFound for a former member of a team deleted.
var ErrTeamDeleted = fmt.Errorf("%w: the team was deleted", ErrNotFound)

// deletedTeamsKept is how long a deleted team's former members are told
// so; a host back after longer drops its copy all the same, unsaid.
const deletedTeamsKept = 90 * 24 * time.Hour

// DeleteTeam deletes team teamID and all it holds (members, invites, keys,
// items, files' records, tickets), for its owner.
func (s *DB) DeleteTeam(ctx context.Context, teamID, accID string) error {
	return s.asRole(ctx, teamID, accID, api.RoleOwner, func(tx *sql.Tx, _ string) error {
		at := now()
		if _, err := tx.ExecContext(ctx, `DELETE FROM deleted_teams WHERE deleted < $1`, stamp(at.Add(-deletedTeamsKept))); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO deleted_teams (team_id, account_id, deleted)
			SELECT team_id, account_id, CAST($2 AS TEXT) FROM members WHERE team_id = $1`, teamID, stamp(at)); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM teams WHERE id = $1`, teamID); err != nil {
			return err
		}
		return audit(ctx, tx, "", accID, "team.deleted", teamID)
	})
}

// wasMember says accID was a member of team teamID when it was deleted.
func wasMember(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, teamID, accID string) bool {
	var n int
	err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM deleted_teams WHERE team_id = $1 AND account_id = $2`, teamID, accID).Scan(&n)
	return err == nil && n > 0
}

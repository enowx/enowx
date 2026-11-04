package store

import (
	"context"
	"fmt"
)

// MarkGitHubContributors records GitHub accounts found to have contributed, keeping
// when each was first found. It's the contributor badge's basis.
func (s *DB) MarkGitHubContributors(ctx context.Context, source string, ids ...int64) error {
	tx, err := s.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	at := stamp(now())
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO github_contributors (github_id, source, since) VALUES ($1, $2, $3) ON CONFLICT (github_id) DO NOTHING`, id, source, at); err != nil {
			return fmt.Errorf("store: contributors: %w", err)
		}
	}
	return tx.Commit()
}

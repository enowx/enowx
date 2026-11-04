package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/enowx/enowx/cloud/api"
)

// A creator's numbers, and picks announced. Counts come in batches, a short
// transaction a minute or so: the teams' sync needs the same connections.

// LibraryCount is how much to add to an item's numbers.
type LibraryCount struct {
	Views, Reuses int64
}

// AddLibraryCounts adds counts in one transaction, in the order of their IDs so
// that two servers can't deadlock. Only what's live counts.
func (s *DB) AddLibraryCounts(ctx context.Context, counts map[string]LibraryCount) error {
	if len(counts) == 0 {
		return nil
	}
	tx, err := s.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, id := range slices.Sorted(maps.Keys(counts)) {
		c := counts[id]
		_, err := tx.ExecContext(ctx, `INSERT INTO library_stats (item_id, views, reuses) SELECT id, CAST($2 AS BIGINT), CAST($3 AS BIGINT) FROM library_items WHERE id = $1 AND status = 'live'
			ON CONFLICT (item_id) DO UPDATE SET views = library_stats.views + excluded.views, reuses = library_stats.reuses + excluded.reuses`, id, c.Views, c.Reuses)
		if err != nil {
			return fmt.Errorf("store: library counts: %w", err)
		}
	}
	return tx.Commit()
}

// LibraryStats are owner's items' numbers, the most viewed first.
func (s *DB) LibraryStats(ctx context.Context, owner string) ([]api.LibraryItemStats, error) {
	rows, err := s.QueryContext(ctx, `SELECT i.id, i.title, i.slug, i.status, COALESCE(st.views, 0), i.saves, COALESCE(st.reuses, 0),
		(SELECT COUNT(*) FROM library_comments c WHERE c.item_id = i.id AND c.status = 'live')
		FROM library_items i LEFT JOIN library_stats st ON st.item_id = i.id
		WHERE i.owner_id = $1 AND i.status <> 'uploading' ORDER BY COALESCE(st.views, 0) DESC, i.created DESC LIMIT 500`, owner)
	if err != nil {
		return nil, fmt.Errorf("store: library stats: %w", err)
	}
	defer rows.Close()
	out := []api.LibraryItemStats{}
	for rows.Next() {
		var it api.LibraryItemStats
		if err := rows.Scan(&it.ID, &it.Title, &it.Slug, &it.Status, &it.Views, &it.Saves, &it.Reuses, &it.Comments); err != nil {
			return nil, fmt.Errorf("store: library stats: %w", err)
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// LibraryPicks are the live picks, the latest first.
func (s *DB) LibraryPicks(ctx context.Context) ([]api.LibraryPick, error) {
	rows, err := s.QueryContext(ctx, `SELECT i.id, i.title, i.slug, COALESCE(p.handle, ''), i.featured_at, i.announced_at
		FROM library_items i LEFT JOIN library_profiles p ON p.account_id = i.owner_id
		WHERE i.status = 'live' AND i.featured_at <> '' ORDER BY i.featured_at DESC LIMIT 50`)
	if err != nil {
		return nil, fmt.Errorf("store: library picks: %w", err)
	}
	defer rows.Close()
	out := []api.LibraryPick{}
	for rows.Next() {
		var p api.LibraryPick
		if err := rows.Scan(&p.ID, &p.Title, &p.Slug, &p.Creator, &p.Featured, &p.Announced); err != nil {
			return nil, fmt.Errorf("store: library picks: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ClaimLibraryAnnouncement marks a live pick announced, so it's said once however
// many ask, and logs it. ErrNotFound when it isn't a live pick not yet announced.
func (s *DB) ClaimLibraryAnnouncement(ctx context.Context, actor, id, reason string) (api.LibraryPick, error) {
	var p api.LibraryPick
	tx, err := s.BeginTx(ctx, nil)
	if err != nil {
		return p, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE library_items SET announced_at = $1 WHERE id = $2 AND status = 'live' AND featured_at <> '' AND announced_at = ''`, stamp(now()), id)
	if err != nil {
		return p, fmt.Errorf("store: library announce: %w", err)
	}
	if done, _ := res.RowsAffected(); done == 0 {
		return p, ErrNotFound
	}
	err = tx.QueryRowContext(ctx, `SELECT i.id, i.title, i.slug, COALESCE(p.handle, ''), i.featured_at, i.announced_at
		FROM library_items i LEFT JOIN library_profiles p ON p.account_id = i.owner_id WHERE i.id = $1`, id).Scan(&p.ID, &p.Title, &p.Slug, &p.Creator, &p.Featured, &p.Announced)
	if errors.Is(err, sql.ErrNoRows) {
		return p, ErrNotFound
	}
	if err != nil {
		return p, fmt.Errorf("store: library announce: %w", err)
	}
	if err := libraryLog(ctx, tx, actor, "item.announced", id, "", 0, reason); err != nil {
		return p, err
	}
	return p, tx.Commit()
}

// ReleaseLibraryAnnouncement undoes a claim whose message didn't go, so that it
// can be tried again, and says so in the log.
func (s *DB) ReleaseLibraryAnnouncement(ctx context.Context, actor, id, reason string) error {
	tx, err := s.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE library_items SET announced_at = '' WHERE id = $1`, id); err != nil {
		return fmt.Errorf("store: library announce: %w", err)
	}
	if err := libraryLog(ctx, tx, actor, "item.announce-failed", id, "", 0, reason); err != nil {
		return err
	}
	return tx.Commit()
}

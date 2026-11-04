package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/enowx/enowx/cloud/api"
)

// Taking one's data out of the library, and deleting it. Only library tables are
// named here: accounts, devices, tokens and teams are the Cloud's, and stay.

// LibraryExport is what the library keeps of accID, profile aside (cloud/library
// fills that, as it knows what a profile shows).
func (s *DB) LibraryExport(ctx context.Context, accID string) (api.LibraryExport, error) {
	out := api.LibraryExport{Format: api.LibraryExportFormat, Exported: now()}
	var err error
	if out.Items, err = s.LibraryMine(ctx, accID); err != nil {
		return out, err
	}
	if out.Likes, err = s.libraryIDs(ctx, `SELECT item_id FROM library_saves WHERE account_id = $1 ORDER BY created`, accID); err != nil {
		return out, err
	}
	if out.Following, err = s.libraryIDs(ctx, `SELECT p.handle FROM library_follows f JOIN library_profiles p ON p.account_id = f.creator_id WHERE f.follower_id = $1 ORDER BY f.created`, accID); err != nil {
		return out, err
	}
	collections, err := s.LibraryCollectionsOf(ctx, accID, true, "")
	if err != nil {
		return out, err
	}
	out.Collections = []api.LibraryExportCollection{}
	for _, c := range collections {
		ids, err := s.libraryIDs(ctx, `SELECT item_id FROM library_collection_items WHERE collection_id = $1 ORDER BY created`, c.ID)
		if err != nil {
			return out, err
		}
		out.Collections = append(out.Collections, api.LibraryExportCollection{LibraryCollection: c, ItemIDs: ids})
	}
	stats, err := s.LibraryCreatorStats(ctx, accID)
	if err != nil {
		return out, err
	}
	out.Followers = stats.Followers
	if out.Reports, err = s.libraryOwnReports(ctx, accID); err != nil {
		return out, err
	}
	out.Decisions, err = s.libraryDecisionsOn(ctx, accID)
	return out, err
}

func (s *DB) libraryOwnReports(ctx context.Context, accID string) ([]api.LibraryExportReport, error) {
	rows, err := s.QueryContext(ctx, `SELECT item_id, account_id, reason, detail, status, created FROM library_reports WHERE reporter_id = $1 ORDER BY created`, accID)
	if err != nil {
		return nil, fmt.Errorf("store: library export: %w", err)
	}
	defer rows.Close()
	out := []api.LibraryExportReport{}
	for rows.Next() {
		var r api.LibraryExportReport
		var created string
		if err := rows.Scan(&r.ItemID, &r.AccountID, &r.Reason, &r.Detail, &r.Status, &created); err != nil {
			return nil, err
		}
		r.Created = parseStamp(created)
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *DB) libraryDecisionsOn(ctx context.Context, accID string) ([]api.LibraryExportDecision, error) {
	rows, err := s.QueryContext(ctx, `SELECT at, action, item_id, version, reason FROM library_log
		WHERE account_id = $1 OR item_id IN (SELECT id FROM library_items WHERE owner_id = $1) ORDER BY id`, accID)
	if err != nil {
		return nil, fmt.Errorf("store: library export: %w", err)
	}
	defer rows.Close()
	out := []api.LibraryExportDecision{}
	for rows.Next() {
		var d api.LibraryExportDecision
		var at string
		if err := rows.Scan(&at, &d.Action, &d.ItemID, &d.Version, &d.Reason); err != nil {
			return nil, err
		}
		d.At = parseStamp(at)
		out = append(out, d)
	}
	return out, rows.Err()
}

// DeleteLibraryAccount deletes what the library keeps of accID, leaving the account
// and its teams as they are. A design's objects go by the sweep's purge, which this
// makes due at once; its row stays, with nothing of it left, so its address goes on
// answering 410. Kept, on purpose:
//   - library_log's rows, with the account's ID: it's append-only, and this adds one.
//   - reports they made, for moderators, no longer naming them.
//   - a blocked account's row, emptied, so that deleting isn't a way out of a block.
func (s *DB) DeleteLibraryAccount(ctx context.Context, accID string) error {
	tx, err := s.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	at := now()
	var trust string
	err = tx.QueryRowContext(ctx, `SELECT trust FROM library_profiles WHERE account_id = $1`, accID).Scan(&trust)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("store: library delete: %w", err)
	}
	const mine = `(SELECT id FROM library_items WHERE owner_id = $1)`
	type step struct {
		query string
		args  []any
	}
	steps := []step{
		// Reports against what's going have nothing left to decide.
		{`UPDATE library_reports SET status = 'closed' WHERE status = 'open' AND (account_id = $1 OR item_id IN ` + mine + `)`, []any{accID}},
		{`UPDATE library_reports SET reporter_id = 'deleted' WHERE reporter_id = $1`, []any{accID}},
		{`UPDATE library_items SET saves = CASE WHEN saves > 0 THEN saves - 1 ELSE 0 END WHERE id IN (SELECT item_id FROM library_saves WHERE account_id = $1)`, []any{accID}},
		{`DELETE FROM library_saves WHERE account_id = $1 OR item_id IN ` + mine, []any{accID}},
		{`DELETE FROM library_collection_items WHERE item_id IN ` + mine + ` OR collection_id IN (SELECT id FROM library_collections WHERE owner_id = $1)`, []any{accID}},
		{`DELETE FROM library_collections WHERE owner_id = $1`, []any{accID}},
		{`DELETE FROM library_follows WHERE follower_id = $1 OR creator_id = $1`, []any{accID}},
		{`DELETE FROM library_tags WHERE item_id IN ` + mine, []any{accID}},
		{`UPDATE library_versions SET prompt_text = '', scan = '[]', meta = '{}' WHERE item_id IN ` + mine, []any{accID}},
		{`UPDATE library_items SET status = CASE WHEN status = 'removed' THEN 'removed' WHEN live_version > 0 THEN 'unpublished' ELSE 'withdrawn' END,
			slug = '', title = '', description = '', style = '', theme = '', device = '', source_url = '', reject_reason = '', featured_at = '', announced_at = '',
			purge_at = $1, updated = $2 WHERE owner_id = $3`, []any{stamp(at.Add(-time.Second)), stamp(at), accID}},
		{`DELETE FROM library_sessions WHERE account_id = $1`, []any{accID}},
		{`DELETE FROM library_auth_codes WHERE account_id = $1`, []any{accID}},
	}
	if trust == api.TrustBlocked {
		steps = append(steps, step{`UPDATE library_profiles SET handle = $1, bio = '', links = '[]', hidden = 1, open_for_work = 0, work_kinds = '[]', skills = '[]', timezone = '', contact_url = '', show_github = 0, show_stats = 0 WHERE account_id = $2`, []any{"deleted-" + accID, accID}})
	} else {
		steps = append(steps, step{`DELETE FROM library_profiles WHERE account_id = $1`, []any{accID}})
	}
	for _, step := range steps {
		if _, err := tx.ExecContext(ctx, step.query, step.args...); err != nil {
			return fmt.Errorf("store: library delete: %w", err)
		}
	}
	if err := libraryLog(ctx, tx, accID, "account.deleted", "", accID, 0, "by its owner"); err != nil {
		return err
	}
	return tx.Commit()
}

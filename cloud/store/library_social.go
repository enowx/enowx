package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/enowx/enowx/cloud/api"
)

// Likes, collections and follows. Each function does its one thing, so the
// community's comments and notifications can call them as they are. Who may ask is
// cloud/library's to decide; what's private here is never joined into anything public.

// SaveLibraryItem makes accID like an item and says how it stands; a like already
// there changes nothing. The count moves with the row, in one step.
func (s *DB) SaveLibraryItem(ctx context.Context, accID, itemID string) (api.LibrarySaved, error) {
	return s.setLibrarySave(ctx, accID, itemID, true)
}

// UnsaveLibraryItem takes accID's like away.
func (s *DB) UnsaveLibraryItem(ctx context.Context, accID, itemID string) (api.LibrarySaved, error) {
	return s.setLibrarySave(ctx, accID, itemID, false)
}

func (s *DB) setLibrarySave(ctx context.Context, accID, itemID string, on bool) (api.LibrarySaved, error) {
	out := api.LibrarySaved{Saved: on}
	tx, err := s.BeginTx(ctx, nil)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	var res sql.Result
	change := `saves + 1`
	if on {
		res, err = tx.ExecContext(ctx, `INSERT INTO library_saves (account_id, item_id, created) VALUES ($1, $2, $3) ON CONFLICT (account_id, item_id) DO NOTHING`, accID, itemID, stamp(now()))
	} else {
		change = `CASE WHEN saves > 0 THEN saves - 1 ELSE 0 END`
		res, err = tx.ExecContext(ctx, `DELETE FROM library_saves WHERE account_id = $1 AND item_id = $2`, accID, itemID)
	}
	if err != nil {
		return out, fmt.Errorf("store: library save: %w", err)
	}
	if done, _ := res.RowsAffected(); done > 0 {
		if _, err := tx.ExecContext(ctx, `UPDATE library_items SET saves = `+change+` WHERE id = $1`, itemID); err != nil {
			return out, fmt.Errorf("store: library save: %w", err)
		}
	}
	err = tx.QueryRowContext(ctx, `SELECT saves FROM library_items WHERE id = $1`, itemID).Scan(&out.Saves)
	if errors.Is(err, sql.ErrNoRows) {
		return out, ErrNotFound
	}
	if err != nil {
		return out, fmt.Errorf("store: library save: %w", err)
	}
	return out, tx.Commit()
}

// LibrarySaveState says how item itemID stands to accID.
func (s *DB) LibrarySaveState(ctx context.Context, accID, itemID string) (api.LibrarySaved, error) {
	var out api.LibrarySaved
	var mine int
	err := s.QueryRowContext(ctx, `SELECT saves, (SELECT COUNT(*) FROM library_saves WHERE account_id = $1 AND item_id = $2) FROM library_items WHERE id = $2`, accID, itemID).Scan(&out.Saves, &mine)
	if errors.Is(err, sql.ErrNoRows) {
		return out, ErrNotFound
	}
	if err != nil {
		return out, fmt.Errorf("store: library save: %w", err)
	}
	out.Saved = mine > 0
	return out, nil
}

// LibrarySaveCount counts accID's likes.
func (s *DB) LibrarySaveCount(ctx context.Context, accID string) (int, error) {
	var n int
	err := s.QueryRowContext(ctx, `SELECT COUNT(*) FROM library_saves WHERE account_id = $1`, accID).Scan(&n)
	return n, err
}

// LibrarySavedItems are the live designs accID likes, the latest liked first. Those of
// creators who hid stay here: accID found them when they showed, and has the address.
func (s *DB) LibrarySavedItems(ctx context.Context, accID string, limit, offset int) ([]api.LibraryItem, bool, error) {
	return s.libraryItemPage(ctx, ` JOIN library_saves sv ON sv.item_id = i.id AND sv.account_id = $1`, `i.status = 'live'`, `sv.created DESC, i.id DESC`, limit, offset, accID)
}

// LibrarySavers are the accounts who like an item.
func (s *DB) LibrarySavers(ctx context.Context, itemID string) ([]string, error) {
	return s.libraryIDs(ctx, `SELECT account_id FROM library_saves WHERE item_id = $1`, itemID)
}

func (s *DB) libraryIDs(ctx context.Context, query string, args ...any) ([]string, error) {
	rows, err := s.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store: library ids: %w", err)
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// Follows.

// FollowLibraryCreator makes follower follow creator; following already changes nothing.
func (s *DB) FollowLibraryCreator(ctx context.Context, follower, creator string) error {
	_, err := s.ExecContext(ctx, `INSERT INTO library_follows (follower_id, creator_id, created) VALUES ($1, $2, $3) ON CONFLICT (follower_id, creator_id) DO NOTHING`, follower, creator, stamp(now()))
	if err != nil {
		return fmt.Errorf("store: library follow: %w", err)
	}
	return nil
}

// UnfollowLibraryCreator ends a follow, if there is one.
func (s *DB) UnfollowLibraryCreator(ctx context.Context, follower, creator string) error {
	_, err := s.ExecContext(ctx, `DELETE FROM library_follows WHERE follower_id = $1 AND creator_id = $2`, follower, creator)
	if err != nil {
		return fmt.Errorf("store: library follow: %w", err)
	}
	return nil
}

// LibraryFollowState says how creator stands to follower.
func (s *DB) LibraryFollowState(ctx context.Context, follower, creator string) (api.LibraryFollowState, error) {
	var out api.LibraryFollowState
	var mine int
	err := s.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM library_follows WHERE follower_id = $1 AND creator_id = $2),
		(SELECT COUNT(*) FROM library_follows WHERE creator_id = $2)`, follower, creator).Scan(&mine, &out.Followers)
	if err != nil {
		return out, fmt.Errorf("store: library follow: %w", err)
	}
	out.Following = mine > 0
	return out, nil
}

// LibraryFollowCount counts who follower follows.
func (s *DB) LibraryFollowCount(ctx context.Context, follower string) (int, error) {
	var n int
	err := s.QueryRowContext(ctx, `SELECT COUNT(*) FROM library_follows WHERE follower_id = $1`, follower).Scan(&n)
	return n, err
}

// LibraryFollowers are the accounts who follow creator.
func (s *DB) LibraryFollowers(ctx context.Context, creator string) ([]string, error) {
	return s.libraryIDs(ctx, `SELECT follower_id FROM library_follows WHERE creator_id = $1`, creator)
}

// LibraryFollowing are the creators follower follows and who show, the latest first.
// A creator who hid comes back here when they show again.
func (s *DB) LibraryFollowing(ctx context.Context, follower string) ([]api.LibraryCreator, error) {
	rows, err := s.QueryContext(ctx, `SELECT a.id, p.handle, a.name, a.avatar FROM library_follows f
		JOIN library_profiles p ON p.account_id = f.creator_id JOIN accounts a ON a.id = f.creator_id
		WHERE f.follower_id = $1 AND p.hidden = 0 AND p.trust <> 'blocked' ORDER BY f.created DESC LIMIT 2000`, follower)
	if err != nil {
		return nil, fmt.Errorf("store: library following: %w", err)
	}
	defer rows.Close()
	out := []api.LibraryCreator{}
	for rows.Next() {
		var c api.LibraryCreator
		if err := rows.Scan(&c.ID, &c.Handle, &c.Name, &c.Avatar); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// LibraryFeed is what those follower follows made, the newest first.
func (s *DB) LibraryFeed(ctx context.Context, follower string, limit, offset int) ([]api.LibraryItem, bool, error) {
	return s.libraryItemPage(ctx, ` JOIN library_follows f ON f.creator_id = i.owner_id AND f.follower_id = $1`, libraryListed, `i.created DESC, i.id DESC`, limit, offset, follower)
}

// Collections.

const libraryCollectionCols = `c.id, c.title, c.description, c.public, c.created, c.updated, a.id, COALESCE(p.handle, ''), a.name, a.avatar`
const libraryCollectionFrom = ` FROM library_collections c JOIN accounts a ON a.id = c.owner_id LEFT JOIN library_profiles p ON p.account_id = c.owner_id`

func scanLibraryCollection(r scanner, extra ...any) (api.LibraryCollection, error) {
	var c api.LibraryCollection
	var public int
	var created, updated string
	dest := append([]any{&c.ID, &c.Title, &c.Description, &public, &created, &updated, &c.Owner.ID, &c.Owner.Handle, &c.Owner.Name, &c.Owner.Avatar}, extra...)
	err := r.Scan(dest...)
	c.Public, c.Created, c.Updated = public != 0, parseStamp(created), parseStamp(updated)
	return c, err
}

// AddLibraryCollection makes a collection, unless accID has too many.
func (s *DB) AddLibraryCollection(ctx context.Context, accID string, p api.LibraryCollectionParams) (string, error) {
	tx, err := s.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM library_collections WHERE owner_id = $1`, accID).Scan(&n); err != nil {
		return "", fmt.Errorf("store: library collection: %w", err)
	}
	if n >= api.MaxLibraryCollections {
		return "", errors.New("store: that's as many collections as an account can have")
	}
	id, at := newID(), stamp(now())
	if _, err := tx.ExecContext(ctx, `INSERT INTO library_collections (id, owner_id, title, description, public, created, updated) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		id, accID, p.Title, p.Description, boolInt(p.Public), at, at); err != nil {
		return "", fmt.Errorf("store: library collection: %w", err)
	}
	return id, tx.Commit()
}

// SetLibraryCollection changes a collection of accID's, ErrNotFound for any other.
func (s *DB) SetLibraryCollection(ctx context.Context, accID, id string, p api.LibraryCollectionParams) error {
	res, err := s.ExecContext(ctx, `UPDATE library_collections SET title = $1, description = $2, public = $3, updated = $4 WHERE id = $5 AND owner_id = $6`,
		p.Title, p.Description, boolInt(p.Public), stamp(now()), id, accID)
	return oneLibraryRow(res, err)
}

// DeleteLibraryCollection removes a collection of accID's, ErrNotFound for any other.
// What it held stays where it is.
func (s *DB) DeleteLibraryCollection(ctx context.Context, accID, id string) error {
	res, err := s.ExecContext(ctx, `DELETE FROM library_collections WHERE id = $1 AND owner_id = $2`, id, accID)
	return oneLibraryRow(res, err)
}

func oneLibraryRow(res sql.Result, err error) error {
	if err != nil {
		return fmt.Errorf("store: library collection: %w", err)
	}
	if done, _ := res.RowsAffected(); done == 0 {
		return ErrNotFound
	}
	return nil
}

// LibraryCollectionRow is collection id, whoever's, whatever it may show: the caller
// decides who sees it.
func (s *DB) LibraryCollectionRow(ctx context.Context, id string) (api.LibraryCollection, error) {
	c, err := scanLibraryCollection(s.QueryRowContext(ctx, `SELECT `+libraryCollectionCols+libraryCollectionFrom+` WHERE c.id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return c, ErrNotFound
	}
	if err != nil {
		return c, fmt.Errorf("store: library collection: %w", err)
	}
	return c, nil
}

// libraryCollectionShown is what a collection's list holds for someone: all that's
// live for its owner, for anyone else what's listed.
func libraryCollectionShown(own bool) string {
	if own {
		return `i.status = 'live'`
	}
	return libraryListed
}

// libraryCollectionCount counts what a collection holds, by libraryCollectionShown.
func libraryCollectionCount(own bool) string {
	return `(SELECT COUNT(*) FROM library_collection_items ci JOIN library_items i ON i.id = ci.item_id LEFT JOIN library_profiles p ON p.account_id = i.owner_id
		WHERE ci.collection_id = c.id AND ` + libraryCollectionShown(own) + `)`
}

// LibraryCollectionsOf are accID's collections: all of them for accID, only the
// public ones for anyone else. item, when set, marks those holding it.
func (s *DB) LibraryCollectionsOf(ctx context.Context, accID string, own bool, item string) ([]api.LibraryCollection, error) {
	where := `c.owner_id = $1`
	if !own {
		where += ` AND c.public = 1`
	}
	rows, err := s.QueryContext(ctx, `SELECT `+libraryCollectionCols+`, `+libraryCollectionCount(own)+`,
		(SELECT COUNT(*) FROM library_collection_items h WHERE h.collection_id = c.id AND h.item_id = $2)`+libraryCollectionFrom+` WHERE `+where+` ORDER BY c.updated DESC, c.id LIMIT 200`, accID, item)
	if err != nil {
		return nil, fmt.Errorf("store: library collections: %w", err)
	}
	defer rows.Close()
	out := []api.LibraryCollection{}
	for rows.Next() {
		var holds int
		var count int
		c, err := scanLibraryCollection(rows, &count, &holds)
		if err != nil {
			return nil, fmt.Errorf("store: library collections: %w", err)
		}
		c.Items, c.Contains = count, holds > 0
		out = append(out, c)
	}
	return out, rows.Err()
}

// LibraryCollectionItems are a page of what a collection holds, the latest added
// first, by libraryCollectionShown.
func (s *DB) LibraryCollectionItems(ctx context.Context, id string, own bool, limit, offset int) ([]api.LibraryItem, bool, error) {
	return s.libraryItemPage(ctx, ` JOIN library_collection_items ci ON ci.item_id = i.id AND ci.collection_id = $1`, libraryCollectionShown(own), `ci.created DESC, i.id DESC`, limit, offset, id)
}

// LibraryCollectionSize counts what a collection holds, by libraryCollectionShown.
func (s *DB) LibraryCollectionSize(ctx context.Context, id string, own bool) (int, error) {
	var n int
	err := s.QueryRowContext(ctx, `SELECT `+libraryCollectionCount(own)+` FROM library_collections c WHERE c.id = $1`, id).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("store: library collection: %w", err)
	}
	return n, nil
}

// SetLibraryCollectionItem puts an item in a collection of accID's or takes it out.
func (s *DB) SetLibraryCollectionItem(ctx context.Context, accID, id, item string, in bool) error {
	tx, err := s.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var n int
	err = tx.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM library_collection_items WHERE collection_id = c.id) FROM library_collections c WHERE c.id = $1 AND c.owner_id = $2`, id, accID).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("store: library collection: %w", err)
	}
	if in {
		if n >= api.MaxLibraryInCollect {
			return errors.New("store: that collection is full: 500 designs")
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO library_collection_items (collection_id, item_id, created) VALUES ($1, $2, $3) ON CONFLICT (collection_id, item_id) DO NOTHING`, id, item, stamp(now()))
	} else {
		_, err = tx.ExecContext(ctx, `DELETE FROM library_collection_items WHERE collection_id = $1 AND item_id = $2`, id, item)
	}
	if err != nil {
		return fmt.Errorf("store: library collection: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE library_collections SET updated = $1 WHERE id = $2`, stamp(now()), id); err != nil {
		return fmt.Errorf("store: library collection: %w", err)
	}
	return tx.Commit()
}

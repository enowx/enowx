package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/enowx/enowx/cloud/api"
)

// Comments on the library's items (cloud/library decides who may). Their text is
// other people's data, kept and given back as it is.

// How a comment stands. Only a live one is ever shown to anyone but a moderator
// reading a report on it.
const (
	commentLive    = "live"
	commentRemoved = "removed" // by a moderator: its text stays for them
	commentDeleted = "deleted" // by its author: its text is gone
)

// AddLibraryComment keeps a comment.
func (s *DB) AddLibraryComment(ctx context.Context, item, author, body string) (string, error) {
	id := newID()
	_, err := s.ExecContext(ctx, `INSERT INTO library_comments (id, item_id, author_id, body, created) VALUES ($1, $2, $3, $4, $5)`, id, item, author, body, stamp(now()))
	if err != nil {
		return "", fmt.Errorf("store: library comment: %w", err)
	}
	return id, nil
}

// LibraryCommentLimits are what an hour's limits are counted from.
type LibraryCommentLimits struct {
	ByAccount, ByAccountHere, ByItem int
}

// LibraryCommentCounts counts the last hour's comments by author, by author on
// item, and on item, whatever became of them: deleting doesn't give turns back.
func (s *DB) LibraryCommentCounts(ctx context.Context, author, item string) (LibraryCommentLimits, error) {
	var c LibraryCommentLimits
	since := stamp(now().Add(-time.Hour))
	err := s.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM library_comments WHERE author_id = $1 AND created > $3),
		(SELECT COUNT(*) FROM library_comments WHERE author_id = $1 AND item_id = $2 AND created > $3),
		(SELECT COUNT(*) FROM library_comments WHERE item_id = $2 AND created > $3)`, author, item, since).Scan(&c.ByAccount, &c.ByAccountHere, &c.ByItem)
	if err != nil {
		return c, fmt.Errorf("store: library comment counts: %w", err)
	}
	return c, nil
}

// LibraryCommentCursor is where a page of comments goes on from.
type LibraryCommentCursor struct {
	Created, ID string
}

// LibraryComments are item's comments shown to anyone, the latest first, those
// of blocked accounts left out; next says where the page after goes on from.
func (s *DB) LibraryComments(ctx context.Context, item string, from LibraryCommentCursor, limit int) (list []api.LibraryComment, next LibraryCommentCursor, err error) {
	limit = max(min(limit, 100), 1)
	args := []any{item}
	where := ``
	if from.ID != "" {
		args = append(args, from.Created, from.ID)
		where = ` AND (c.created < $2 OR (c.created = $2 AND c.id < $3))`
	}
	rows, err := s.QueryContext(ctx, `SELECT c.id, c.item_id, c.body, c.created, a.id, COALESCE(p.handle, ''), a.name, a.avatar
		FROM library_comments c JOIN accounts a ON a.id = c.author_id LEFT JOIN library_profiles p ON p.account_id = c.author_id
		WHERE c.item_id = $1 AND c.status = 'live' AND (p.trust IS NULL OR p.trust <> 'blocked')`+where+
		fmt.Sprintf(` ORDER BY c.created DESC, c.id DESC LIMIT %d`, limit+1), args...)
	if err != nil {
		return nil, next, fmt.Errorf("store: library comments: %w", err)
	}
	defer rows.Close()
	list = []api.LibraryComment{}
	for rows.Next() {
		var c api.LibraryComment
		var created string
		if err := rows.Scan(&c.ID, &c.ItemID, &c.Body, &created, &c.Author.ID, &c.Author.Handle, &c.Author.Name, &c.Author.Avatar); err != nil {
			return nil, next, fmt.Errorf("store: library comments: %w", err)
		}
		c.Created = parseStamp(created)
		list = append(list, c)
		if len(list) == limit+1 {
			last := list[limit-1]
			list = list[:limit]
			next = LibraryCommentCursor{Created: stamp(last.Created), ID: last.ID}
			break
		}
	}
	return list, next, rows.Err()
}

// LibraryCommentsInfo says how many comments show on item and whether its
// creator closed them.
func (s *DB) LibraryCommentsInfo(ctx context.Context, item string) (count int, off bool, err error) {
	var closed int
	err = s.QueryRowContext(ctx, `SELECT i.comments_off, (SELECT COUNT(*) FROM library_comments c LEFT JOIN library_profiles p ON p.account_id = c.author_id
		WHERE c.item_id = i.id AND c.status = 'live' AND (p.trust IS NULL OR p.trust <> 'blocked')) FROM library_items i WHERE i.id = $1`, item).Scan(&closed, &count)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, ErrNotFound
	}
	if err != nil {
		return 0, false, fmt.Errorf("store: library comments: %w", err)
	}
	return count, closed != 0, nil
}

// SetLibraryCommentsOff is an item's creator opening or closing its comments.
func (s *DB) SetLibraryCommentsOff(ctx context.Context, actor, item string, off bool) error {
	tx, err := s.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE library_items SET comments_off = $1 WHERE id = $2`, boolInt(off), item)
	if err != nil {
		return fmt.Errorf("store: library comments off: %w", err)
	}
	if done, _ := res.RowsAffected(); done == 0 {
		return ErrNotFound
	}
	action := "item.comments-on"
	if off {
		action = "item.comments-off"
	}
	if err := libraryLog(ctx, tx, actor, action, item, "", 0, "by its creator"); err != nil {
		return err
	}
	return tx.Commit()
}

// LibraryCommentRow is a comment as the database holds it.
type LibraryCommentRow struct {
	ID, ItemID, AuthorID, Status string
}

// LibraryCommentByID is comment id, ErrNotFound when there's none.
func (s *DB) LibraryCommentByID(ctx context.Context, id string) (LibraryCommentRow, error) {
	var c LibraryCommentRow
	err := s.QueryRowContext(ctx, `SELECT id, item_id, author_id, status FROM library_comments WHERE id = $1`, id).Scan(&c.ID, &c.ItemID, &c.AuthorID, &c.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return c, ErrNotFound
	}
	if err != nil {
		return c, fmt.Errorf("store: library comment: %w", err)
	}
	return c, nil
}

// DeleteLibraryComment is an author taking their own words back: the text goes,
// and a report on it has nothing left to decide.
func (s *DB) DeleteLibraryComment(ctx context.Context, item, id, author string) error {
	tx, err := s.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE library_comments SET status = $1, body = '' WHERE id = $2 AND item_id = $3 AND author_id = $4 AND status = 'live'`, commentDeleted, id, item, author)
	if err != nil {
		return fmt.Errorf("store: library comment: %w", err)
	}
	if done, _ := res.RowsAffected(); done == 0 {
		return ErrNotFound
	}
	if _, err := tx.ExecContext(ctx, `UPDATE library_reports SET status = 'moot' WHERE comment_id = $1 AND status = 'open'`, id); err != nil {
		return fmt.Errorf("store: library comment: %w", err)
	}
	return tx.Commit()
}

// removeLibraryComment takes comment id off, in tx, ErrNotFound when it isn't
// showing.
func removeLibraryComment(ctx context.Context, tx *sql.Tx, id string) error {
	res, err := tx.ExecContext(ctx, `UPDATE library_comments SET status = $1 WHERE id = $2 AND status = 'live'`, commentRemoved, id)
	if err != nil {
		return fmt.Errorf("store: library comment: %w", err)
	}
	if done, _ := res.RowsAffected(); done == 0 {
		return ErrNotFound
	}
	return nil
}

// RemoveLibraryComment is a moderator taking a comment off, with its reason in
// the log; open reports on it are settled with it.
func (s *DB) RemoveLibraryComment(ctx context.Context, actor, id, reason string) (LibraryCommentRow, error) {
	c, err := s.LibraryCommentByID(ctx, id)
	if err != nil {
		return c, err
	}
	tx, err := s.BeginTx(ctx, nil)
	if err != nil {
		return c, err
	}
	defer tx.Rollback()
	if err := removeLibraryComment(ctx, tx, id); err != nil {
		return c, err
	}
	if err := libraryLog(ctx, tx, actor, "comment.removed", c.ItemID, c.AuthorID, 0, reason); err != nil {
		return c, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE library_reports SET status = 'upheld', decision_id = (SELECT MAX(id) FROM library_log) WHERE comment_id = $1 AND status = 'open'`, id); err != nil {
		return c, fmt.Errorf("store: library comment: %w", err)
	}
	return c, tx.Commit()
}

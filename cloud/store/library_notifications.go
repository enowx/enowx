package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/enowx/enowx/cloud/api"
)

// What happens to a creator, as cloud/library says it. A row never names who
// reported or which moderator decided.

// LibraryNotifyMemory is how long a notification stays.
const LibraryNotifyMemory = 180 * 24 * time.Hour

// AddLibraryNotification keeps one for account.
func (s *DB) AddLibraryNotification(ctx context.Context, account, kind, item, actor, reason string) error {
	_, err := s.ExecContext(ctx, `INSERT INTO library_notifications (id, account_id, kind, item_id, actor_id, reason, created) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		newID(), account, kind, item, actor, reason, stamp(now()))
	if err != nil {
		return fmt.Errorf("store: library notification: %w", err)
	}
	return nil
}

// LibraryNotifyUnreadOn says account has an unread one of kind on item already.
func (s *DB) LibraryNotifyUnreadOn(ctx context.Context, account, kind, item string) (bool, error) {
	var n int
	err := s.QueryRowContext(ctx, `SELECT COUNT(*) FROM library_notifications WHERE account_id = $1 AND kind = $2 AND item_id = $3 AND read_at = ''`, account, kind, item).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("store: library notification: %w", err)
	}
	return n > 0, nil
}

// LibraryNotificationCursor is where a page goes on from.
type LibraryNotificationCursor struct {
	Created, ID string
}

// LibraryNotifications are a page of account's, the latest first.
func (s *DB) LibraryNotifications(ctx context.Context, account string, from LibraryNotificationCursor, limit int) (list []api.LibraryNotification, next LibraryNotificationCursor, err error) {
	limit = max(min(limit, 100), 1)
	args := []any{account}
	where := ``
	if from.ID != "" {
		args = append(args, from.Created, from.ID)
		where = ` AND (n.created < $2 OR (n.created = $2 AND n.id < $3))`
	}
	rows, err := s.QueryContext(ctx, `SELECT n.id, n.kind, n.item_id, COALESCE(i.title, ''), COALESCE(i.slug, ''), COALESCE(p.handle, ''), n.reason, n.read_at, n.created
		FROM library_notifications n LEFT JOIN library_items i ON i.id = n.item_id LEFT JOIN library_profiles p ON p.account_id = n.actor_id
		WHERE n.account_id = $1`+where+fmt.Sprintf(` ORDER BY n.created DESC, n.id DESC LIMIT %d`, limit+1), args...)
	if err != nil {
		return nil, next, fmt.Errorf("store: library notifications: %w", err)
	}
	defer rows.Close()
	list = []api.LibraryNotification{}
	for rows.Next() {
		var n api.LibraryNotification
		var read, created string
		if err := rows.Scan(&n.ID, &n.Kind, &n.ItemID, &n.Item, &n.Slug, &n.Actor, &n.Reason, &read, &created); err != nil {
			return nil, next, fmt.Errorf("store: library notifications: %w", err)
		}
		n.Read, n.Created = read != "", parseStamp(created)
		list = append(list, n)
		if len(list) == limit+1 {
			last := list[limit-1]
			list = list[:limit]
			next = LibraryNotificationCursor{Created: stamp(last.Created), ID: last.ID}
			break
		}
	}
	return list, next, rows.Err()
}

// LibraryUnread counts account's unread ones.
func (s *DB) LibraryUnread(ctx context.Context, account string) (int, error) {
	var n int
	err := s.QueryRowContext(ctx, `SELECT COUNT(*) FROM library_notifications WHERE account_id = $1 AND read_at = ''`, account).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("store: library notifications: %w", err)
	}
	return n, nil
}

// MarkLibraryNotificationsRead marks account's ids read, all of them with none.
func (s *DB) MarkLibraryNotificationsRead(ctx context.Context, account string, ids []string) error {
	args := []any{stamp(now()), account}
	where := ``
	if len(ids) > 0 {
		marks := make([]string, len(ids))
		for i, id := range ids {
			args = append(args, id)
			marks[i] = fmt.Sprintf("$%d", i+3)
		}
		where = ` AND id IN (` + strings.Join(marks, ",") + `)`
	}
	if _, err := s.ExecContext(ctx, `UPDATE library_notifications SET read_at = $1 WHERE account_id = $2 AND read_at = ''`+where, args...); err != nil {
		return fmt.Errorf("store: library notifications: %w", err)
	}
	return nil
}

// LibraryNotifySettings are account's, nothing off and no email until it says.
func (s *DB) LibraryNotifySettings(ctx context.Context, account string) (api.LibraryNotifySettings, error) {
	out := api.LibraryNotifySettings{Off: []string{}}
	var off string
	var email int
	err := s.QueryRowContext(ctx, `SELECT off, email FROM library_notify_settings WHERE account_id = $1`, account).Scan(&off, &email)
	if errors.Is(err, sql.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return out, fmt.Errorf("store: library notify settings: %w", err)
	}
	if err := json.Unmarshal([]byte(off), &out.Off); err != nil {
		return out, fmt.Errorf("store: library notify settings: %w", err)
	}
	out.Email = email != 0
	return out, nil
}

// SetLibraryNotifySettings keeps account's.
func (s *DB) SetLibraryNotifySettings(ctx context.Context, account string, set api.LibraryNotifySettings) error {
	off, err := json.Marshal(set.Off)
	if err != nil {
		return fmt.Errorf("store: library notify settings: %w", err)
	}
	_, err = s.ExecContext(ctx, `INSERT INTO library_notify_settings (account_id, off, email) VALUES ($1, $2, $3)
		ON CONFLICT (account_id) DO UPDATE SET off = $2, email = $3`, account, string(off), boolInt(set.Email))
	if err != nil {
		return fmt.Errorf("store: library notify settings: %w", err)
	}
	return nil
}

// ForgetOldLibraryNotifications drops those past LibraryNotifyMemory.
func (s *DB) ForgetOldLibraryNotifications(ctx context.Context) error {
	if _, err := s.ExecContext(ctx, `DELETE FROM library_notifications WHERE created < $1`, stamp(now().Add(-LibraryNotifyMemory))); err != nil {
		return fmt.Errorf("store: library notifications: %w", err)
	}
	return nil
}

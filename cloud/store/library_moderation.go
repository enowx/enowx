package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/enowx/enowx/cloud/api"
)

// The library's moderation: its log, reports, decisions, moderators, and
// the web's sessions. The log has an append and a read here and nothing
// else: no decision can be rewritten after it was made.

// libraryLog appends to the log, in transaction tx.
func libraryLog(ctx context.Context, tx *sql.Tx, actor, action, item, account string, version int, reason string) error {
	if strings.TrimSpace(reason) == "" {
		return errors.New("store: a moderation action needs a reason")
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO library_log (at, actor_id, action, item_id, account_id, version, reason) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		stamp(now()), actor, action, item, account, version, reason)
	if err != nil {
		return fmt.Errorf("store: library log: %w", err)
	}
	return nil
}

// LibraryLog is a page of the log, the latest first, before entry before (0
// from the latest).
func (s *DB) LibraryLog(ctx context.Context, before int64, limit int) ([]api.LibraryLogEntry, error) {
	q := `SELECT l.id, l.at, COALESCE(p.handle, l.actor_id), l.action, l.item_id, l.account_id, l.version, l.reason
		FROM library_log l LEFT JOIN library_profiles p ON p.account_id = l.actor_id`
	args := []any{}
	if before > 0 {
		q += ` WHERE l.id < $1`
		args = append(args, before)
	}
	rows, err := s.QueryContext(ctx, q+fmt.Sprintf(` ORDER BY l.id DESC LIMIT %d`, max(min(limit, 200), 1)), args...)
	if err != nil {
		return nil, fmt.Errorf("store: library log: %w", err)
	}
	defer rows.Close()
	out := []api.LibraryLogEntry{}
	for rows.Next() {
		var e api.LibraryLogEntry
		var at string
		if err := rows.Scan(&e.ID, &at, &e.Actor, &e.Action, &e.ItemID, &e.AccountID, &e.Version, &e.Reason); err != nil {
			return nil, err
		}
		e.At = parseStamp(at)
		out = append(out, e)
	}
	return out, rows.Err()
}

// LibraryReview are the versions waiting for a moderator, the oldest first.
func (s *DB) LibraryReview(ctx context.Context, limit int) ([]api.LibraryQueueItem, error) {
	rows, err := s.QueryContext(ctx, `SELECT v.item_id, v.version, COALESCE(p.trust, 'new') FROM library_versions v
		JOIN library_items i ON i.id = v.item_id LEFT JOIN library_profiles p ON p.account_id = i.owner_id
		WHERE v.status = 'review' AND i.status NOT IN ('withdrawn', 'unpublished', 'removed') ORDER BY v.created LIMIT $1`, max(min(limit, 100), 1))
	if err != nil {
		return nil, fmt.Errorf("store: library review: %w", err)
	}
	type waiting struct {
		id, trust string
		n         int
	}
	var list []waiting
	for rows.Next() {
		var w waiting
		if err := rows.Scan(&w.id, &w.n, &w.trust); err != nil {
			rows.Close()
			return nil, err
		}
		list = append(list, w)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := []api.LibraryQueueItem{}
	for _, w := range list {
		it, err := s.LibraryItem(ctx, w.id)
		if err != nil {
			return nil, err
		}
		v, err := s.LibraryVersion(ctx, w.id, w.n)
		if err != nil {
			return nil, err
		}
		out = append(out, api.LibraryQueueItem{LibraryItem: it, Version: v.API(), Trust: w.trust})
	}
	return out, nil
}

// DecideLibraryVersion settles version n of item id: approve (it goes live,
// its owner trusted from now on) or reject (the reason reaches the app).
func (s *DB) DecideLibraryVersion(ctx context.Context, actor, id string, n int, approve bool, reason string) error {
	tx, err := s.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var owner, status string
	var live int
	err = tx.QueryRowContext(ctx, `SELECT i.owner_id, i.status, i.live_version FROM library_items i JOIN library_versions v ON v.item_id = i.id
		WHERE i.id = $1 AND v.version = $2 AND v.status = 'review'`, id, n).Scan(&owner, &status, &live)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("store: library decision: %w", err)
	}
	if status == api.LibraryWithdrawn || status == api.LibraryUnpublished || status == api.LibraryRemoved {
		return ErrNotFound
	}
	at := stamp(now())
	action := "version.rejected"
	if approve {
		action = "version.approved"
		if _, err := tx.ExecContext(ctx, `UPDATE library_versions SET status = 'live' WHERE item_id = $1 AND version = $2`, id, n); err != nil {
			return fmt.Errorf("store: library decision: %w", err)
		}
		if err := goLive(ctx, tx, id, n, at); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE library_profiles SET trust = 'trusted' WHERE account_id = $1 AND trust = 'new'`, owner); err != nil {
			return fmt.Errorf("store: library decision: %w", err)
		}
	} else {
		if _, err := tx.ExecContext(ctx, `UPDATE library_versions SET status = 'rejected' WHERE item_id = $1 AND version = $2`, id, n); err != nil {
			return fmt.Errorf("store: library decision: %w", err)
		}
		// What shows goes on showing; a first version rejected leaves nothing to.
		if _, err := tx.ExecContext(ctx, `UPDATE library_items SET status = CASE WHEN status = 'live' THEN status ELSE 'rejected' END, reject_reason = $1, updated = $2 WHERE id = $3`, reason, at, id); err != nil {
			return fmt.Errorf("store: library decision: %w", err)
		}
	}
	if err := libraryLog(ctx, tx, actor, action, id, owner, n, reason); err != nil {
		return err
	}
	return tx.Commit()
}

// RemoveLibraryItem takes item id down, as a moderator: its address answers
// 410, its objects go after LibraryPurgeAfter.
func (s *DB) RemoveLibraryItem(ctx context.Context, actor, id, reason string) error {
	tx, err := s.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := removeLibraryItem(ctx, tx, id); err != nil {
		return err
	}
	if err := libraryLog(ctx, tx, actor, "item.removed", id, "", 0, reason); err != nil {
		return err
	}
	return tx.Commit()
}

func removeLibraryItem(ctx context.Context, tx *sql.Tx, id string) error {
	at := now()
	res, err := tx.ExecContext(ctx, `UPDATE library_items SET status = 'removed', featured_at = '', purge_at = $1, updated = $2 WHERE id = $3 AND status <> 'removed'`,
		stamp(at.Add(LibraryPurgeAfter)), stamp(at), id)
	if err != nil {
		return fmt.Errorf("store: library remove: %w", err)
	}
	if done, _ := res.RowsAffected(); done == 0 {
		return ErrNotFound
	}
	return nil
}

// FeatureLibraryItem marks a live item as a pick, or takes that off.
func (s *DB) FeatureLibraryItem(ctx context.Context, actor, id string, on bool, reason string) error {
	tx, err := s.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	at, action := "", "item.unfeatured"
	if on {
		at, action = stamp(now()), "item.featured"
	}
	res, err := tx.ExecContext(ctx, `UPDATE library_items SET featured_at = $1 WHERE id = $2 AND status = 'live'`, at, id)
	if err != nil {
		return fmt.Errorf("store: library feature: %w", err)
	}
	if done, _ := res.RowsAffected(); done == 0 {
		return ErrNotFound
	}
	if err := libraryLog(ctx, tx, actor, action, id, "", 0, reason); err != nil {
		return err
	}
	return tx.Commit()
}

// AddLibraryReport keeps a report; a reporter reporting the same thing again
// while it's open adds nothing. reporter is empty for the library's own.
func (s *DB) AddLibraryReport(ctx context.Context, reporter string, p api.LibraryReportParams) error {
	var had int
	err := s.QueryRowContext(ctx, `SELECT COUNT(*) FROM library_reports WHERE reporter_id = $1 AND item_id = $2 AND account_id = $3 AND reason = $4 AND comment_id = $5 AND status = 'open'`,
		reporter, p.ItemID, p.AccountID, p.Reason, p.CommentID).Scan(&had)
	if err != nil {
		return fmt.Errorf("store: library report: %w", err)
	}
	if had > 0 {
		return nil
	}
	copyright := ""
	if p.Copyright != nil {
		data, err := json.Marshal(p.Copyright)
		if err != nil {
			return fmt.Errorf("store: library report: %w", err)
		}
		copyright = string(data)
	}
	_, err = s.ExecContext(ctx, `INSERT INTO library_reports (id, item_id, account_id, reporter_id, reason, detail, created, comment_id, copyright) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		newID(), p.ItemID, p.AccountID, reporter, p.Reason, p.Detail, stamp(now()), p.CommentID, copyright)
	if err != nil {
		return fmt.Errorf("store: library report: %w", err)
	}
	return nil
}

// libraryEscapeEvery is how long an item's frame saying it moved stays said, however
// that report was settled.
const libraryEscapeEvery = 6 * time.Hour

// AddLibraryEscape keeps the library's own report of item id's frame moving, unless
// it has one from the last while.
func (s *DB) AddLibraryEscape(ctx context.Context, id, detail string) error {
	var had int
	err := s.QueryRowContext(ctx, `SELECT COUNT(*) FROM library_reports WHERE item_id = $1 AND reason = 'escaped-preview' AND (status = 'open' OR created > $2)`, id, stamp(now().Add(-libraryEscapeEvery))).Scan(&had)
	if err != nil {
		return fmt.Errorf("store: library report: %w", err)
	}
	if had > 0 {
		return nil
	}
	return s.AddLibraryReport(ctx, "", api.LibraryReportParams{ItemID: id, Reason: "escaped-preview", Detail: detail})
}

// LibraryReportsToday counts what reporter reported in the last day.
func (s *DB) LibraryReportsToday(ctx context.Context, reporter string) (int, error) {
	var n int
	err := s.QueryRowContext(ctx, `SELECT COUNT(*) FROM library_reports WHERE reporter_id = $1 AND created > $2`, reporter, stamp(now().Add(-24*time.Hour))).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("store: library report: %w", err)
	}
	return n, nil
}

// LibraryOpenReports are the reports waiting, the oldest first: people's before the
// library's own, which anyone can cause.
func (s *DB) LibraryOpenReports(ctx context.Context, limit int) ([]api.LibraryReport, error) {
	rows, err := s.QueryContext(ctx, `SELECT r.id, r.item_id, COALESCE(i.title, ''), r.account_id, COALESCE(ap.handle, ''), COALESCE(rp.handle, ''), r.reason, r.detail, r.status, r.created, r.comment_id, COALESCE(c.body, ''), r.copyright
		FROM library_reports r LEFT JOIN library_items i ON i.id = r.item_id LEFT JOIN library_comments c ON c.id = r.comment_id
		LEFT JOIN library_profiles ap ON ap.account_id = r.account_id LEFT JOIN library_profiles rp ON rp.account_id = r.reporter_id
		WHERE r.status = 'open' ORDER BY CASE WHEN r.reason = 'escaped-preview' THEN 1 ELSE 0 END, r.created LIMIT $1`, max(min(limit, 100), 1))
	if err != nil {
		return nil, fmt.Errorf("store: library reports: %w", err)
	}
	defer rows.Close()
	out := []api.LibraryReport{}
	for rows.Next() {
		var r api.LibraryReport
		var created, copyright string
		if err := rows.Scan(&r.ID, &r.ItemID, &r.Item, &r.AccountID, &r.Account, &r.Reporter, &r.Reason, &r.Detail, &r.Status, &created, &r.CommentID, &r.Comment, &copyright); err != nil {
			return nil, err
		}
		r.Created = parseStamp(created)
		if copyright != "" {
			r.Copyright = &api.LibraryCopyright{}
			if err := json.Unmarshal([]byte(copyright), r.Copyright); err != nil {
				return nil, fmt.Errorf("store: library reports: %w", err)
			}
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// LibraryReportParty is who an open report's decision falls on.
type LibraryReportParty struct {
	ItemID string
	// Account is who's told when it's upheld: the item's owner, or whose comment.
	Account string
}

// LibraryReportTarget is open report id's target.
func (s *DB) LibraryReportTarget(ctx context.Context, id string) (LibraryReportParty, error) {
	var t LibraryReportParty
	var comment string
	err := s.QueryRowContext(ctx, `SELECT r.item_id, r.comment_id, CASE WHEN r.comment_id <> '' THEN r.account_id ELSE COALESCE(i.owner_id, r.account_id) END
		FROM library_reports r LEFT JOIN library_items i ON i.id = r.item_id WHERE r.id = $1 AND r.status = 'open'`, id).Scan(&t.ItemID, &comment, &t.Account)
	if errors.Is(err, sql.ErrNoRows) {
		return t, ErrNotFound
	}
	if err != nil {
		return t, fmt.Errorf("store: library report: %w", err)
	}
	return t, nil
}

// DecideLibraryReport settles an open report. Upheld, the item reported is
// taken down and its owner is new again, so what they send is reviewed; a
// comment reported goes alone, and the design it's on stays.
func (s *DB) DecideLibraryReport(ctx context.Context, actor, id string, uphold bool, reason string) error {
	tx, err := s.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var item, account, comment, why string
	err = tx.QueryRowContext(ctx, `SELECT item_id, account_id, comment_id, reason FROM library_reports WHERE id = $1 AND status = 'open'`, id).Scan(&item, &account, &comment, &why)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("store: library report: %w", err)
	}
	action, status := "report.dismissed", "dismissed"
	if uphold && comment != "" {
		action, status = "comment.report-upheld", "upheld"
		if err := removeLibraryComment(ctx, tx, comment); err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
	} else if uphold {
		action, status = "report.upheld", "upheld"
		if why == "copyright" {
			action = "report.upheld.copyright"
		}
		if item != "" {
			var owner string
			if err := tx.QueryRowContext(ctx, `SELECT owner_id FROM library_items WHERE id = $1`, item).Scan(&owner); err == nil {
				if err := removeLibraryItem(ctx, tx, item); err != nil && !errors.Is(err, ErrNotFound) {
					return err
				}
				account = owner
			}
		}
		if account != "" {
			if _, err := tx.ExecContext(ctx, `UPDATE library_profiles SET trust = 'new' WHERE account_id = $1 AND trust = 'trusted'`, account); err != nil {
				return fmt.Errorf("store: library report: %w", err)
			}
		}
	}
	if err := libraryLog(ctx, tx, actor, action, item, account, 0, reason); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE library_reports SET status = $1, decision_id = (SELECT MAX(id) FROM library_log) WHERE id = $2`, status, id); err != nil {
		return fmt.Errorf("store: library report: %w", err)
	}
	return tx.Commit()
}

// BlockLibraryAccount stops an account from publishing and takes what it
// shows down; undone, it starts again as new, what was taken down stays so.
func (s *DB) BlockLibraryAccount(ctx context.Context, actor, accID string, block bool, reason string) error {
	if _, err := s.LibraryProfile(ctx, accID); err != nil {
		return err
	}
	tx, err := s.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	trust, action := api.TrustNew, "account.unblocked"
	if block {
		trust, action = api.TrustBlocked, "account.blocked"
		at := now()
		if _, err := tx.ExecContext(ctx, `UPDATE library_items SET status = 'removed', featured_at = '', purge_at = $1, updated = $2 WHERE owner_id = $3 AND status IN ('live', 'review')`,
			stamp(at.Add(LibraryPurgeAfter)), stamp(at), accID); err != nil {
			return fmt.Errorf("store: library block: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE library_profiles SET trust = $1 WHERE account_id = $2`, trust, accID); err != nil {
		return fmt.Errorf("store: library block: %w", err)
	}
	if err := libraryLog(ctx, tx, actor, action, "", accID, 0, reason); err != nil {
		return err
	}
	return tx.Commit()
}

// IsLibraryModerator says whether accID was appointed one.
func (s *DB) IsLibraryModerator(ctx context.Context, accID string) (bool, error) {
	var n int
	err := s.QueryRowContext(ctx, `SELECT COUNT(*) FROM library_moderators WHERE account_id = $1`, accID).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("store: library moderator: %w", err)
	}
	return n > 0, nil
}

// LibraryModerators are the appointed moderators, as the owner's list shows.
func (s *DB) LibraryModerators(ctx context.Context) ([]api.LibraryModerator, error) {
	rows, err := s.QueryContext(ctx, `SELECT a.id, COALESCE(p.handle, ''), a.name, a.avatar, a.github_login, COALESCE(bp.handle, m.appointed_by), m.created
		FROM library_moderators m JOIN accounts a ON a.id = m.account_id
		LEFT JOIN library_profiles p ON p.account_id = m.account_id LEFT JOIN library_profiles bp ON bp.account_id = m.appointed_by ORDER BY m.created`)
	if err != nil {
		return nil, fmt.Errorf("store: library moderators: %w", err)
	}
	defer rows.Close()
	out := []api.LibraryModerator{}
	for rows.Next() {
		var m api.LibraryModerator
		var at string
		if err := rows.Scan(&m.ID, &m.Handle, &m.Name, &m.Avatar, &m.GitHub, &m.AppointedBy, &at); err != nil {
			return nil, err
		}
		m.Since = parseStamp(at)
		out = append(out, m)
	}
	return out, rows.Err()
}

// AppointLibraryModerator makes accID a moderator, recorded in the log.
func (s *DB) AppointLibraryModerator(ctx context.Context, actor, accID, reason string) error {
	if _, err := s.LibraryProfile(ctx, accID); err != nil {
		return err
	}
	tx, err := s.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `INSERT INTO library_moderators (account_id, appointed_by, created) VALUES ($1, $2, $3) ON CONFLICT (account_id) DO NOTHING`, accID, actor, stamp(now()))
	if err != nil {
		return fmt.Errorf("store: library moderator: %w", err)
	}
	if done, _ := res.RowsAffected(); done == 0 {
		return errors.New("store: they are a moderator already")
	}
	if err := libraryLog(ctx, tx, actor, "moderator.appointed", "", accID, 0, reason); err != nil {
		return err
	}
	return tx.Commit()
}

// RemoveLibraryModerator takes accID's moderating away, recorded in the log.
func (s *DB) RemoveLibraryModerator(ctx context.Context, actor, accID, reason string) error {
	tx, err := s.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `DELETE FROM library_moderators WHERE account_id = $1`, accID)
	if err != nil {
		return fmt.Errorf("store: library moderator: %w", err)
	}
	if done, _ := res.RowsAffected(); done == 0 {
		return ErrNotFound
	}
	if err := libraryLog(ctx, tx, actor, "moderator.removed", "", accID, 0, reason); err != nil {
		return err
	}
	return tx.Commit()
}

// LibraryAccounts are the accounts whose GitHub login, name or handle starts
// with q, for the owner to pick a moderator from. Never their emails.
func (s *DB) LibraryAccounts(ctx context.Context, q string) ([]api.LibraryAccount, error) {
	like := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(strings.ToLower(strings.TrimSpace(q))) + "%"
	rows, err := s.QueryContext(ctx, `SELECT a.id, COALESCE(p.handle, ''), a.name, a.avatar, a.github_login FROM accounts a LEFT JOIN library_profiles p ON p.account_id = a.id
		WHERE LOWER(a.github_login) LIKE $1 ESCAPE '\' OR LOWER(a.name) LIKE $1 ESCAPE '\' OR LOWER(COALESCE(p.handle, '')) LIKE $1 ESCAPE '\' ORDER BY a.github_login LIMIT 20`, like)
	if err != nil {
		return nil, fmt.Errorf("store: library accounts: %w", err)
	}
	defer rows.Close()
	out := []api.LibraryAccount{}
	for rows.Next() {
		var a api.LibraryAccount
		if err := rows.Scan(&a.ID, &a.Handle, &a.Name, &a.Avatar, &a.GitHub); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// libraryTokenPrefix marks a library session's token, so a leaked one is
// recognizable and never taken for a device's.
const libraryTokenPrefix = "enxl_"

// librarySessionLife is how long a browser stays signed in to the library.
const librarySessionLife = 30 * 24 * time.Hour

// libraryCodeLife is how long a sign-in's code can be traded.
const libraryCodeLife = time.Minute

func libraryHash(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

func randomLibraryToken() string {
	var raw [32]byte
	rand.Read(raw[:])
	return base64.RawURLEncoding.EncodeToString(raw[:])
}

// IsLibraryToken says token is a library session's, not a device's.
func IsLibraryToken(token string) bool { return strings.HasPrefix(token, libraryTokenPrefix) }

// AddLibraryCode keeps a one-time code for accID, to be traded with the
// verifier of challenge (PKCE), and returns it.
func (s *DB) AddLibraryCode(ctx context.Context, accID, challenge string) (string, error) {
	s.ExecContext(ctx, `DELETE FROM library_auth_codes WHERE expires < $1`, stamp(now()))
	code := randomLibraryToken()
	_, err := s.ExecContext(ctx, `INSERT INTO library_auth_codes (hash, account_id, challenge, expires) VALUES ($1, $2, $3, $4)`,
		libraryHash(code), accID, challenge, stamp(now().Add(libraryCodeLife)))
	if err != nil {
		return "", fmt.Errorf("store: library code: %w", err)
	}
	return code, nil
}

// TakeLibraryCode uses code up, whether or not the caller then accepts it,
// and says whose it was and its challenge. ErrExpired when it can't be used.
func (s *DB) TakeLibraryCode(ctx context.Context, code string) (accID, challenge string, err error) {
	err = s.QueryRowContext(ctx, `DELETE FROM library_auth_codes WHERE hash = $1 AND expires > $2 RETURNING account_id, challenge`, libraryHash(code), stamp(now())).Scan(&accID, &challenge)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", ErrExpired
	}
	if err != nil {
		return "", "", fmt.Errorf("store: library code: %w", err)
	}
	return accID, challenge, nil
}

// StartLibrarySession signs accID in to the library alone.
func (s *DB) StartLibrarySession(ctx context.Context, accID string) (string, time.Time, error) {
	s.ExecContext(ctx, `DELETE FROM library_sessions WHERE expires < $1`, stamp(now()))
	token := libraryTokenPrefix + randomLibraryToken()
	expires := now().Add(librarySessionLife)
	_, err := s.ExecContext(ctx, `INSERT INTO library_sessions (hash, account_id, created, expires) VALUES ($1, $2, $3, $4)`, libraryHash(token), accID, stamp(now()), stamp(expires))
	if err != nil {
		return "", expires, fmt.Errorf("store: library session: %w", err)
	}
	return token, expires, nil
}

// LibrarySessionAccount is the account of a session's token, ErrNotFound
// when it isn't one, or no longer.
func (s *DB) LibrarySessionAccount(ctx context.Context, token string) (api.Account, error) {
	var accID string
	err := s.QueryRowContext(ctx, `SELECT account_id FROM library_sessions WHERE hash = $1 AND expires > $2`, libraryHash(token), stamp(now())).Scan(&accID)
	if errors.Is(err, sql.ErrNoRows) {
		return api.Account{}, ErrNotFound
	}
	if err != nil {
		return api.Account{}, fmt.Errorf("store: library session: %w", err)
	}
	return s.Account(ctx, accID)
}

// EndLibrarySession signs a browser out of the library.
func (s *DB) EndLibrarySession(ctx context.Context, token string) error {
	_, err := s.ExecContext(ctx, `DELETE FROM library_sessions WHERE hash = $1`, libraryHash(token))
	if err != nil {
		return fmt.Errorf("store: library session: %w", err)
	}
	return nil
}

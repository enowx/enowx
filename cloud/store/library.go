package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/enowx/enowx/cloud/api"
)

// The library's items and versions (cloud/library decides what may happen;
// this keeps it). The rows hold no secret: a design's content is in the
// library's bucket.

// LibraryVersionRow is a version as the database holds it.
type LibraryVersionRow struct {
	ItemID                                                       string
	Version                                                      int
	Status, Kind, License, Format, Prompt                        string
	SourceSize, DocSize, DocPhoneSize, ThumbSize, ThumbPhoneSize int64
	Width, Height                                                int
	PromptApproved, Rights                                       string
	Scan                                                         string
	// Meta is what the version says of the item (LibraryMeta): what shows
	// changes only as the version goes live, so a review sees it first.
	Meta    string
	Created time.Time
}

// Size is all the version keeps in the bucket.
func (v LibraryVersionRow) Size() int64 {
	return v.SourceSize + v.DocSize + v.DocPhoneSize + v.ThumbSize + v.ThumbPhoneSize
}

// API is v as the API shows it.
func (v LibraryVersionRow) API() api.LibraryVersion {
	out := api.LibraryVersion{Version: v.Version, Status: v.Status, Kind: v.Kind, License: v.License, Width: v.Width, Height: v.Height,
		HasPhone: v.DocPhoneSize > 0, Size: v.Size(), Created: v.Created, Prompt: v.Prompt}
	json.Unmarshal([]byte(v.Scan), &out.Scan)
	return out
}

const libraryItemCols = `i.id, i.slug, i.kind, i.license, i.title, i.description, i.category, i.style, i.theme, i.device, i.has_motion,
	i.made_with, i.source_url, i.status, i.live_version, i.reject_reason, i.saves, i.featured_at, i.created, i.updated,
	a.id, COALESCE(p.handle, ''), a.name, a.avatar,
	COALESCE(v.width, 0), COALESCE(v.height, 0), COALESCE(v.doc_phone_size, 0)`

const libraryItemFrom = ` FROM library_items i JOIN accounts a ON a.id = i.owner_id LEFT JOIN library_profiles p ON p.account_id = i.owner_id
	LEFT JOIN library_versions v ON v.item_id = i.id AND v.version = i.live_version`

type scanner interface{ Scan(dest ...any) error }

func scanLibraryItem(r scanner) (api.LibraryItem, error) {
	var it api.LibraryItem
	var motion int
	var featured, created, updated string
	var phone int64
	err := r.Scan(&it.ID, &it.Slug, &it.Kind, &it.License, &it.Title, &it.Description, &it.Category, &it.Style, &it.Theme, &it.Device, &motion,
		&it.MadeWith, &it.SourceURL, &it.Status, &it.LiveVersion, &it.RejectReason, &it.Saves, &featured, &created, &updated,
		&it.Creator.ID, &it.Creator.Handle, &it.Creator.Name, &it.Creator.Avatar, &it.Width, &it.Height, &phone)
	it.HasMotion, it.Featured, it.HasPhone = motion != 0, featured != "", phone > 0
	it.Created, it.Updated = parseStamp(created), parseStamp(updated)
	it.Tags = []string{}
	return it, err
}

// libraryTags fills items' tags in one query.
func (s *DB) libraryTags(ctx context.Context, items []api.LibraryItem) error {
	if len(items) == 0 {
		return nil
	}
	args := make([]any, len(items))
	marks := make([]string, len(items))
	at := map[string]int{}
	for i, it := range items {
		args[i], marks[i], at[it.ID] = it.ID, "$"+strconv.Itoa(i+1), i
	}
	rows, err := s.QueryContext(ctx, `SELECT item_id, tag FROM library_tags WHERE item_id IN (`+strings.Join(marks, ",")+`) ORDER BY tag`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, tag string
		if err := rows.Scan(&id, &tag); err != nil {
			return err
		}
		items[at[id]].Tags = append(items[at[id]].Tags, tag)
	}
	return rows.Err()
}

// LibraryQuery is what a browse asks for.
type LibraryQuery struct {
	Q, Category, Kind, License, Tag, Sort string
	Style, Color, Theme, Device           string
	Motion                                bool
	Limit, Offset                         int
}

// LibraryItems are live items, as q asks; more says there are others after. A
// search ranks by how well it matches unless q asks for another order.
func (s *DB) LibraryItems(ctx context.Context, q LibraryQuery) (items []api.LibraryItem, more bool, err error) {
	a := &queryArgs{}
	where := libraryFilters(q, a)
	rank := ""
	if text := strings.TrimSpace(q.Q); text != "" {
		var match string
		match, rank = s.librarySearch(text, a)
		where = append(where, match)
	}
	order := `i.created DESC, i.id DESC`
	switch q.Sort {
	case "popular":
		order = `i.saves DESC, i.created DESC, i.id DESC`
	case "featured":
		where = append(where, `i.featured_at <> ''`)
		order = `i.featured_at DESC, i.id DESC`
	case "new":
	default:
		if rank != "" {
			order = rank + ` DESC, ` + order
		}
	}
	limit := max(min(q.Limit, 60), 1)
	rows, err := s.QueryContext(ctx, `SELECT `+libraryItemCols+libraryItemFrom+` WHERE `+strings.Join(where, " AND ")+
		` ORDER BY `+order+` LIMIT `+strconv.Itoa(limit+1)+` OFFSET `+strconv.Itoa(max(q.Offset, 0)), a.list...)
	if err != nil {
		return nil, false, fmt.Errorf("store: library items: %w", err)
	}
	if items, err = s.readLibraryItems(ctx, rows); err != nil {
		return nil, false, err
	}
	if len(items) > limit {
		items, more = items[:limit], true
	}
	return items, more, nil
}

// LibraryItem is item id, whatever its status.
func (s *DB) LibraryItem(ctx context.Context, id string) (api.LibraryItem, error) {
	it, err := scanLibraryItem(s.QueryRowContext(ctx, `SELECT `+libraryItemCols+libraryItemFrom+` WHERE i.id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return it, ErrNotFound
	}
	if err != nil {
		return it, fmt.Errorf("store: library item: %w", err)
	}
	list := []api.LibraryItem{it}
	err = s.libraryTags(ctx, list)
	return list[0], err
}

const libraryVersionCols = `item_id, version, status, kind, license, format, prompt_text, source_size, doc_size, doc_phone_size, thumb_size, thumb_phone_size,
	width, height, prompt_approved_at, rights_at, scan, meta, created`

func scanLibraryVersion(r scanner) (LibraryVersionRow, error) {
	var v LibraryVersionRow
	var created string
	err := r.Scan(&v.ItemID, &v.Version, &v.Status, &v.Kind, &v.License, &v.Format, &v.Prompt, &v.SourceSize, &v.DocSize, &v.DocPhoneSize, &v.ThumbSize, &v.ThumbPhoneSize,
		&v.Width, &v.Height, &v.PromptApproved, &v.Rights, &v.Scan, &v.Meta, &created)
	v.Created = parseStamp(created)
	return v, err
}

// LibraryVersion is version n of item id.
func (s *DB) LibraryVersion(ctx context.Context, id string, n int) (LibraryVersionRow, error) {
	v, err := scanLibraryVersion(s.QueryRowContext(ctx, `SELECT `+libraryVersionCols+` FROM library_versions WHERE item_id = $1 AND version = $2`, id, n))
	if errors.Is(err, sql.ErrNoRows) {
		return v, ErrNotFound
	}
	if err != nil {
		return v, fmt.Errorf("store: library version: %w", err)
	}
	return v, nil
}

// LibraryVersions are item id's versions, the latest first.
func (s *DB) LibraryVersions(ctx context.Context, id string) ([]LibraryVersionRow, error) {
	rows, err := s.QueryContext(ctx, `SELECT `+libraryVersionCols+` FROM library_versions WHERE item_id = $1 ORDER BY version DESC`, id)
	if err != nil {
		return nil, fmt.Errorf("store: library versions: %w", err)
	}
	defer rows.Close()
	out := []LibraryVersionRow{}
	for rows.Next() {
		v, err := scanLibraryVersion(rows)
		if err != nil {
			return nil, fmt.Errorf("store: library versions: %w", err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// LibraryOwner is who item id belongs to.
func (s *DB) LibraryOwner(ctx context.Context, id string) (string, error) {
	var owner string
	err := s.QueryRowContext(ctx, `SELECT owner_id FROM library_items WHERE id = $1`, id).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return owner, err
}

var handleJunk = regexp.MustCompile(`[^a-z0-9-]+`)

// LibraryProfile is accID's profile, made the first time it's asked for,
// its handle their GitHub login, or that and digits when taken.
func (s *DB) LibraryProfile(ctx context.Context, accID string) (api.LibraryMe, error) {
	var me api.LibraryMe
	err := s.QueryRowContext(ctx, `SELECT a.id, p.handle, a.name, a.avatar, p.trust FROM library_profiles p JOIN accounts a ON a.id = p.account_id WHERE p.account_id = $1`, accID).
		Scan(&me.Account.ID, &me.Account.Handle, &me.Account.Name, &me.Account.Avatar, &me.Trust)
	if err == nil {
		return me, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return me, fmt.Errorf("store: library profile: %w", err)
	}
	acc, err := s.Account(ctx, accID)
	if err != nil {
		return me, err
	}
	base := strings.Trim(handleJunk.ReplaceAllString(strings.ToLower(acc.GitHub), "-"), "-")
	if base == "" {
		base = "creator"
	}
	handle := base
	for try := 2; try < 1000; try++ {
		_, err := s.ExecContext(ctx, `INSERT INTO library_profiles (account_id, handle, created) VALUES ($1, $2, $3)`, accID, handle, stamp(now()))
		if err == nil {
			me.Account = api.LibraryCreator{ID: accID, Handle: handle, Name: acc.Name, Avatar: acc.Avatar}
			me.Trust = api.TrustNew
			return me, nil
		}
		if !isUnique(err) {
			return me, fmt.Errorf("store: library profile: %w", err)
		}
		// Made by a request at the same time, or the handle is someone else's.
		var had string
		if s.QueryRowContext(ctx, `SELECT handle FROM library_profiles WHERE account_id = $1`, accID).Scan(&had) == nil {
			return s.LibraryProfile(ctx, accID)
		}
		handle = base + "-" + strconv.Itoa(try)
	}
	return me, errors.New("store: no handle left for this account")
}

// LibraryMeta is what a version says of its item.
type LibraryMeta struct {
	Title, Description, Category, Style, Theme, Device, MadeWith, SourceURL string
	Tags                                                                    []string
	HasMotion                                                               bool
}

// LibraryNewItem is a new item's row and first version.
type LibraryNewItem struct {
	Owner, Slug, Kind, License, Title, Description, Category string
	Style, Theme, Device, MadeWith, SourceURL                string
	Tags                                                     []string
	HasMotion                                                bool
	Version                                                  LibraryVersionRow
}

// AddLibraryItem makes an item, not public yet, its first version awaiting
// its objects.
func (s *DB) AddLibraryItem(ctx context.Context, n LibraryNewItem) (string, error) {
	id, at := newID(), stamp(now())
	tx, err := s.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO library_items (id, owner_id, slug, kind, license, title, description, category, style, theme, device, has_motion, made_with, source_url, status, created, updated)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)`,
		id, n.Owner, n.Slug, n.Kind, n.License, n.Title, n.Description, n.Category, n.Style, n.Theme, n.Device, boolInt(n.HasMotion), n.MadeWith, n.SourceURL, api.LibraryUploading, at, at); err != nil {
		return "", fmt.Errorf("store: library item: %w", err)
	}
	if err := setLibraryTags(ctx, tx, id, n.Tags); err != nil {
		return "", err
	}
	if err := insertLibraryVersion(ctx, tx, id, 1, n.Version, at); err != nil {
		return "", err
	}
	return id, tx.Commit()
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func setLibraryTags(ctx context.Context, tx *sql.Tx, id string, tags []string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM library_tags WHERE item_id = $1`, id); err != nil {
		return fmt.Errorf("store: library tags: %w", err)
	}
	for _, tag := range tags {
		if _, err := tx.ExecContext(ctx, `INSERT INTO library_tags (item_id, tag) VALUES ($1, $2)`, id, tag); err != nil {
			return fmt.Errorf("store: library tags: %w", err)
		}
	}
	// Search reads them from the item.
	if _, err := tx.ExecContext(ctx, `UPDATE library_items SET tags_text = $1 WHERE id = $2`, strings.Join(tags, " "), id); err != nil {
		return fmt.Errorf("store: library tags: %w", err)
	}
	return nil
}

func insertLibraryVersion(ctx context.Context, tx *sql.Tx, id string, n int, v LibraryVersionRow, at string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO library_versions (item_id, version, status, kind, license, width, height, format, prompt_text, prompt_approved_at, rights_at, meta, created)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		id, n, api.LibraryUploading, v.Kind, v.License, v.Width, v.Height, v.Format, v.Prompt, at, at, v.Meta, at)
	if err != nil {
		return fmt.Errorf("store: library version: %w", err)
	}
	return nil
}

// AddLibraryVersion adds item id's next version, awaiting its objects; an
// earlier one still uploading is replaced by being reused. It says which.
func (s *DB) AddLibraryVersion(ctx context.Context, id string, v LibraryVersionRow) (int, error) {
	tx, err := s.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var last int
	var status string
	err = tx.QueryRowContext(ctx, `SELECT version, status FROM library_versions WHERE item_id = $1 ORDER BY version DESC LIMIT 1`, id).Scan(&last, &status)
	if err != nil {
		return 0, fmt.Errorf("store: library version: %w", err)
	}
	if status == api.LibraryUploading {
		_, err := tx.ExecContext(ctx, `UPDATE library_versions SET kind = $1, license = $2, width = $3, height = $4, format = $5, prompt_text = $6, prompt_approved_at = $7, rights_at = $7, meta = $8 WHERE item_id = $9 AND version = $10`,
			v.Kind, v.License, v.Width, v.Height, v.Format, v.Prompt, stamp(now()), v.Meta, id, last)
		if err != nil {
			return 0, fmt.Errorf("store: library version: %w", err)
		}
		return last, tx.Commit()
	}
	if err := insertLibraryVersion(ctx, tx, id, last+1, v, stamp(now())); err != nil {
		return 0, err
	}
	return last + 1, tx.Commit()
}

// SetLibraryPartSize records the size of a version's object, what the
// owner's account keeps changing with it, in one step: a quota can't be
// raced past by uploads at once.
func (s *DB) SetLibraryPartSize(ctx context.Context, owner, id string, n int, part string, size, accountLimit int64) error {
	col, ok := map[string]string{api.PartSource: "source_size", api.PartDoc: "doc_size", api.PartDocPhone: "doc_phone_size", api.PartThumb: "thumb_size", api.PartThumbPhone: "thumb_phone_size"}[part]
	if !ok {
		return errors.New("store: no such part")
	}
	tx, err := s.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var was int64
	if err := tx.QueryRowContext(ctx, `SELECT `+col+` FROM library_versions WHERE item_id = $1 AND version = $2`, id, n).Scan(&was); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("store: library part: %w", err)
	}
	res, err := tx.ExecContext(ctx, `UPDATE library_profiles SET used_bytes = used_bytes + $1 WHERE account_id = $2 AND used_bytes + $3 <= $4`, size-was, owner, size-was, accountLimit)
	if err != nil {
		return fmt.Errorf("store: library part: %w", err)
	}
	if done, _ := res.RowsAffected(); done == 0 {
		return ErrLibraryQuota
	}
	if _, err := tx.ExecContext(ctx, `UPDATE library_versions SET `+col+` = $1 WHERE item_id = $2 AND version = $3`, size, id, n); err != nil {
		return fmt.Errorf("store: library part: %w", err)
	}
	return tx.Commit()
}

// ErrLibraryQuota is an account's storage full.
var ErrLibraryQuota = errors.New("store: this account's library storage is full")

// LibraryCounts are what an account's limits are counted from.
type LibraryCounts struct {
	Live, InReview, UploadsToday int
	Used                         int64
}

// LibraryCounts counts accID's items.
func (s *DB) LibraryCounts(ctx context.Context, accID string) (LibraryCounts, error) {
	var c LibraryCounts
	err := s.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM library_items WHERE owner_id = $1 AND status = 'live'),
		(SELECT COUNT(*) FROM library_items WHERE owner_id = $1 AND status = 'review'),
		(SELECT COUNT(*) FROM library_versions v JOIN library_items i ON i.id = v.item_id WHERE i.owner_id = $1 AND v.created > $2),
		COALESCE((SELECT used_bytes FROM library_profiles WHERE account_id = $1), 0)`,
		accID, stamp(now().Add(-24*time.Hour))).Scan(&c.Live, &c.InReview, &c.UploadsToday, &c.Used)
	if err != nil {
		return c, fmt.Errorf("store: library counts: %w", err)
	}
	return c, nil
}

// SubmitLibraryVersion settles a version sent whole: status its new status
// (review or live), its check's findings recorded, the item following it.
func (s *DB) SubmitLibraryVersion(ctx context.Context, actor, id string, n int, status, scan string) error {
	tx, err := s.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE library_versions SET status = $1, scan = $2 WHERE item_id = $3 AND version = $4 AND status = 'uploading'`, status, scan, id, n)
	if err != nil {
		return fmt.Errorf("store: library submit: %w", err)
	}
	if done, _ := res.RowsAffected(); done == 0 {
		return errors.New("store: this version was submitted already")
	}
	at := stamp(now())
	if status == api.LibraryLive {
		if err := goLive(ctx, tx, id, n, at); err != nil {
			return err
		}
	} else if _, err := tx.ExecContext(ctx, `UPDATE library_items SET status = CASE WHEN status = 'live' THEN status ELSE 'review' END, reject_reason = '', updated = $1 WHERE id = $2`, at, id); err != nil {
		return fmt.Errorf("store: library submit: %w", err)
	}
	if err := libraryLog(ctx, tx, actor, "version.submitted", id, "", n, "sent as "+status); err != nil {
		return err
	}
	return tx.Commit()
}

// goLive makes version n what item id shows, with that version's kind and
// license, the one it replaces superseded.
func goLive(ctx context.Context, tx *sql.Tx, id string, n int, at string) error {
	var meta string
	if err := tx.QueryRowContext(ctx, `SELECT meta FROM library_versions WHERE item_id = $1 AND version = $2`, id, n).Scan(&meta); err != nil {
		return fmt.Errorf("store: library live: %w", err)
	}
	var m LibraryMeta
	if err := json.Unmarshal([]byte(meta), &m); err != nil {
		return fmt.Errorf("store: library live: %w", err)
	}
	if m.Title != "" {
		_, err := tx.ExecContext(ctx, `UPDATE library_items SET title = $1, description = $2, category = $3, style = $4, theme = $5, device = $6, has_motion = $7, made_with = $8, source_url = $9 WHERE id = $10`,
			m.Title, m.Description, m.Category, m.Style, m.Theme, m.Device, boolInt(m.HasMotion), m.MadeWith, m.SourceURL, id)
		if err != nil {
			return fmt.Errorf("store: library live: %w", err)
		}
		if err := setLibraryTags(ctx, tx, id, m.Tags); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE library_versions SET status = 'superseded' WHERE item_id = $1 AND status = 'live' AND version <> $2`, id, n); err != nil {
		return fmt.Errorf("store: library live: %w", err)
	}
	_, err := tx.ExecContext(ctx, `UPDATE library_items SET status = 'live', live_version = $1, reject_reason = '', purge_at = '', updated = $2,
		kind = (SELECT kind FROM library_versions WHERE item_id = $3 AND version = $1),
		license = (SELECT license FROM library_versions WHERE item_id = $3 AND version = $1) WHERE id = $3`, n, at, id)
	if err != nil {
		return fmt.Errorf("store: library live: %w", err)
	}
	return nil
}

// LibraryPurgeAfter is how long a taken-back item's objects stay.
const LibraryPurgeAfter = 30 * 24 * time.Hour

// WithdrawLibraryItem is the owner taking item id back: its address answers
// 410 from now on, its objects go after LibraryPurgeAfter.
func (s *DB) WithdrawLibraryItem(ctx context.Context, actor, id, reason string) error {
	tx, err := s.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	at := now()
	res, err := tx.ExecContext(ctx, `UPDATE library_items SET status = CASE WHEN live_version > 0 THEN 'unpublished' ELSE 'withdrawn' END, featured_at = '', purge_at = $1, updated = $2
		WHERE id = $3 AND status NOT IN ('withdrawn', 'unpublished', 'removed')`, stamp(at.Add(LibraryPurgeAfter)), stamp(at), id)
	if err != nil {
		return fmt.Errorf("store: library withdraw: %w", err)
	}
	if done, _ := res.RowsAffected(); done == 0 {
		return ErrNotFound
	}
	if err := libraryLog(ctx, tx, actor, "item.withdrawn", id, "", 0, reason); err != nil {
		return err
	}
	return tx.Commit()
}

// LibraryPurge is an item's version's objects to delete.
type LibraryPurge struct {
	ItemID  string
	Version int
}

// LibraryPurgeDue are the versions whose objects have to go now: of items
// past their purge time, and uploads abandoned for a day.
func (s *DB) LibraryPurgeDue(ctx context.Context) ([]LibraryPurge, error) {
	rows, err := s.QueryContext(ctx, `SELECT v.item_id, v.version FROM library_versions v JOIN library_items i ON i.id = v.item_id
		WHERE (i.purge_at <> '' AND i.purge_at < $1) OR (v.status = 'uploading' AND v.created < $2) LIMIT 200`, stamp(now()), stamp(now().Add(-24*time.Hour)))
	if err != nil {
		return nil, fmt.Errorf("store: library purge: %w", err)
	}
	defer rows.Close()
	var out []LibraryPurge
	for rows.Next() {
		var p LibraryPurge
		if err := rows.Scan(&p.ItemID, &p.Version); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ForgetLibraryVersion drops a version whose objects are gone, and what its
// owner's account kept for it; an item without versions goes with it.
func (s *DB) ForgetLibraryVersion(ctx context.Context, p LibraryPurge) error {
	tx, err := s.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var size int64
	var owner string
	err = tx.QueryRowContext(ctx, `SELECT v.source_size + v.doc_size + v.doc_phone_size + v.thumb_size + v.thumb_phone_size, i.owner_id
		FROM library_versions v JOIN library_items i ON i.id = v.item_id WHERE v.item_id = $1 AND v.version = $2`, p.ItemID, p.Version).Scan(&size, &owner)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("store: library forget: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM library_versions WHERE item_id = $1 AND version = $2`, p.ItemID, p.Version); err != nil {
		return fmt.Errorf("store: library forget: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE library_profiles SET used_bytes = CASE WHEN used_bytes > $1 THEN used_bytes - $1 ELSE 0 END WHERE account_id = $2`, size, owner); err != nil {
		return fmt.Errorf("store: library forget: %w", err)
	}
	// A taken-back item keeps its row, so its address goes on answering 410;
	// one that never was submitted goes.
	if _, err := tx.ExecContext(ctx, `DELETE FROM library_items WHERE id = $1 AND status = 'uploading' AND NOT EXISTS (SELECT 1 FROM library_versions WHERE item_id = $1)`, p.ItemID); err != nil {
		return fmt.Errorf("store: library forget: %w", err)
	}
	return tx.Commit()
}

// LibraryMine are accID's items, the latest changed first, each with its
// latest version.
func (s *DB) LibraryMine(ctx context.Context, accID string) ([]api.LibraryMineItem, error) {
	rows, err := s.QueryContext(ctx, `SELECT `+libraryItemCols+libraryItemFrom+` WHERE i.owner_id = $1 ORDER BY i.updated DESC LIMIT 500`, accID)
	if err != nil {
		return nil, fmt.Errorf("store: library mine: %w", err)
	}
	defer rows.Close()
	var items []api.LibraryItem
	for rows.Next() {
		it, err := scanLibraryItem(rows)
		if err != nil {
			return nil, fmt.Errorf("store: library mine: %w", err)
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	if err := s.libraryTags(ctx, items); err != nil {
		return nil, err
	}
	out := make([]api.LibraryMineItem, 0, len(items))
	for _, it := range items {
		m := api.LibraryMineItem{LibraryItem: it}
		versions, err := s.LibraryVersions(ctx, it.ID)
		if err != nil {
			return nil, err
		}
		if len(versions) > 0 {
			m.Latest = versions[0].API()
		}
		out = append(out, m)
	}
	return out, nil
}

// AddLibraryUsage counts bytes written to the bucket in this month.
func (s *DB) AddLibraryUsage(ctx context.Context, written int64) error {
	month := now().Format("2006-01")
	_, err := s.ExecContext(ctx, `INSERT INTO library_usage (month, bytes_written, write_ops) VALUES ($1, $2, 1)
		ON CONFLICT (month) DO UPDATE SET bytes_written = library_usage.bytes_written + $2, write_ops = library_usage.write_ops + 1`, month, written)
	if err != nil {
		return fmt.Errorf("store: library usage: %w", err)
	}
	return nil
}

// LibraryUsage is this month's use of the bucket, what all accounts keep
// counted as it's asked.
func (s *DB) LibraryUsage(ctx context.Context) (api.LibraryUsage, error) {
	u := api.LibraryUsage{Month: now().Format("2006-01"), StoredLimit: api.MaxLibraryBucket, WrittenLimit: api.MaxLibraryMonthWrite}
	err := s.QueryRowContext(ctx, `SELECT bytes_written, write_ops FROM library_usage WHERE month = $1`, u.Month).Scan(&u.BytesWritten, &u.WriteOps)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return u, fmt.Errorf("store: library usage: %w", err)
	}
	if err := s.QueryRowContext(ctx, `SELECT COALESCE(SUM(used_bytes), 0) FROM library_profiles`).Scan(&u.BytesStored); err != nil {
		return u, fmt.Errorf("store: library usage: %w", err)
	}
	if _, err := s.ExecContext(ctx, `INSERT INTO library_usage (month, bytes_stored) VALUES ($1, $2) ON CONFLICT (month) DO UPDATE SET bytes_stored = $2`, u.Month, u.BytesStored); err != nil {
		return u, fmt.Errorf("store: library usage: %w", err)
	}
	u.Warn = u.BytesStored*100 >= u.StoredLimit*api.LibraryWarnPercent || u.BytesWritten*100 >= u.WrittenLimit*api.LibraryWarnPercent
	return u, nil
}

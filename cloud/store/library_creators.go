package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/enowx/enowx/cloud/api"
)

// A library's creators: profiles, the open for work list, the contributor badge.
// cloud/library checks what a profile may say; this keeps it.

// LibraryProfileRow is a profile as the database holds it.
type LibraryProfileRow struct {
	AccountID, Handle, Name, Avatar, GitHub string
	Bio                                     string
	Links                                   []string
	Hidden, ShowGitHub, ShowStats           bool
	OpenForWork                             bool
	WorkKinds, Skills                       []string
	TimeZone, ContactURL                    string
	Trust                                   string
	Contributor                             bool
	Created, HandleChanged                  time.Time
}

const libraryProfileCols = `p.account_id, p.handle, a.name, a.avatar, a.github_login, p.bio, p.links, p.hidden, p.show_github, p.show_stats,
	p.open_for_work, p.work_kinds, p.skills, p.timezone, p.contact_url, p.trust, p.created, p.handle_changed,
	CASE WHEN EXISTS (SELECT 1 FROM github_contributors c WHERE c.github_id = a.github_id) THEN 1 ELSE 0 END`

const libraryProfileFrom = ` FROM library_profiles p JOIN accounts a ON a.id = p.account_id`

func scanLibraryProfile(r scanner) (LibraryProfileRow, error) {
	var p LibraryProfileRow
	var links, kinds, skills, created, changed string
	var hidden, github, stats, open, contributor int
	err := r.Scan(&p.AccountID, &p.Handle, &p.Name, &p.Avatar, &p.GitHub, &p.Bio, &links, &hidden, &github, &stats,
		&open, &kinds, &skills, &p.TimeZone, &p.ContactURL, &p.Trust, &created, &changed, &contributor)
	if err != nil {
		return p, err
	}
	p.Hidden, p.ShowGitHub, p.ShowStats, p.OpenForWork, p.Contributor = hidden != 0, github != 0, stats != 0, open != 0, contributor != 0
	p.Created = parseStamp(created)
	if changed != "" {
		p.HandleChanged = parseStamp(changed)
	}
	p.Links, p.WorkKinds, p.Skills = stringList(links), stringList(kinds), stringList(skills)
	return p, nil
}

func stringList(raw string) []string {
	out := []string{}
	json.Unmarshal([]byte(raw), &out)
	return out
}

func jsonList(list []string) string {
	if list == nil {
		list = []string{}
	}
	data, _ := json.Marshal(list)
	return string(data)
}

// LibraryCreator is the profile of handle, ErrNotFound when no one has it. It says
// nothing of whether it may show: Hidden and Trust are the caller's to read.
func (s *DB) LibraryCreator(ctx context.Context, handle string) (LibraryProfileRow, error) {
	return s.libraryProfileRow(ctx, `p.handle = $1`, strings.ToLower(handle))
}

// LibraryProfileOf is accID's profile, made if it has none yet.
func (s *DB) LibraryProfileOf(ctx context.Context, accID string) (LibraryProfileRow, error) {
	if _, err := s.LibraryProfile(ctx, accID); err != nil {
		return LibraryProfileRow{}, err
	}
	return s.libraryProfileRow(ctx, `p.account_id = $1`, accID)
}

func (s *DB) libraryProfileRow(ctx context.Context, where, arg string) (LibraryProfileRow, error) {
	p, err := scanLibraryProfile(s.QueryRowContext(ctx, `SELECT `+libraryProfileCols+libraryProfileFrom+` WHERE `+where, arg))
	if errors.Is(err, sql.ErrNoRows) {
		return p, ErrNotFound
	}
	if err != nil {
		return p, fmt.Errorf("store: library creator: %w", err)
	}
	return p, nil
}

// LibraryHandleEvery is how often a creator may change their handle: an old handle
// is free for anyone to take, so links to it can come to mean someone else.
const LibraryHandleEvery = 30 * 24 * time.Hour

// SetLibraryProfile keeps what accID set of their profile. A handle that changes
// must be free, not soon after another change, and not another account's GitHub
// login, which would pass for them.
func (s *DB) SetLibraryProfile(ctx context.Context, accID string, p api.LibraryProfileParams) error {
	if _, err := s.LibraryProfile(ctx, accID); err != nil {
		return err
	}
	tx, err := s.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var handle, changed string
	if err := tx.QueryRowContext(ctx, `SELECT handle, handle_changed FROM library_profiles WHERE account_id = $1`, accID).Scan(&handle, &changed); err != nil {
		return fmt.Errorf("store: library profile: %w", err)
	}
	if p.Handle != handle {
		if changed != "" && now().Sub(parseStamp(changed)) < LibraryHandleEvery {
			return errors.New("store: a handle can change once in 30 days")
		}
		var others int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM accounts WHERE LOWER(github_login) = $1 AND id <> $2`, p.Handle, accID).Scan(&others); err != nil {
			return fmt.Errorf("store: library profile: %w", err)
		}
		if others > 0 {
			return errors.New("store: that handle is taken")
		}
		if _, err := tx.ExecContext(ctx, `UPDATE library_profiles SET handle = $1, handle_changed = $2 WHERE account_id = $3`, p.Handle, stamp(now()), accID); err != nil {
			if isUnique(err) {
				return errors.New("store: that handle is taken")
			}
			return fmt.Errorf("store: library profile: %w", err)
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE library_profiles SET bio = $1, links = $2, hidden = $3, show_github = $4, show_stats = $5,
		open_for_work = $6, work_kinds = $7, skills = $8, timezone = $9, contact_url = $10 WHERE account_id = $11`,
		p.Bio, jsonList(p.Links), boolInt(p.Hidden), boolInt(p.ShowGitHub), boolInt(p.ShowStats),
		boolInt(p.OpenForWork), jsonList(p.WorkKinds), jsonList(p.Skills), p.TimeZone, p.ContactURL, accID)
	if err != nil {
		return fmt.Errorf("store: library profile: %w", err)
	}
	return tx.Commit()
}

// LibraryCreatorStats counts what shows of accID.
func (s *DB) LibraryCreatorStats(ctx context.Context, accID string) (api.LibraryProfileStats, error) {
	var out api.LibraryProfileStats
	err := s.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM library_items WHERE owner_id = $1 AND status = 'live'),
		(SELECT COALESCE(SUM(saves), 0) FROM library_items WHERE owner_id = $1 AND status = 'live'),
		(SELECT COUNT(*) FROM library_follows WHERE creator_id = $1)`, accID).Scan(&out.Designs, &out.Saves, &out.Followers)
	if err != nil {
		return out, fmt.Errorf("store: library stats: %w", err)
	}
	return out, nil
}

// libraryItemPage is a page of items: join and where hold their own arguments.
func (s *DB) libraryItemPage(ctx context.Context, join, where, order string, limit, offset int, args ...any) ([]api.LibraryItem, bool, error) {
	limit = max(min(limit, 60), 1)
	rows, err := s.QueryContext(ctx, `SELECT `+libraryItemCols+libraryItemFrom+join+` WHERE `+where+` ORDER BY `+order+
		` LIMIT `+strconv.Itoa(limit+1)+` OFFSET `+strconv.Itoa(max(offset, 0)), args...)
	if err != nil {
		return nil, false, fmt.Errorf("store: library items: %w", err)
	}
	defer rows.Close()
	items := []api.LibraryItem{}
	for rows.Next() {
		it, err := scanLibraryItem(rows)
		if err != nil {
			return nil, false, fmt.Errorf("store: library items: %w", err)
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	rows.Close()
	more := len(items) > limit
	if more {
		items = items[:limit]
	}
	return items, more, s.libraryTags(ctx, items)
}

// libraryListed is what a list may hold: live, and of a creator who shows.
const libraryListed = `i.status = 'live' AND (p.trust IS NULL OR p.trust <> 'blocked') AND (p.hidden IS NULL OR p.hidden = 0)`

// LibraryCreatorItems are accID's live designs: the newest first, or by what's most
// saved.
func (s *DB) LibraryCreatorItems(ctx context.Context, accID, sortBy string, limit, offset int) ([]api.LibraryItem, bool, error) {
	order := `i.created DESC, i.id DESC`
	if sortBy == "popular" {
		order = `i.saves DESC, i.created DESC, i.id DESC`
	}
	return s.libraryItemPage(ctx, "", `i.owner_id = $1 AND i.status = 'live'`, order, limit, offset, accID)
}

// LibraryOwnerUnlisted says accID's designs are left out of every list.
func (s *DB) LibraryOwnerUnlisted(ctx context.Context, accID string) (bool, error) {
	var n int
	err := s.QueryRowContext(ctx, `SELECT COUNT(*) FROM library_profiles WHERE account_id = $1 AND (hidden <> 0 OR trust = 'blocked')`, accID).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("store: library creator: %w", err)
	}
	return n > 0, nil
}

// LibraryOpenQuery is what the open for work list asks for. TimeZones empty is any.
type LibraryOpenQuery struct {
	Kind, Skill   string
	TimeZones     []string
	Limit, Offset int
}

// libraryOpenWhere is who is on the list: they say they're open and how to reach
// them, they show, and at least one design of theirs passed: an account made to post
// a link has nothing to list it by.
const libraryOpenWhere = `p.open_for_work = 1 AND p.contact_url <> '' AND p.hidden = 0 AND p.trust <> 'blocked'
	AND EXISTS (SELECT 1 FROM library_items w WHERE w.owner_id = p.account_id AND w.status = 'live')`

// jsonListHolds is a LIKE pattern for a string in a JSON list, as jsonList writes it.
func jsonListHolds(v string) string {
	data, _ := json.Marshal(v)
	return "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(string(data)) + "%"
}

// LibraryOpenCreators are those on the list, by q; more says there are others after.
func (s *DB) LibraryOpenCreators(ctx context.Context, q LibraryOpenQuery) (out []LibraryProfileRow, more bool, err error) {
	where := []string{libraryOpenWhere}
	var args []any
	arg := func(v any) string {
		args = append(args, v)
		return "$" + strconv.Itoa(len(args))
	}
	if q.Kind != "" {
		where = append(where, `p.work_kinds LIKE `+arg(jsonListHolds(q.Kind))+` ESCAPE '\'`)
	}
	if q.Skill != "" {
		where = append(where, `p.skills LIKE `+arg(jsonListHolds(strings.ToLower(q.Skill)))+` ESCAPE '\'`)
	}
	if len(q.TimeZones) > 0 {
		marks := make([]string, len(q.TimeZones))
		for i, z := range q.TimeZones {
			marks[i] = arg(z)
		}
		where = append(where, `p.timezone IN (`+strings.Join(marks, ",")+`)`)
	}
	limit := max(min(q.Limit, 60), 1)
	rows, err := s.QueryContext(ctx, `SELECT `+libraryProfileCols+libraryProfileFrom+` WHERE `+strings.Join(where, " AND ")+
		` ORDER BY p.created DESC, p.account_id LIMIT `+strconv.Itoa(limit+1)+` OFFSET `+strconv.Itoa(max(q.Offset, 0)), args...)
	if err != nil {
		return nil, false, fmt.Errorf("store: library open: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		p, err := scanLibraryProfile(rows)
		if err != nil {
			return nil, false, fmt.Errorf("store: library open: %w", err)
		}
		out = append(out, p)
	}
	if len(out) > limit {
		out, more = out[:limit], true
	}
	return out, more, rows.Err()
}

// LibraryOpenSkills are the skills the list's people name most, the most named first.
func (s *DB) LibraryOpenSkills(ctx context.Context, limit int) ([]string, error) {
	rows, err := s.QueryContext(ctx, `SELECT p.skills`+libraryProfileFrom+` WHERE `+libraryOpenWhere+` LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("store: library skills: %w", err)
	}
	defer rows.Close()
	count := map[string]int{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		for _, skill := range stringList(raw) {
			count[skill]++
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(count))
	for skill := range count {
		out = append(out, skill)
	}
	sort.Slice(out, func(i, j int) bool {
		if count[out[i]] != count[out[j]] {
			return count[out[i]] > count[out[j]]
		}
		return out[i] < out[j]
	})
	return out[:min(limit, len(out))], nil
}

// LibraryWorksOf are the latest few live designs of each account in ids.
func (s *DB) LibraryWorksOf(ctx context.Context, ids []string, each int) (map[string][]api.LibraryWork, error) {
	out := map[string][]api.LibraryWork{}
	if len(ids) == 0 {
		return out, nil
	}
	args := make([]any, len(ids))
	marks := make([]string, len(ids))
	for i, id := range ids {
		args[i], marks[i] = id, "$"+strconv.Itoa(i+1)
	}
	rows, err := s.QueryContext(ctx, `SELECT owner_id, id, slug, title, live_version FROM library_items WHERE status = 'live' AND owner_id IN (`+strings.Join(marks, ",")+`) ORDER BY created DESC, id DESC`, args...)
	if err != nil {
		return nil, fmt.Errorf("store: library works: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var owner string
		var w api.LibraryWork
		if err := rows.Scan(&owner, &w.ID, &w.Slug, &w.Title, &w.LiveVersion); err != nil {
			return nil, err
		}
		if len(out[owner]) < each {
			out[owner] = append(out[owner], w)
		}
	}
	return out, rows.Err()
}

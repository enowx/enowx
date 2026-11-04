package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/enowx/enowx/cloud/api"
)

// What a browse filters and searches by. Nothing a request says becomes SQL: every
// value here is a parameter, and what is SQL is written here.

// queryArgs gathers a query's parameters.
type queryArgs struct{ list []any }

func (a *queryArgs) add(v any) string {
	a.list = append(a.list, v)
	return "$" + strconv.Itoa(len(a.list))
}

// libraryFilters are the conditions of what shows and what q filters by, words
// apart.
func libraryFilters(q LibraryQuery, a *queryArgs) []string {
	where := []string{`i.status = 'live'`, `(p.trust IS NULL OR p.trust <> 'blocked')`, `(p.hidden IS NULL OR p.hidden = 0)`}
	if q.Category != "" {
		where = append(where, `i.category = `+a.add(q.Category))
	}
	if q.Kind != "" {
		where = append(where, `i.kind = `+a.add(q.Kind))
	}
	if q.License != "" {
		where = append(where, `i.license = `+a.add(q.License))
	}
	if q.Tag != "" {
		where = append(where, `EXISTS (SELECT 1 FROM library_tags t WHERE t.item_id = i.id AND t.tag = `+a.add(q.Tag)+`)`)
	}
	if q.Style != "" {
		where = append(where, `i.style = `+a.add(q.Style))
	}
	if q.Theme != "" {
		where = append(where, `i.theme = `+a.add(q.Theme))
	}
	switch q.Device {
	case "":
	case "phone":
		// A wide design with a phone version of it is made for a phone too.
		where = append(where, `(i.device = 'phone' OR v.doc_phone_size > 0)`)
	default:
		where = append(where, `i.device = `+a.add(q.Device))
	}
	if q.Color != "" {
		where = append(where, `EXISTS (SELECT 1 FROM library_version_colors c WHERE c.item_id = i.id AND c.version = i.live_version AND c.color = `+a.add(q.Color)+`)`)
	}
	if q.Motion {
		where = append(where, `i.has_motion = 1`)
	}
	return where
}

// searchWords are the words of a search: letters and digits only, so nothing in
// it reads as an operator, and a dozen at most.
func searchWords(text string) []string {
	words := strings.FieldsFunc(text, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) && !unicode.IsMark(r) })
	if len(words) > 12 {
		words = words[:12]
	}
	return words
}

// librarySearch is the condition that text matches, and the expression its
// matches rank by. Postgres uses its index, the last word a prefix as someone may
// be typing it; SQLite, for tests and small clouds, LIKE with the same meaning.
func (s *DB) librarySearch(text string, a *queryArgs) (where, rank string) {
	words := searchWords(text)
	if len(words) == 0 {
		return `1 = 0`, ""
	}
	if s.postgres {
		m := a.add(strings.Join(words, " & ") + ":*")
		return `i.search @@ to_tsquery('english', ` + m + `)`, `ts_rank(i.search, to_tsquery('english', ` + m + `), 1)`
	}
	var all, score []string
	for _, w := range words {
		m := a.add("%" + strings.ToLower(w) + "%")
		title, tags, about := `LOWER(i.title) LIKE `+m, `LOWER(i.tags_text) LIKE `+m, `LOWER(i.description) LIKE `+m
		all = append(all, `(`+title+` OR `+tags+` OR `+about+`)`)
		score = append(score, `(CASE WHEN `+title+` THEN 4 WHEN `+tags+` THEN 2 ELSE 1 END)`)
	}
	return strings.Join(all, " AND "), strings.Join(score, " + ")
}

// LibraryColor is a color a version shows, and how much of it out of 100.
type LibraryColor struct {
	Color string
	Share int
}

// LibraryTraits is what the server read of a version as it was submitted.
type LibraryTraits struct {
	Theme, Device string
	Colors        []LibraryColor
	// Phash is its picture's perceptual hash; empty when it has none to tell by.
	Phash string
}

// SetLibraryTraits keeps t for version n of item id: the item takes the theme and
// device as the version goes live, like all else a version says of it.
func (s *DB) SetLibraryTraits(ctx context.Context, id string, n int, t LibraryTraits) error {
	tx, err := s.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var raw string
	if err := tx.QueryRowContext(ctx, `SELECT meta FROM library_versions WHERE item_id = $1 AND version = $2`, id, n).Scan(&raw); err != nil {
		return fmt.Errorf("store: library traits: %w", err)
	}
	var m LibraryMeta
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return fmt.Errorf("store: library traits: %w", err)
	}
	m.Theme, m.Device = t.Theme, t.Device
	meta, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("store: library traits: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE library_versions SET meta = $1, phash = $2 WHERE item_id = $3 AND version = $4`, string(meta), t.Phash, id, n); err != nil {
		return fmt.Errorf("store: library traits: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM library_version_colors WHERE item_id = $1 AND version = $2`, id, n); err != nil {
		return fmt.Errorf("store: library traits: %w", err)
	}
	for _, c := range t.Colors {
		if _, err := tx.ExecContext(ctx, `INSERT INTO library_version_colors (item_id, version, color, share) VALUES ($1, $2, $3, $4)`, id, n, c.Color, c.Share); err != nil {
			return fmt.Errorf("store: library traits: %w", err)
		}
	}
	return tx.Commit()
}

// LibraryVersionColors are the colors kept for version n of item id, the most
// shown first.
func (s *DB) LibraryVersionColors(ctx context.Context, id string, n int) ([]LibraryColor, error) {
	rows, err := s.QueryContext(ctx, `SELECT color, share FROM library_version_colors WHERE item_id = $1 AND version = $2 ORDER BY share DESC, color`, id, n)
	if err != nil {
		return nil, fmt.Errorf("store: library colors: %w", err)
	}
	defer rows.Close()
	var out []LibraryColor
	for rows.Next() {
		var c LibraryColor
		if err := rows.Scan(&c.Color, &c.Share); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// readLibraryItems reads rows of libraryItemCols, closes them, and fills in tags.
func (s *DB) readLibraryItems(ctx context.Context, rows *sql.Rows) ([]api.LibraryItem, error) {
	defer rows.Close()
	items := []api.LibraryItem{}
	for rows.Next() {
		it, err := scanLibraryItem(rows)
		if err != nil {
			return nil, fmt.Errorf("store: library items: %w", err)
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	return items, s.libraryTags(ctx, items)
}

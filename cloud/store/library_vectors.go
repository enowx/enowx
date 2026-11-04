package store

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/enowx/enowx/cloud/api"
)

// Meaning search's vectors. Postgres compares them with pgvector when the server
// has it, and without it there is no meaning search at all: a cloud must start
// without it, and comparing every vector here is no load for a shared server.
// SQLite compares them here, for tests and small clouds, as it does LIKE.

// LibraryVectors keeps and searches vectors. Made once at the start, then only read.
type LibraryVectors struct {
	db *DB
	// ns is the Postgres schema pgvector is in, empty on SQLite.
	ns string
}

// ErrNoVectors says this database can't search by meaning.
var ErrNoVectors = errors.New("store: this database has no pgvector")

var vectorSchema = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)

// OpenLibraryVectors is the database's vectors, ErrNoVectors on a Postgres without
// pgvector. Where the extension is available but not on, it's turned on, in public
// so that no schema's removal takes it from another's; a role that may not is
// the same as not having it.
func (s *DB) OpenLibraryVectors(ctx context.Context) (*LibraryVectors, error) {
	if !s.postgres {
		return &LibraryVectors{db: s}, nil
	}
	var available int
	if err := s.QueryRowContext(ctx, `SELECT COUNT(*) FROM pg_available_extensions WHERE name = 'vector'`).Scan(&available); err != nil {
		return nil, fmt.Errorf("store: pgvector: %w", err)
	}
	if available == 0 {
		return nil, ErrNoVectors
	}
	if _, err := s.ExecContext(ctx, `CREATE EXTENSION IF NOT EXISTS vector SCHEMA public`); err != nil {
		if _, err := s.ExecContext(ctx, `CREATE EXTENSION IF NOT EXISTS vector`); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrNoVectors, err)
		}
	}
	var ns string
	err := s.QueryRowContext(ctx, `SELECT n.nspname FROM pg_extension e JOIN pg_namespace n ON n.oid = e.extnamespace WHERE e.extname = 'vector'`).Scan(&ns)
	if err != nil {
		return nil, fmt.Errorf("store: pgvector: %w", err)
	}
	if !vectorSchema.MatchString(ns) {
		return nil, fmt.Errorf("%w: it's in a schema named oddly", ErrNoVectors)
	}
	if _, err := s.ExecContext(ctx, `ALTER TABLE library_embeddings ADD COLUMN IF NOT EXISTS vec_pg `+ns+`.vector`); err != nil {
		return nil, fmt.Errorf("store: pgvector: %w", err)
	}
	return &LibraryVectors{db: s, ns: ns}, nil
}

func vectorBytes(v []float32) []byte {
	out := make([]byte, 4*len(v))
	for i, x := range v {
		binary.LittleEndian.PutUint32(out[i*4:], math.Float32bits(x))
	}
	return out
}

func vectorOf(b []byte) []float32 {
	out := make([]float32, len(b)/4)
	for i := range out {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return out
}

// vectorText is v as pgvector reads it.
func vectorText(v []float32) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, x := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(x), 'g', -1, 32))
	}
	b.WriteByte(']')
	return b.String()
}

// LibraryEmbedding is the public text of an item to embed.
type LibraryEmbedding struct {
	ItemID, Title, Description, Tags string
}

// Due are live items with no vector of model, or one older than their last change.
func (l *LibraryVectors) Due(ctx context.Context, model string, limit int) ([]LibraryEmbedding, error) {
	rows, err := l.db.QueryContext(ctx, `SELECT i.id, i.title, i.description, i.tags_text FROM library_items i LEFT JOIN library_embeddings e ON e.item_id = i.id
		WHERE i.status = 'live' AND (e.item_id IS NULL OR e.model <> $1 OR e.created < i.updated) ORDER BY i.updated LIMIT $2`, model, limit)
	if err != nil {
		return nil, fmt.Errorf("store: library embeddings: %w", err)
	}
	defer rows.Close()
	var out []LibraryEmbedding
	for rows.Next() {
		var e LibraryEmbedding
		if err := rows.Scan(&e.ItemID, &e.Title, &e.Description, &e.Tags); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Put keeps item id's vector.
func (l *LibraryVectors) Put(ctx context.Context, id, model string, v []float32) error {
	var err error
	if l.ns != "" {
		_, err = l.db.ExecContext(ctx, `INSERT INTO library_embeddings (item_id, model, vec, created, vec_pg) VALUES ($1, $2, $3, $4, $5::text::`+l.ns+`.vector)
			ON CONFLICT (item_id) DO UPDATE SET model = $2, vec = $3, created = $4, vec_pg = $5::text::`+l.ns+`.vector`, id, model, vectorBytes(v), stamp(now()), vectorText(v))
	} else {
		_, err = l.db.ExecContext(ctx, `INSERT INTO library_embeddings (item_id, model, vec, created) VALUES ($1, $2, $3, $4)
			ON CONFLICT (item_id) DO UPDATE SET model = $2, vec = $3, created = $4`, id, model, vectorBytes(v), stamp(now()))
	}
	if err != nil {
		return fmt.Errorf("store: library embedding: %w", err)
	}
	return nil
}

// Prune drops the vectors of what no longer shows.
func (l *LibraryVectors) Prune(ctx context.Context) error {
	_, err := l.db.ExecContext(ctx, `DELETE FROM library_embeddings WHERE item_id IN (SELECT id FROM library_items WHERE status <> 'live')`)
	if err != nil {
		return fmt.Errorf("store: library embeddings: %w", err)
	}
	return nil
}

// comparedHere is the most vectors SQLite's way compares for one search.
const comparedHere = 2000

// Search are live items passing q's filters closest to v by cosine similarity,
// the closest first, none below least.
func (l *LibraryVectors) Search(ctx context.Context, q LibraryQuery, model string, v []float32, least float64) ([]api.LibraryItem, error) {
	limit := max(min(q.Limit, 60), 1)
	a := &queryArgs{}
	where := libraryFilters(q, a)
	where = append(where, `e.model = `+a.add(model))
	from := libraryItemFrom + ` JOIN library_embeddings e ON e.item_id = i.id WHERE ` + strings.Join(where, " AND ")
	if l.ns == "" {
		return l.searchHere(ctx, from, a, v, least, limit)
	}
	m := a.add(vectorText(v))
	distance := `e.vec_pg OPERATOR(` + l.ns + `.<=>) ` + m + `::text::` + l.ns + `.vector`
	rows, err := l.db.QueryContext(ctx, `SELECT `+libraryItemCols+from+` AND `+distance+` <= `+a.add(1-least)+` ORDER BY `+distance+`, i.id LIMIT `+strconv.Itoa(limit), a.list...)
	if err != nil {
		return nil, fmt.Errorf("store: library meaning: %w", err)
	}
	return l.db.readLibraryItems(ctx, rows)
}

func (l *LibraryVectors) searchHere(ctx context.Context, from string, a *queryArgs, v []float32, least float64, limit int) ([]api.LibraryItem, error) {
	rows, err := l.db.QueryContext(ctx, `SELECT i.id, e.vec`+from+` LIMIT `+strconv.Itoa(comparedHere), a.list...)
	if err != nil {
		return nil, fmt.Errorf("store: library meaning: %w", err)
	}
	defer rows.Close()
	type scored struct {
		id    string
		score float64
	}
	var near []scored
	for rows.Next() {
		var id string
		var raw []byte
		if err := rows.Scan(&id, &raw); err != nil {
			return nil, err
		}
		if score := cosine(v, vectorOf(raw)); score >= least {
			near = append(near, scored{id, score})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	slices.SortFunc(near, func(x, y scored) int {
		switch {
		case x.score > y.score:
			return -1
		case x.score < y.score:
			return 1
		}
		return strings.Compare(x.id, y.id)
	})
	out := []api.LibraryItem{}
	for _, n := range near[:min(limit, len(near))] {
		it, err := l.db.LibraryItem(ctx, n.id)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, nil
}

func cosine(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return -1
	}
	var dot, na, nb float64
	for i := range a {
		x, y := float64(a[i]), float64(b[i])
		dot += x * y
		na += x * x
		nb += y * y
	}
	if na == 0 || nb == 0 {
		return -1
	}
	return dot / math.Sqrt(na*nb)
}

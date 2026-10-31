package rag

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib" // registers "pgx"
)

// pgStore keeps points in the user's PostgreSQL: vectors in pgvector's
// type, searched by its cosine distance, words by full text search.
type pgStore struct{ db *sql.DB }

// openPostgres connects to the PostgreSQL at url (pgvector installed, or
// installable by its user) and makes its table.
func openPostgres(ctx context.Context, url string) (*pgStore, error) {
	db, err := sql.Open("pgx", url)
	if err != nil {
		return nil, fmt.Errorf("rag: PostgreSQL: %w", err)
	}
	db.SetMaxOpenConns(4)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("rag: PostgreSQL: %w", err)
	}
	// The extension may be there already, made by an admin.
	_, extErr := db.ExecContext(ctx, `CREATE EXTENSION IF NOT EXISTS vector`)
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS enowx_rag_points (project TEXT NOT NULL, id TEXT NOT NULL, file TEXT NOT NULL, start INTEGER NOT NULL, stop INTEGER NOT NULL,
			symbol TEXT NOT NULL, source TEXT NOT NULL, text TEXT NOT NULL, embedding vector NOT NULL,
			words tsvector GENERATED ALWAYS AS (to_tsvector('simple', text || ' ' || symbol || ' ' || file)) STORED,
			PRIMARY KEY (project, id))`,
		`CREATE INDEX IF NOT EXISTS enowx_rag_points_words ON enowx_rag_points USING gin (words)`,
	} {
		if _, err := db.ExecContext(ctx, q); err != nil {
			db.Close()
			if extErr != nil {
				return nil, fmt.Errorf("rag: PostgreSQL has no pgvector (%v): %w", extErr, err)
			}
			return nil, fmt.Errorf("rag: PostgreSQL: %w", err)
		}
	}
	return &pgStore{db: db}, nil
}

// vecText is v as pgvector reads it: [1,2,3].
func vecText(v []float32) string {
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

func (s *pgStore) ids(ctx context.Context, project string) (map[string]lines, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, start, stop, symbol FROM enowx_rag_points WHERE project = $1`, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]lines{}
	for rows.Next() {
		var id string
		var l lines
		if err := rows.Scan(&id, &l.start, &l.end, &l.symbol); err != nil {
			return nil, err
		}
		out[id] = l
	}
	return out, rows.Err()
}

func (s *pgStore) put(ctx context.Context, project string, pts []point) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, p := range pts {
		if p.vec == nil {
			_, err = tx.ExecContext(ctx, `UPDATE enowx_rag_points SET file = $3, start = $4, stop = $5, symbol = $6 WHERE project = $1 AND id = $2`,
				project, p.id, p.file, p.start, p.end, p.symbol)
		} else {
			_, err = tx.ExecContext(ctx, `INSERT INTO enowx_rag_points (project, id, file, start, stop, symbol, source, text, embedding)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::vector)
				ON CONFLICT (project, id) DO UPDATE SET file = $3, start = $4, stop = $5, symbol = $6, source = $7, text = $8, embedding = $9::vector`,
				project, p.id, p.file, p.start, p.end, p.symbol, p.source, p.text, vecText(normalize(p.vec)))
		}
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *pgStore) remove(ctx context.Context, project string, ids []string) error {
	for i := 0; i < len(ids); i += 500 {
		if _, err := s.db.ExecContext(ctx, `DELETE FROM enowx_rag_points WHERE project = $1 AND id = ANY($2)`, project, ids[i:min(len(ids), i+500)]); err != nil {
			return err
		}
	}
	return nil
}

func (s *pgStore) drop(ctx context.Context, project string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM enowx_rag_points WHERE project = $1`, project)
	return err
}

func (s *pgStore) count(ctx context.Context, project string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM enowx_rag_points WHERE project = $1`, project).Scan(&n)
	return n, err
}

func (s *pgStore) nearest(ctx context.Context, project string, q []float32, k int) ([]scored, error) {
	return s.scores(ctx, `SELECT id, 1 - (embedding <=> $2::vector) FROM enowx_rag_points WHERE project = $1 ORDER BY embedding <=> $2::vector LIMIT $3`,
		project, vecText(q), k)
}

func (s *pgStore) keyword(ctx context.Context, project, query string, k int) ([]scored, error) {
	ws := words(query) // letters, digits and _ only: safe in a tsquery
	if len(ws) == 0 {
		return nil, nil
	}
	return s.scores(ctx, `SELECT id, ts_rank(words, q) FROM enowx_rag_points, to_tsquery('simple', $2) q WHERE project = $1 AND words @@ q ORDER BY 2 DESC LIMIT $3`,
		project, strings.Join(ws, " | "), k)
}

func (s *pgStore) scores(ctx context.Context, q string, args ...any) ([]scored, error) {
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []scored
	for rows.Next() {
		var sc scored
		if err := rows.Scan(&sc.id, &sc.score); err != nil {
			return nil, err
		}
		out = append(out, sc)
	}
	return out, rows.Err()
}

func (s *pgStore) get(ctx context.Context, project string, ids []string) (map[string]point, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, file, start, stop, symbol, source, text FROM enowx_rag_points WHERE project = $1 AND id = ANY($2)`, project, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]point{}
	for rows.Next() {
		var p point
		if err := rows.Scan(&p.id, &p.file, &p.start, &p.end, &p.symbol, &p.source, &p.text); err != nil {
			return nil, err
		}
		out[p.id] = p
	}
	return out, rows.Err()
}

func (s *pgStore) close() error { return s.db.Close() }

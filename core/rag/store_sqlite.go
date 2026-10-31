package rag

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	_ "modernc.org/sqlite" // registers "sqlite"
)

// sqliteStore keeps points in a SQLite file: vectors as blobs, searched by
// brute force over a copy in memory (a project's few thousand points take
// milliseconds), their text in FTS5 for words.
type sqliteStore struct {
	db *sql.DB
	// write lets one writer at a time: SQLite refuses a second writer's
	// transaction rather than wait for it.
	write sync.Mutex
	mu    sync.Mutex
	// vecs are each project's vectors, read once and dropped as it changes.
	vecs map[string][]idVec
}

type idVec struct {
	id  string
	vec []float32
}

func openSQLite(file string) (*sqliteStore, error) {
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", "file:"+file+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)")
	if err != nil {
		return nil, err
	}
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS points (project TEXT NOT NULL, id TEXT NOT NULL, file TEXT NOT NULL, start INTEGER NOT NULL, stop INTEGER NOT NULL,
			symbol TEXT NOT NULL, source TEXT NOT NULL, text TEXT NOT NULL, vec BLOB NOT NULL, PRIMARY KEY (project, id))`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS points_fts USING fts5(text, symbol, file, project UNINDEXED, id UNINDEXED, tokenize = 'unicode61')`,
	} {
		if _, err := db.Exec(q); err != nil {
			db.Close()
			return nil, fmt.Errorf("rag: the store %s: %w", file, err)
		}
	}
	return &sqliteStore{db: db, vecs: map[string][]idVec{}}, nil
}

func (s *sqliteStore) changed(project string) {
	s.mu.Lock()
	delete(s.vecs, project)
	s.mu.Unlock()
}

func (s *sqliteStore) ids(ctx context.Context, project string) (map[string]lines, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, start, stop, symbol FROM points WHERE project = ?`, project)
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

func (s *sqliteStore) put(ctx context.Context, project string, pts []point) error {
	s.write.Lock()
	defer s.write.Unlock()
	defer s.changed(project)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, p := range pts {
		if p.vec == nil {
			if _, err := tx.ExecContext(ctx, `UPDATE points SET file = ?, start = ?, stop = ?, symbol = ? WHERE project = ? AND id = ?`,
				p.file, p.start, p.end, p.symbol, project, p.id); err != nil {
				return err
			}
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO points (project, id, file, start, stop, symbol, source, text, vec) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			project, p.id, p.file, p.start, p.end, p.symbol, p.source, p.text, vecBytes(normalize(p.vec))); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM points_fts WHERE project = ? AND id = ?`, project, p.id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO points_fts (text, symbol, file, project, id) VALUES (?, ?, ?, ?, ?)`, p.text, p.symbol, p.file, project, p.id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *sqliteStore) remove(ctx context.Context, project string, ids []string) error {
	s.write.Lock()
	defer s.write.Unlock()
	defer s.changed(project)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, `DELETE FROM points WHERE project = ? AND id = ?`, project, id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM points_fts WHERE project = ? AND id = ?`, project, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *sqliteStore) drop(ctx context.Context, project string) error {
	s.write.Lock()
	defer s.write.Unlock()
	defer s.changed(project)
	if _, err := s.db.ExecContext(ctx, `DELETE FROM points WHERE project = ?`, project); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM points_fts WHERE project = ?`, project)
	return err
}

func (s *sqliteStore) count(ctx context.Context, project string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM points WHERE project = ?`, project).Scan(&n)
	return n, err
}

func (s *sqliteStore) vectors(ctx context.Context, project string) ([]idVec, error) {
	s.mu.Lock()
	vs, ok := s.vecs[project]
	s.mu.Unlock()
	if ok {
		return vs, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, vec FROM points WHERE project = ?`, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var iv idVec
		var b []byte
		if err := rows.Scan(&iv.id, &b); err != nil {
			return nil, err
		}
		iv.vec = bytesVec(b)
		vs = append(vs, iv)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.vecs[project] = vs
	s.mu.Unlock()
	return vs, nil
}

func (s *sqliteStore) nearest(ctx context.Context, project string, q []float32, k int) ([]scored, error) {
	vs, err := s.vectors(ctx, project)
	if err != nil {
		return nil, err
	}
	out := make([]scored, len(vs))
	for i, v := range vs {
		out[i] = scored{v.id, float64(dot(q, v.vec))}
	}
	sortScored(out)
	return out[:min(k, len(out))], nil
}

func (s *sqliteStore) keyword(ctx context.Context, project, query string, k int) ([]scored, error) {
	ws := words(query)
	if len(ws) == 0 {
		return nil, nil
	}
	for i, w := range ws {
		ws[i] = `"` + strings.ReplaceAll(w, `"`, `""`) + `"`
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, bm25(points_fts) FROM points_fts WHERE points_fts MATCH ? AND project = ? ORDER BY bm25(points_fts) LIMIT ?`,
		strings.Join(ws, " OR "), project, k)
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
		sc.score = -sc.score // bm25: lower is better
		out = append(out, sc)
	}
	return out, rows.Err()
}

func (s *sqliteStore) get(ctx context.Context, project string, ids []string) (map[string]point, error) {
	out := map[string]point{}
	for _, id := range ids {
		var p point
		err := s.db.QueryRowContext(ctx, `SELECT id, file, start, stop, symbol, source, text FROM points WHERE project = ? AND id = ?`, project, id).
			Scan(&p.id, &p.file, &p.start, &p.end, &p.symbol, &p.source, &p.text)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return nil, err
		}
		out[id] = p
	}
	return out, nil
}

func (s *sqliteStore) close() error { return s.db.Close() }

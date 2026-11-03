// Package store is the cloud server's database: Postgres when given one,
// else SQLite in the server's data folder. Its schema is brought up to date
// by numbered migrations when it opens, one set per database; the queries
// are the same for both.
package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // registers "pgx"
)

// migrations are the schema's steps, <database>/NNNN_name.sql, run once
// each in order. A released one never changes: a change is a new one, made
// for both databases.
//
//go:embed migrations
var migrations embed.FS

// DefaultSchema is the Postgres schema the cloud keeps its tables in, so it
// can share a database with other apps.
const DefaultSchema = "enowx_cloud"

// DefaultMaxConns keeps the server within a role's connection limit, which a
// database shared with other apps shares with them.
const DefaultMaxConns = 8

// Config says which database to open.
type Config struct {
	// Postgres is a Postgres URL; empty, SQLite in Dir.
	Postgres string
	// Schema is the Postgres schema, made when missing; DefaultSchema when
	// empty.
	Schema string
	// Dir is the folder SQLite's cloud.db is in.
	Dir string
	// MaxConns is the most Postgres connections this server holds;
	// DefaultMaxConns when 0.
	MaxConns int
}

// DB is the server's database.
type DB struct {
	*sql.DB
	postgres bool
	// itemReads holds a place per page of items being read, so that they
	// leave connections to everything else; nil on SQLite.
	itemReads chan struct{}
}

var schemaName = regexp.MustCompile(`^[a-z_][a-z0-9_]{0,62}$`)

// Open opens the database cfg says, and migrates it.
func Open(ctx context.Context, cfg Config) (*DB, error) {
	var s *DB
	if cfg.Postgres != "" {
		db, err := openPostgres(ctx, cfg)
		if err != nil {
			return nil, err
		}
		s = &DB{DB: db, postgres: true, itemReads: make(chan struct{}, max(db.Stats().MaxOpenConnections/2, 1))}
	} else {
		if !slices.Contains(sql.Drivers(), "sqlite") {
			return nil, errors.New("store: this build has no SQLite (built with -tags nosqlite): set ENOWX_CLOUD_DATABASE_URL to a Postgres URL")
		}
		path := filepath.Join(cfg.Dir, "cloud.db")
		// WAL: readers don't wait for the writer. One connection writes at
		// a time (SQLite's own rule), busy_timeout queues the others.
		dsn := "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)&_pragma=synchronous(NORMAL)"
		db, err := sql.Open("sqlite", dsn)
		if err != nil {
			return nil, fmt.Errorf("store: open %s: %w", path, err)
		}
		s = &DB{DB: db}
	}
	if err := s.migrate(ctx); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

// openPostgres connects to cfg's database, its tables in cfg's schema.
func openPostgres(ctx context.Context, cfg Config) (*sql.DB, error) {
	schema := cfg.Schema
	if schema == "" {
		schema = DefaultSchema
	}
	if !schemaName.MatchString(schema) {
		return nil, fmt.Errorf("store: schema %q: lower case letters, digits and _ only", schema)
	}
	u, err := url.Parse(cfg.Postgres)
	if err != nil {
		return nil, errors.New("store: the Postgres URL doesn't parse") // not err: it holds the password
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, err := sql.Open("pgx", u.String())
	if err != nil {
		return nil, fmt.Errorf("store: open Postgres: %w", err)
	}
	conns := cfg.MaxConns
	if conns <= 0 {
		conns = DefaultMaxConns
	}
	db.SetMaxOpenConns(conns)
	db.SetMaxIdleConns(max(conns/2, 1))
	db.SetConnMaxIdleTime(5 * time.Minute)
	db.SetConnMaxLifetime(30 * time.Minute)
	if _, err := db.ExecContext(ctx, `CREATE SCHEMA IF NOT EXISTS `+schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: Postgres schema %s: %w", schema, err)
	}
	return db, nil
}

// SchemaVersion is the last migration run.
func (s *DB) SchemaVersion(ctx context.Context) (int, error) {
	var v int
	err := s.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&v)
	return v, err
}

func (s *DB) migrate(ctx context.Context) error {
	if _, err := s.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("store: migrations table: %w", err)
	}
	done, err := s.SchemaVersion(ctx)
	if err != nil {
		return fmt.Errorf("store: schema version: %w", err)
	}
	steps, err := migrationSteps(s.dialect())
	if err != nil {
		return err
	}
	for _, m := range steps {
		if m.version <= done {
			continue
		}
		tx, err := s.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, m.sql); err != nil {
			tx.Rollback()
			return fmt.Errorf("store: migration %s: %w", m.name, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version, applied) VALUES ($1, $2)`, m.version, stamp(now())); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("store: migration %s: %w", m.name, err)
		}
	}
	return nil
}

func (s *DB) dialect() string {
	if s.postgres {
		return "postgres"
	}
	return "sqlite"
}

// isUnique says err is a unique constraint refusing a row.
func isUnique(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "SQLSTATE 23505"))
}

type migration struct {
	version int
	name    string
	sql     string
}

func migrationSteps(dialect string) ([]migration, error) {
	dir := "migrations/" + dialect
	entries, err := fs.ReadDir(migrations, dir)
	if err != nil {
		return nil, err
	}
	var out []migration
	for _, e := range entries {
		num, _, ok := strings.Cut(e.Name(), "_")
		v, err := strconv.Atoi(num)
		if !ok || err != nil {
			return nil, fmt.Errorf("store: migration %s isn't named NNNN_name.sql", e.Name())
		}
		data, err := migrations.ReadFile(dir + "/" + e.Name())
		if err != nil {
			return nil, err
		}
		out = append(out, migration{v, e.Name(), string(data)})
	}
	slices.SortFunc(out, func(a, b migration) int { return a.version - b.version })
	return out, nil
}

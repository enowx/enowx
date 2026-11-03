// Package storetest opens a database for a test: SQLite in a temporary
// folder, or, when ENOWX_CLOUD_TEST_POSTGRES names a Postgres server, a
// schema of its own there, dropped after.
package storetest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"

	"github.com/enowx/enowx/cloud/store"
)

// Env names the Postgres URL tests run against too.
const Env = "ENOWX_CLOUD_TEST_POSTGRES"

// Open is a new, migrated database for t, closed (and its schema dropped)
// when t ends.
func Open(t testing.TB) *store.DB {
	t.Helper()
	ctx := context.Background()
	cfg := store.Config{Dir: t.TempDir()}
	if pg := os.Getenv(Env); pg != "" {
		var b [6]byte
		rand.Read(b[:])
		cfg.Postgres, cfg.Schema = pg, "test_"+hex.EncodeToString(b[:])
	}
	db, err := store.Open(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	// One connection, so that a call asking for a second while it holds one, which
	// a full pool would leave waiting for itself, hangs its test here.
	db.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if cfg.Schema != "" {
			db.ExecContext(ctx, `DROP SCHEMA `+cfg.Schema+` CASCADE`)
		}
		db.Close()
	})
	return db
}

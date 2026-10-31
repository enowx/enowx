package tokens

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/klauspost/compress/zstd"
)

// The SQLite databases agents keep, made as their sources lay them out,
// with a row or two each.

func makeDB(t *testing.T, path string, stmts ...string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
}

// openCodeDB is OpenCode 1.x's: an answer of two steps (the message
// holding only the last), and the user's message.
func openCodeDB(t *testing.T, path string) {
	makeDB(t, path,
		"CREATE TABLE `message` (`id` text PRIMARY KEY, `session_id` text NOT NULL, `time_created` integer NOT NULL, `time_updated` integer NOT NULL, `data` text NOT NULL)",
		"CREATE TABLE `part` (`id` text PRIMARY KEY, `message_id` text NOT NULL, `session_id` text NOT NULL, `time_created` integer NOT NULL, `time_updated` integer NOT NULL, `data` text NOT NULL)",
		`INSERT INTO message VALUES ('msg_u', 'ses_1', 1792764190000, 1792764190000, '{"role":"user","time":{"created":1792764190000}}')`,
		`INSERT INTO message VALUES ('msg_a', 'ses_1', 1792764194880, 1792764241637, '{"role":"assistant","path":{"cwd":"/work/oc","root":"/"},"cost":0,"tokens":{"input":181,"output":10,"reasoning":0,"cache":{"write":0,"read":71936}},"modelID":"deepseek-chat","providerID":"deepseek","time":{"created":1792764194880,"completed":1792764241637}}')`,
		`INSERT INTO part VALUES ('prt_1', 'msg_a', 'ses_1', 1792764200000, 1792764200000, '{"type":"step-start"}')`,
		`INSERT INTO part VALUES ('prt_2', 'msg_a', 'ses_1', 1792764220000, 1792764220000, '{"reason":"tool-calls","type":"step-finish","tokens":{"total":72055,"input":72013,"output":42,"reasoning":0,"cache":{"write":0,"read":0}},"cost":0}')`,
		`INSERT INTO part VALUES ('prt_3', 'msg_a', 'ses_1', 1792764241000, 1792764241000, '{"reason":"stop","type":"step-finish","tokens":{"total":72127,"input":181,"output":10,"reasoning":0,"cache":{"write":0,"read":71936}},"cost":0}')`,
	)
}

func gooseDB(t *testing.T, path string) {
	makeDB(t, path,
		`CREATE TABLE sessions (id TEXT PRIMARY KEY, working_dir TEXT, provider_name TEXT)`,
		`CREATE TABLE usage_ledger (id TEXT PRIMARY KEY, session_id TEXT, created_timestamp INTEGER, model TEXT, input_tokens INTEGER, output_tokens INTEGER, total_tokens INTEGER, cache_read_tokens INTEGER, cache_write_tokens INTEGER, cost REAL, cost_source TEXT, is_compaction INTEGER)`,
		`INSERT INTO sessions VALUES ('gs-1', '/work/goose', 'anthropic')`,
		`INSERT INTO usage_ledger VALUES ('ul-1', 'gs-1', 1792800000, 'claude-sonnet-5-5', 11000, 400, 11400, 9000, 1000, 0.01, 'provider', 0)`,
	)
}

func copilotDB(t *testing.T, path string) {
	makeDB(t, path,
		`CREATE TABLE sessions (id TEXT PRIMARY KEY, cwd TEXT, created_at TEXT)`,
		`CREATE TABLE assistant_usage_events (id TEXT PRIMARY KEY, session_id TEXT, turn_index INTEGER, model TEXT, copilot_usage_model TEXT, input_tokens INTEGER, output_tokens INTEGER, cache_read_tokens INTEGER, cache_write_tokens INTEGER, reasoning_tokens INTEGER, total_nano_aiu INTEGER, duration_ms INTEGER, created_at TEXT)`,
		`INSERT INTO sessions VALUES ('cp-1', '/work/cp', '2026-10-26T12:00:00Z')`,
		`INSERT INTO assistant_usage_events VALUES ('ue-1', 'cp-1', 0, 'gpt-5.1', 'gpt-5.1', 20000, 500, 16000, 0, 200, 0, 1200, '2026-10-26T12:00:05Z')`,
	)
}

func devinDB(t *testing.T, path string) {
	makeDB(t, path,
		`CREATE TABLE sessions (id TEXT PRIMARY KEY, working_directory TEXT, model TEXT)`,
		`CREATE TABLE message_nodes (row_id INTEGER PRIMARY KEY, session_id TEXT, node_id TEXT, chat_message TEXT, created_at INTEGER)`,
		`INSERT INTO sessions VALUES ('dv-1', '/work/devin', 'claude-opus-5-5')`,
		`INSERT INTO message_nodes VALUES (1, 'dv-1', 'n1', '{"role":"user"}', 1792810000)`,
		`INSERT INTO message_nodes VALUES (2, 'dv-1', 'n2', '{"role":"assistant","metadata":{"generation_model":"claude-opus-5-5","request_id":"dreq-1","metrics":{"input_tokens":30000,"output_tokens":700,"cache_read_tokens":29000,"cache_creation_tokens":500,"total_time_ms":4000}}}', 1792810005)`,
	)
}

func grokDB(t *testing.T, path string) {
	makeDB(t, path,
		`CREATE TABLE sessions (id TEXT PRIMARY KEY, cwd_at_start TEXT, cwd_last TEXT)`,
		`CREATE TABLE usage_events (id TEXT PRIMARY KEY, session_id TEXT, message_seq INTEGER, source TEXT, model TEXT, input_tokens INTEGER, output_tokens INTEGER, total_tokens INTEGER, cost_micros INTEGER, created_at INTEGER)`,
		`INSERT INTO sessions VALUES ('gr-1', '/work/grok', '/work/grok')`,
		`INSERT INTO usage_events VALUES ('ge-1', 'gr-1', 1, 'chat', 'grok-code-fast-1', 900, 100, 1000, 330, 1792820000)`,
	)
}

// openClawDB holds a session's header plainly and its answer compressed.
func openClawDB(t *testing.T, path string) {
	enc, _ := zstd.NewWriter(nil)
	answer := enc.EncodeAll([]byte(`{"type":"message","id":"oc2","timestamp":"2026-10-26T13:00:04.000Z","message":{"role":"assistant","provider":"anthropic","model":"claude-haiku-5-5","usage":{"input":50,"output":20,"cacheRead":1000,"cacheWrite":0,"totalTokens":1070,"cost":{"total":0.0001}}}}`), nil)
	enc.Close()
	makeDB(t, path,
		`CREATE TABLE transcript_events (session_id TEXT, seq INTEGER, event_json TEXT, event_zstd BLOB, event_utf8_bytes INTEGER, created_at INTEGER)`,
		`INSERT INTO transcript_events VALUES ('ocs-1', 0, '{"type":"session","version":3,"id":"ocs-1","timestamp":"2026-10-26T13:00:00.000Z","cwd":"/work/claw"}', NULL, 0, 0)`,
	)
	db, _ := sql.Open("sqlite", "file:"+path)
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO transcript_events VALUES ('ocs-1', 1, NULL, ?, 300, 0)`, answer); err != nil {
		t.Fatal(err)
	}
}

func zstdFile(t *testing.T, from, to string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", from))
	if err != nil {
		t.Fatal(err)
	}
	enc, _ := zstd.NewWriter(nil)
	packed := enc.EncodeAll(data, nil)
	enc.Close()
	os.MkdirAll(filepath.Dir(to), 0o700)
	if err := os.WriteFile(to, packed, 0o600); err != nil {
		t.Fatal(err)
	}
}

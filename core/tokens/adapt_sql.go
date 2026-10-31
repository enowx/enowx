package tokens

import (
	"cmp"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// Agents keeping a row per model call in SQLite, read from the last row
// read on (FileState.Mark: the last rowid or time).
//
//   - Goose: ~/.local/share/goose/sessions/sessions.db ($GOOSE_PATH_ROOT/
//     data over ~/.local/share/goose), usage_ledger a row a call (model,
//     input_tokens with the cache in, output_tokens, cache_read_tokens,
//     cache_write_tokens, cost), sessions its working_dir and provider.
//   - GitHub Copilot CLI: ~/.copilot/session-store.db,
//     assistant_usage_events a row a call (model, input_tokens with the
//     cache in, output_tokens, cache_read_tokens, cache_write_tokens,
//     reasoning_tokens, created_at), sessions its cwd.
//   - Devin CLI: ~/.local/share/devin/cli/sessions.db, message_nodes'
//     chat_message.metadata (generation_model, request_id, metrics:
//     input_tokens with the cache read in, output_tokens,
//     cache_read_tokens, cache_creation_tokens), sessions its
//     working_directory.
//   - Grok CLI: ~/.grok/grok.db, usage_events a row a call (model,
//     input_tokens, output_tokens, cost_micros), sessions its
//     cwd_at_start; no cache split.

func oneFile(rel ...string) func(env Env) []string {
	return func(env Env) []string { return []string{filepath.Join(append([]string{env.Home}, rel...)...)} }
}

func gooseFiles(env Env) []string {
	if root := os.Getenv("GOOSE_PATH_ROOT"); root != "" {
		return []string{filepath.Join(root, "data", "sessions", "sessions.db")}
	}
	return []string{filepath.Join(env.Home, ".local", "share", "goose", "sessions", "sessions.db")}
}

// query runs q on the database at path, read-only, calling row for each
// row.
func query(path, q string, args []any, row func(*sql.Rows) error) error {
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro&_pragma=busy_timeout(3000)")
	if err != nil {
		return err
	}
	defer db.Close()
	rows, err := db.Query(q, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		if err := row(rows); err != nil {
			continue
		}
	}
	return rows.Err()
}

// sqlTime reads a column that holds Unix seconds, milliseconds, or a
// date as text.
func sqlTime(v any) time.Time {
	switch t := v.(type) {
	case int64:
		return anyTime(json.RawMessage(strconv.FormatInt(t, 10)))
	case float64:
		return anyTime(json.RawMessage(strconv.FormatFloat(t, 'f', -1, 64)))
	case string:
		for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05", "2006-01-02T15:04:05"} {
			if at, err := time.Parse(layout, t); err == nil {
				return at
			}
		}
	case time.Time:
		return t
	}
	return time.Time{}
}

func readGoose(path string, st *FileState) ([]Call, error) {
	var out []Call
	err := query(path, `SELECT u.rowid, u.id, u.session_id, u.created_timestamp, u.model, u.input_tokens, u.output_tokens,
		u.cache_read_tokens, u.cache_write_tokens, u.cost, s.working_dir, s.provider_name
		FROM usage_ledger u LEFT JOIN sessions s ON s.id = u.session_id WHERE u.rowid > ? ORDER BY u.rowid`, []any{st.Mark},
		func(r *sql.Rows) error {
			var rowid int64
			var id, session, model, dir, provider sql.NullString
			var at any
			var in, out_, cr, cw sql.NullInt64
			var cost sql.NullFloat64
			if err := r.Scan(&rowid, &id, &session, &at, &model, &in, &out_, &cr, &cw, &cost, &dir, &provider); err != nil {
				return err
			}
			st.Mark = max(st.Mark, rowid)
			out = append(out, Call{ID: "goose:" + cmp.Or(id.String, strconv.FormatInt(rowid, 10)), Time: sqlTime(at), Model: model.String,
				Provider: providerName(provider.String), Dir: dir.String, Conversation: session.String, Cost: cost.Float64,
				Input: max(in.Int64-cr.Int64-cw.Int64, 0), CacheRead: cr.Int64, CacheWrite: cw.Int64, Output: out_.Int64})
			return nil
		})
	return out, err
}

func readCopilot(path string, st *FileState) ([]Call, error) {
	var out []Call
	err := query(path, `SELECT e.rowid, e.id, e.session_id, e.created_at, e.model, e.input_tokens, e.output_tokens,
		e.cache_read_tokens, e.cache_write_tokens, e.reasoning_tokens, s.cwd
		FROM assistant_usage_events e LEFT JOIN sessions s ON s.id = e.session_id WHERE e.rowid > ? ORDER BY e.rowid`, []any{st.Mark},
		func(r *sql.Rows) error {
			var rowid int64
			var id, session, model, dir sql.NullString
			var at any
			var in, out_, cr, cw, rs sql.NullInt64
			if err := r.Scan(&rowid, &id, &session, &at, &model, &in, &out_, &cr, &cw, &rs, &dir); err != nil {
				return err
			}
			st.Mark = max(st.Mark, rowid)
			out = append(out, Call{ID: "copilot:" + cmp.Or(id.String, strconv.FormatInt(rowid, 10)), Time: sqlTime(at), Model: model.String,
				Dir: dir.String, Conversation: session.String,
				Input: max(in.Int64-cr.Int64-cw.Int64, 0), CacheRead: cr.Int64, CacheWrite: cw.Int64, Output: out_.Int64, Reasoning: rs.Int64})
			return nil
		})
	return out, err
}

func readDevin(path string, st *FileState) ([]Call, error) {
	var out []Call
	err := query(path, `SELECT n.row_id, n.session_id, n.node_id, n.chat_message, n.created_at, s.working_directory
		FROM message_nodes n LEFT JOIN sessions s ON s.id = n.session_id WHERE n.row_id > ? ORDER BY n.row_id`, []any{st.Mark},
		func(r *sql.Rows) error {
			var rowid int64
			var session, node, msg, dir sql.NullString
			var at any
			if err := r.Scan(&rowid, &session, &node, &msg, &at, &dir); err != nil {
				return err
			}
			st.Mark = max(st.Mark, rowid)
			var m struct {
				Metadata *struct {
					Model     string `json:"generation_model"`
					RequestID string `json:"request_id"`
					Metrics   *struct {
						Input       int64 `json:"input_tokens"`
						Output      int64 `json:"output_tokens"`
						CacheRead   int64 `json:"cache_read_tokens"`
						CacheCreate int64 `json:"cache_creation_tokens"`
					} `json:"metrics"`
				} `json:"metadata"`
			}
			if json.Unmarshal([]byte(msg.String), &m) != nil || m.Metadata == nil || m.Metadata.Metrics == nil {
				return nil
			}
			u := m.Metadata.Metrics
			out = append(out, Call{ID: "devin:" + cmp.Or(m.Metadata.RequestID, session.String+":"+node.String), Time: sqlTime(at),
				Model: m.Metadata.Model, Dir: dir.String, Conversation: session.String,
				Input: max(u.Input-u.CacheRead, 0), CacheRead: u.CacheRead, CacheWrite: u.CacheCreate, Output: u.Output})
			return nil
		})
	return out, err
}

func readGrokDB(path string, st *FileState) ([]Call, error) {
	var out []Call
	err := query(path, `SELECT u.rowid, u.id, u.session_id, u.created_at, u.model, u.input_tokens, u.output_tokens, u.cost_micros, s.cwd_at_start
		FROM usage_events u LEFT JOIN sessions s ON s.id = u.session_id WHERE u.rowid > ? ORDER BY u.rowid`, []any{st.Mark},
		func(r *sql.Rows) error {
			var rowid int64
			var id, session, model, dir sql.NullString
			var at any
			var in, out_, micros sql.NullInt64
			if err := r.Scan(&rowid, &id, &session, &at, &model, &in, &out_, &micros, &dir); err != nil {
				return err
			}
			st.Mark = max(st.Mark, rowid)
			out = append(out, Call{ID: "grok:" + cmp.Or(id.String, strconv.FormatInt(rowid, 10)), Time: sqlTime(at), Model: model.String,
				Provider: "xai", Dir: dir.String, Conversation: session.String, Cost: float64(micros.Int64) / 1e6,
				Input: in.Int64, Output: out_.Int64})
			return nil
		})
	return out, err
}

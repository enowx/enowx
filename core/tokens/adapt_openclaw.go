package tokens

import (
	"cmp"
	"database/sql"
	"os"
	"path/filepath"
	"strings"

	"github.com/klauspost/compress/zstd"
)

// OpenClaw now keeps its transcripts in SQLite:
// ~/.openclaw/agents/<agent>/agent/openclaw-agent.sqlite
// ($OPENCLAW_STATE_DIR over ~/.openclaw), transcript_events a Pi line a
// row (event_json, or zstd-compressed in event_zstd), its session's
// header the first row; before, Pi's JSON lines in
// ~/.openclaw/agents/<agent>/sessions.

func openclawFiles(env Env) []string {
	root := cmp.Or(os.Getenv("OPENCLAW_STATE_DIR"), filepath.Join(env.Home, ".openclaw"))
	var out []string
	agents, _ := os.ReadDir(filepath.Join(root, "agents"))
	for _, a := range agents {
		if a.IsDir() {
			dir := filepath.Join(root, "agents", a.Name())
			out = append(out, filepath.Join(dir, "agent", "openclaw-agent.sqlite"))
			out = append(out, walk(filepath.Join(dir, "sessions"), ".jsonl")...)
		}
	}
	return out
}

func readOpenClaw(path string, st *FileState) ([]Call, error) {
	if strings.HasSuffix(path, ".jsonl") {
		return readPi(path, st)
	}
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro&_pragma=busy_timeout(3000)")
	if err != nil {
		return nil, err
	}
	defer db.Close()
	dec, err := zstd.NewReader(nil)
	if err != nil {
		return nil, err
	}
	defer dec.Close()
	text := func(plain sql.NullString, packed []byte) []byte {
		if plain.Valid && plain.String != "" {
			return []byte(plain.String)
		}
		if b, err := dec.DecodeAll(packed, nil); err == nil {
			return b
		}
		return nil
	}
	headers := map[string]*FileState{}
	header := func(session string) *FileState {
		if h := headers[session]; h != nil {
			return h
		}
		h := &FileState{}
		var plain sql.NullString
		var packed []byte
		if db.QueryRow(`SELECT event_json, event_zstd FROM transcript_events WHERE session_id = ? ORDER BY seq LIMIT 1`, session).Scan(&plain, &packed) == nil {
			piCall(text(plain, packed), h, session)
		}
		headers[session] = h
		return h
	}
	rows, err := db.Query(`SELECT rowid, session_id, event_json, event_zstd FROM transcript_events WHERE rowid > ? ORDER BY rowid`, st.Mark)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Call
	for rows.Next() {
		var rowid int64
		var session string
		var plain sql.NullString
		var packed []byte
		if rows.Scan(&rowid, &session, &plain, &packed) != nil {
			continue
		}
		st.Mark = max(st.Mark, rowid)
		if c, ok := piCall(text(plain, packed), header(session), session); ok {
			out = append(out, c)
		}
	}
	return out, rows.Err()
}

// Prime Agent keeps Pi's sessions in ~/.prime/agent/sessions.
func primeFiles(env Env) []string {
	return walk(filepath.Join(env.Home, ".prime", "agent", "sessions"), ".jsonl")
}

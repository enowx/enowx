package tokens

import (
	"cmp"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite" // OpenCode's database
)

// OpenCode keeps its sessions in $XDG_DATA_HOME/opencode (~/.local/share
// /opencode): in opencode.db (SQLite; opencode-<channel>.db, or
// $OPENCODE_DB) since 1.x, before that as JSON files under storage/
// (message/<session>/<id>.json, part/<message>/<id>.json). Each model
// call of an answer is a step-finish part with its tokens (input without
// the cache, output without reasoning, reasoning, cache.read,
// cache.write) and cost; the answer's message holds only its last step's,
// with providerID, modelID and path.cwd. Kilo Code's CLI (kilo.db) and
// MiMo Code (mimocode.db) keep the same.

// opencodeStore lists an OpenCode-like agent's records: the databases
// in its folders (and dbEnv's), and the step parts of the older layout.
func opencodeStore(dbEnv string, names ...string) func(env Env) []string {
	return func(env Env) []string {
		base := os.Getenv("XDG_DATA_HOME")
		if base == "" {
			base = filepath.Join(env.Home, ".local", "share")
		}
		var out []string
		if f := os.Getenv(dbEnv); dbEnv != "" && f != "" {
			out = append(out, f)
		}
		for _, n := range names {
			dir := filepath.Join(base, n)
			entries, _ := os.ReadDir(dir)
			for _, e := range entries {
				if !e.IsDir() && strings.HasSuffix(e.Name(), ".db") {
					out = append(out, filepath.Join(dir, e.Name()))
				}
			}
			out = append(out, walk(filepath.Join(dir, "storage", "part"), ".json")...)
		}
		return out
	}
}

// opencodeStep is a step-finish part.
type opencodeStep struct {
	ID        string  `json:"id"`
	SessionID string  `json:"sessionID"`
	MessageID string  `json:"messageID"`
	Type      string  `json:"type"`
	Cost      float64 `json:"cost"`
	Tokens    *struct {
		Input     int64 `json:"input"`
		Output    int64 `json:"output"`
		Reasoning int64 `json:"reasoning"`
		Cache     struct {
			Read  int64 `json:"read"`
			Write int64 `json:"write"`
		} `json:"cache"`
	} `json:"tokens"`
}

// opencodeMessage is what a step's message says of where it ran.
type opencodeMessage struct {
	ProviderID string `json:"providerID"`
	ModelID    string `json:"modelID"`
	Path       struct {
		Cwd string `json:"cwd"`
	} `json:"path"`
	Time struct {
		Created int64 `json:"created"`
	} `json:"time"`
}

func (s opencodeStep) call(m opencodeMessage, at time.Time) (Call, bool) {
	if s.Type != "step-finish" || s.Tokens == nil || s.ID == "" {
		return Call{}, false
	}
	t := s.Tokens
	return Call{ID: "opencode:" + s.ID, Time: at, Provider: providerName(m.ProviderID), Model: m.ModelID,
		Dir: m.Path.Cwd, Conversation: s.SessionID, Cost: s.Cost,
		Input: t.Input, CacheRead: t.Cache.Read, CacheWrite: t.Cache.Write, Output: t.Output + t.Reasoning, Reasoning: t.Reasoning}, true
}

func readOpenCode(path string, st *FileState) ([]Call, error) {
	if strings.HasSuffix(path, ".json") {
		return readOpenCodeFile(path)
	}
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro&_pragma=busy_timeout(3000)")
	if err != nil {
		return nil, err
	}
	defer db.Close()
	// A step's part is written once its call is done; those written
	// since the last read.
	rows, err := db.Query(`SELECT p.id, p.session_id, p.time_created, p.time_updated, p.data, m.data
		FROM part p JOIN message m ON m.id = p.message_id
		WHERE p.time_updated > ? AND p.data LIKE '%step-finish%' ORDER BY p.time_updated`, st.Mark)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Call
	for rows.Next() {
		var id, session, part, message string
		var created, updated int64
		if rows.Scan(&id, &session, &created, &updated, &part, &message) != nil {
			continue
		}
		st.Mark = max(st.Mark, updated)
		var s opencodeStep
		var m opencodeMessage
		if json.Unmarshal([]byte(part), &s) != nil || json.Unmarshal([]byte(message), &m) != nil {
			continue
		}
		s.ID, s.SessionID = cmp.Or(s.ID, id), cmp.Or(s.SessionID, session)
		if c, ok := s.call(m, time.UnixMilli(created)); ok {
			out = append(out, c)
		}
	}
	return out, rows.Err()
}

// readOpenCodeFile reads a step part of the older layout, its message
// beside it under storage/message.
func readOpenCodeFile(path string) ([]Call, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s opencodeStep
	if json.Unmarshal(data, &s) != nil || s.Type != "step-finish" {
		return nil, nil
	}
	storage := filepath.Dir(filepath.Dir(filepath.Dir(path))) // storage/part/<message>/<part>.json
	var m opencodeMessage
	if data, err := os.ReadFile(filepath.Join(storage, "message", s.SessionID, s.MessageID+".json")); err == nil {
		_ = json.Unmarshal(data, &m)
	}
	at := time.UnixMilli(m.Time.Created)
	if info, err := os.Stat(path); err == nil && m.Time.Created == 0 {
		at = info.ModTime()
	}
	if c, ok := s.call(m, at); ok {
		return []Call{c}, nil
	}
	return nil, nil
}

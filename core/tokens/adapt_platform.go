package tokens

import (
	"cmp"
	"encoding/json"
	"path/filepath"
	"strings"
	"time"

	"github.com/enowx/enowx/core/platformagent/record"
)

// Claude Platform, enowX's own agent, keeps a conversation in one file of JSON lines
// (package record): in a session's own folder, or its default one when run by hand. An
// Assistant record is a model call, and so is a Compact record's summary: its message ID,
// usage and cost, its model in the record or the last Meta. A subagent's calls are there too. The stores' folders hold
// other agents' files as well (OMP's), whose lines this reads as nothing.

func platformFiles(env Env) []string {
	return append(walk(record.SessionsDir(env.Home), ".jsonl"), walk(record.StoresDir(env.Home), ".jsonl")...)
}

func readPlatform(path string, st *FileState) ([]Call, error) {
	var out []Call
	err := readLines(path, st, func(line []byte) {
		if c, ok := platformCall(line, st, path); ok {
			out = append(out, c)
		}
	})
	return out, err
}

// platformCall reads one line of a conversation: its Meta into st, an answer as a call.
func platformCall(line []byte, st *FileState, path string) (Call, bool) {
	var rec record.Record
	if json.Unmarshal(line, &rec) != nil {
		return Call{}, false
	}
	switch rec.Type {
	case record.Meta:
		st.Conv, st.Dir, st.Model = cmp.Or(rec.Conversation, st.Conv), cmp.Or(rec.Dir, st.Dir), cmp.Or(rec.Model, st.Model)
		return Call{}, false
	case record.Assistant, record.Compact:
	default:
		return Call{}, false
	}
	// A compaction is a call too, older ones saying no message: their time tells them apart.
	msg := rec.Msg
	if rec.Type == record.Compact {
		msg = "compact:" + cmp.Or(rec.Msg, rec.Time.UTC().Format(time.RFC3339Nano))
	}
	if rec.Usage == nil || msg == "" {
		return Call{}, false
	}
	conv := cmp.Or(st.Conv, strings.TrimSuffix(filepath.Base(path), ".jsonl"))
	u := rec.Usage
	return Call{ID: "cp:" + conv + ":" + msg, Time: rec.Time, Provider: "anthropic", Model: cmp.Or(rec.Model, st.Model),
		ModelInferred: rec.Model == "" && rec.Cost == 0, Dir: st.Dir, Conversation: conv, Cost: rec.Cost,
		Input: u.Input, CacheRead: u.CacheRead, CacheWrite: u.CacheWrite, Output: u.Output}, true
}

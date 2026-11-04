package usersync

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/enowx/enowx/proto"
)

// Some files are logs: each host only ever adds lines to them, so changes on two hosts
// never really conflict. Merged by lines against a base they would, every time, both
// having added at the same place, and each conflict holds two copies of the whole log.
var appendOnly = []string{"vault/audit.jsonl"}

// AppendOnly says synced file p is a log, whose hosts' lines all stay when it merges.
func AppendOnly(p string) bool { return slices.Contains(appendOnly, p) }

// mergeLog is every line of logs a and b: a line both have once, as many times as the
// one having it most, each log's lines in their order, and between them by the time they
// say, so hosts' clocks can't reorder a host's own lines. The bytes of a line are kept as
// they are. It needs no base: what was in common is in both, and what one trimmed as too
// old the next merge brings back until it trims again (core/vault.Audit.Add).
func mergeLog(a, b []byte) []byte {
	la, lb := logLines(a), logLines(b)
	ta, tb := logTimes(la), logTimes(lb)
	aheadA, aheadB := lineCounts(la), lineCounts(lb)
	// A line written from one log, due in the other further on: skipped when reached.
	owedA, owedB := map[string]int{}, map[string]int{}
	out := make([]byte, 0, max(len(a), len(b)))
	i, j := 0, 0
	takeA := func() {
		x := la[i]
		out = append(out, x...)
		out = append(out, '\n')
		i++
		aheadA[x]--
		if aheadB[x] > 0 {
			aheadB[x]--
			owedB[x]++
		}
	}
	takeB := func() {
		x := lb[j]
		out = append(out, x...)
		out = append(out, '\n')
		j++
		aheadB[x]--
		if aheadA[x] > 0 {
			aheadA[x]--
			owedA[x]++
		}
	}
	for i < len(la) || j < len(lb) {
		switch {
		case i < len(la) && owedA[la[i]] > 0:
			owedA[la[i]]--
			i++
		case j < len(lb) && owedB[lb[j]] > 0:
			owedB[lb[j]]--
			j++
		case i == len(la):
			takeB()
		case j == len(lb):
			takeA()
		case logLineBefore(ta[i], la[i], tb[j], lb[j]):
			takeA() // the same line in both goes this way too, once
		default:
			takeB()
		}
	}
	return out
}

// logLineBefore says line x, of time tx, goes before y whichever log is merged into which.
func logLineBefore(tx time.Time, x string, ty time.Time, y string) bool {
	if c := tx.Compare(ty); c != 0 {
		return c < 0
	}
	return x <= y
}

// logLines are a log's lines without their ends, the empty ones gone.
func logLines(data []byte) []string {
	var out []string
	for line := range strings.Lines(string(data)) {
		if line = strings.TrimRight(line, "\r\n"); line != "" {
			out = append(out, line)
		}
	}
	return out
}

// logTimes is the time each line says; one saying none stays by the line before it.
func logTimes(lines []string) []time.Time {
	out := make([]time.Time, len(lines))
	var last time.Time
	for i, line := range lines {
		var e struct {
			Time time.Time `json:"time"`
		}
		if json.Unmarshal([]byte(line), &e) == nil && !e.Time.IsZero() {
			last = e.Time
		}
		out[i] = last
	}
	return out
}

func lineCounts(lines []string) map[string]int {
	out := make(map[string]int, len(lines))
	for _, l := range lines {
		out[l]++
	}
	return out
}

// settleLogs merges away the conflicts on logs, made by an older app or before logs
// merged: the file in place with every version the conflict kept, no line lost, then the
// conflict to the trash. A failure leaves that conflict, to try next time.
func (f *Folder) settleLogs(heads []proto.SyncConflict) []proto.SyncConflict {
	return slices.DeleteFunc(heads, func(c proto.SyncConflict) bool {
		return AppendOnly(c.Path) && f.settleLog(c.ID, c.Path) == nil
	})
}

func (f *Folder) settleLog(id, p string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if g := f.guards[p]; g != nil {
		g.Lock()
		defer g.Unlock()
	}
	data, err := os.ReadFile(f.conflictFile(id))
	if errors.Is(err, fs.ErrNotExist) {
		return nil // settled meanwhile
	}
	if err != nil {
		return fmt.Errorf("usersync: %w", err)
	}
	var c proto.SyncConflict
	if err := json.Unmarshal(data, &c); err != nil {
		return fmt.Errorf("usersync: conflict %s: %w", id, err)
	}
	file := filepath.Join(f.dir, filepath.FromSlash(p))
	cur, err := os.ReadFile(file)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("usersync: %w", err)
	}
	merged := cur
	for _, v := range c.Versions {
		if !v.Deleted {
			merged = mergeLog(merged, v.Data)
		}
	}
	if !bytes.Equal(merged, cur) {
		if err := writeAtomic(file, merged); err != nil {
			return err
		}
		delete(f.cache, p)
	}
	rel := ConflictsDir + c.ID + ".json"
	delete(f.cache, rel)
	return f.trash(rel)
}

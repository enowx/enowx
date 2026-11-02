package usersync

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"maps"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/enowx/enowx/proto"
)

// Side is a host in a sync: a daemon's folder, through its connection.
type Side interface {
	Manifest() (proto.SyncManifest, error)
	Read(paths []string) (map[string][]byte, error)
	Write(changes []proto.SyncChange) (applied, stale []string, err error)
}

// Report is what a sync did.
type Report struct {
	Sent, Received, Merged, Conflicts, Deduped int
	// Changed are the paths changed on each side: [0] the first's.
	Changed [2][]string
	// Skipped are files too large to carry (maxFile), left as they are on
	// each side.
	Skipped []string
}

// mergeMax is the largest file kept in a Base to merge against.
const mergeMax = 1 << 20

// maxFile is the largest file a sync carries: one message holds it, as
// base64 in JSON, with room for the message around it. A larger one is
// skipped, so the rest still syncs.
const maxFile = proto.MaxPayload/4*3 - 256<<10

// Sync brings sides a and b to the same files: what changed on one since
// base (their last sync, updated here) goes to the other, what changed on
// both merges, and what doesn't merge stays as a's, b's version kept in a
// conflict on both. Stale writes (the file changed meanwhile) wait for the
// next sync.
func Sync(a, b Side, base *Base) (Report, error) {
	var rep Report
	ma, err := a.Manifest()
	if err != nil {
		return rep, err
	}
	mb, err := b.Manifest()
	if err != nil {
		return rep, err
	}
	ha, hb := hashes(ma.Files), hashes(mb.Files)
	// What is no longer synced stays as it is on each side, and isn't "deleted" on
	// one by the side that stopped listing it. A host still on an older build lists it.
	for _, m := range []map[string]string{ha, hb} {
		maps.DeleteFunc(m, func(p, _ string) bool { return !Synced(p) })
	}
	maps.DeleteFunc(base.Files, func(p string, _ BaseFile) bool { return !Synced(p) })
	skip := map[string]bool{}
	for _, f := range append(slices.Clip(ma.Files), mb.Files...) {
		if f.Size > maxFile && Synced(f.Path) && !skip[f.Path] {
			skip[f.Path] = true
			rep.Skipped = append(rep.Skipped, f.Path)
			delete(ha, f.Path)
			delete(hb, f.Path)
		}
	}
	slices.Sort(rep.Skipped)
	if len(base.Files) == 0 {
		if rep.Deduped, err = dedupe(a, b, ha, hb); err != nil {
			return rep, err
		}
	}
	var paths []string
	for _, m := range []map[string]string{ha, hb} {
		for p := range m {
			paths = append(paths, p)
		}
	}
	for p := range base.Files {
		if !skip[p] {
			paths = append(paths, p)
		}
	}
	slices.Sort(paths)
	paths = slices.Compact(paths)

	type plan struct {
		path         string
		toA, toB     bool // copy the other's
		merge        bool
		hashA, hashB string
	}
	var plans []plan
	var readA, readB []string
	for _, p := range paths {
		x, y, was := ha[p], hb[p], base.Files[p].Hash
		switch {
		case x == y:
			base.set(p, x, nil)
		case y == was:
			plans = append(plans, plan{path: p, toB: true, hashA: x, hashB: y})
			if x != "" {
				readA = append(readA, p)
			}
		case x == was:
			plans = append(plans, plan{path: p, toA: true, hashA: x, hashB: y})
			if y != "" {
				readB = append(readB, p)
			}
		case x == "": // a deleted what b changed: b's comes back
			plans = append(plans, plan{path: p, toA: true, hashA: x, hashB: y})
			readB = append(readB, p)
		case y == "":
			plans = append(plans, plan{path: p, toB: true, hashA: x, hashB: y})
			readA = append(readA, p)
		default:
			plans = append(plans, plan{path: p, merge: true, hashA: x, hashB: y})
			readA, readB = append(readA, p), append(readB, p)
		}
	}
	da, err := readAll(a, readA)
	if err != nil {
		return rep, err
	}
	db, err := readAll(b, readB)
	if err != nil {
		return rep, err
	}
	var toA, toB []proto.SyncChange
	final := map[string][]byte{} // what both will have, by path
	var conflicts []proto.SyncConflict
	inConflict := map[string]bool{}
	for _, pl := range plans {
		switch {
		case pl.toB:
			data, ok := da[pl.path]
			if pl.hashA != "" && !ok {
				continue // gone meanwhile
			}
			toB = append(toB, proto.SyncChange{Path: pl.path, Data: data, Delete: pl.hashA == "", Expect: pl.hashB})
			final[pl.path] = data
		case pl.toA:
			data, ok := db[pl.path]
			if pl.hashB != "" && !ok {
				continue
			}
			toA = append(toA, proto.SyncChange{Path: pl.path, Data: data, Delete: pl.hashB == "", Expect: pl.hashA})
			final[pl.path] = data
		default:
			x, okx := da[pl.path]
			y, oky := db[pl.path]
			if !okx || !oky {
				continue
			}
			if merged, ok := mergeFile(pl.path, base.Files[pl.path].Data, x, y); ok {
				toA = append(toA, proto.SyncChange{Path: pl.path, Data: merged, Expect: pl.hashA})
				toB = append(toB, proto.SyncChange{Path: pl.path, Data: merged, Expect: pl.hashB})
				final[pl.path] = merged
				rep.Merged++
				continue
			}
			c := newConflict(pl.path, [2]string{ma.Name, mb.Name}, x, y)
			conflicts = append(conflicts, c)
			inConflict[pl.path] = true
			toB = append(toB, proto.SyncChange{Path: pl.path, Data: x, Expect: pl.hashB})
			final[pl.path] = x
			rep.Conflicts++
		}
	}
	for _, c := range conflicts {
		data, err := encodeConflict(c)
		if err != nil {
			return rep, err
		}
		p := ConflictsDir + c.ID + ".json"
		toA = append(toA, proto.SyncChange{Path: p, Data: data})
		toB = append(toB, proto.SyncChange{Path: p, Data: data})
		final[p] = data
	}
	appliedA, _, err := writeAll(a, toA)
	if err != nil {
		return rep, err
	}
	appliedB, _, err := writeAll(b, toB)
	if err != nil {
		return rep, err
	}
	// The base takes what both sides now have.
	ok := map[string]int{}
	for _, p := range appliedA {
		ok[p]++
	}
	for _, p := range appliedB {
		ok[p]++
	}
	for _, pl := range plans {
		p := pl.path
		switch {
		case pl.toB && ok[p] == 1:
			base.set(p, pl.hashA, final[p])
			rep.Sent++
		case pl.toA && ok[p] == 1:
			base.set(p, pl.hashB, final[p])
			rep.Received++
		case inConflict[p] && ok[p] == 1, !pl.toA && !pl.toB && ok[p] == 2:
			data := final[p]
			base.set(p, Hash(data), data)
		}
	}
	for _, c := range conflicts {
		p := ConflictsDir + c.ID + ".json"
		if ok[p] == 2 {
			base.set(p, Hash(final[p]), nil)
		}
	}
	rep.Changed = [2][]string{appliedA, appliedB}
	base.Last = time.Now().UTC()
	return rep, nil
}

func hashes(files []proto.SyncEntry) map[string]string {
	out := map[string]string{}
	for _, f := range files {
		if Synced(f.Path) {
			out[f.Path] = f.Hash
		}
	}
	return out
}

func readAll(s Side, paths []string) (map[string][]byte, error) {
	out := map[string][]byte{}
	for i := 0; i < len(paths); i += 200 {
		got, err := s.Read(paths[i:min(i+200, len(paths))])
		if err != nil {
			return nil, err
		}
		for p, d := range got {
			out[p] = d
		}
	}
	return out, nil
}

func writeAll(s Side, changes []proto.SyncChange) (applied, stale []string, err error) {
	for i := 0; i < len(changes); i += 100 {
		a, st, err := s.Write(changes[i:min(i+100, len(changes))])
		applied, stale = append(applied, a...), append(stale, st...)
		if err != nil {
			return applied, stale, err
		}
	}
	return applied, stale, nil
}

func newConflict(p string, names [2]string, a, b []byte) proto.SyncConflict {
	var id [6]byte
	rand.Read(id[:])
	c := proto.SyncConflict{ID: hex.EncodeToString(id[:]), Path: p, Created: time.Now().UTC(),
		Versions: []proto.SyncVersion{{Host: names[0], Data: a}, {Host: names[1], Data: b}}}
	if strings.HasPrefix(p, "notes/") && path.Ext(p) == ".md" {
		c.Title = noteTitle(a)
	}
	return c
}

// dedupe drops b's notes that are a's notes under another ID, same title
// and body, at the first sync of two hosts that kept notes apart.
func dedupe(a, b Side, ha, hb map[string]string) (int, error) {
	var onlyA, onlyB []string
	for p := range ha {
		if _, ok := hb[p]; !ok && isNote(p) {
			onlyA = append(onlyA, p)
		}
	}
	for p := range hb {
		if _, ok := ha[p]; !ok && isNote(p) {
			onlyB = append(onlyB, p)
		}
	}
	if len(onlyA) == 0 || len(onlyB) == 0 {
		return 0, nil
	}
	da, err := readAll(a, onlyA)
	if err != nil {
		return 0, err
	}
	db, err := readAll(b, onlyB)
	if err != nil {
		return 0, err
	}
	seen := map[string]bool{}
	for _, d := range da {
		seen[noteKey(d)] = true
	}
	parents := map[string]bool{}
	for _, d := range db {
		head, _, _ := splitNote(string(d))
		for _, kv := range head {
			if kv[0] == "parent" {
				parents[kv[1]] = true
			}
		}
	}
	var drop []proto.SyncChange
	for p, d := range db {
		id := strings.TrimSuffix(path.Base(p), ".md")
		if k := noteKey(d); k != "" && seen[k] && !parents[id] {
			drop = append(drop, proto.SyncChange{Path: p, Delete: true, Expect: hb[p]})
		}
	}
	applied, _, err := writeAll(b, drop)
	for _, p := range applied {
		delete(hb, p)
	}
	return len(applied), err
}

func isNote(p string) bool {
	return strings.HasPrefix(p, "notes/") && !strings.Contains(p[6:], "/") && path.Ext(p) == ".md"
}

// noteKey is what makes two notes the same: title and body.
func noteKey(data []byte) string {
	head, body, ok := splitNote(string(data))
	if !ok {
		return ""
	}
	title := ""
	for _, kv := range head {
		if kv[0] == "title" {
			title = kv[1]
		}
	}
	return fmt.Sprintf("%s\x00%s", title, strings.TrimSpace(body))
}

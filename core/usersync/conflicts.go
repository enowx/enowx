package usersync

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/enowx/enowx/proto"
)

func encodeConflict(c proto.SyncConflict) ([]byte, error) { return json.MarshalIndent(c, "", "  ") }

func (f *Folder) conflictFile(id string) string {
	return filepath.Join(f.dir, filepath.FromSlash(ConflictsDir), filepath.Base(id)+".json")
}

// conflictHead is a conflict's file's list entry, while the file stays as it was.
type conflictHead struct {
	size int64
	mod  time.Time
	c    proto.SyncConflict
}

// Conflicts lists the conflicts waiting for the user, the oldest first, without their
// versions' data, which ConflictVersion reads. A file is
// read once: asked often and holding whole versions, they ran to 340 MB allocated a call.
func (f *Folder) Conflicts() ([]proto.SyncConflict, error) {
	dir := filepath.Join(f.dir, filepath.FromSlash(ConflictsDir))
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return []proto.SyncConflict{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("usersync: %w", err)
	}
	out := []proto.SyncConflict{}
	keep := make(map[string]conflictHead, len(entries))
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		f.mu.Lock()
		h, ok := f.heads[e.Name()]
		f.mu.Unlock()
		if !ok || h.size != info.Size() || !h.mod.Equal(info.ModTime()) {
			c, err := readConflictHead(filepath.Join(dir, e.Name()))
			if err != nil {
				continue
			}
			h = conflictHead{info.Size(), info.ModTime(), c}
		}
		keep[e.Name()] = h
		out = append(out, h.c)
	}
	f.mu.Lock()
	f.heads = keep
	f.mu.Unlock()
	slices.SortFunc(out, func(a, b proto.SyncConflict) int { return a.Created.Compare(b.Created) })
	return out, nil
}

// readConflictHead reads a conflict's file, its versions' data left as sizes.
func readConflictHead(file string) (proto.SyncConflict, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return proto.SyncConflict{}, err
	}
	var c proto.SyncConflict
	if err := json.Unmarshal(data, &c); err != nil {
		return c, err
	}
	if c.ID == "" {
		return c, errors.New("no ID")
	}
	for i := range c.Versions {
		v := &c.Versions[i]
		v.Size, v.Data = int64(len(v.Data)), nil
	}
	return c, nil
}

// ConflictVersion is version n, from 1, of conflict id, with its data.
func (f *Folder) ConflictVersion(id string, n int) (proto.SyncVersion, error) {
	data, err := os.ReadFile(f.conflictFile(id))
	if errors.Is(err, fs.ErrNotExist) {
		return proto.SyncVersion{}, fmt.Errorf("usersync: no conflict %q", id)
	}
	if err != nil {
		return proto.SyncVersion{}, fmt.Errorf("usersync: %w", err)
	}
	var c proto.SyncConflict
	if err := json.Unmarshal(data, &c); err != nil {
		return proto.SyncVersion{}, fmt.Errorf("usersync: conflict %s: %w", id, err)
	}
	if n < 1 || n > len(c.Versions) {
		return proto.SyncVersion{}, fmt.Errorf("usersync: conflict %s has %d versions", id, len(c.Versions))
	}
	v := c.Versions[n-1]
	v.Size = int64(len(v.Data))
	return v, nil
}

// Resolve settles conflict id: the version of host keep (or numbered
// keep, from 1) takes the file's place, or with keep "both", the one in place stays and each other
// version is kept beside it, a note as a copy of its own.
func (f *Folder) Resolve(id, keep string) error {
	data, err := os.ReadFile(f.conflictFile(id))
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("usersync: no conflict %q", id)
	}
	if err != nil {
		return fmt.Errorf("usersync: %w", err)
	}
	var c proto.SyncConflict
	if err := json.Unmarshal(data, &c); err != nil {
		return fmt.Errorf("usersync: conflict %s: %w", id, err)
	}
	if !Synced(c.Path) {
		return fmt.Errorf("usersync: conflict %s names %q", id, c.Path)
	}
	cur, err := f.Read([]string{c.Path})
	if err != nil {
		return err
	}
	expect := ""
	if d, ok := cur[c.Path]; ok {
		expect = Hash(d)
	}
	var changes []proto.SyncChange
	if keep == "both" {
		for i, v := range c.Versions {
			if v.Deleted || expect != "" && Hash(v.Data) == expect {
				continue
			}
			p, data := copyPath(c.Path, i), v.Data
			if isNote(c.Path) {
				data = renameNote(data, v.Host)
			}
			changes = append(changes, proto.SyncChange{Path: p, Data: data})
		}
	} else {
		i := slices.IndexFunc(c.Versions, func(v proto.SyncVersion) bool { return v.Host == keep })
		if n, err := strconv.Atoi(keep); err == nil && n >= 1 && n <= len(c.Versions) {
			i = n - 1
		}
		if i < 0 {
			var hosts []string
			for _, v := range c.Versions {
				hosts = append(hosts, v.Host)
			}
			return fmt.Errorf("usersync: conflict %s has versions of %s, or keep both", id, strings.Join(hosts, ", "))
		}
		v := c.Versions[i]
		changes = append(changes, proto.SyncChange{Path: c.Path, Data: v.Data, Delete: v.Deleted, Expect: expect})
	}
	changes = append(changes, proto.SyncChange{Path: ConflictsDir + c.ID + ".json", Delete: true, Expect: Hash(data)})
	_, stale, err := f.Write(changes)
	if err != nil {
		return err
	}
	if len(stale) > 0 {
		return fmt.Errorf("usersync: %s changed meanwhile; try again", strings.Join(stale, ", "))
	}
	return nil
}

// copyPath is where version i of p goes beside it: a note gets an ID of
// its own.
func copyPath(p string, i int) string {
	if isNote(p) {
		var b [6]byte
		rand.Read(b[:])
		return "notes/" + hex.EncodeToString(b[:]) + ".md"
	}
	ext := path.Ext(p)
	return fmt.Sprintf("%s.conflict-%d%s", strings.TrimSuffix(p, ext), i+1, ext)
}

// renameNote titles a note's copy after the host it came from.
func renameNote(data []byte, host string) []byte {
	head, body, ok := splitNote(string(data))
	if !ok {
		return data
	}
	var b strings.Builder
	b.WriteString("---\n")
	for _, kv := range head {
		if kv[0] == "title" {
			kv[1] += " (" + host + ")"
		}
		b.WriteString(kv[0] + ": " + kv[1] + "\n")
	}
	b.WriteString("---\n" + body)
	return []byte(b.String())
}

package usersync

import (
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

// The trash keeps deleted synced files 30 days, per host, to restore:
// trash/<unix nanos>/<path>.

// TrashDays is how long deleted files are kept.
const TrashDays = 30

func (f *Folder) trashDir() string { return filepath.Join(f.dir, "trash") }

// trash moves synced file p to the trash; f.mu is held.
func (f *Folder) trash(p string) error {
	src := filepath.Join(f.dir, filepath.FromSlash(p))
	dst := filepath.Join(f.trashDir(), fmt.Sprint(time.Now().UnixNano()), filepath.FromSlash(p))
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return fmt.Errorf("usersync: %w", err)
	}
	if err := os.Rename(src, dst); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("usersync: %w", err)
	}
	return nil
}

// Trash lists what was deleted lately, the latest first; notes say their
// titles.
func (f *Folder) Trash() ([]proto.SyncTrashItem, error) {
	var out []proto.SyncTrashItem
	err := filepath.WalkDir(f.trashDir(), func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(f.trashDir(), p)
		stamp, path, ok := strings.Cut(filepath.ToSlash(rel), "/")
		if !ok {
			return nil
		}
		var nanos int64
		if _, err := fmt.Sscan(stamp, &nanos); err != nil {
			return nil
		}
		if strings.HasPrefix(path, ConflictsDir) {
			return nil // settled: nothing the user would bring back
		}
		item := proto.SyncTrashItem{ID: filepath.ToSlash(rel), Path: path, Deleted: time.Unix(0, nanos).UTC()}
		if strings.HasSuffix(path, ".md") {
			if data, err := os.ReadFile(p); err == nil {
				item.Title = noteTitle(data)
			}
		}
		out = append(out, item)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("usersync: %w", err)
	}
	slices.SortFunc(out, func(a, b proto.SyncTrashItem) int { return b.Deleted.Compare(a.Deleted) })
	return out, nil
}

// Restore puts trash item id back, unless a file took its place.
func (f *Folder) Restore(id string) error {
	id = cleanRel(id)
	_, p, ok := strings.Cut(id, "/")
	if id == "" || !ok || !Synced(p) {
		return fmt.Errorf("usersync: no deleted file %q", id)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	dst := filepath.Join(f.dir, filepath.FromSlash(p))
	if _, err := os.Stat(dst); err == nil {
		return fmt.Errorf("usersync: %s is there again; delete it first", p)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return fmt.Errorf("usersync: %w", err)
	}
	if err := os.Rename(filepath.Join(f.trashDir(), filepath.FromSlash(id)), dst); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("usersync: no deleted file %q", id)
		}
		return fmt.Errorf("usersync: %w", err)
	}
	delete(f.cache, p)
	return nil
}

// EmptyTrash forgets what was deleted more than TrashDays ago.
func (f *Folder) EmptyTrash(now time.Time) {
	entries, _ := os.ReadDir(f.trashDir())
	for _, e := range entries {
		var nanos int64
		if _, err := fmt.Sscan(e.Name(), &nanos); err != nil {
			continue
		}
		if now.Sub(time.Unix(0, nanos)) > TrashDays*24*time.Hour {
			os.RemoveAll(filepath.Join(f.trashDir(), e.Name()))
		}
	}
}

// noteTitle is a note file's title, from its header.
func noteTitle(data []byte) string {
	head, _, ok := splitNote(string(data))
	if !ok {
		return ""
	}
	for _, kv := range head {
		if kv[0] == "title" {
			return kv[1]
		}
	}
	return ""
}

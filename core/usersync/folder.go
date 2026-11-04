package usersync

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/enowx/enowx/core/notes"
	"github.com/enowx/enowx/proto"
)

// Folder is a daemon's folder as a side of a sync: it lists, reads and
// writes the synced files in it. Safe for concurrent use.
type Folder struct {
	dir string
	// Name is the host's, for conflicts to say whose version is whose.
	Name string

	mu    sync.Mutex
	cache map[string]cached // hashes, by path, while size and time stay
	// heads are the conflicts without their data, by file name, while size and time stay.
	heads map[string]conflictHead
	// guards are held around replacing a file something else appends to.
	guards map[string]sync.Locker
}

type cached struct {
	size int64
	mod  time.Time
	hash string
}

// OpenFolder is daemon folder dir's synced files.
func OpenFolder(dir string) *Folder {
	return &Folder{dir: dir, cache: map[string]cached{}, heads: map[string]conflictHead{}, guards: map[string]sync.Locker{}}
}

// Guard has writes replacing synced file p hold l: what appends to p holds it too, or
// a line it adds as the file is replaced goes with the old file. Call it before syncing.
func (f *Folder) Guard(p string, l sync.Locker) { f.guards[p] = l }

// ID is the folder's identity for syncs, made the first time: the
// daemon's own (daemon-id, proto.DaemonInfo.ID).
func (f *Folder) ID() (string, error) {
	file := filepath.Join(f.dir, "daemon-id")
	if data, err := os.ReadFile(file); err == nil && len(strings.TrimSpace(string(data))) > 0 {
		return strings.TrimSpace(string(data)), nil
	}
	var b [8]byte
	rand.Read(b[:])
	id := hex.EncodeToString(b[:])
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		return "", fmt.Errorf("usersync: %w", err)
	}
	if err := os.WriteFile(file, []byte(id+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("usersync: %w", err)
	}
	return id, nil
}

// Hash is content's hash as manifests give it.
func Hash(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:16])
}

// Files lists the synced files, sorted by path.
func (f *Folder) Files() ([]proto.SyncEntry, error) {
	var out []proto.SyncEntry
	seen := map[string]bool{}
	for _, root := range Roots {
		top := filepath.Join(f.dir, filepath.FromSlash(root))
		err := filepath.WalkDir(top, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					return nil
				}
				return err
			}
			rel, _ := filepath.Rel(f.dir, p)
			rel = filepath.ToSlash(rel)
			if d.IsDir() {
				if strings.HasPrefix(d.Name(), ".") && p != top {
					return filepath.SkipDir
				}
				return nil
			}
			if !d.Type().IsRegular() || !Synced(rel) || seen[rel] {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return nil // gone meanwhile
			}
			hash, err := f.hash(rel, p, info)
			if err != nil {
				return nil
			}
			seen[rel] = true
			out = append(out, proto.SyncEntry{Path: rel, Hash: hash, Size: info.Size(), Mod: info.ModTime().UTC()})
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("usersync: %w", err)
		}
	}
	slices.SortFunc(out, func(a, b proto.SyncEntry) int { return strings.Compare(a.Path, b.Path) })
	return out, nil
}

func (f *Folder) hash(rel, p string, info fs.FileInfo) (string, error) {
	f.mu.Lock()
	c, ok := f.cache[rel]
	f.mu.Unlock()
	if ok && c.size == info.Size() && c.mod.Equal(info.ModTime()) {
		return c.hash, nil
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	h := Hash(data)
	f.mu.Lock()
	f.cache[rel] = cached{info.Size(), info.ModTime(), h}
	f.mu.Unlock()
	return h, nil
}

// Read reads synced files; those missing are left out.
func (f *Folder) Read(paths []string) (map[string][]byte, error) {
	out := map[string][]byte{}
	for _, p := range paths {
		if !Synced(p) {
			return nil, fmt.Errorf("usersync: %q is not synced", p)
		}
		data, err := os.ReadFile(filepath.Join(f.dir, filepath.FromSlash(p)))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("usersync: %w", err)
		}
		out[p] = data
	}
	return out, nil
}

// Write makes the changes whose files still have the hash they expect,
// deleted files going to the trash; the others come back as stale.
func (f *Folder) Write(changes []proto.SyncChange) (applied, stale []string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, ch := range changes {
		if g := f.guards[ch.Path]; g != nil {
			g.Lock()
			defer g.Unlock()
		}
	}
	for _, ch := range changes {
		if !Synced(ch.Path) {
			return applied, stale, fmt.Errorf("usersync: %q is not synced", ch.Path)
		}
		file := filepath.Join(f.dir, filepath.FromSlash(ch.Path))
		now := ""
		if data, err := os.ReadFile(file); err == nil {
			now = Hash(data)
		} else if !errors.Is(err, fs.ErrNotExist) {
			return applied, stale, fmt.Errorf("usersync: %w", err)
		}
		if now != ch.Expect {
			stale = append(stale, ch.Path)
			continue
		}
		if now != "" && isNote(ch.Path) {
			if old, err := os.ReadFile(file); err == nil {
				_ = notes.KeepVersion(filepath.Join(f.dir, "notes"), strings.TrimSuffix(path.Base(ch.Path), ".md"), old, true)
			}
		}
		if ch.Delete {
			if now != "" {
				if err := f.trash(ch.Path); err != nil {
					return applied, stale, err
				}
			}
		} else if err := writeAtomic(file, ch.Data); err != nil {
			return applied, stale, err
		}
		delete(f.cache, ch.Path)
		applied = append(applied, ch.Path)
	}
	return applied, stale, nil
}

func writeAtomic(file string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		return fmt.Errorf("usersync: %w", err)
	}
	tmp := file + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("usersync: %w", err)
	}
	if err := os.Rename(tmp, file); err != nil {
		return fmt.Errorf("usersync: %w", err)
	}
	return nil
}

// Remove takes the synced data off this host: every synced file to the
// trash, for when the user drops the host.
func (f *Folder) Remove() error {
	files, err := f.Files()
	if err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, file := range files {
		if err := f.trash(file.Path); err != nil {
			return err
		}
	}
	f.cache = map[string]cached{}
	return nil
}

// cleanRel is p as a slash path, or "" when it leaves the folder.
func cleanRel(p string) string {
	p = path.Clean(strings.ReplaceAll(p, "\\", "/"))
	if strings.HasPrefix(p, "../") || p == ".." || path.IsAbs(p) {
		return ""
	}
	return p
}

// Manifest is the folder's synced files, with its ID and name (the
// host's), so a Folder is a Side.
func (f *Folder) Manifest() (proto.SyncManifest, error) {
	id, err := f.ID()
	if err != nil {
		return proto.SyncManifest{}, err
	}
	if _, err := f.Conflicts(); err != nil { // those on logs settle, by themselves
		return proto.SyncManifest{}, err
	}
	files, err := f.Files()
	if err != nil {
		return proto.SyncManifest{}, err
	}
	return proto.SyncManifest{ID: id, Name: f.Name, Files: files}, nil
}

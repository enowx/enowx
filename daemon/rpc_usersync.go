package daemon

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/enowx/enowx/core/usersync"
	"github.com/enowx/enowx/proto"
)

// The user's data synced between hosts (core/usersync): the app lists,
// reads and writes this host's files and reports how its syncs went.

var syncHandlers = map[string]handler{
	proto.MethodSyncManifest: func(c *conn, raw json.RawMessage) (any, error) {
		var p proto.SyncManifestParams
		if len(raw) > 0 {
			if err := decode(raw, &p); err != nil {
				return nil, err
			}
		}
		if !p.KeepsOwnHistory {
			// Syncing from an older app: it'd take this host not listing a canvas's versions
			// for their deletion, and delete them on the host it syncs this one with.
			return nil, errSyncNeedsNewerApp
		}
		return c.srv.userdata.Manifest()
	},
	proto.MethodSyncRead: with(func(c *conn, p proto.SyncReadParams) (any, error) {
		files, err := c.srv.userdata.Read(p.Paths)
		return proto.SyncReadResult{Files: files}, err
	}),
	proto.MethodSyncWrite: with(func(c *conn, p proto.SyncWriteParams) (any, error) {
		applied, stale, err := c.srv.userdata.Write(p.Changes)
		c.srv.syncApplied(applied, p.From)
		if err != nil {
			return nil, err
		}
		return proto.SyncWriteResult{Applied: applied, Stale: stale}, nil
	}),
	proto.MethodSyncReport: with(func(c *conn, p proto.SyncPeerStatus) (any, error) {
		if err := c.srv.syncPeers.report(c.srv.syncStatusFile(), p); err != nil {
			return nil, err
		}
		if st, err := c.srv.syncStatus(); err == nil {
			c.srv.broadcast(proto.EventSyncStatus, st)
		}
		return ok, nil
	}),
	proto.MethodSyncStatus: bare(func(c *conn) (any, error) { return c.srv.syncStatus() }),
	proto.MethodSyncRequest: bare(func(c *conn) (any, error) {
		c.srv.broadcast(proto.EventSyncRequested, struct{}{})
		return ok, nil
	}),
	proto.MethodSyncConflicts: bare(func(c *conn) (any, error) { return c.srv.userdata.Conflicts() }),
	proto.MethodSyncConflict: with(func(c *conn, p proto.SyncConflictParams) (any, error) {
		return c.srv.userdata.ConflictVersion(p.ID, p.Version)
	}),
	proto.MethodSyncResolve: with(func(c *conn, p proto.SyncResolveParams) (any, error) {
		if err := c.srv.userdata.Resolve(p.ID, p.Keep); err != nil {
			return nil, err
		}
		c.srv.syncApplied([]string{usersync.ConflictsDir, "notes/"}, "")
		return ok, nil
	}),
	proto.MethodSyncTrash: bare(func(c *conn) (any, error) { return c.srv.userdata.Trash() }),
	proto.MethodSyncRestore: with(func(c *conn, p proto.SyncTrashItem) (any, error) {
		if err := c.srv.userdata.Restore(p.ID); err != nil {
			return nil, err
		}
		_, path, _ := strings.Cut(p.ID, "/")
		c.srv.syncApplied([]string{path}, "")
		return ok, nil
	}),
	proto.MethodCatalogList: bare(func(c *conn) (any, error) { return c.srv.catalogList() }),
	proto.MethodSyncForget: bare(func(c *conn) (any, error) {
		if err := c.srv.userdata.Remove(); err != nil {
			return nil, err
		}
		c.srv.syncApplied([]string{"notes/"}, "")
		return ok, nil
	}),
}

// syncApplied tells the apps what changed in the synced data, and the
// notes and their groups again when they did.
func (s *Server) syncApplied(paths []string, from string) {
	if len(paths) == 0 {
		return
	}
	s.broadcast(proto.EventSyncApplied, proto.SyncApplied{Paths: paths, From: from})
	s.notifySynced(paths)
	s.synced(paths)
	if slices.ContainsFunc(paths, func(p string) bool { return strings.HasPrefix(p, "notes/") }) {
		s.groupsChanged(nil)
	}
	if slices.Contains(paths, "settings/traffic.json") {
		s.capturePrefsChanged()
	}
}

func (s *Server) syncStatusFile() string { return filepath.Join(s.dataDir, "sync", "status.json") }

// syncStatus is this host's synced data and how its syncs went.
func (s *Server) syncStatus() (proto.SyncStatus, error) {
	m, err := s.userdata.Manifest()
	if err != nil {
		return proto.SyncStatus{}, err
	}
	st := proto.SyncStatus{ID: m.ID, Name: m.Name, Files: len(m.Files), Peers: s.syncPeers.load(s.syncStatusFile())}
	for _, f := range m.Files {
		st.Size += f.Size
		if strings.HasPrefix(f.Path, usersync.ConflictsDir) {
			st.Conflicts++
		}
	}
	return st, nil
}

// syncPeers keeps how the syncs with each peer went, in a file.
type syncPeers struct{ mu sync.Mutex }

func (sp *syncPeers) load(file string) []proto.SyncPeerStatus {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	return readPeers(file)
}

func readPeers(file string) []proto.SyncPeerStatus {
	out := []proto.SyncPeerStatus{}
	if data, err := os.ReadFile(file); err == nil {
		_ = json.Unmarshal(data, &out)
	}
	return out
}

func (sp *syncPeers) report(file string, p proto.SyncPeerStatus) error {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	list := readPeers(file)
	if p.Last.IsZero() {
		p.Last = time.Now().UTC()
	}
	if i := slices.IndexFunc(list, func(o proto.SyncPeerStatus) bool { return o.Peer == p.Peer }); i >= 0 {
		list[i] = p
	} else {
		list = append(list, p)
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		return err
	}
	return os.WriteFile(file, data, 0o600)
}

// errSyncNeedsNewerApp is what an older app syncing with this host is told.
var errSyncNeedsNewerApp = errors.New("this host runs a newer enowX than the app syncing it: update the app, and sync goes on")

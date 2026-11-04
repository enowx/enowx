package client

import (
	"fmt"
	"strings"

	"github.com/enowx/enowx/proto"
)

// SyncSide is a host's synced data, through c, as a side of a sync
// (usersync.Side); From names the other host in what it writes.
type SyncSide struct {
	C    *Client
	From string
	// Name, when set, names the host instead of its host name, as the
	// user knows it.
	Name string
}

// Manifest lists the host's synced files.
func (s SyncSide) Manifest() (proto.SyncManifest, error) {
	m, err := s.C.SyncManifest()
	if s.Name != "" {
		m.Name = s.Name
	}
	return m, err
}

// Read reads synced files.
func (s SyncSide) Read(paths []string) (map[string][]byte, error) {
	files := make(map[string][]byte, len(paths))
	if len(paths) == 0 {
		return files, nil
	}
	manifest, err := s.C.SyncManifest()
	if err != nil {
		return nil, err
	}
	batches, err := syncReadBatches(paths, manifest.Files)
	if err != nil {
		return nil, err
	}
	for _, batch := range batches {
		var out proto.SyncReadResult
		if err := s.C.Call(proto.MethodSyncRead, proto.SyncReadParams{Paths: batch}, &out); err != nil {
			return nil, err
		}
		for path, data := range out.Files {
			files[path] = data
		}
	}
	return files, nil
}

// Write makes changes to synced files.
func (s SyncSide) Write(changes []proto.SyncChange) (applied, stale []string, err error) {
	batches, err := syncWriteBatches(changes, s.From)
	if err != nil {
		return nil, nil, err
	}
	for _, batch := range batches {
		var out proto.SyncWriteResult
		if err = s.C.Call(proto.MethodSyncWrite, proto.SyncWriteParams{Changes: batch, From: s.From}, &out); err != nil {
			return applied, stale, err
		}
		applied = append(applied, out.Applied...)
		stale = append(stale, out.Stale...)
	}
	return applied, stale, nil
}

// SyncManifest lists the host's synced files and who it is.
func (c *Client) SyncManifest() (proto.SyncManifest, error) {
	var m proto.SyncManifest
	err := c.Call(proto.MethodSyncManifest, proto.SyncManifestParams{KeepsOwnHistory: true}, &m)
	return m, err
}

// SyncReport records how a sync with another host went.
func (c *Client) SyncReport(st proto.SyncPeerStatus) error {
	return c.Call(proto.MethodSyncReport, st, nil)
}

// SyncStatus is the host's synced data and its syncs.
func (c *Client) SyncStatus() (proto.SyncStatus, error) {
	var st proto.SyncStatus
	err := c.Call(proto.MethodSyncStatus, nil, &st)
	return st, err
}

// SyncRequest asks the apps attached to sync the host now.
func (c *Client) SyncRequest() error { return c.Call(proto.MethodSyncRequest, nil, nil) }

// SyncConflicts lists the conflicts waiting for the user, without their versions' data,
// which an older daemon sends and isn't kept: SyncConflictVersion reads it.
func (c *Client) SyncConflicts() ([]proto.SyncConflict, error) {
	var list []proto.SyncConflict
	err := c.Call(proto.MethodSyncConflicts, nil, &list)
	for _, cf := range list {
		for i := range cf.Versions {
			v := &cf.Versions[i]
			if v.Data != nil {
				v.Size, v.Data = int64(len(v.Data)), nil
			}
		}
	}
	return list, err
}

// SyncConflictVersion reads version n, from 1, of conflict id, data and all. An older
// daemon has no such call: its list has the data.
func (c *Client) SyncConflictVersion(id string, n int) (proto.SyncVersion, error) {
	var v proto.SyncVersion
	err := c.Call(proto.MethodSyncConflict, proto.SyncConflictParams{ID: id, Version: n}, &v)
	if err == nil || !strings.Contains(err.Error(), "unknown method") {
		return v, err
	}
	var list []proto.SyncConflict
	if err := c.Call(proto.MethodSyncConflicts, nil, &list); err != nil {
		return v, err
	}
	for _, cf := range list {
		if cf.ID == id && n >= 1 && n <= len(cf.Versions) {
			return cf.Versions[n-1], nil
		}
	}
	return v, fmt.Errorf("sync: conflict %s has no version %d", id, n)
}

// SyncResolve settles a conflict, keeping a host's version or "both".
func (c *Client) SyncResolve(id, keep string) error {
	return c.Call(proto.MethodSyncResolve, proto.SyncResolveParams{ID: id, Keep: keep}, nil)
}

// SyncTrash lists the synced files deleted lately.
func (c *Client) SyncTrash() ([]proto.SyncTrashItem, error) {
	var list []proto.SyncTrashItem
	err := c.Call(proto.MethodSyncTrash, nil, &list)
	return list, err
}

// SyncRestore brings a deleted file back.
func (c *Client) SyncRestore(id string) error {
	return c.Call(proto.MethodSyncRestore, proto.SyncTrashItem{ID: id}, nil)
}

// SyncForget removes the synced data from the host, to its trash.
func (c *Client) SyncForget() error { return c.Call(proto.MethodSyncForget, nil, nil) }

// Catalogs lists every host's projects the host knows.
func (c *Client) Catalogs() ([]proto.Catalog, error) {
	var list []proto.Catalog
	err := c.Call(proto.MethodCatalogList, nil, &list)
	return list, err
}

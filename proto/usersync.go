package proto

import "time"

// The user's data (notes, canvases, settings, the vault's ciphertext…)
// is kept the same on every host: the app syncs each host it reaches with
// its own device's daemon, file by file, three ways against what the two
// had at their last sync (core/usersync). A daemon lists, reads and
// writes the files; the app decides what goes where.
const (
	MethodSyncManifest = "sync.manifest" // SyncManifestParams → SyncManifest
	MethodSyncRead     = "sync.read"     // SyncReadParams → SyncReadResult
	MethodSyncWrite    = "sync.write"    // SyncWriteParams → SyncWriteResult
	MethodSyncReport   = "sync.report"   // SyncPeerStatus: how a sync with a peer went
	MethodSyncStatus   = "sync.status"   // → SyncStatus
	MethodSyncRequest  = "sync.request"  // asks the app to sync now (EventSyncRequested)
	// MethodSyncConflicts lists them without their versions' data: a daemon before 0.7
	// sends it, but no app ever read it from here. MethodSyncConflict reads one version's.
	MethodSyncConflicts = "sync.conflicts" // → []SyncConflict
	MethodSyncConflict  = "sync.conflict"  // SyncConflictParams → SyncVersion
	MethodSyncResolve   = "sync.resolve"   // SyncResolveParams
	MethodSyncTrash     = "sync.trash"     // → []SyncTrashItem
	MethodSyncRestore   = "sync.restore"   // SyncTrashItem (its ID)
	MethodSyncForget    = "sync.forget"    // removes the synced data from this host
)

// Sync events.
const (
	// EventSyncRequested asks the apps attached to sync this host now.
	EventSyncRequested = "sync.requested"
	// EventSyncApplied carries SyncApplied after a sync changed files here.
	EventSyncApplied = "sync.applied"
	// EventSyncStatus carries SyncStatus after a sync was reported.
	EventSyncStatus = "sync.status"
)

// SyncEntry is a synced file: its path under the daemon's folder, with
// slashes, and a hash of its content.
type SyncEntry struct {
	Path string    `json:"path"`
	Hash string    `json:"hash"`
	Size int64     `json:"size"`
	Mod  time.Time `json:"mod"`
}

// SyncManifest is a host's synced files, and who it is: ID stays the same
// for its daemon's folder.
type SyncManifest struct {
	ID    string      `json:"id"`
	Name  string      `json:"name"`
	Files []SyncEntry `json:"files"`
}

// SyncReadParams names files to read.
type SyncReadParams struct {
	Paths []string `json:"paths"`
}

// SyncReadResult is the files read, by path; those gone are missing.
type SyncReadResult struct {
	Files map[string][]byte `json:"files"`
}

// SyncChange writes a file (Data) or deletes it (Delete), only when it
// still has hash Expect ("" for a file that must not exist yet).
type SyncChange struct {
	Path   string `json:"path"`
	Data   []byte `json:"data,omitempty"`
	Delete bool   `json:"delete,omitempty"`
	Expect string `json:"expect"`
}

// SyncWriteParams are changes to make; From names the host they came from.
type SyncWriteParams struct {
	Changes []SyncChange `json:"changes"`
	From    string       `json:"from,omitempty"`
}

// SyncWriteResult says which changes were made, and which were skipped
// because the file changed meanwhile.
type SyncWriteResult struct {
	Applied []string `json:"applied"`
	Stale   []string `json:"stale,omitempty"`
}

// SyncApplied is what a sync changed on a host.
type SyncApplied struct {
	Paths []string `json:"paths"`
	From  string   `json:"from,omitempty"`
}

// SyncPeerStatus is how this host's last sync with another went.
type SyncPeerStatus struct {
	Peer     string    `json:"peer"` // the other's SyncManifest.ID
	Name     string    `json:"name"`
	Last     time.Time `json:"last"`
	Err      string    `json:"err,omitempty"`
	Sent     int       `json:"sent"`
	Received int       `json:"received"`
	Merged   int       `json:"merged"`
	Conflict int       `json:"conflicts"`
}

// SyncStatus is a host's synced data and its syncs.
type SyncStatus struct {
	ID        string           `json:"id"`
	Name      string           `json:"name"`
	Files     int              `json:"files"`
	Size      int64            `json:"size"`
	Peers     []SyncPeerStatus `json:"peers"`
	Conflicts int              `json:"conflicts"`
}

// SyncConflict is a file changed on two hosts in ways that don't merge:
// the first version stays in place until the user picks.
type SyncConflict struct {
	ID       string        `json:"id"`
	Path     string        `json:"path"`
	Title    string        `json:"title,omitempty"` // a note's
	Created  time.Time     `json:"created"`
	Versions []SyncVersion `json:"versions"`
}

// SyncVersion is one host's version of a file in conflict; Deleted when
// that host deleted it.
type SyncVersion struct {
	Host    string `json:"host"`
	Data    []byte `json:"data,omitempty"`
	Deleted bool   `json:"deleted,omitempty"`
	// Size is Data's, said where Data is left out.
	Size int64 `json:"size,omitempty"`
}

// SyncConflictParams names a version of a conflict, from 1.
type SyncConflictParams struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
}

// SyncResolveParams keeps conflict ID's version of host Keep (or its
// number, from "1"), or with
// Keep "both", the first in place and the others beside it as copies.
type SyncResolveParams struct {
	ID   string `json:"id"`
	Keep string `json:"keep"`
}

// SyncTrashItem is a synced file deleted lately, kept a while to restore.
type SyncTrashItem struct {
	ID      string    `json:"id"`
	Path    string    `json:"path"`
	Title   string    `json:"title,omitempty"`
	Deleted time.Time `json:"deleted"`
}

// SyncManifestParams is who asks for a manifest. KeepsOwnHistory says the one syncing leaves
// a canvas's versions on their host. An app that doesn't say so reads their absence here as
// their deletion, and would delete them on the other host: it's refused.
type SyncManifestParams struct {
	KeepsOwnHistory bool `json:"keepsOwnHistory,omitempty"`
}

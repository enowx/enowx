package proto

import (
	"encoding/json"
	"time"
)

// A project copied to another host remembers where it came from: a link
// from its source (host and folder) to its copy, kept with the user's
// data on every host (settings/project-links.json, synced). A link can
// sync the copies on its own, at an interval or as files change, while
// the app reaches both hosts. A project is known by an ID kept in its
// folder (.enowx/local/project.json, never in git), the same in every
// copy, so a link holds when a folder is renamed or moved.
const (
	MethodProjectID          = "project.id"           // ProjectRef → ProjectIdentity (made when it has none)
	MethodProjectFind        = "project.find"         // ProjectIdentity (its ID) → ProjectInfo, here
	MethodProjectLinks       = "projectlink.list"     // ProjectLinksParams → []ProjectLinkState
	MethodProjectLinkSave    = "projectlink.save"     // ProjectLink → ProjectLink (a new one without ID)
	MethodProjectLinkDelete  = "projectlink.delete"   // ProjectLinkRef
	MethodProjectLinkReport  = "projectlink.report"   // ProjectLinkReport: how a sync went, from the app
	MethodProjectHistory     = "projectlink.history"  // ProjectLinksParams → []ProjectHistoryEntry
	MethodProjectWatch       = "project.watch"        // ProjectWatchParams: events as its files change
	MethodProjectCopyRequest = "project.copy.request" // ProjectCopyParams → ProjectCopyResult, by the app
	MethodProjectSyncRequest = "projectlink.sync.now" // ProjectLinkRef → asks the app to sync it now
	MethodAppReply           = "app.reply"            // AppReply: the app's answer to an EventAppRequest
	MethodProjectKeepCopy    = "project.keepcopy"     // ProjectKeepCopyParams → its new path
	MethodProjectLinkBase    = "projectlink.base"     // ProjectLinkRef → its base (ProjectLinkReport.Base), null for none
	MethodAppServe           = "app.serve"            // this connection is an app's: it takes EventAppRequest
	MethodProjectFileRead    = "project.file.read"    // ProjectFileParams → the file's bytes (a few MB at most)
)

// ProjectFileParams names file Path (with slashes) of the project at
// Project.
type ProjectFileParams struct {
	Project string `json:"project"`
	Path    string `json:"path"`
}

// Auto pushes: a project's work committed and pushed on a branch of its
// own at an interval, by the daemon of one host holding it
// (settings/autopush.json, synced).
const (
	MethodAutopushList = "autopush.list" // → []AutopushStatus
	MethodAutopushSet  = "autopush.set"  // AutopushSettings → AutopushStatus
	MethodAutopushNow  = "autopush.now"  // ProjectRef → AutopushStatus, pushed now
)

// Events of links and auto pushes.
const (
	// EventProjectLinksChanged carries the links ([]ProjectLink) after a
	// change.
	EventProjectLinksChanged = "projectlinks.changed"
	// EventProjectFilesChanged carries ProjectRef: a watched project's
	// files changed (a while after the last change).
	EventProjectFilesChanged = "project.files.changed"
	// EventAutopush carries AutopushStatus after an auto push ran.
	EventAutopush = "autopush.ran"
	// EventProjectHistory carries a ProjectHistoryEntry just recorded.
	EventProjectHistory = "projectlink.history"
	// EventAppRequest carries an AppRequest for the app to do (it alone
	// reaches other hosts); it answers with app.reply.
	EventAppRequest = "app.request"
)

// ProjectIdentity is a project's ID, the same in all its copies.
type ProjectIdentity struct {
	ID   string `json:"id"`
	Path string `json:"path,omitempty"`
}

// ProjectEnd is a copy of a project: the host's daemon (DaemonInfo.ID),
// its name as last seen, and the folder there.
type ProjectEnd struct {
	Daemon string `json:"daemon"`
	Host   string `json:"host"`
	Path   string `json:"path"`
}

// Directions a link syncs.
const (
	SyncOneWay  = "one-way" // source → copy
	SyncTwoWay  = "two-way"
	SyncReverse = "reverse" // copy → source
)

// What happens to a file deleted on the side a sync takes from.
const (
	DeletesMirror = "mirror" // deleted on the other side too
	DeletesNever  = "never"  // kept there
)

// What a sync does with a large file, or a binary one changed on both
// sides.
const (
	LargeSkip   = "skip"   // left as each side has it
	LargeSource = "source" // the source's wins
)

// ProjectLink is a project copied from Source to Target, and how they
// sync.
type ProjectLink struct {
	ID      string     `json:"id"`
	Project string     `json:"project"` // ProjectIdentity.ID
	Name    string     `json:"name"`
	Source  ProjectEnd `json:"source"`
	Target  ProjectEnd `json:"target"`
	Created time.Time  `json:"created"`
	// Auto syncs on its own: every Interval seconds, or as files change
	// when Realtime.
	Auto      bool   `json:"auto,omitempty"`
	Interval  int    `json:"interval,omitempty"`
	Realtime  bool   `json:"realtime,omitempty"`
	Direction string `json:"direction,omitempty"` // Sync*; "" two-way
	// Include, when any, are the only files synced; Exclude are never.
	// Both are patterns of paths with slashes (* within a folder, **
	// across, a name alone anywhere).
	Include []string `json:"include,omitempty"`
	Exclude []string `json:"exclude,omitempty"`
	// All syncs what .gitignore ignores too (but dependencies' folders).
	All     bool   `json:"all,omitempty"`
	Deletes string `json:"deletes,omitempty"` // Deletes*; "" mirror
	Env     bool   `json:"env,omitempty"`     // .env files go too
	// Bandwidth caps the carrying, in KB/s; 0 doesn't.
	Bandwidth int `json:"bandwidth,omitempty"`
	// LargeMB is the size from which a file is large (0: 50 MB); Large
	// is what happens to large files and binaries in conflict.
	LargeMB int    `json:"largeMB,omitempty"`
	Large   string `json:"large,omitempty"` // Large*; "" skip
	// Paused when the host is busy or the computer on battery.
	PauseBusy    bool `json:"pauseBusy,omitempty"`
	PauseBattery bool `json:"pauseBattery,omitempty"`
}

// ProjectLinkRef names a link.
type ProjectLinkRef struct {
	ID string `json:"id"`
}

// ProjectLinksParams lists the links (or history) of the project with ID
// Project, or every one with Project "".
type ProjectLinksParams struct {
	Project string `json:"project,omitempty"`
}

// ProjectSyncConflict is a file changed on both sides since the last
// sync, left as each has it until the user picks.
type ProjectSyncConflict struct {
	Path   string      `json:"path"`
	Sides  [2]SyncFile `json:"sides"`          // Source's, Target's
	Gone   [2]bool     `json:"gone,omitempty"` // deleted on that side
	Binary bool        `json:"binary,omitempty"`
	Found  time.Time   `json:"found"`
}

// ProjectLinkStatus is how a link's syncs went, as this host was told.
type ProjectLinkStatus struct {
	Last      time.Time             `json:"last,omitempty"`
	Next      time.Time             `json:"next,omitempty"`
	Err       string                `json:"err,omitempty"`
	Sent      int                   `json:"sent,omitempty"` // files, source → copy
	Received  int                   `json:"received,omitempty"`
	Conflicts []ProjectSyncConflict `json:"conflicts,omitempty"`
}

// ProjectLinkState is a link and its status.
type ProjectLinkState struct {
	ProjectLink
	Status ProjectLinkStatus `json:"status"`
}

// ProjectLinkReport records how a sync of link Link went.
type ProjectLinkReport struct {
	Link   string              `json:"link"`
	Status ProjectLinkStatus   `json:"status"`
	Entry  ProjectHistoryEntry `json:"entry"`
	// Base is what both copies have alike after it (a projectsync.Base),
	// kept for the next sync whatever app runs it; none keeps the last.
	Base json.RawMessage `json:"base,omitempty"`
}

// ProjectHistoryEntry is a sync or a push, done or failed.
type ProjectHistoryEntry struct {
	Time    time.Time `json:"time"`
	Project string    `json:"project"` // its ID
	Kind    string    `json:"kind"`    // "sync" or "push"
	Link    string    `json:"link,omitempty"`
	OK      bool      `json:"ok"`
	Summary string    `json:"summary"`
	Err     string    `json:"err,omitempty"`
}

// ProjectWatchParams watches the project at Project, or stops with Off.
type ProjectWatchParams struct {
	Project string `json:"project"`
	Off     bool   `json:"off,omitempty"`
}

// ProjectCopyParams copies the project at Project here to host To (a
// name or ID in the app's list), into the folder Path there (default:
// beside where it is here), remembering where it came from.
type ProjectCopyParams struct {
	Project string `json:"project"`
	To      string `json:"to"`
	Path    string `json:"path,omitempty"`
	Env     bool   `json:"env,omitempty"`
}

// ProjectCopyResult is the copy made, and its link.
type ProjectCopyResult struct {
	Path string      `json:"path"`
	Link ProjectLink `json:"link"`
}

// AppRequest is work for the app: Kind names it ("project.copy",
// "project.sync"), Params its params.
type AppRequest struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Params []byte `json:"params"`
}

// AppReply is the app's answer to request ID: Result, or Error.
type AppReply struct {
	ID     string `json:"id"`
	Result []byte `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
	// Bypassed: a host.call whose HostCallParams.Bypass the app honored,
	// doing it without asking the user.
	Bypassed bool `json:"bypassed,omitempty"`
}

// ProjectKeepCopyParams copies file Path of the project at Project beside
// it, its name marked with Suffix, to keep a version in conflict.
type ProjectKeepCopyParams struct {
	Project string `json:"project"`
	Path    string `json:"path"`
	Suffix  string `json:"suffix"`
}

// AutopushSettings are a project's auto pushes: On every Interval
// seconds, to Branch of Remote, by the daemon Host (DaemonInfo.ID) alone,
// with a pull request into Base when PR.
type AutopushSettings struct {
	Project  string `json:"project"` // its ID
	Name     string `json:"name,omitempty"`
	On       bool   `json:"on"`
	Interval int    `json:"interval,omitempty"` // 0: 30 minutes
	Branch   string `json:"branch,omitempty"`   // "": enowx/autosync
	Remote   string `json:"remote,omitempty"`   // "": origin
	PR       bool   `json:"pr,omitempty"`
	Base     string `json:"base,omitempty"` // the PR's; "": the repository's default
	Host     string `json:"host,omitempty"`
	HostName string `json:"hostName,omitempty"`
	// Path is the project's folder on the host asking (autopush.set takes
	// it to know the project).
	Path string `json:"path,omitempty"`
}

// AutopushStatus is a project's auto pushes and how the last went, on
// the host asked.
type AutopushStatus struct {
	AutopushSettings
	Here       bool      `json:"here,omitempty"` // this host pushes it
	LastRun    time.Time `json:"lastRun,omitempty"`
	LastPush   time.Time `json:"lastPush,omitempty"`
	Next       time.Time `json:"next,omitempty"`
	Commit     string    `json:"commit,omitempty"`
	Files      int       `json:"files,omitempty"`
	Err        string    `json:"err,omitempty"`
	BranchURL  string    `json:"branchURL,omitempty"`
	Visibility string    `json:"visibility,omitempty"` // "public", "private", "" unknown
	PRURL      string    `json:"prURL,omitempty"`
}

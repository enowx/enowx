package proto

// Taking something from the community library, on a host (daemon/library_reuse.go):
// a copy on a new canvas, a reference on a canvas, a skill. The daemon fetches
// through its cloud token alone. What comes back was written by other people.
const (
	// MethodLibraryUseInfo says what the cloud says of an item, for the dialog
	// that asks before anything happens.
	MethodLibraryUseInfo = "library.useInfo" // LibraryUseRef → LibraryUseInfo
	// MethodLibraryOpen makes a copy of a design: a new canvas with one design.
	MethodLibraryOpen = "library.open" // LibraryOpenParams → LibraryOpened
	// MethodLibraryReference keeps a design's picture in a canvas's files and
	// one credit line in its brief.
	MethodLibraryReference = "library.reference" // LibraryReferenceParams → LibraryReferenced
	// MethodLibrarySkill reads a design's skill, and installs it when asked.
	MethodLibrarySkill = "library.skill" // LibrarySkillParams → LibrarySkillAnswer
	// MethodLibraryNewer says whether a canvas copied from the library has a
	// newer version there. The copy never changes by itself.
	MethodLibraryNewer = "library.newer" // CanvasRef → LibraryNewer
)

// CanvasSource says where a canvas was copied from in the community library, and
// stays with it: the credit its copy keeps.
type CanvasSource struct {
	// ItemID and Version are the item and version it was copied from.
	ItemID  string `json:"itemId"`
	Version int    `json:"version"`
	// Title and Creator (their handle) are other people's words.
	Title   string `json:"title"`
	Creator string `json:"creator"`
	License string `json:"license"`
	Address string `json:"address,omitempty"`
}

// LibraryUseRef names an item, at a version; 0 is what shows.
type LibraryUseRef struct {
	ItemID  string `json:"itemId"`
	Version int    `json:"version,omitempty"`
}

// LibraryUseInfo is an item as a dialog says it, and how this host stands.
type LibraryUseInfo struct {
	// On says the host's cloud has a library, SignedIn that it's signed in.
	On       bool   `json:"on"`
	SignedIn bool   `json:"signedIn,omitempty"`
	ItemID   string `json:"itemId,omitempty"`
	// Version is the version a copy would be of.
	Version int    `json:"version,omitempty"`
	Title   string `json:"title,omitempty"`
	Creator string `json:"creator,omitempty"`
	License string `json:"license,omitempty"`
	Kind    string `json:"kind,omitempty"`
	URL     string `json:"url,omitempty"`
	// CanOpen, CanReference and CanSkill are what its license and kind allow:
	// the cloud's word, for showing; it's the cloud that refuses.
	CanOpen      bool `json:"canOpen,omitempty"`
	CanReference bool `json:"canReference,omitempty"`
	// Refused says why it can't be had, for showing.
	Refused string `json:"refused,omitempty"`
	// SkillAgents are the agents on this host that keep skills.
	SkillAgents []string `json:"skillAgents,omitempty"`
}

// LibraryOpenParams makes a copy.
type LibraryOpenParams struct {
	ItemID  string `json:"itemId"`
	Version int    `json:"version,omitempty"`
	// Projects are the project folders the new canvas belongs to.
	Projects []string `json:"projects,omitempty"`
}

// LibraryOpened is the new canvas.
type LibraryOpened struct {
	Canvas CanvasInfo `json:"canvas"`
	// Files are the files that came with it.
	Files int `json:"files"`
}

// LibraryReferenceParams keeps a design as a canvas's reference.
type LibraryReferenceParams struct {
	ItemID  string `json:"itemId"`
	Version int    `json:"version,omitempty"`
	Canvas  string `json:"canvas"`
}

// LibraryReferenced says what a canvas now holds.
type LibraryReferenced struct {
	Canvas string `json:"canvas"`
	// File is the picture among the canvas's files.
	File string `json:"file"`
	// Credit is the line in its brief, labelled as other people's data.
	Credit string `json:"credit"`
}

// LibrarySkillParams reads a design's skill.
type LibrarySkillParams struct {
	ItemID  string `json:"itemId"`
	Version int    `json:"version,omitempty"`
	// Install writes it where Agent keeps skills, for Project when set (a folder
	// on the host), else for the user.
	Install bool   `json:"install,omitempty"`
	Agent   string `json:"agent,omitempty"`
	Project string `json:"project,omitempty"`
}

// LibrarySkillAnswer is a design's SKILL.md, labelled inside as other people's
// data, and where it went when installed.
type LibrarySkillAnswer struct {
	Name string `json:"name"`
	Text string `json:"text"`
	// Installed is the item's ID in the agent's items, "" when only read.
	Installed string `json:"installed,omitempty"`
}

// LibraryNewer is how a copy stands against what it was copied from.
type LibraryNewer struct {
	// Source is what the canvas was copied from, nil when it wasn't.
	Source *CanvasSource `json:"source,omitempty"`
	// Latest is the version that shows now, 0 when it can't be known.
	Latest int `json:"latest,omitempty"`
	// Gone says the design was taken back or down. A copy stays.
	Gone bool `json:"gone,omitempty"`
	// URL is its page on the library's website.
	URL string `json:"url,omitempty"`
}

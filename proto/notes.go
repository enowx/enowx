package proto

import "time"

// Notes: the user's pages on a host, nested like a notebook's, their
// bodies Markdown; agents read and write them too.
const (
	MethodNoteList   = "note.list"   // → []NoteInfo
	MethodNoteGet    = "note.get"    // NoteRef → Note
	MethodNoteTick   = "note.tick"   // NoteTick: who ticked a to-do
	MethodNoteTicks  = "note.ticks"  // NoteRef → []NoteTick
	MethodNoteSave   = "note.save"   // Note → NoteInfo (a new one without ID)
	MethodNoteDelete = "note.delete" // NoteRef: it and the pages under it
	MethodNoteMove   = "note.move"   // NoteMoveParams
)

// EventNotesChanged carries the full []NoteInfo after a change.
const EventNotesChanged = "notes.changed"

// NoteInfo is a page without its body. Pages under another name it as
// Parent; Order sorts a page's children.
type NoteInfo struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	// Icon is an SVG icon: "lucide:rocket" (a library's, "#rrggbb" after
	// it for a color) or "svg:<base64>" (the user's own); "" for the
	// default. Notes of before had emoji.
	Icon string `json:"icon,omitempty"`
	// IconReview marks an icon given for an emoji with none close, for
	// the user to pick again.
	IconReview bool   `json:"iconReview,omitempty"`
	Parent     string `json:"parent,omitempty"`
	// Group is the group a top-level note is in (NoteGroup.Name), ""
	// for none; pages under it are in its group.
	Group   string    `json:"group,omitempty"`
	Order   int       `json:"order"`
	Created time.Time `json:"created"`
	Updated time.Time `json:"updated"`
	// Projects are the projects it is attached to, on any host.
	Projects []NoteProject `json:"projects,omitempty"`
	// Agent is the agent the Notes page talks to about it, picked last.
	Agent string `json:"agent,omitempty"`
	// Team is the team whose space it is in, "" for the user's own
	// (docs/cloud.md). Saving a new top-level note puts it there; a page
	// is in its parent's.
	Team string `json:"team,omitempty"`
}

// Note is a page with its body.
type Note struct {
	NoteInfo
	Body string `json:"body"`
}

// NoteTick is a to-do an agent ticked: by which session, at which commit, which
// the daemon fills in.
type NoteTick struct {
	Note    string    `json:"note"`
	Item    string    `json:"item"`
	Session string    `json:"session,omitempty"`
	Commit  string    `json:"commit,omitempty"`
	Time    time.Time `json:"time"`
}

// NoteRef names a page.
type NoteRef struct {
	ID string `json:"id"`
}

// NoteMoveParams puts page ID under Parent ("" for the top), before page
// Before of it, or last with Before "".
type NoteMoveParams struct {
	ID     string `json:"id"`
	Parent string `json:"parent,omitempty"`
	Before string `json:"before,omitempty"`
}

// An agent asked to help with notes from the Notes page proposes its
// changes rather than making them: the user sees each against the note
// and accepts or rejects it.
const (
	MethodNoteProposals = "note.proposals" // → []NoteProposal
	MethodNotePropose   = "note.propose"   // NoteProposal (its Note and Session) → NoteProposal
	MethodNoteAccept    = "note.accept"    // NoteRef (a proposal's ID) → NoteInfo
	MethodNoteReject    = "note.reject"    // NoteRef (a proposal's ID)
)

// EventNoteProposalsChanged carries the full []NoteProposal after a change.
const EventNoteProposalsChanged = "notes.proposals"

// NoteProposal is a change an agent proposes: Note is the note as it would
// be (its ID "" for a new one, under Note.Parent); Before is the note's
// body when proposed, which the change is shown against.
type NoteProposal struct {
	ID      string    `json:"id"`
	Note    Note      `json:"note"`
	Before  string    `json:"before,omitempty"`
	Session string    `json:"session,omitempty"` // the agent's
	Created time.Time `json:"created"`
}

// EnvNotesPropose, set in a session's environment, makes its enowx-app
// notes propose changes rather than make them: the Notes page's agents.
const EnvNotesPropose = "ENOWX_NOTES_PROPOSE"

// Groups of notes, like folders: a top-level note is in one or none.
const (
	MethodNoteGroups      = "note.groups"       // → []NoteGroup
	MethodNoteGroupSave   = "note.group.save"   // NoteGroupSaveParams → []NoteGroup
	MethodNoteGroupDelete = "note.group.delete" // NoteGroupRef → []NoteGroup (its notes left without a group)
	MethodNoteGroupMove   = "note.group.move"   // NoteGroupMoveParams → []NoteGroup
	MethodNoteSetGroup    = "note.setGroup"     // NoteSetGroupParams → NoteInfo
)

// EventNoteGroupsChanged carries the full []NoteGroup after a change.
const EventNoteGroupsChanged = "notes.groups"

// NoteGroup is a group of notes, in the user's order.
type NoteGroup struct {
	Name   string `json:"name"`
	Icon   string `json:"icon,omitempty"`  // as a note's (NoteInfo.Icon)
	Color  string `json:"color,omitempty"` // #rrggbb
	Folded bool   `json:"folded,omitempty"`
	// IconReview marks an icon given for an emoji with none close.
	IconReview bool `json:"iconReview,omitempty"`
	// Team is the team whose space it is in, as a note's (NoteInfo.Team).
	Team string `json:"team,omitempty"`
}

// Icon preferences of the Notes page's icon picker.
const (
	MethodNoteIconPrefs     = "note.iconPrefs"      // → NoteIconPrefs
	MethodNoteIconPrefsSave = "note.iconPrefs.save" // NoteIconPrefs (its Favorites and Libraries) → NoteIconPrefs
)

// NoteIconPrefs is the icons used lately (newest first, kept as notes'
// and groups' icons are saved), the user's favorites, and the libraries
// the picker shows (none: all).
type NoteIconPrefs struct {
	Recent    []string `json:"recent"`
	Favorites []string `json:"favorites"`
	Libraries []string `json:"libraries"`
}

// NoteGroupSaveParams makes a group, or changes group From ("" makes
// one): its name (its notes follow), icon, color and folding.
type NoteGroupSaveParams struct {
	From  string    `json:"from,omitempty"`
	Group NoteGroup `json:"group"`
}

// NoteGroupRef names a group.
type NoteGroupRef struct {
	Name string `json:"name"`
	Team string `json:"team,omitempty"` // the space it is in
}

// NoteGroupMoveParams moves group Name before group Before, or last.
type NoteGroupMoveParams struct {
	Name   string `json:"name"`
	Before string `json:"before,omitempty"`
	Team   string `json:"team,omitempty"` // the space they are in
}

// NoteSetGroupParams puts note ID in Group ("" for none), at the top level.
type NoteSetGroupParams struct {
	ID    string `json:"id"`
	Group string `json:"group,omitempty"`
}

// A note's earlier versions, kept on each host, to see and bring back.
const (
	MethodNoteHistory = "note.history" // NoteRef → []NoteVersion, the latest first
	MethodNoteVersion = "note.version" // NoteVersionRef → Note
	MethodNoteRestore = "note.restore" // NoteVersionRef → NoteInfo: that version back
)

// NoteVersion is an earlier version of a note.
type NoteVersion struct {
	ID    string    `json:"id"`
	Time  time.Time `json:"time"`
	Title string    `json:"title"`
	Size  int       `json:"size"` // its body's, in bytes
}

// NoteVersionRef names version Version of note ID.
type NoteVersionRef struct {
	ID      string `json:"id"`
	Version string `json:"version"`
}

// A note's agent in the Notes page, for agents elsewhere too: asking it,
// reading what it said, picking which agent it is.
const (
	MethodNoteChat     = "note.chat"     // NoteChatParams → NoteChatLog: asks the note's agent
	MethodNoteChatLog  = "note.chatLog"  // NoteRef → NoteChatLog
	MethodNoteSetAgent = "note.setAgent" // NoteSetAgentParams → NoteInfo
)

// EventNoteChat carries a NoteChatLog after a turn of a note's agent was
// kept.
const EventNoteChat = "note.chat"

// NoteChatParams ask note ID's agent Prompt: Agent ("" for the note's own,
// or Claude Code) in its conversation running, or a new one.
type NoteChatParams struct {
	ID     string `json:"id"`
	Prompt string `json:"prompt"`
	Agent  string `json:"agent,omitempty"`
	Model  string `json:"model,omitempty"`
	Effort string `json:"effort,omitempty"`
	New    bool   `json:"new,omitempty"` // a new conversation
}

// NoteSetAgentParams pick note ID's agent.
type NoteSetAgentParams struct {
	ID    string `json:"id"`
	Agent string `json:"agent"`
}

// NoteChatLog is note Note's conversations with its agent, on any host;
// Session is the conversation running here, if any.
type NoteChatLog struct {
	Note     string            `json:"note"`
	Session  string            `json:"session,omitempty"`
	Updated  time.Time         `json:"updated,omitzero"`
	Messages []NoteChatMessage `json:"messages"`
}

// NoteChatMessage is what the user asked (Role "user") or the agent
// answered ("assistant"), in which session of which agent on which host.
type NoteChatMessage struct {
	Time    time.Time `json:"time"`
	Role    string    `json:"role"`
	Text    string    `json:"text"`
	Agent   string    `json:"agent,omitempty"`
	Session string    `json:"session,omitempty"`
	Host    string    `json:"host,omitempty"`
}

// MethodNoteAsset reads a file a note shows ("assets/<note ID>/<file>",
// an image an import brought): NoteAssetRef → NoteAsset.
const MethodNoteAsset = "note.asset"

// NoteAssetRef names a note's file as its text does.
type NoteAssetRef struct {
	Ref string `json:"ref"`
}

// NoteAsset is a note's file.
type NoteAsset struct {
	Data []byte `json:"data"`
}

// MethodNoteImage stores an image on an existing note: NoteImage → NoteAssetRef.
const MethodNoteImage = "note.image"

// MaxNoteImage is the largest image accepted, leaving room for base64 in a frame.
const MaxNoteImage = 8 << 20

// NoteImage is an image uploaded from the app's device.
type NoteImage struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Data []byte `json:"data"`
}

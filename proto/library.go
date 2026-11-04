package proto

import "time"

// enowX UI, the community library: sharing a canvas's design, from the host
// (daemon/library.go). Publishing goes from the app's form only: an agent or
// the command line can read how a design stands, not publish one.
const (
	// MethodLibraryPrepare checks a design and packs it as a draft on the host:
	// what the form shows before anything leaves it.
	MethodLibraryPrepare = "library.prepare" // LibraryPrepareParams → LibraryPrepared
	// MethodLibrarySaveDraft keeps what the form holds, on the host.
	MethodLibrarySaveDraft = "library.saveDraft" // LibraryDraftParams
	// MethodLibraryDeleteDraft forgets a draft.
	MethodLibraryDeleteDraft = "library.deleteDraft" // LibraryDraftParams
	// MethodLibraryPublish sends a draft to the cloud.
	MethodLibraryPublish = "library.publish" // LibraryPublishParams → LibraryDesign
	// MethodLibraryStatus says whether the host's cloud has a library, and how
	// the account's designs stand.
	MethodLibraryStatus = "library.status" // → LibraryState
	// MethodLibraryWithdraw takes a published design back.
	MethodLibraryWithdraw = "library.withdraw" // LibraryItemRef → LibraryDesign
)

// Where a design stands, for the app: a draft is on the host, the others are
// the cloud's statuses.
const LibraryStatusDraft = "draft"

// LibraryPrepareParams says which design to share.
type LibraryPrepareParams struct {
	Canvas string `json:"canvas"`
	Frame  string `json:"frame"`
	// NoPhone leaves out the second design a phone would be shown.
	NoPhone bool `json:"noPhone,omitempty"`
}

// LibraryFinding is what a check found: where and of what kind, never the value.
type LibraryFinding struct {
	Severity string `json:"severity"` // refuse or warn
	Kind     string `json:"kind"`
	Where    string `json:"where"`
}

// LibraryForm is what the creator says of a design.
type LibraryForm struct {
	Kind           string   `json:"kind"`
	License        string   `json:"license"`
	Title          string   `json:"title"`
	Description    string   `json:"description"`
	Category       string   `json:"category"`
	Tags           []string `json:"tags,omitempty"`
	SourceURL      string   `json:"sourceUrl,omitempty"`
	Prompt         string   `json:"prompt"`
	PromptApproved bool     `json:"promptApproved,omitempty"`
	Rights         bool     `json:"rights,omitempty"`
}

// LibraryPrepared is a design packed as a draft, and what the form says of it.
type LibraryPrepared struct {
	DraftID   string `json:"draftId"`
	Canvas    string `json:"canvas"`
	Frame     string `json:"frame"`
	FrameName string `json:"frameName"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	// Phone is the second design of the same page shown on a phone, if any.
	Phone       string `json:"phone,omitempty"`
	PhoneName   string `json:"phoneName,omitempty"`
	PhoneWidth  int    `json:"phoneWidth,omitempty"`
	PhoneHeight int    `json:"phoneHeight,omitempty"`
	// PhoneOffered says such a design exists, even when left out.
	PhoneOffered bool `json:"phoneOffered,omitempty"`
	// Files are the pictures and other files that go with it.
	Files int `json:"files"`
	// OtherDesigns are this canvas's designs that stay private.
	OtherDesigns int              `json:"otherDesigns"`
	Size         int64            `json:"size"`
	Findings     []LibraryFinding `json:"findings,omitempty"`
	MadeWith     string           `json:"madeWith"`
	HasMotion    bool             `json:"hasMotion,omitempty"`
	// Form starts from the draft saved before, or else the design itself.
	Form LibraryForm `json:"form"`
	// ItemID is the published design this would update, "" for a new one.
	ItemID string `json:"itemId,omitempty"`
	// Trust is the account's standing: a new one's designs are reviewed first.
	Trust string `json:"trust,omitempty"`
	// SiteURL is the library's website, for showing the address it gets.
	SiteURL string `json:"siteUrl,omitempty"`
}

// LibraryDraftParams names a draft, with what to keep in it.
type LibraryDraftParams struct {
	DraftID string      `json:"draftId"`
	Form    LibraryForm `json:"form"`
}

// LibraryPublishParams sends a draft.
type LibraryPublishParams struct {
	DraftID string      `json:"draftId"`
	Form    LibraryForm `json:"form"`
	// AcceptWarnings says the creator saw the check's warnings and goes on.
	AcceptWarnings bool `json:"acceptWarnings,omitempty"`
}

// LibraryItemRef names a published design.
type LibraryItemRef struct {
	ItemID string `json:"itemId"`
}

// LibraryDesign is how one of the account's designs stands.
type LibraryDesign struct {
	Canvas string `json:"canvas,omitempty"`
	Frame  string `json:"frame,omitempty"`
	// DraftID is its draft on this host, ItemID its item in the cloud.
	DraftID      string           `json:"draftId,omitempty"`
	ItemID       string           `json:"itemId,omitempty"`
	Title        string           `json:"title"`
	Status       string           `json:"status"`
	RejectReason string           `json:"rejectReason,omitempty"`
	Version      int              `json:"version,omitempty"`
	URL          string           `json:"url,omitempty"`
	Findings     []LibraryFinding `json:"findings,omitempty"`
	Updated      time.Time        `json:"updated,omitzero"`
}

// LibraryState is whether sharing is on, and the account's designs.
type LibraryState struct {
	// On says the host's cloud has a library; without it nothing shows.
	On       bool            `json:"on"`
	SignedIn bool            `json:"signedIn,omitempty"`
	SiteURL  string          `json:"siteUrl,omitempty"`
	Used     int64           `json:"used,omitempty"`
	Limit    int64           `json:"limit,omitempty"`
	Designs  []LibraryDesign `json:"designs,omitempty"`
}

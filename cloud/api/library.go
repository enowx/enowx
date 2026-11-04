package api

import "time"

// enowX UI, the community library of canvas designs (docs/cloud.md). All of
// it is under /v1/library and only grows, like the rest of the API. The
// server says it has one in Info.Features.

// FeatureLibrary in Info.Features says this cloud has the library.
const FeatureLibrary = "library"

const (
	PathLibraryItems      = "/v1/library/items"
	PathLibraryItem       = "/v1/library/items/{id}"
	PathLibraryVersions   = "/v1/library/items/{id}/versions"
	PathLibraryPrompt     = "/v1/library/items/{id}/prompt"
	PathLibraryPart       = "/v1/library/items/{id}/versions/{version}/{part}"
	PathLibrarySubmit     = "/v1/library/items/{id}/versions/{version}/submit"
	PathLibraryEscaped    = "/v1/library/items/{id}/escaped"
	PathLibraryMine       = "/v1/library/mine"
	PathLibraryMe         = "/v1/library/me"
	PathLibraryReports    = "/v1/library/reports"
	PathLibraryExchange   = "/v1/library/auth/exchange"
	PathLibrarySignOut    = "/v1/library/auth/signout"
	PathLibraryQueue      = "/v1/library/admin/queue"
	PathLibraryItemDecide = "/v1/library/admin/items/{id}/decisions"
	PathLibraryAdminPart  = "/v1/library/admin/items/{id}/versions/{version}/{part}"
	PathLibraryRepDecide  = "/v1/library/admin/reports/{id}/decisions"
	PathLibraryBlock      = "/v1/library/admin/accounts/{id}/block"
	PathLibraryUnblock    = "/v1/library/admin/accounts/{id}/unblock"
	PathLibraryAccounts   = "/v1/library/admin/accounts"
	PathLibraryModerators = "/v1/library/admin/moderators"
	PathLibraryModerator  = "/v1/library/admin/moderators/{id}"
	PathLibraryLog        = "/v1/library/admin/log"
	PathLibraryUsage      = "/v1/library/admin/usage"
	// PathLibraryAuthorize is a page, for the browser signing in.
	PathLibraryAuthorize = "/library/authorize"
	// HeaderLibraryWeb carries the web server's secret: its requests aren't
	// counted as one address, and it alone may trade a sign-in's code.
	HeaderLibraryWeb = "X-Enowx-UI-Secret"
)

// What an item is, under which license, and where it stands.
const (
	LibraryComponent = "component"
	LibraryPage      = "page"

	LicenseMIT      = "mit"
	LicenseCCBY     = "cc-by-4.0"
	LicenseViewOnly = "view-only"

	LibraryUploading   = "uploading" // not submitted: never public
	LibraryReview      = "review"
	LibraryLive        = "live"
	LibraryRejected    = "rejected"
	LibraryWithdrawn   = "withdrawn"   // taken back before it showed
	LibraryUnpublished = "unpublished" // taken back after it showed
	LibraryRemoved     = "removed"     // taken down by a moderator
	LibrarySuperseded  = "superseded"  // a version a later one replaced

	TrustNew     = "new"
	TrustTrusted = "trusted"
	TrustBlocked = "blocked"

	RoleLibraryOwner     = "owner"
	RoleLibraryAdmin     = "admin"
	RoleLibraryModerator = "moderator"
)

// A version's objects.
const (
	PartSource     = "source"
	PartDoc        = "doc"
	PartDocPhone   = "doc-phone"
	PartThumb      = "thumb"
	PartThumbPhone = "thumb-phone"
)

// The limits the owner approved (docs/cloud.md).
const (
	MaxLibraryBundle     = 10 << 20
	MaxLibraryThumb      = 300 << 10
	MaxLibraryThumbWidth = 1200
	MaxLibraryComponent  = 64 << 10
	MaxLibraryAccount    = 200 << 20
	MaxLibraryLiveItems  = 50
	MaxLibraryVersions   = 5
	MaxLibraryUploadsDay = 20
	MaxLibraryNewReview  = 3
	MaxLibraryBucket     = 100 << 30
	MaxLibraryMonthWrite = 30 << 30
	LibraryWarnPercent   = 70
)

// LibraryCategories are the categories an item may be in.
var LibraryCategories = []string{"component", "dashboard", "landing-page", "mobile", "onboarding", "motion", "diagram", "icon-illustration", "other"}

// LibraryReasons are why an item or an account may be reported.
var LibraryReasons = []string{"spam", "copyright", "impersonation", "malicious", "private-data", "abuse", "other"}

// LibraryCreator is who made an item, as anyone sees them: never an email.
type LibraryCreator struct {
	ID     string `json:"id"`
	Handle string `json:"handle"`
	Name   string `json:"name"`
	Avatar string `json:"avatar,omitempty"`
}

// LibraryActions say what a viewer may do with an item: what the API
// answers too, not only what a page shows.
type LibraryActions struct {
	CopyPrompt   bool `json:"copyPrompt"`
	UseComponent bool `json:"useComponent"`
	UseReference bool `json:"useReference"`
	OpenInEnowX  bool `json:"openInEnowX"`
}

// LibraryItem is an item as the API shows it.
type LibraryItem struct {
	ID           string         `json:"id"`
	Slug         string         `json:"slug"`
	Kind         string         `json:"kind"`
	License      string         `json:"license"`
	Title        string         `json:"title"`
	Description  string         `json:"description"`
	Category     string         `json:"category"`
	Tags         []string       `json:"tags"`
	Style        string         `json:"style,omitempty"`
	Theme        string         `json:"theme,omitempty"`
	Device       string         `json:"device,omitempty"`
	HasMotion    bool           `json:"hasMotion"`
	MadeWith     string         `json:"madeWith"`
	SourceURL    string         `json:"sourceUrl,omitempty"`
	Status       string         `json:"status"`
	LiveVersion  int            `json:"liveVersion"`
	RejectReason string         `json:"rejectReason,omitempty"`
	Saves        int            `json:"saves"`
	Featured     bool           `json:"featured"`
	Width        int            `json:"width,omitempty"`
	Height       int            `json:"height,omitempty"`
	HasPhone     bool           `json:"hasPhone"`
	Creator      LibraryCreator `json:"creator"`
	Actions      LibraryActions `json:"actions"`
	Created      time.Time      `json:"created"`
	Updated      time.Time      `json:"updated"`
}

// LibraryItems is a page of items; Next asks for the following one.
type LibraryItems struct {
	Items []LibraryItem `json:"items"`
	Next  string        `json:"next,omitempty"`
}

// LibraryVersion is a version as its owner and moderators see it.
type LibraryVersion struct {
	Version  int              `json:"version"`
	Status   string           `json:"status"`
	Kind     string           `json:"kind"`
	License  string           `json:"license"`
	Width    int              `json:"width"`
	Height   int              `json:"height"`
	HasPhone bool             `json:"hasPhone"`
	Size     int64            `json:"size"`
	Created  time.Time        `json:"created"`
	Scan     []LibraryFinding `json:"scan,omitempty"`
	Prompt   string           `json:"prompt,omitempty"`
}

// LibraryFinding is what a design's check found: where and of what kind,
// never the value.
type LibraryFinding struct {
	Severity string `json:"severity"` // refuse or warn
	Kind     string `json:"kind"`
	Where    string `json:"where"`
}

// LibraryPublishParams starts an item, or with ItemID a new version of one.
type LibraryPublishParams struct {
	ItemID         string   `json:"itemId,omitempty"`
	Kind           string   `json:"kind"`
	License        string   `json:"license"`
	Title          string   `json:"title"`
	Description    string   `json:"description"`
	Category       string   `json:"category"`
	Tags           []string `json:"tags,omitempty"`
	Style          string   `json:"style,omitempty"`
	Theme          string   `json:"theme,omitempty"`
	Device         string   `json:"device,omitempty"`
	HasMotion      bool     `json:"hasMotion,omitempty"`
	MadeWith       string   `json:"madeWith,omitempty"`
	SourceURL      string   `json:"sourceUrl,omitempty"`
	Prompt         string   `json:"prompt"`
	PromptApproved bool     `json:"promptApproved"`
	Rights         bool     `json:"rights"`
	Format         string   `json:"format"`
	Width          int      `json:"width"`
	Height         int      `json:"height"`
}

// LibraryPublished says which item and version to upload to.
type LibraryPublished struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
}

// LibrarySubmitted is where a submitted version stands.
type LibrarySubmitted struct {
	ID       string           `json:"id"`
	Version  int              `json:"version"`
	Status   string           `json:"status"`
	Findings []LibraryFinding `json:"findings,omitempty"`
}

// LibraryPrompt is what Copy prompt copies: the text the creator approved,
// its credit under it.
type LibraryPrompt struct {
	Prompt string `json:"prompt"`
}

// LibraryMine is an account's own items with their latest version.
type LibraryMine struct {
	Items []LibraryMineItem `json:"items"`
	// Used and Limit are the bytes kept for them and the most allowed.
	Used  int64  `json:"used"`
	Limit int64  `json:"limit"`
	Trust string `json:"trust"`
}

// LibraryMineItem is an item of the asker's.
type LibraryMineItem struct {
	LibraryItem
	Latest LibraryVersion `json:"latest"`
}

// LibraryMe is who a session is, in the library.
type LibraryMe struct {
	Account LibraryCreator `json:"account"`
	Trust   string         `json:"trust"`
	// Role is the asker's moderating role, empty for none.
	Role string `json:"role,omitempty"`
}

// LibraryExchange trades a sign-in's code for a session.
type LibraryExchange struct {
	Code     string `json:"code"`
	Verifier string `json:"verifier"`
}

// LibrarySession is a web session: its token opens /v1/library alone.
type LibrarySession struct {
	Token   string    `json:"token"`
	Expires time.Time `json:"expires"`
	Me      LibraryMe `json:"me"`
}

// LibraryReportParams reports an item or an account.
type LibraryReportParams struct {
	ItemID    string `json:"itemId,omitempty"`
	AccountID string `json:"accountId,omitempty"`
	Reason    string `json:"reason"`
	Detail    string `json:"detail"`
}

// LibraryReport is a report as a moderator sees it. Who reported is for
// moderators alone: a creator never sees it.
type LibraryReport struct {
	ID        string    `json:"id"`
	ItemID    string    `json:"itemId,omitempty"`
	Item      string    `json:"item,omitempty"`
	AccountID string    `json:"accountId,omitempty"`
	Account   string    `json:"account,omitempty"`
	Reporter  string    `json:"reporter"`
	Reason    string    `json:"reason"`
	Detail    string    `json:"detail,omitempty"`
	Status    string    `json:"status"`
	Created   time.Time `json:"created"`
}

// LibraryQueueItem is an item waiting for a decision.
type LibraryQueueItem struct {
	LibraryItem
	Version LibraryVersion `json:"version"`
	Trust   string         `json:"trust"`
}

// LibraryQueue is the moderators' work: new accounts' designs, then reports.
type LibraryQueue struct {
	Items   []LibraryQueueItem `json:"items"`
	Reports []LibraryReport    `json:"reports"`
	Role    string             `json:"role"`
}

// The decisions a moderator can make.
const (
	DecisionApprove   = "approve"
	DecisionReject    = "reject"
	DecisionRemove    = "remove"
	DecisionFeature   = "feature"
	DecisionUnfeature = "unfeature"
	DecisionDismiss   = "dismiss"
	DecisionUphold    = "uphold"
)

// LibraryDecision is a decision on an item or a report; a reason is a must,
// as the log keeps it.
type LibraryDecision struct {
	Action  string `json:"action"`
	Version int    `json:"version,omitempty"`
	Reason  string `json:"reason"`
}

// LibraryReason says why, for actions that hold nothing else.
type LibraryReason struct {
	Reason string `json:"reason"`
}

// LibraryAppoint makes an account a moderator.
type LibraryAppoint struct {
	AccountID string `json:"accountId"`
	Reason    string `json:"reason"`
}

// LibraryAccount is an account as the owner's moderator picker shows it.
type LibraryAccount struct {
	LibraryCreator
	GitHub string `json:"github,omitempty"`
}

// LibraryModerator is a moderator as the owner's list shows them.
type LibraryModerator struct {
	LibraryAccount
	AppointedBy string    `json:"appointedBy,omitempty"`
	Since       time.Time `json:"since"`
}

// LibraryLogEntry is a line of the moderation log.
type LibraryLogEntry struct {
	ID        int64     `json:"id"`
	At        time.Time `json:"at"`
	Actor     string    `json:"actor"`
	Action    string    `json:"action"`
	ItemID    string    `json:"itemId,omitempty"`
	AccountID string    `json:"accountId,omitempty"`
	Version   int       `json:"version,omitempty"`
	Reason    string    `json:"reason"`
}

// LibraryLog is a page of the log, the latest first.
type LibraryLog struct {
	Entries []LibraryLogEntry `json:"entries"`
	Next    string            `json:"next,omitempty"`
}

// LibraryUsage says how far the bucket is from its limits.
type LibraryUsage struct {
	Month        string `json:"month"`
	BytesWritten int64  `json:"bytesWritten"`
	BytesStored  int64  `json:"bytesStored"`
	WriteOps     int64  `json:"writeOps"`
	StoredLimit  int64  `json:"storedLimit"`
	WrittenLimit int64  `json:"writtenLimit"`
	// Warn says one is past LibraryWarnPercent.
	Warn bool `json:"warn"`
}

package api

import "time"

// enowX UI's creators: profiles, open for work, likes, collections, follows, and
// taking one's data out or deleting it. Nothing here has an email, by construction:
// no type holds one.

const (
	PathLibraryCreators           = "/v1/library/creators"
	PathLibraryCreator            = "/v1/library/creators/{handle}"
	PathLibraryCreatorItems       = "/v1/library/creators/{handle}/items"
	PathLibraryCreatorCollections = "/v1/library/creators/{handle}/collections"
	PathLibraryFollow             = "/v1/library/creators/{handle}/follow"
	PathLibraryFollowing          = "/v1/library/following"
	PathLibraryFeed               = "/v1/library/feed"
	PathLibraryExport             = "/v1/library/me/export"
	PathLibrarySaves              = "/v1/library/saves"
	PathLibrarySave               = "/v1/library/saves/{id}"
	PathLibraryCollections        = "/v1/library/collections"
	PathLibraryCollection         = "/v1/library/collections/{id}"
	PathLibraryCollectionItem     = "/v1/library/collections/{id}/items/{item}"
)

// BadgeContributor is on a profile whose GitHub account the Cloud found to have
// contributed to enowX.
const BadgeContributor = "contributor"

// LibraryWorkKinds are the kinds of work a creator may say they're open to.
var LibraryWorkKinds = []string{"freelance", "part-time", "full-time"}

// LibraryZone is a span of time zones the open for work list filters by.
type LibraryZone struct {
	ID string
	// From and To are hours from UTC, both within the zone.
	From, To float64
}

// LibraryZones are the spans the list offers; together they hold every time zone.
var LibraryZones = []LibraryZone{{"west", -12, -3}, {"middle", -2.75, 4.75}, {"east", 5, 14}}

// The limits on what a profile and a collection hold.
const (
	MaxLibraryBio         = 500
	MaxLibraryLinks       = 5
	MaxLibrarySkills      = 12
	MaxLibraryCollections = 100
	MaxLibraryInCollect   = 500
	MaxLibraryFollows     = 2000
	MaxLibrarySaves       = 5000
)

// LibraryProfileStats are what a creator's public numbers are. Absent when they
// chose not to show them.
type LibraryProfileStats struct {
	Designs   int `json:"designs"`
	Saves     int `json:"saves"`
	Followers int `json:"followers"`
}

// LibraryOpenForWork is what a creator open for work shows. Contact is a link and
// nothing else: the platform has no messages or forms.
type LibraryOpenForWork struct {
	Kinds      []string `json:"kinds"`
	Skills     []string `json:"skills"`
	TimeZone   string   `json:"timeZone,omitempty"`
	ContactURL string   `json:"contactUrl"`
}

// LibraryProfileParams is all a creator sets of their profile, sent whole.
type LibraryProfileParams struct {
	Handle     string   `json:"handle"`
	Bio        string   `json:"bio"`
	Links      []string `json:"links"`
	ShowGitHub bool     `json:"showGitHub"`
	ShowStats  bool     `json:"showStats"`
	Hidden     bool     `json:"hidden"`

	OpenForWork bool     `json:"openForWork"`
	WorkKinds   []string `json:"workKinds"`
	Skills      []string `json:"skills"`
	TimeZone    string   `json:"timeZone"`
	ContactURL  string   `json:"contactUrl"`
}

// LibraryViewer says how a profile stands to who asks, when they're signed in.
type LibraryViewer struct {
	Own       bool `json:"own"`
	Following bool `json:"following"`
}

// LibraryProfile is a creator as anyone sees them. Hidden and Settings are for the
// creator; Hidden for moderators too.
type LibraryProfile struct {
	LibraryCreator
	Bio         string               `json:"bio"`
	Links       []string             `json:"links"`
	GitHub      string               `json:"github,omitempty"`
	Badges      []string             `json:"badges"`
	Stats       *LibraryProfileStats `json:"stats,omitempty"`
	OpenForWork *LibraryOpenForWork  `json:"openForWork,omitempty"`
	Since       time.Time            `json:"since"`

	Hidden   bool                  `json:"hidden,omitempty"`
	Viewer   *LibraryViewer        `json:"viewer,omitempty"`
	Settings *LibraryProfileParams `json:"settings,omitempty"`
}

// LibraryWork is a design as a line in a list of people shows it.
type LibraryWork struct {
	ID          string `json:"id"`
	Slug        string `json:"slug"`
	Title       string `json:"title"`
	LiveVersion int    `json:"liveVersion"`
}

// LibraryOpenCreator is a line of the open for work list.
type LibraryOpenCreator struct {
	LibraryCreator
	Badges      []string           `json:"badges"`
	OpenForWork LibraryOpenForWork `json:"openForWork"`
	Works       []LibraryWork      `json:"works"`
}

// LibraryOpenCreators is a page of the list; Skills are what its people say most,
// for its filter.
type LibraryOpenCreators struct {
	Creators []LibraryOpenCreator `json:"creators"`
	Next     string               `json:"next,omitempty"`
	Skills   []string             `json:"skills"`
}

// LibrarySaved says whether the asker likes an item, and how many do.
type LibrarySaved struct {
	Saved bool `json:"saved"`
	Saves int  `json:"saves"`
}

// LibraryFollowState says whether the asker follows a creator, and how many do.
type LibraryFollowState struct {
	Following bool `json:"following"`
	Followers int  `json:"followers"`
}

// LibraryFollowing is who the asker follows. Only they get this.
type LibraryFollowing struct {
	Creators []LibraryCreator `json:"creators"`
}

// LibraryCollectionParams makes or changes a collection.
type LibraryCollectionParams struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Public      bool   `json:"public"`
}

// LibraryCollection is a list of designs someone keeps. A private one is answered to
// its owner alone: to anyone else it isn't there. Contains answers ?item= on the
// owner's own list.
type LibraryCollection struct {
	ID          string         `json:"id"`
	Owner       LibraryCreator `json:"owner"`
	Title       string         `json:"title"`
	Description string         `json:"description"`
	Public      bool           `json:"public"`
	Items       int            `json:"items"`
	Contains    bool           `json:"contains,omitempty"`
	Created     time.Time      `json:"created"`
	Updated     time.Time      `json:"updated"`
}

// LibraryCollections is a list of collections.
type LibraryCollections struct {
	Collections []LibraryCollection `json:"collections"`
}

// LibraryCollectionPage is a collection and a page of what it holds.
type LibraryCollectionPage struct {
	Collection LibraryCollection `json:"collection"`
	Own        bool              `json:"own"`
	Items      []LibraryItem     `json:"items"`
	Next       string            `json:"next,omitempty"`
}

// LibraryDeleteMe confirms deleting a profile: its handle, typed.
type LibraryDeleteMe struct {
	Confirm string `json:"confirm"`
}

// LibraryExportReport is a report the account made.
type LibraryExportReport struct {
	ItemID    string    `json:"itemId,omitempty"`
	AccountID string    `json:"accountId,omitempty"`
	Reason    string    `json:"reason"`
	Detail    string    `json:"detail,omitempty"`
	Status    string    `json:"status"`
	Created   time.Time `json:"created"`
}

// LibraryExportDecision is a line of the moderation log about the account or its
// items. Who decided isn't in it, as that's a moderator's.
type LibraryExportDecision struct {
	At      time.Time `json:"at"`
	Action  string    `json:"action"`
	ItemID  string    `json:"itemId,omitempty"`
	Version int       `json:"version,omitempty"`
	Reason  string    `json:"reason"`
}

// LibraryExportCollection is a collection with what it holds.
type LibraryExportCollection struct {
	LibraryCollection
	ItemIDs []string `json:"itemIds"`
}

// LibraryExport is what the library keeps of an account, for its owner: never
// another person's identity, nor the account's email, which is the Cloud's.
// A design's files aren't inside it: the app that published them has them.
type LibraryExport struct {
	Format      string                    `json:"format"`
	Exported    time.Time                 `json:"exported"`
	Profile     LibraryProfile            `json:"profile"`
	Trust       string                    `json:"trust"`
	Items       []LibraryMineItem         `json:"items"`
	Likes       []string                  `json:"likes"`
	Collections []LibraryExportCollection `json:"collections"`
	Following   []string                  `json:"following"`
	Followers   int                       `json:"followers"`
	Reports     []LibraryExportReport     `json:"reports"`
	Decisions   []LibraryExportDecision   `json:"decisions"`
	// Comments they wrote, and what the library told them. Who else was
	// involved isn't in it.
	Comments      []LibraryExportComment      `json:"comments"`
	Notifications []LibraryExportNotification `json:"notifications"`
}

// LibraryExportComment is a comment the account wrote.
type LibraryExportComment struct {
	ItemID  string    `json:"itemId"`
	Body    string    `json:"body"`
	Status  string    `json:"status"`
	Created time.Time `json:"created"`
}

// LibraryExportNotification is something the library told the account.
type LibraryExportNotification struct {
	Kind    string    `json:"kind"`
	ItemID  string    `json:"itemId,omitempty"`
	Reason  string    `json:"reason,omitempty"`
	Created time.Time `json:"created"`
}

// LibraryExportFormat names LibraryExport's shape.
const LibraryExportFormat = "enowx-library-export/1"

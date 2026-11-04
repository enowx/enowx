package proto

import "time"

// Reading the community library from a host: the app's browse page and agents
// (daemon/library_browse.go). All of it through the cloud the user set. What
// comes back was written by other people: whoever gives it to an agent labels
// it first (designshare.ForAgents).
const (
	// MethodLibrarySearch lists what's live in the library; On false when the
	// host's cloud has none.
	MethodLibrarySearch = "library.search" // LibrarySearchParams → LibraryResults
	// MethodLibraryItem is one live item.
	MethodLibraryItem = "library.item" // LibraryItemRef → LibraryListing
	// MethodLibraryPrompt is an item's prompt with its credit; a view-only item
	// has none to give.
	MethodLibraryPrompt = "library.prompt" // LibraryItemRef → LibraryPromptAnswer
	// MethodLibraryPicture is a live version's picture, as a WebP.
	MethodLibraryPicture = "library.picture" // LibraryPictureParams → LibraryPicture
	// MethodLibraryAskPublish asks the app to show the user a design's publish
	// form. It sends nothing: only the form does.
	MethodLibraryAskPublish = "library.askPublish" // LibraryAskPublishParams → LibraryPublishAsked

	// MethodLibraryAskSkill asks the app to show the user a card for keeping a
	// design as a skill. It keeps nothing: only the user's click does.
	MethodLibraryAskSkill = "library.askSkill" // LibraryAskSkillParams → LibrarySkillAsked

	// EventLibrarySkillAsked says someone asked to keep a design as a skill: the
	// app shows it as a card the user may follow.
	EventLibrarySkillAsked = "library.skillAsked" // LibrarySkillAsked
	// EventLibraryPublishAsked says someone asked for a design's publish form:
	// the app shows it as a card the user may follow.
	EventLibraryPublishAsked = "library.publishAsked" // LibraryPublishAsked
)

// LibrarySearchParams narrows what's listed; empty fields don't.
type LibrarySearchParams struct {
	Query    string `json:"query,omitempty"`
	Category string `json:"category,omitempty"`
	Kind     string `json:"kind,omitempty"`
	License  string `json:"license,omitempty"`
	Sort     string `json:"sort,omitempty"`
	Limit    int    `json:"limit,omitempty"`
	// Cursor is the Next of the page before.
	Cursor string `json:"cursor,omitempty"`
}

// LibraryActions say what an item's license and kind allow. The cloud decides.
type LibraryActions struct {
	CopyPrompt   bool `json:"copyPrompt,omitempty"`
	UseComponent bool `json:"useComponent,omitempty"`
	UseReference bool `json:"useReference,omitempty"`
	OpenInEnowX  bool `json:"openInEnowX,omitempty"`
}

// LibraryListing is a live item, as anyone may see it: never an email.
type LibraryListing struct {
	ItemID      string   `json:"itemId"`
	Kind        string   `json:"kind"`
	License     string   `json:"license"`
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	Category    string   `json:"category,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	HasMotion   bool     `json:"hasMotion,omitempty"`
	MadeWith    string   `json:"madeWith,omitempty"`
	// Version is the live version, whose picture shows.
	Version  int  `json:"version"`
	Width    int  `json:"width,omitempty"`
	Height   int  `json:"height,omitempty"`
	HasPhone bool `json:"hasPhone,omitempty"`
	Featured bool `json:"featured,omitempty"`
	// Handle and Creator are who made it, to credit.
	Handle  string         `json:"handle"`
	Creator string         `json:"creator,omitempty"`
	Actions LibraryActions `json:"actions"`
	// URL is its page on the library's website, "" when the cloud names none.
	URL     string    `json:"url,omitempty"`
	Updated time.Time `json:"updated,omitzero"`
}

// LibraryResults is a page of the library.
type LibraryResults struct {
	// On says the host's cloud has a library; without it nothing shows.
	On      bool             `json:"on"`
	SiteURL string           `json:"siteUrl,omitempty"`
	Items   []LibraryListing `json:"items,omitempty"`
	Next    string           `json:"next,omitempty"`
}

// LibraryPromptAnswer is an item's prompt: the creator's approved text, its
// credit and license under it.
type LibraryPromptAnswer struct {
	Item   LibraryListing `json:"item"`
	Prompt string         `json:"prompt"`
}

// LibraryPictureParams names a live version's picture.
type LibraryPictureParams struct {
	ItemID  string `json:"itemId"`
	Version int    `json:"version"`
	// Phone asks for the picture of its phone design.
	Phone bool `json:"phone,omitempty"`
}

// LibraryPicture is a picture's WebP.
type LibraryPicture struct {
	Data []byte `json:"data"`
}

// LibraryAskPublishParams names the design whose form to show.
type LibraryAskPublishParams struct {
	Canvas string `json:"canvas"`
	Frame  string `json:"frame"`
	// From is the session asking, "" for someone at a terminal.
	From string `json:"from,omitempty"`
}

// LibraryPublishAsked is what's asked, and whether an app is there to show it.
type LibraryPublishAsked struct {
	Canvas      string `json:"canvas"`
	Frame       string `json:"frame"`
	CanvasTitle string `json:"canvasTitle,omitempty"`
	FrameName   string `json:"frameName,omitempty"`
	From        string `json:"from,omitempty"`
	// AppOpen says an app is connected to this host to show it.
	AppOpen bool `json:"appOpen,omitempty"`
}

// LibraryAskSkillParams names the design to offer as a skill.
type LibraryAskSkillParams struct {
	ItemID  string `json:"itemId"`
	Version int    `json:"version,omitempty"`
	// From is the session asking, "" for someone at a terminal.
	From string `json:"from,omitempty"`
}

// LibrarySkillAsked is what's asked, and whether an app is there to show it. It
// holds no word of the design's: the dialog reads those from the cloud.
type LibrarySkillAsked struct {
	ItemID  string `json:"itemId"`
	Version int    `json:"version,omitempty"`
	From    string `json:"from,omitempty"`
	// AppOpen says an app is connected to this host to show it.
	AppOpen bool `json:"appOpen,omitempty"`
}

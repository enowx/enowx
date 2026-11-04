package api

import "encoding/json"

// Taking something from the library. Each answers for what shows, or with
// ?version=N for a version that showed before: under that version's own license
// and kind, since a license change applies from its version on.
const (
	// PathLibraryCode is a component's HTML and CSS. A full page has none.
	PathLibraryCode = "/v1/library/items/{id}/code"
	// PathLibrarySource is a version's bundle, for a device opening a copy.
	PathLibrarySource = "/v1/library/items/{id}/source"
	// PathLibrarySkill is a full page's SKILL.md, as text/markdown.
	PathLibrarySkill = "/v1/library/items/{id}/skill"
	// PathLibraryReference is a full page's picture, as a WebP, for a device
	// keeping it as a canvas's reference.
	PathLibraryReference = "/v1/library/items/{id}/reference"
)

// LibraryCode is what Use component copies: both copies open with the notice,
// and Assets are the files they name that don't come along.
type LibraryCode struct {
	Version int      `json:"version"`
	License string   `json:"license"`
	HTML    string   `json:"html"`
	CSS     string   `json:"css"`
	Assets  []string `json:"assets"`
}

// LibrarySource is a version of a design for a copy: its bundle, and the credit
// the copy keeps.
type LibrarySource struct {
	ItemID  string `json:"itemId"`
	Version int    `json:"version"`
	Slug    string `json:"slug"`
	Title   string `json:"title"`
	Handle  string `json:"handle"`
	License string `json:"license"`
	Address string `json:"address"`
	// Bundle is the design's enowx-design/1 bundle, as designshare reads it.
	Bundle json.RawMessage `json:"bundle"`
}

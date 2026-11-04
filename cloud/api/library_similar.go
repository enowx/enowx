package api

// LibrarySimilar is a design whose picture looks close to one under review:
// advice for a moderator to check by hand, never acted on by itself.
type LibrarySimilar struct {
	ItemID  string `json:"itemId"`
	Slug    string `json:"slug"`
	Title   string `json:"title"`
	Creator string `json:"creator"`
	// Status is the item's, as a design taken down and put back up matters too.
	Status  string `json:"status"`
	Version int    `json:"version"`
	// SameCreator says it's the same account's.
	SameCreator bool `json:"sameCreator,omitempty"`
	// Distance is how many of the hash's 64 bits differ: fewer is closer.
	Distance int `json:"distance"`
}

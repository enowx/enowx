package proto

import "time"

// A team's space gone from a host because its owner deleted the team
// (docs/cloud.md, Deleting a team): the host keeps saying so until the
// user has seen it, so the app shows it however long it was away.
const (
	MethodSpacesGone = "space.gone" // → []SpaceGone
	// MethodSpaceGoneSeen forgets a team's notice: the user saw it.
	MethodSpaceGoneSeen = "space.gone.seen" // SpaceRef
)

// EventSpacesGone carries the full []SpaceGone as one comes or goes.
const EventSpacesGone = "spaces.gone"

// SpaceGone is a team whose space left the host as it was deleted.
type SpaceGone struct {
	Team string    `json:"team"`
	Name string    `json:"name"`
	At   time.Time `json:"at"`
	// Shared says the host was shared with the team, and no longer is.
	Shared bool `json:"shared,omitempty"`
}

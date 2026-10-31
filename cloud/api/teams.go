package api

import "time"

// Teams, their members and invites. A member's role says what they may do
// (docs/cloud.md, Roles).
const (
	PathTeams = "/v1/teams"
	// PathTeam is one team: GET it with its members, PATCH its name and
	// rules (admins), DELETE it (its owner).
	PathTeam = "/v1/teams/{team}"
	// PathMember is one member: PATCH their role, DELETE them (admins, or
	// the member leaving).
	PathMember  = "/v1/teams/{team}/members/{account}"
	PathInvites = "/v1/teams/{team}/invites"
	PathInvite  = "/v1/teams/{team}/invites/{invite}"
	// PathJoin accepts an invite's token.
	PathJoin = "/v1/invites/accept"
	// PathInvitePeek says, without signing in, what an invite's token is
	// for: the team, the role and who invites.
	PathInvitePeek = "/v1/invites/peek"
)

// ErrTeamDeleted is the Error a team's former member is answered, with
// status 404, once its owner deleted it: their hosts drop their copy of
// its space as for any team they aren't in, and say it was deleted.
const ErrTeamDeleted = "the team was deleted by its owner"

// Roles, the highest first.
const (
	RoleOwner  = "owner"
	RoleAdmin  = "admin"
	RoleMember = "member"
	RoleViewer = "viewer"
)

// Team is a team as one of its members sees it.
type Team struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// CopyOut lets members copy the team's items into their personal space.
	CopyOut bool `json:"copyOut"`
	// Epoch is the team key's epoch: it moves on when a member goes.
	Epoch   int       `json:"epoch"`
	Created time.Time `json:"created"`
	// Role is the asker's.
	Role string `json:"role"`
	// Members, when one team is asked for.
	Members []Member `json:"members,omitempty"`
}

// Member is a team's member.
type Member struct {
	Account Account   `json:"account"`
	Role    string    `json:"role"`
	Joined  time.Time `json:"joined"`
}

// TeamParams makes or changes a team; a field left out stays.
type TeamParams struct {
	Name    *string `json:"name,omitempty"`
	CopyOut *bool   `json:"copyOut,omitempty"`
}

// RoleParams changes a member's role. Making someone owner hands the team
// over: its owner becomes an admin.
type RoleParams struct {
	Role string `json:"role"`
}

// InviteParams invites to a team: the role given (admin, member or
// viewer), and an email to send it to when the server sends emails.
type InviteParams struct {
	Role  string `json:"role"`
	Email string `json:"email,omitempty"`
}

// Invite is an invite to a team. Token, once, to whoever made it: it is
// what the invited accept with, alone or in URL.
type Invite struct {
	ID      string    `json:"id"`
	Role    string    `json:"role"`
	Email   string    `json:"email,omitempty"`
	Created time.Time `json:"created"`
	Expires time.Time `json:"expires"`
	Token   string    `json:"token,omitempty"`
	URL     string    `json:"url,omitempty"`
}

// InvitePeek is what an invite is for, shown before it is accepted.
type InvitePeek struct {
	Team    string    `json:"team"` // its name
	Role    string    `json:"role"`
	By      string    `json:"by"`                // who invites, by name
	ByLogin string    `json:"byLogin,omitempty"` // and their GitHub login, which names them for sure
	Expires time.Time `json:"expires"`
}

// JoinParams accepts an invite, or peeks at it.
type JoinParams struct {
	Token string `json:"token"`
}

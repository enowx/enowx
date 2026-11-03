package proto

import "time"

// An agent reading other sessions, when the user asks it to (to copy a
// feature from another project, say): the sessions on every host the
// user's app reaches, and one's conversation and project, read-only. They
// come live from the session's host, through the app for another host,
// and are never kept in a cloud.
const (
	MethodSessionsEverywhere = "sessions.everywhere" // → []HostSessions
	MethodSessionPeek        = "session.peek"        // SessionPeekParams → SessionPeek
)

// HostSessions are a host's sessions, or why they couldn't be read.
type HostSessions struct {
	Host     string        `json:"host"` // its daemon's ID
	HostName string        `json:"hostName"`
	Sessions []SessionInfo `json:"sessions"`
	Err      string        `json:"err,omitempty"`
}

// SessionPeekParams names a session (its ID or name) on Host (a host's
// name or daemon ID; "" this one), and how many of its last messages to
// read (Last; 40 when 0).
type SessionPeekParams struct {
	Host    string `json:"host,omitempty"`
	Session string `json:"session"`
	Last    int    `json:"last,omitempty"`
	// From is the session reading it, as in SessionRef.
	From string `json:"from,omitempty"`
}

// SessionPeek is a session's conversation as another agent reads it: its
// last messages, what looked like a secret taken out, and where its
// project is, to read with project context and project read.
type SessionPeek struct {
	Host     string        `json:"host"`
	HostName string        `json:"hostName"`
	Session  SessionInfo   `json:"session"`
	Messages []PeekMessage `json:"messages"`
	Earlier  int           `json:"earlier,omitempty"` // messages before those
	Reason   string        `json:"reason,omitempty"`  // why there are none
}

// PeekMessage is a message of a conversation: the user's, the agent's, or
// a tool the agent used.
type PeekMessage struct {
	Role string    `json:"role"` // TranscriptUser, TranscriptAssistant, or "tool"
	Time time.Time `json:"time,omitzero"`
	Text string    `json:"text"`
}

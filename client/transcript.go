package client

import "github.com/enowx/enowx/proto"

// Transcript returns what changed in a session's conversation after
// revision since.
func (c *Client) Transcript(id string, since int64) (proto.Transcript, error) {
	return c.TranscriptPage(proto.TranscriptParams{ID: id, Since: since})
}

// TranscriptPage reads a part of session p.ID's conversation, as
// TranscriptParams says.
func (c *Client) TranscriptPage(p proto.TranscriptParams) (proto.Transcript, error) {
	var out proto.Transcript
	err := c.Call(proto.MethodSessionTranscript, p, &out)
	return out, err
}

// SwitchMode switches session id's agent to permission mode mode; the
// session's next transcript says the mode it reached.
func (c *Client) SwitchMode(id, mode string) error {
	return c.Call(proto.MethodSessionMode, proto.SessionModeParams{ID: id, Mode: mode}, nil)
}

// SwitchSettings changes a session's model, its effort or both (SessionSettingsParams).
func (c *Client) SwitchSettings(p proto.SessionSettingsParams) error {
	return c.Call(proto.MethodSessionSettings, p, nil)
}

// Rewind takes session id's agent back to before the user's message back
// messages from the end, whose text is text; the message sent next takes
// its place.
func (c *Client) Rewind(id string, back int, text string) error {
	return c.Call(proto.MethodSessionRewind, proto.SessionRewindParams{ID: id, Back: back, Text: text}, nil)
}

// Commands lists session id's agent's slash commands; Loading says they
// are on the way: ask again.
func (c *Client) Commands(id string) (proto.AgentCommands, error) {
	var out proto.AgentCommands
	err := c.Call(proto.MethodSessionCommands, proto.SessionRef{ID: id}, &out)
	return out, err
}

// History lists the conversations agents had in project's folder and its
// worktrees, newest first.
func (c *Client) History(project string) ([]proto.Conversation, error) {
	var out []proto.Conversation
	err := c.Call(proto.MethodAgentHistory, proto.HistoryParams{Project: project}, &out)
	return out, err
}

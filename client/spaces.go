package client

import "github.com/enowx/enowx/proto"

// Spaces lists the teams' spaces on the host; none from a daemon of
// before, or while it isn't signed in to the cloud.
func (c *Client) Spaces() ([]proto.Space, error) {
	var list []proto.Space
	err := c.Call(proto.MethodSpaces, nil, &list)
	if IsUnknownMethod(err) {
		return nil, nil
	}
	return list, err
}

// SyncSpace syncs team's space with the cloud now, and says how it went.
func (c *Client) SyncSpace(team string) (proto.Space, error) {
	var sp proto.Space
	err := c.Call(proto.MethodSpaceSync, proto.SpaceRef{Team: team}, &sp)
	return sp, err
}

// TeamProjects lists every team's projects, each with its copies on the
// host; none from a daemon of before.
func (c *Client) TeamProjects() ([]proto.TeamProject, error) {
	var list []proto.TeamProject
	err := c.Call(proto.MethodTeamProjects, nil, &list)
	if IsUnknownMethod(err) {
		return nil, nil
	}
	return list, err
}

// AddTeamProject adds a Git link to a team, as one of its admins.
func (c *Client) AddTeamProject(p proto.TeamProjectAddParams) (proto.TeamProject, error) {
	var tp proto.TeamProject
	err := c.Call(proto.MethodTeamProjectAdd, p, &tp)
	return tp, err
}

// RemoveTeamProject takes a project out of a team, as one of its admins.
func (c *Client) RemoveTeamProject(team, id string) error {
	return c.Call(proto.MethodTeamProjectRemove, proto.TeamProjectRef{Team: team, ID: id}, nil)
}

// MoveNoteToSpace takes a note, and the pages under it, to team's space
// ("" the user's own), or copies them there.
func (c *Client) MoveNoteToSpace(id, team string, copy bool) (proto.NoteInfo, error) {
	var info proto.NoteInfo
	err := c.Call(proto.MethodNoteMoveSpace, proto.SpaceMoveParams{ID: id, Team: team, Copy: copy}, &info)
	return info, err
}

// MoveCanvasToSpace takes a canvas to team's space, or copies it there.
func (c *Client) MoveCanvasToSpace(id, team string, copy bool) (proto.CanvasInfo, error) {
	var info proto.CanvasInfo
	err := c.Call(proto.MethodCanvasMoveSpace, proto.SpaceMoveParams{ID: id, Team: team, Copy: copy}, &info)
	return info, err
}

// TeamKit lists every team's kit.
func (c *Client) TeamKit() ([]proto.TeamKitItem, error) {
	var list []proto.TeamKitItem
	err := c.Call(proto.MethodTeamKit, nil, &list)
	if IsUnknownMethod(err) {
		return nil, nil
	}
	return list, err
}

// AddToTeamKit copies the host's agent item (an EnvItem's ID) into team's
// kit.
func (c *Client) AddToTeamKit(team, item string) (proto.TeamKitItem, error) {
	var it proto.TeamKitItem
	err := c.Call(proto.MethodTeamKitAdd, proto.TeamKitAddParams{Team: team, Item: item}, &it)
	return it, err
}

// RemoveFromTeamKit takes an item out of team's kit.
func (c *Client) RemoveFromTeamKit(team, kind, name string) error {
	return c.Call(proto.MethodTeamKitRemove, proto.TeamKitRef{Team: team, Kind: kind, Name: name}, nil)
}

// ProjectRules reads the rules of the project at folder path.
func (c *Client) ProjectRules(path string) (proto.ProjectRules, error) {
	var r proto.ProjectRules
	err := c.Call(proto.MethodProjectRules, proto.ProjectRulesRef{Path: path}, &r)
	return r, err
}

// SaveProjectRules writes a project's rules (and, a team's admins, who
// edit them).
func (c *Client) SaveProjectRules(r proto.ProjectRules) (proto.ProjectRules, error) {
	var out proto.ProjectRules
	err := c.Call(proto.MethodProjectRulesSave, r, &out)
	return out, err
}

// RulesHistory lists rules as they were before their saves, newest first.
func (c *Client) RulesHistory(p proto.RulesDraftParams) ([]proto.RulesVersion, error) {
	var out []proto.RulesVersion
	err := c.Call(proto.MethodRulesHistory, p, &out)
	return out, err
}

// DraftRules has the host's AI write new rules; nothing is saved.
func (c *Client) DraftRules(p proto.RulesDraftParams) (proto.RulesDraft, error) {
	var out proto.RulesDraft
	err := c.Call(proto.MethodRulesDraft, p, &out)
	return out, err
}

// SessionsEverywhere lists the sessions of the host and of every host
// the app reaches.
func (c *Client) SessionsEverywhere() ([]proto.HostSessions, error) {
	var out []proto.HostSessions
	err := c.Call(proto.MethodSessionsEverywhere, nil, &out)
	return out, err
}

// PeekSession reads a session's conversation, on the host or another.
func (c *Client) PeekSession(p proto.SessionPeekParams) (proto.SessionPeek, error) {
	var out proto.SessionPeek
	err := c.Call(proto.MethodSessionPeek, p, &out)
	return out, err
}

// TeamAuth hands a host, connected to through its team listener, a ticket
// the cloud gave this member for it: who they are there from now on.
func (c *Client) TeamAuth(ticket string) (proto.Visitor, error) {
	var v proto.Visitor
	err := c.Call(proto.MethodTeamAuth, proto.TeamAuthParams{Ticket: ticket}, &v)
	return v, err
}

// HostVisitors lists the teams' members connected to the host.
func (c *Client) HostVisitors() ([]proto.Visitor, error) {
	var out []proto.Visitor
	err := c.Call(proto.MethodHostVisitors, nil, &out)
	return out, err
}

// HostShare is what the host shares with its teams.
func (c *Client) HostShare() (proto.HostShare, error) {
	var out proto.HostShare
	err := c.Call(proto.MethodHostShare, nil, &out)
	return out, err
}

// SetHostShare shares projects (their folders) with team; none unshares.
func (c *Client) SetHostShare(team string, projects []string) (proto.HostShare, error) {
	var out proto.HostShare
	err := c.Call(proto.MethodHostShareSet, proto.HostShareTeam{Team: team, Projects: projects}, &out)
	return out, err
}

// SetHostShareTeam is SetHostShare saying, too, whether the team's
// members may start sessions of their own (p.AllowSessions).
func (c *Client) SetHostShareTeam(p proto.HostShareTeam) (proto.HostShare, error) {
	var out proto.HostShare
	err := c.Call(proto.MethodHostShareSet, p, &out)
	return out, err
}

// TeamHosts lists the hosts members share with the host's teams.
func (c *Client) TeamHosts() ([]proto.TeamHost, error) {
	var out []proto.TeamHost
	err := c.Call(proto.MethodTeamHosts, nil, &out)
	if IsUnknownMethod(err) {
		return nil, nil
	}
	return out, err
}

// AskTakeover asks the host's user to let this visitor type into session.
func (c *Client) AskTakeover(session string) error {
	return c.Call(proto.MethodTakeoverAsk, proto.SessionRef{ID: session}, nil)
}

// AnswerTakeover lets visitor account take over session, or not.
func (c *Client) AnswerTakeover(session, account string, allow bool) error {
	return c.Call(proto.MethodTakeoverAnswer, proto.TakeoverAnswer{Session: session, Account: account, Allow: allow}, nil)
}

// EndTakeover takes session back from visitor account (the host's user),
// or gives it back (the visitor, account "").
func (c *Client) EndTakeover(session, account string) error {
	return c.Call(proto.MethodTakeoverEnd, proto.TakeoverAnswer{Session: session, Account: account}, nil)
}

// SetSessionPrivate keeps a session from the host's teams, or shows it
// to them again.
func (c *Client) SetSessionPrivate(id string, private bool) error {
	return c.Call(proto.MethodSessionPrivate, proto.SessionPrivateParams{ID: id, Private: private}, nil)
}

// TeamChat reads team's last messages (100 when last is 0), oldest first.
func (c *Client) TeamChat(team string, last int) ([]proto.ChatMessage, error) {
	var out []proto.ChatMessage
	err := c.Call(proto.MethodTeamChat, proto.TeamChatParams{Team: team, Last: last}, &out)
	return out, err
}

// SendChat sends a message to team's chat.
func (c *Client) SendChat(p proto.ChatSendParams) (proto.ChatMessage, error) {
	var m proto.ChatMessage
	err := c.Call(proto.MethodTeamChatSend, p, &m)
	return m, err
}

// RefreshChat takes what team sent, now.
func (c *Client) RefreshChat(team string) error {
	return c.Call(proto.MethodTeamChatRefresh, proto.SpaceRef{Team: team}, nil)
}

// EditChat writes new text over the user's own message.
func (c *Client) EditChat(p proto.ChatEditParams) (proto.ChatMessage, error) {
	var m proto.ChatMessage
	err := c.Call(proto.MethodTeamChatEdit, p, &m)
	return m, err
}

// DeleteChat deletes a message of a team's chat: the user's own, or any
// when they administer the team.
func (c *Client) DeleteChat(team, id string) error {
	return c.Call(proto.MethodTeamChatDelete, proto.ChatRef{Team: team, ID: id}, nil)
}

// ReactChat adds the user's reaction to a message of a team's chat, or with
// off takes it back.
func (c *Client) ReactChat(p proto.ChatReactParams) (proto.ChatMessage, error) {
	var m proto.ChatMessage
	err := c.Call(proto.MethodTeamChatReact, p, &m)
	return m, err
}

// UploadChatImage uploads an image for a message of a team's chat to
// carry.
func (c *Client) UploadChatImage(p proto.ChatUploadParams) (proto.ChatAttachment, error) {
	var a proto.ChatAttachment
	err := c.Call(proto.MethodTeamChatUpload, p, &a)
	return a, err
}

// ChatImage is an attachment's image.
func (c *Client) ChatImage(p proto.ChatImageParams) ([]byte, error) {
	var img proto.ChatImage
	err := c.Call(proto.MethodTeamChatImage, p, &img)
	return img.Data, err
}

// SaveToTeamKit keeps a definition (a pipeline, an automation, a project
// template) in a team's kit.
func (c *Client) SaveToTeamKit(p proto.TeamKitSaveParams) (proto.TeamKitItem, error) {
	var it proto.TeamKitItem
	err := c.Call(proto.MethodTeamKitSave, p, &it)
	return it, err
}

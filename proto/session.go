package proto

import "time"

// Sessions: programs running in a PTY, and the streams that show them.
const (
	MethodSessionCreate = "session.create" // SessionCreateParams → SessionInfo
	MethodSessionList   = "session.list"   // → []SessionInfo
	MethodSessionAttach = "session.attach" // SessionRef → AttachResult
	MethodSessionDetach = "session.detach" // StreamRef
	MethodSessionResize = "session.resize" // ResizeParams
	MethodSessionClear  = "session.clear"  // SessionRef
	MethodSessionKill   = "session.kill"   // SessionRef
	// MethodSessionState is how an agent's hooks report its state (enowx
	// hook, run by the agent inside the session).
	MethodSessionState = "session.state" // SessionStateParams
	// MethodSessionContinue starts an ended session's program again in it:
	// its agent on its last conversation, or its shell.
	MethodSessionContinue = "session.continue" // SessionRef → SessionInfo
	// MethodSessionDisplay gives the session a virtual display, if it has
	// none yet (enowx display, run by the agent inside the session).
	MethodSessionDisplay = "session.display" // SessionRef → DisplayInfo
)

// MethodSessionDisplayStop stops the display serving Port, of session ID
// or one of its tabs, and what runs on it: the user closed the app's
// window (DisplayStopParams). The session's own program goes on.
const MethodSessionDisplayStop = "session.displayStop"

// DisplayStopParams names a display by its session and port.
type DisplayStopParams struct {
	ID   string `json:"id"`
	Port int    `json:"port"`
}

// DisplayInfo is a session's virtual display.
type DisplayInfo struct {
	Display string `json:"display"` // as DISPLAY: ":101"
	Port    int    `json:"port"`    // its HTML5 client, a port of the session
	// Env is the environment its apps run with: DISPLAY, and their scale.
	Env []string `json:"env,omitempty"`
}

// MethodDisplayScale tells the daemon the scale of the screen the app
// shows displays on (DisplayScaleParams), which displays started later
// draw at. Older daemons don't know it: their displays draw at 1.
const MethodDisplayScale = "display.scale"

// DisplayScaleParams is a screen's scale: 2 for a Retina screen.
type DisplayScaleParams struct {
	Scale int `json:"scale"`
}

// Agent states, as the agent's hooks report them (SessionInfo.State).
// Agents without hooks have none: Active tells whether they work.
const (
	StateWorking = "working" // on a turn
	StateWaiting = "waiting" // asking the user: a permission, a question
	StateDone    = "done"    // finished its turn, waiting for the next prompt
)

// SessionStateParams reports the state of the agent in session ID.
type SessionStateParams struct {
	ID    string `json:"id"`
	State string `json:"state"`
}

const (
	// EventSessionUpdated carries a SessionInfo whenever a session is
	// created, changes title, starts or stops producing output, or exits.
	EventSessionUpdated = "session.updated"
	// EventSessionRemoved carries a SessionRef after a session is killed.
	EventSessionRemoved = "session.removed"
)

// Branch modes for SessionCreateParams.
const (
	BranchNone     = ""         // run in the project folder
	BranchNew      = "new"      // new branch in its own worktree
	BranchExisting = "existing" // existing branch, in a worktree unless checked out in the project folder
)

// MethodSessionScreen returns the text a session's terminal shows:
// SessionRef → SessionScreen. For scripts and agents reading another
// session (the enowx command line), not for drawing it.
const MethodSessionScreen = "session.screen"

// SessionScreen is a session's screen as text, a line per row.
type SessionScreen struct {
	Text string `json:"text"`
}

// MethodSessionInput types text into a session as its user would:
// SessionInputParams. Paste sends it as a paste, so a program takes its
// lines as one message; Enter presses Enter after it.
const MethodSessionInput = "session.input"

// SessionInputParams are what to type, and how.
type SessionInputParams struct {
	ID    string `json:"id"`
	Text  string `json:"text"`
	Paste bool   `json:"paste,omitempty"`
	Enter bool   `json:"enter,omitempty"`
}

// SessionInfo describes a session.
type SessionInfo struct {
	// Member is the team's member who started it from their device, nil
	// for the host's own user's (docs/cloud.md, Hosts and sessions).
	Member *SessionMember `json:"member,omitempty"`
	// Private keeps it from the host's teams, in a shared project too.
	Private  bool      `json:"private,omitempty"`
	ID       string    `json:"id"`
	Name     string    `json:"name,omitempty"` // given by the user
	Project  string    `json:"project,omitempty"`
	Agent    string    `json:"agent,omitempty"`
	Branch   string    `json:"branch,omitempty"`
	Worktree bool      `json:"worktree,omitempty"` // Dir is a worktree, not the project folder
	Command  []string  `json:"command"`
	Dir      string    `json:"dir"`
	Title    string    `json:"title"`            // set by the program (OSC 0/2)
	Prompt   string    `json:"prompt,omitempty"` // first prompt, given or typed
	Created  time.Time `json:"created"`
	Active   bool      `json:"active"` // the program is producing output
	// State is what the agent's hooks last said (StateWorking, …), "" when
	// it has none or they went stale.
	State string `json:"state,omitempty"`
	// SkipPermissions: the agent runs tools without asking.
	SkipPermissions bool `json:"skipPermissions,omitempty"`
	// Restored: the daemon restarted and relaunched (or kept) this session.
	Restored bool `json:"restored,omitempty"`
	Exited   bool `json:"exited"`
	ExitCode int  `json:"exitCode"`
	// Queued: the session waits for fewer agents to work on this host
	// (DaemonSettings.MaxAgents) before its agent starts.
	Queued bool `json:"queued,omitempty"`
	// Budget is how long, in seconds, the agent may work (0: no limit);
	// Worked how long it has, while the daemon ran; OverBudget that it
	// reached the budget, which interrupted it.
	Budget     int64 `json:"budget,omitempty"`
	Worked     int64 `json:"worked,omitempty"`
	OverBudget bool  `json:"overBudget,omitempty"`
	// Display is the loopback port of the session's virtual display (its
	// HTML5 client), 0 without one.
	Display int `json:"display,omitempty"`
	// DisplayScale is the scale its apps draw at (display.Display.Scale).
	DisplayScale int `json:"displayScale,omitempty"`
	// Parent is the session this one is a tab of: it runs in the parent's
	// folder and stops with it. "" for a session of its own.
	Parent string `json:"parent,omitempty"`
	// Note is what the session is at, a line or two its agent (enowx note)
	// or the user last set; "" for none.
	Note string `json:"note,omitempty"`
	// Group is the fan-out the session is part of: sessions given one
	// prompt, each in its own worktree, to compare and keep the best of.
	Group string `json:"group,omitempty"`
	// Account is the agent's account it runs as, "" for its own.
	Account string `json:"account,omitempty"`
	// NotesFor is the note whose Notes page agent the session is (an
	// agent on notes only, out of the session list).
	NotesFor string `json:"notesFor,omitempty"`
	// CanvasFor is the canvas whose Canvas page agent the session is (an
	// agent on canvases, out of the session list).
	CanvasFor string `json:"canvasFor,omitempty"`
	// Creds is the vault's credentials lent to the session: their names.
	Creds []string `json:"creds,omitempty"`
	// Hibernated: the daemon stopped its idle agent (DaemonSettings.
	// HibernateAfter); continuing it (session.continue) brings it back.
	Hibernated bool `json:"hibernated,omitempty"`
	// HibernatedHow is how and when it hibernated, while Hibernated.
	HibernatedHow *HibernateState `json:"hibernatedHow,omitempty"`
	// NoAutoHibernate keeps the daemon from hibernating it on its own.
	NoAutoHibernate bool `json:"noAutoHibernate,omitempty"`
}

// MethodSessionPrivate keeps a session from the host's teams, or shows it
// to them again: SessionPrivateParams; the host's own connections.
const MethodSessionPrivate = "session.private"

// SessionPrivateParams marks session ID private, or not.
type SessionPrivateParams struct {
	ID      string `json:"id"`
	Private bool   `json:"private"`
}

// MethodSessionNote sets a session's note: SessionNoteParams.
const MethodSessionNote = "session.note"

// SessionNoteParams are the note, "" to clear it.
type SessionNoteParams struct {
	ID   string `json:"id"`
	Note string `json:"note"`
}

// MethodSessionRename renames a session, the name every app of the host
// shows: SessionRenameParams; the host's own connections.
const MethodSessionRename = "session.rename"

// SessionRenameParams names session ID; "" goes back to the name it
// takes by itself (its conversation's title, its prompt, its branch).
type SessionRenameParams struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// SessionCreateParams starts a session.
//
// With Project set, the daemon picks the directory from BranchMode: the
// project folder, a new worktree on a new branch, or an existing branch.
// Agent picks the command from the daemon's presets; "custom" (or an empty
// Agent) runs Command, and an empty Command runs the login shell. Prompt
// is passed to agents that take one, or typed once the agent is ready.
type SessionCreateParams struct {
	Project    string `json:"project,omitempty"`
	Name       string `json:"name,omitempty"`
	Agent      string `json:"agent,omitempty"`
	BranchMode string `json:"branchMode,omitempty"`
	Branch     string `json:"branch,omitempty"`
	Base       string `json:"base,omitempty"`
	Prompt     string `json:"prompt,omitempty"`
	// Resume goes on with the agent's conversation of this ID
	// (Conversation.ID) instead of starting one; older daemons ignore it.
	Resume string `json:"resume,omitempty"`
	// RunSetup runs the project's worktree setup (ProjectDetail.Setup) in
	// a tab of the session, when it gets a new worktree.
	RunSetup bool `json:"runSetup,omitempty"`
	// SkipPermissions starts agents that support it in their mode that
	// runs tools without asking (see docs/agents.md).
	SkipPermissions bool `json:"skipPermissions,omitempty"`
	// SignIn runs the agent's sign-in (AgentInfo.SignIn) in the project
	// folder instead of the agent. Only sent to daemons whose AgentInfo
	// has a SignIn, so older daemons never get it.
	SignIn bool `json:"signIn,omitempty"`
	// Budget is how long, in seconds, the agent may work; 0: no limit.
	Budget int64 `json:"budget,omitempty"`
	// Display runs the session with a virtual display of its own, which
	// its programs' windows open on (DaemonInfo.Display tells whether the
	// host can).
	Display bool `json:"display,omitempty"`
	// Parent opens the session as a tab of session Parent, in its folder
	// (Project and the branch fields are taken from it). Older daemons
	// ignore it and open a session of its own in the project folder.
	Parent string `json:"parent,omitempty"`
	// Group puts the session with others given one prompt, to compare:
	// a fan-out (SessionInfo.Group).
	Group string `json:"group,omitempty"`
	// Account signs the agent in as one of its other accounts
	// (MethodAccountList); "" is its own. AccountAuto, with no Account,
	// picks it by the account preferences' defaults.
	Account     string `json:"account,omitempty"`
	AccountAuto bool   `json:"accountAuto,omitempty"`
	// Shell starts a terminal (Agent "shell") with the host's shell at
	// this path (ShellInfo.Path), not the one terminals start with.
	// Older daemons ignore it.
	Shell string `json:"shell,omitempty"`
	// Creds starts the session with the vault's credentials of these
	// names, the app holding the vault lending them. Older daemons ignore
	// it.
	Creds []string `json:"creds,omitempty"`
	// NotesFor starts the agent as note NotesFor's in the Notes page: in
	// a folder of its own, on notes only, its changes proposals
	// (SessionInfo.NotesFor). Older daemons ignore it.
	NotesFor string `json:"notesFor,omitempty"`
	// CanvasFor starts the agent as canvas CanvasFor's in the Canvas page:
	// in a folder of its own, with the canvas tools, reading the projects
	// the canvas is attached to (SessionInfo.CanvasFor). Older daemons
	// ignore it.
	CanvasFor string `json:"canvasFor,omitempty"`
	// Features are the session's own feature switches, over the user's
	// (FeatureSetParams' session scope). Older daemons ignore them.
	Features map[string]bool `json:"features,omitempty"`
	// Model and Effort start the agent on a model and a reasoning effort
	// (agent.Settings' choices), where its command line takes them.
	// Older daemons ignore them.
	Model   string   `json:"model,omitempty"`
	Effort  string   `json:"effort,omitempty"`
	Command []string `json:"command,omitempty"`
	Dir     string   `json:"dir,omitempty"`
	Env     []string `json:"env,omitempty"`
	Cols    int      `json:"cols,omitempty"`
	Rows    int      `json:"rows,omitempty"`
	// TokenBudgetOK starts an agent although a token budget at its limit
	// asks first (TokenBudget.AtLimit BudgetAsk): the user said yes.
	// Older daemons ignore it.
	TokenBudgetOK bool `json:"tokenBudgetOK,omitempty"`
}

// SessionRef names a session.
type SessionRef struct {
	ID string `json:"id"`
}

// AttachResult names the stream that carries a session's frames.
type AttachResult struct {
	Stream uint32 `json:"stream"`
}

// StreamRef names a stream.
type StreamRef struct {
	Stream uint32 `json:"stream"`
}

// ResizeParams asks the daemon to resize the session behind a stream.
// Cell sizes are in pixels; zero keeps the previous ones.
type ResizeParams struct {
	Stream     uint32 `json:"stream"`
	Cols       int    `json:"cols"`
	Rows       int    `json:"rows"`
	CellWidth  int    `json:"cellWidth,omitempty"`
	CellHeight int    `json:"cellHeight,omitempty"`
}

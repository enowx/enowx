package ui

import (
	"slices"
	"strings"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/proto"
)

// newSessionForm is the state of the New session dialog.
type newSessionForm struct {
	open    bool
	project proto.ProjectDetail
	loaded  bool // project details arrived

	agent         string
	command       string // for the custom agent
	branchMode    string // proto.BranchNew or proto.BranchExisting on git projects
	branch        string // name of the new branch
	base          string
	existing      string
	prompt        string
	display       bool      // run with a virtual display
	runSetup      bool      // run the project's worktree setup (.enowx.toml)
	also          []string  // more agents given the prompt, each on its own branch
	account       string    // the agent's account to sign in as, "" for its own
	accountPicked bool      // picked by the user, not the preferences' default
	creds         []string  // the vault's credentials it starts with
	route         routeForm // the judge's model for the first prompt (newsession_route.go)
	// cont is a session of the project to go on with instead, nil for a
	// new one (newsession_continue.go); convs its agents' past
	// conversations, to pick from.
	cont       *sessionChoice
	contOpen   bool
	contFilter string
	convs      []proto.Conversation

	busy bool
	err  string
}

func (a *App) openNewSession(p proto.ProjectInfo) {
	f := &a.newSession
	prevAgent := f.agent
	*f = newSessionForm{open: true, project: proto.ProjectDetail{ProjectInfo: p}, agent: prevAgent}
	if f.agent == "" || a.state.DefaultAgent != "" {
		f.agent = a.defaultAgent()
	}
	if p.IsGit && !a.state.ProjectFolder {
		f.branchMode = proto.BranchNew
	}
	cl := a.client
	if cl == nil {
		f.open = false
		return
	}
	a.loadConversations(cl, p.Path)
	a.loadRouteOn(cl, p.Path)
	go func() {
		// Agents installed outside enowX since the last look show up.
		if agents, err := cl.Agents(); err == nil {
			a.onMain(func() { a.agents = agents })
		}
		d, err := cl.InspectProject(p.Path)
		a.onMain(func() {
			if !f.open || f.project.Path != p.Path {
				return
			}
			if err != nil {
				f.err = err.Error()
				return
			}
			f.project, f.loaded = d, true
			f.runSetup = d.Setup != ""
			f.base = d.Branch
			if f.base == "" && len(d.Branches) > 0 {
				f.base = d.Branches[0]
			}
			if len(d.Branches) > 0 {
				f.existing = d.Branches[0]
			}
			if !d.IsGit {
				f.branchMode = proto.BranchNone
			}
		})
	}()
}

// defaultAgent is the first installed agent, else the shell.
func (a *App) defaultAgent() string {
	// The one Settings names, while it is installed here.
	if id := a.state.DefaultAgent; id != "" && slices.ContainsFunc(a.agents, func(ag proto.AgentInfo) bool {
		return ag.ID == id && (ag.Installed || ag.Command == "")
	}) {
		return id
	}
	for _, ag := range a.agents {
		if ag.Installed && ag.Command != "" {
			return ag.ID
		}
	}
	return agent.Shell
}

func (a *App) agentInfo(id string) proto.AgentInfo {
	for _, ag := range a.agents {
		if ag.ID == id {
			return ag
		}
	}
	return proto.AgentInfo{ID: id, Name: id}
}

func (a *App) startSession() {
	f := &a.newSession
	params := proto.SessionCreateParams{
		Project:         f.project.Path,
		Agent:           f.agent,
		BranchMode:      f.branchMode,
		Prompt:          f.prompt,
		SkipPermissions: a.skipPermissions,
		Display:         f.display && a.daemonInfo.Display,
		RunSetup:        f.runSetup,
		Account:         f.account,
		Creds:           f.creds,
	}
	f.routeParams(&params)
	if _, isAgent := agent.Find(f.agent); isAgent {
		params.Budget = int64(a.state.DefaultBudget) * 60 // Settings' time budget
	}
	switch f.branchMode {
	case proto.BranchNew:
		params.Branch, params.Base = strings.TrimSpace(f.branch), f.base
		if params.Branch == "" {
			f.err = "Name the new branch."
			return
		}
	case proto.BranchExisting:
		params.Branch = f.existing
	}
	if f.agent == agent.Custom {
		params.Command = strings.Fields(f.command)
		if len(params.Command) == 0 {
			f.err = "Type the command to run."
			return
		}
	}
	cl := a.client
	if cl == nil {
		f.err = "Not connected."
		return
	}
	if len(f.also) > 0 && f.branchMode == proto.BranchNew {
		a.startFanOut(cl, params)
		return
	}
	a.createFromForm(cl, params)
}

// startSignIn starts a session that signs the form's agent in, in the
// project folder; it drops to a shell once done.
func (a *App) startSignIn(ag proto.AgentInfo) {
	f := &a.newSession
	cl := a.client
	if cl == nil {
		f.err = "Not connected."
		return
	}
	a.createFromForm(cl, proto.SessionCreateParams{Project: f.project.Path, Agent: ag.ID, SignIn: true})
}

// createFromForm creates the New session form's session and shows it.
func (a *App) createFromForm(cl *client.Client, params proto.SessionCreateParams) {
	f := &a.newSession
	f.busy, f.err = true, ""
	go func() {
		info, err := cl.CreateSession(params)
		a.onMain(func() {
			f.busy = false
			if err != nil {
				f.err = strings.TrimPrefix(err.Error(), proto.MethodSessionCreate+": ")
				return
			}
			f.open = false
			a.upsert(info)
			a.selectSession(info)
		})
	}()
}

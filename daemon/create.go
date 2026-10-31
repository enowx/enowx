package daemon

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/enowx/enowx/core/accounts"
	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/agentapi"
	"github.com/enowx/enowx/core/project"
	"github.com/enowx/enowx/core/session"
	"github.com/enowx/enowx/core/shells"
	"github.com/enowx/enowx/core/worktree"
	"github.com/enowx/enowx/proto"
)

// How long a typed first prompt waits for the agent to show its input.
const (
	promptQuiet   = 1500 * time.Millisecond
	promptTimeout = 20 * time.Second
	// pasteSettle is how long a pasted prompt shows before Enter sends it.
	pasteSettle = 300 * time.Millisecond
)

// createSession resolves where and what to run for p, then starts it.
func (s *Server) createSession(p proto.SessionCreateParams) (*session.Session, error) {
	return s.createSessionFor(p, nil)
}

// createSessionFor is createSession for member, a team's member visiting,
// or the host's own user (nil): a member's session gets none of the host's
// credentials (membersession.go).
func (s *Server) createSessionFor(p proto.SessionCreateParams, member *proto.SessionMember) (*session.Session, error) {
	meta := session.Meta{Member: member, Name: strings.TrimSpace(p.Name), Prompt: p.Prompt, Project: p.Project, Agent: p.Agent, Group: p.Group, Account: p.Account, NotesFor: p.NotesFor, CanvasFor: p.CanvasFor, Features: p.Features,
		NoAutoHibernate: s.hibernatePrefs().NeverByDefault}
	setup := "" // the project's worktree setup, to run in a tab once started
	dir := p.Dir
	if p.NotesFor != "" {
		// On notes only: a folder of its own, empty, no project or branch.
		if _, err := s.notesFor(p.NotesFor).Get(p.NotesFor); err != nil {
			return nil, err
		}
		dir = filepath.Join(filepath.Dir(s.WorktreeRoot), "notes-chat", p.NotesFor)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, err
		}
		p.Project, p.Parent, p.BranchMode, meta.Project = "", "", proto.BranchNone, ""
		if meta.Name == "" {
			meta.Name = "Notes"
		}
	}
	if p.CanvasFor != "" {
		// On canvases: a folder of its own; the canvas's projects it reads.
		if _, err := s.canvasFor(p.CanvasFor).Get(p.CanvasFor); err != nil {
			return nil, err
		}
		dir = filepath.Join(filepath.Dir(s.WorktreeRoot), "canvas-chat", p.CanvasFor)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, err
		}
		p.Project, p.Parent, p.BranchMode, meta.Project = "", "", proto.BranchNone, ""
		if meta.Name == "" {
			meta.Name = "Canvas"
		}
	}
	// Settle what to run before creating a worktree it would leave behind.
	id := session.NewID()
	guide := s.guideFor(meta)
	if meta.Project == "" && p.NotesFor == "" && p.CanvasFor == "" {
		guide += s.projectGuide(dir) // a folder of no project's, a team's maybe
	}
	chat := s.chatOf(p.NotesFor, p.CanvasFor, guide)
	chat.member = member != nil
	if !p.SignIn {
		chat.store = s.agentStore(p.Agent, id, true)
	}
	if p.Account == "" && p.AccountAuto && member == nil {
		p.Account = s.defaultAccount(p.Agent, p.Project)
		meta.Account = p.Account
	}
	if acc, ok := s.accounts.Get(p.Account); ok {
		if !accounts.Allowed(s.accountPrefs.Of(acc.Agent), acc.Name, p.Project) {
			return nil, fmt.Errorf("accounts: %s is kept to other projects", acc.Name)
		}
		if acc.Cred != "" && !slices.Contains(p.Creds, acc.Cred) {
			p.Creds = append(p.Creds, acc.Cred) // a key: lent as the vault lends
		}
	}
	if _, isAgent := agent.Find(p.Agent); isAgent && !p.SignIn {
		// Its config, applied as it starts (configs.go).
		run, err := s.configRunFor(id, meta)
		if err != nil {
			return nil, err
		}
		chat.config, meta.Config = run, run.sessionInfo()
	}
	launch, err := launchFor(p, chat)
	if err != nil {
		s.configForget(id)
		return nil, err
	}
	if p.SignIn {
		// Signing in has nothing to do with branches: the project folder.
		p.BranchMode, meta.Prompt = proto.BranchNone, ""
	}
	if meta.Name == "" {
		meta.Name = launch.name
	}
	accountEnv, err := s.accounts.Env(agent.Base(p.Agent), p.Account)
	if err != nil {
		s.configForget(id)
		return nil, err
	}
	launch.env = append(append(launch.env, accountEnv...), chat.config.environ()...)

	if p.Parent != "" {
		// A tab: the parent's folder, whatever it is.
		parent, err := s.sessions.Get(p.Parent)
		if err != nil {
			return nil, err
		}
		pm := parent.Meta()
		dir, meta.Parent = parent.Info().Dir, p.Parent
		meta.Project, meta.Branch, meta.Worktree = pm.Project, pm.Branch, pm.Worktree
	} else if p.Project != "" {
		proj := project.Inspect(p.Project)
		dir = proj.Path
		switch p.BranchMode {
		case proto.BranchNone:
			meta.Branch = proj.Branch
		case proto.BranchNew, proto.BranchExisting:
			if !proj.IsGit {
				return nil, fmt.Errorf("%s is not a git repository, so it has no branches", proj.Name)
			}
			branch := strings.TrimSpace(p.Branch)
			if branch == "" {
				return nil, errors.New("branch name is empty")
			}
			meta.Branch = branch
			if p.BranchMode == proto.BranchExisting && branch == proj.Branch {
				break // already checked out in the project folder
			}
			if p.BranchMode == proto.BranchNew && contains(proj.Branches, branch) {
				return nil, fmt.Errorf("branch %s already exists", branch)
			}
			if p.BranchMode == proto.BranchExisting && !contains(proj.Branches, branch) {
				return nil, fmt.Errorf("branch %s does not exist", branch)
			}
			wt, err := worktree.Create(s.WorktreeRoot, proto.WorktreeCreateParams{
				Repo: proj.Path, Name: worktree.FolderName(branch), Branch: branch, Base: p.Base,
			})
			if err != nil {
				return nil, err
			}
			dir, meta.Worktree = wt.Path, true
			if wt.Created {
				setup = s.readyWorktree(proj.Path, wt.Path, p.RunSetup)
			}
		default:
			return nil, fmt.Errorf("unknown branch mode %q", p.BranchMode)
		}
	}

	meta.Restart, meta.Relaunch, meta.SkipPermissions = launch.restart, launch.relaunch, launch.skip
	meta.Budget = time.Duration(p.Budget) * time.Second
	spec := session.Spec{
		ID:      id,
		Command: launch.command, Dir: dir, Env: append(append(slices.Clone(p.Env), launch.env...), s.hostEnvFor(member)...), Cols: p.Cols, Rows: p.Rows, Meta: meta,
	}
	// In a team's project its agent sees the team's notes and canvases.
	if sp, tp, ok := s.teamOfFolder(dir); ok && p.NotesFor == "" && p.CanvasFor == "" {
		spec.Env = append(spec.Env, proto.EnvTeam+"="+sp.Team, proto.EnvTeamProject+"="+remoteRefOf(tp.Repo))
	}
	// A session whose traffic is caught: its new tabs, and its agent when
	// asked (traffic.go).
	if meta.Parent != "" {
		spec.Env = append(spec.Env, s.trafficEnvFor(meta.Parent, "")...)
	}
	if p.Display {
		disp, err := s.displays.start(spec.ID)
		if err != nil {
			return nil, err
		}
		spec.Env = append(spec.Env, disp.Env()...)
		spec.Meta.Display, spec.Meta.DisplayScale = disp.Port, disp.Scale
	}
	if p.NotesFor == "" && p.CanvasFor == "" && !p.SignIn && member == nil {
		// The vault's credentials it asked for, and its helpers.
		env, names := s.vaultSessionEnv(spec.ID, meta.Project, meta.Parent, p.Creds)
		spec.Env, spec.Meta.Creds = append(spec.Env, env...), names
	}
	if member != nil {
		spec.Env = memberEnviron(spec.Env)
	}
	if _, isAgent := agent.Find(p.Agent); isAgent && !p.SignIn && s.mustQueue() {
		return s.enqueue(spec, launch.command, launch.typed)
	}
	sess, err := s.sessions.Create(spec)
	if err != nil {
		s.displays.stop(spec.ID)
		s.vaultRelease(spec.ID)
		s.configForget(spec.ID)
		return nil, err
	}
	s.began(sess, launch.command, launch.typed)
	if _, isAgent := agent.Find(p.Agent); isAgent && !p.SignIn && p.NotesFor == "" && p.CanvasFor == "" {
		s.recordAccount(proto.AccountLogEntry{Session: sess.ID(), Agent: agent.Base(p.Agent), Account: s.accountName(p.Account), Event: "start"})
	}
	if setup != "" {
		s.runSetup(sess, setup)
	}
	return sess, nil
}

// readyWorktree gives a worktree just made from the project at repo what
// its .enowx.toml says: the ignored files to carry over, now; and its
// setup script, returned to run once the session started when runSetup.
func (s *Server) readyWorktree(repo, wt string, runSetup bool) string {
	cfg, err := project.LoadConfig(repo)
	if err != nil {
		s.logger().Warn("could not read the project's settings", "project", repo, "err", err)
		return ""
	}
	if copied, err := cfg.CopyInto(repo, wt); err != nil {
		s.logger().Warn("could not copy files into a new worktree", "worktree", wt, "err", err)
	} else if len(copied) > 0 {
		s.logger().Info("copied into a new worktree", "worktree", wt, "files", strings.Join(copied, ", "))
	}
	if !runSetup {
		return ""
	}
	return cfg.Worktree.Setup
}

// runSetup runs a worktree's setup script in a tab of sess, named Setup,
// which keeps a shell once it ends, so its output stays to read.
func (s *Server) runSetup(sess *session.Session, script string) {
	_, err := s.createSession(proto.SessionCreateParams{
		Parent: sess.ID(), Name: "Setup", Agent: agent.Custom, Command: thenShell(scriptArgv(script)),
	})
	if err != nil {
		s.logger().Warn("could not run the worktree's setup", "session", sess.ID(), "err", err)
	}
}

// hostEnvFor is sessionEnv for the host's user's sessions; a team's
// member's (member) gets none of it: the user's variables are their
// secrets, and the hooks' program reaches the host's daemon.
func (s *Server) hostEnvFor(member *proto.SessionMember) []string {
	if member != nil {
		return nil
	}
	return s.sessionEnv()
}

// sessionEnv is added to every session's environment: the variables the
// user set for this host, and the program the agents' hooks run to report
// their state (agent.EnvBin).
func (s *Server) sessionEnv() []string {
	env := s.vars.env() // the user's variables for this host
	if s.HookBin != "" {
		env = append(env, agent.EnvBin+"="+s.HookBin)
	}
	return env
}

// Bracketed paste marks typed text as pasted, for programs that read it.
const (
	pasteStart = "\x1b[200~"
	pasteEnd   = "\x1b[201~"
)

// launch is how a session's program starts, now and after a restart.
type launch struct {
	command  []string // nil: the login shell
	env      []string // added to the session's environment
	name     string   // the session's name when it was given none
	typed    string   // prompt typed in once the program is ready
	restart  []string // command after a daemon restart
	relaunch bool     // restart the program at all after a daemon restart
	skip     bool     // the agent skips permission prompts
}

func launchFor(p proto.SessionCreateParams, chat chatFor) (launch, error) {
	prompt := strings.TrimSpace(p.Prompt)
	if p.SignIn {
		preset, ok := agent.Find(p.Agent)
		if !ok || len(preset.SignIn) == 0 {
			return launch{}, fmt.Errorf("enowX doesn't know how to sign in to %q", p.Agent)
		}
		return launch{command: thenShell(preset.SignIn), env: preset.SignInEnv, name: "Sign in to " + preset.Name}, nil
	}
	switch p.Agent {
	case "", agent.Custom:
		// A custom command may not be safe to run twice, so it isn't.
		return launch{command: p.Command, typed: prompt}, nil
	case agent.Shell:
		l := launch{typed: prompt, relaunch: true}
		shell := p.Shell
		if shell == "" {
			shell = defaultShell()
		} else if _, ok := shells.Find(shell); !ok {
			return launch{}, fmt.Errorf("%s is not a shell of this host", shell)
		}
		if shell != "" {
			l.command = shells.Argv(shell)
		}
		return l, nil
	}
	preset, ok := agent.Find(p.Agent)
	if !ok {
		return launch{}, fmt.Errorf("unknown agent %q", p.Agent)
	}
	skip := p.SkipPermissions && len(preset.SkipPermissions)+len(preset.SkipEnv) > 0
	if skip && preset.RefusesRootSkip && os.Geteuid() == 0 {
		return launch{}, fmt.Errorf("%s refuses to skip permission prompts as root; reconnect and let enowX run as a regular user (Manage hosts → Ask user again), or turn off Skip permission prompts", preset.Name)
	}
	switch {
	case chat.notes != "":
		if note := agent.NotesNote(preset.ID, chat.notes); note != "" && prompt != "" {
			prompt = note + "\n\n" + prompt
		}
	case chat.canvas != "":
		if note := agent.CanvasNote(preset.ID, chat.canvas); note != "" && prompt != "" {
			prompt = note + "\n\n" + prompt
		}
	default:
		if note := agent.GuideNote(preset.ID); note != "" && prompt != "" {
			prompt += "\n\n" + note
		}
	}
	first := agentLaunch(preset, skip, prompt, false, chat)
	first.Hooks = append(first.Hooks, agent.LaunchArgs(preset.ID, p.Model, p.Effort)...)
	if p.Resume != "" {
		first.Resume = p.Resume
	}
	l := launch{
		command:  thenShell(preset.Argv(first)),
		env:      append(append(append(preset.Environ(skip), chat.apiEnv(preset.ID)...), notesEnv(chat.notes != "")...), canvasEnv(chat.canvas != "")...),
		restart:  thenShell(preset.Argv(agentLaunch(preset, skip, "", true, chat))),
		relaunch: len(preset.Continue) > 0,
		skip:     skip,
	}
	switch {
	case p.Resume != "" && strings.TrimSpace(p.Prompt) != "":
		// A conversation resumed takes no first prompt on its command
		// line: the next message is typed once it is back.
		l.typed = pasteStart + strings.TrimSpace(p.Prompt) + pasteEnd
	case preset.TypePrompt && prompt != "":
		// Pasted, so that its lines don't send it early.
		l.typed = pasteStart + prompt + pasteEnd
	}
	return l, nil
}

// agentStore is the folder session id's agent keeps its conversations in,
// for an agent that takes one (Preset.SessionDir), made when make; "" for
// others, which keep theirs where they do.
func (s *Server) agentStore(agentID, id string, make bool) string {
	preset, ok := agent.Find(agentID)
	if !ok || len(preset.SessionDir) == 0 {
		return ""
	}
	dir := filepath.Join(s.dataDir, "agent-sessions", id)
	if make {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return ""
		}
	}
	return dir
}

// agentLaunch is how this daemon starts preset: with its hooks and guide,
// so a session continued after an upgrade gets the current ones; notes
// starts it as note notes' agent in the Notes page (agent.NotesArgs).
// chatFor is what an agent is for: a session's (its guide, "" for
// the Guide), a note's (the Notes page) or a canvas's (the Canvas page),
// with the folders a canvas's agent reads.
type chatFor struct {
	notes, canvas, guide string
	dirs                 []string
	store                string // the session's conversations' folder (agentStore)
	// member: a team's member's session, which gets nothing of the host's
	// user's model API (agentapi).
	member bool
	// config is the session's config's, nil for the official one.
	config *configRun
}

// apiEnv is apiEnv(id) for the host's user's sessions, nothing for a
// member's, nor for a session whose config is a model API of its own.
func (ch chatFor) apiEnv(id string) []string {
	if ch.member || ch.config != nil && ch.config.api {
		return nil
	}
	return apiEnv(id)
}

// chatOf is the agent of note notes, or of canvas cv, with the folders it
// reads (the canvas's own and its projects'), or a session's with guide.
func (s *Server) chatOf(notes, cv, guide string) chatFor {
	ch := chatFor{notes: notes, canvas: cv, guide: guide}
	if cv != "" {
		if dir, err := s.canvasFor(cv).Dir(cv); err == nil {
			ch.dirs = append(ch.dirs, dir)
		}
		if c, err := s.canvasFor(cv).Get(cv); err == nil {
			ch.dirs = append(ch.dirs, s.projectDirs(c.Projects)...)
		}
	}
	return ch
}

func agentLaunch(preset agent.Preset, skip bool, prompt string, cont bool, chat chatFor) agent.Launch {
	guide := chat.guide
	if guide == "" {
		guide = agent.Guide
	}
	// Skipping stays a mode to switch to from the chat, except as root,
	// where the agent would refuse to start with it.
	allow := !(preset.RefusesRootSkip && os.Geteuid() == 0)
	if home, err := os.UserHomeDir(); err == nil {
		// Agents whose command line takes no guide read it from their home.
		if err := agent.PlaceGuideText(preset.ID, home, guide); err != nil {
			slog.Warn("could not give the agent its guide", "agent", preset.ID, "err", err)
		}
	}
	hooks := append(agent.HookArgs(preset.ID), agent.GuideArgsText(preset.ID, guide)...)
	if agent.Base(preset.ID) == "codex" && len(hooks) > 0 {
		// Codex runs only hooks it trusts: enowX's own are trusted here.
		if err := agent.TrustCodexHooks(""); err != nil {
			slog.Warn("could not trust enowX's hooks in Codex", "err", err)
		}
	}
	switch {
	case chat.notes != "":
		exe, _ := os.Executable()
		hooks = append(agent.HookArgs(preset.ID), agent.NotesArgs(preset.ID, exe, chat.notes)...)
	case chat.canvas != "":
		exe, _ := os.Executable()
		hooks = append(agent.HookArgs(preset.ID), agent.CanvasArgs(preset.ID, exe, chat.canvas, chat.dirs)...)
	}
	if home, err := os.UserHomeDir(); err == nil && !chat.member && (chat.config == nil || !chat.config.api) {
		hooks = append(hooks, agentapi.LaunchArgs(home, preset.ID)...) // its model API's config
	}
	if run := chat.config; run != nil {
		// Its config's (configs.go): Claude Code's settings go into the
		// --settings enowX's hooks use.
		hooks = append(agentapi.WithSettings(hooks, run.settings), run.args...)
	}
	return agent.Launch{Prompt: prompt, SkipPermissions: skip, AllowSkip: allow, Continue: cont, SessionDir: chat.store, Hooks: hooks}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// apiEnv is what agent id's sessions get for the model API it is set to
// (agentapi): its key, for agents that read it from the environment.
func apiEnv(id string) []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return agentapi.LaunchEnv(home, id)
}

// canvasEnv is what a Canvas page agent's session adds to its
// environment: its MCP tools are the canvas's (proto.EnvCanvasChat).
func canvasEnv(canvas bool) []string {
	if !canvas {
		return nil
	}
	return []string{proto.EnvCanvasChat + "=1"}
}

// notesEnv is what a Notes page agent's session adds to its environment:
// its notes are proposed (proto.EnvNotesPropose).
func notesEnv(notes bool) []string {
	if !notes {
		return nil
	}
	return []string{proto.EnvNotesPropose + "=1"}
}

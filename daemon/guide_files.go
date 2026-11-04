package daemon

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/git"
	"github.com/enowx/enowx/core/project"
	"github.com/enowx/enowx/core/session"
)

// dataDir is the data folder, chat.files being a folder of a session under it
// (agentFilesDir).
func (ch chatFor) dataDir() string { return filepath.Dir(filepath.Dir(ch.files)) }

// agentGuide is what session chat's agent id is given as its system prompt: its inject
// with the guide for an agent that loads a file, its inject alone, naming the guide's
// file, for another. Without a folder of its own, the guide as it is.
func (ch chatFor) agentGuide(id, guide string) string {
	if ch.files == "" || ch.notes != "" || ch.canvas != "" {
		return guide
	}
	var path string
	var err error
	if ch.goa {
		path, err = agent.PlaceGOAGuide(filepath.Join(ch.dataDir(), "goa"), id, guide)
	} else {
		path, err = agent.PlaceSessionGuide(ch.files, guide)
		if err == nil && agent.TakesGuideFile(id) {
			err = agent.PlaceSessionFileGuide(ch.files, guideInjected(id, guide))
		}
	}
	if err != nil {
		slog.Warn("could not write the agent's guide", "agent", id, "err", err)
		return guide
	}
	host, _ := os.Hostname()
	in := agent.Inject{Agent: id, Session: filepath.Base(ch.files), Host: host, Guide: path, Loaded: agent.GuideLoaded(id), GOA: ch.goa}
	if in.Loaded {
		return in.Text() + "\n\n" + guide
	}
	return in.Text()
}

// ownGuideFile is the file of the session's that agent id reads by name, "" when it has
// none (agent.TakesGuideFile).
func (ch chatFor) ownGuideFile(id string) string {
	if ch.files == "" || ch.goa || ch.notes != "" || ch.canvas != "" || !agent.TakesGuideFile(id) {
		return ""
	}
	return agent.SessionFileGuide(ch.files)
}

// ownGuideEnv is what brings back a session's agent's environment naming its own guide
// file, in place of the host's its restored environment names; a later value wins.
func (ch chatFor) ownGuideEnv(id string) []string {
	if ch.member {
		return nil
	}
	if file := ch.ownGuideFile(id); file != "" {
		return agent.ToolEnvGuide(id, programPath(), file)
	}
	return nil
}

// guideInjected is what goes in the files of its home an agent loads by itself, whichever
// session it runs in: the rules that must hold, then the guide.
func guideInjected(id, guide string) string {
	return agent.Inject{Agent: id, Loaded: true}.Text() + "\n\n" + guide
}

// writeHostGuide writes this host's guide, what most sessions read.
func (s *Server) writeHostGuide() {
	if err := agent.WriteEnowxGuide(s.dataDir, guideWithoutSecrets(agent.SharedGuide(s.guideFor(session.Meta{})))); err != nil {
		s.logger().Warn("could not write the guide", "err", err)
	}
}

// keepGuideBlock gives an agent with no other way to its guide a block in project dir's
// AGENTS.md or CLAUDE.md, unless the project turned it off. A file it makes is kept out
// of git's way; one of the user's shows the block in a diff, which the docs say.
func (s *Server) keepGuideBlock(id, dir string) {
	if dir == "" || !agent.NeedsProjectBlock(id) {
		return
	}
	cfg, err := project.LoadConfig(dir)
	if err != nil {
		s.logger().Warn("could not read a project's settings", "dir", dir, "err", err)
		return
	}
	if cfg.GuideBlockOff() {
		if err := agent.RemoveProjectBlock(dir); err != nil {
			s.logger().Warn("could not take the guide's block out", "dir", dir, "err", err)
		}
		return
	}
	created, err := agent.KeepProjectBlock(dir)
	if err != nil {
		s.logger().Warn("could not keep the guide's block", "dir", dir, "err", err)
		return
	}
	if created {
		excludeFromGit(dir, "AGENTS.md")
	}
}

// excludeFromGit has git ignore name in dir, in this copy alone.
func excludeFromGit(dir, name string) {
	path, err := git.Run(dir, "rev-parse", "--git-path", "info/exclude")
	if err != nil {
		return // not a repository
	}
	path = strings.TrimSpace(path)
	if !filepath.IsAbs(path) {
		path = filepath.Join(dir, path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	old, _ := os.ReadFile(path)
	if strings.Contains("\n"+string(old), "\n"+name+"\n") {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(name + "\n")
}

package daemon

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/session"
	"github.com/enowx/enowx/core/transcript"
	"github.com/enowx/enowx/proto"
)

// Forking a session: a new one goes on from a compacted copy of its
// conversation. The agent copies the conversation (Preset.Fork) and the
// compaction runs in the copy, so the old session keeps its whole history and
// isn't touched. Both share the folder and branch.

// forkCompactTime is the longest a compaction gets; it asks the agent's model.
const forkCompactTime = 5 * time.Minute

// forkSession starts the fork of p.ID, and returns once it is compacted.
// Should anything fail, the new session goes and the old one stays as it was.
func (s *Server) forkSession(p proto.SessionForkParams) (proto.SessionInfo, error) {
	sess, err := s.sessions.Get(p.ID)
	if err != nil {
		return proto.SessionInfo{}, err
	}
	info, meta := sess.Info(), sess.Meta()
	preset, ok := agent.Find(info.Agent)
	switch {
	case !ok || len(preset.Fork) == 0:
		return proto.SessionInfo{}, fmt.Errorf("fork: not supported for %s yet: Claude Code and Codex only", nonEmpty(info.Agent, "this program"))
	case info.GOA || info.Parent != "" || info.NotesFor != "" || info.CanvasFor != "":
		return proto.SessionInfo{}, errors.New("fork: only a session of its own forks")
	case info.Member != nil:
		return proto.SessionInfo{}, errors.New("fork: this is a team member's session")
	case !info.Exited && info.State == proto.StateWorking:
		return proto.SessionInfo{}, errors.New("fork: its agent is working: fork it once its turn ends")
	}
	convo := transcript.ForkID(info.Agent, s.transcriptPath(p.ID))
	if convo == "" {
		convo = meta.Conversation
	}
	if convo == "" {
		return proto.SessionInfo{}, errors.New("fork: its agent has no conversation to fork yet")
	}
	if info.Exited {
		// Hibernated or ended: back first, as the user decides of a session
		// shown, on the conversation it had.
		if sess, err = s.continueSession(p.ID); err != nil {
			return proto.SessionInfo{}, fmt.Errorf("fork: bringing it back: %w", err)
		}
		if err := s.waitSessionReady(sess.ID()); err != nil {
			return proto.SessionInfo{}, fmt.Errorf("fork: bringing it back: %w", err)
		}
	}
	c := proto.SessionCreateParams{Agent: info.Agent, Project: info.Project, Dir: info.Dir, Name: nonEmpty(info.Name, agentName(info.Agent)) + " (fork)",
		BranchMode: proto.BranchNone, Resume: convo, Fork: true, SkipPermissions: info.SkipPermissions,
		Model: info.Model, Effort: info.Effort, Mode: meta.Mode,
		Account: meta.Account, Creds: meta.Creds, Cols: 120, Rows: 40}
	if info.Worktree {
		c.BranchMode, c.Branch = proto.BranchExisting, info.Branch
	}
	fork, err := s.createSession(c)
	if err != nil {
		return proto.SessionInfo{}, fmt.Errorf("fork: starting it: %w", err)
	}
	if err := s.compactFork(fork, preset, p.Prompt); err != nil {
		s.dropFork(fork)
		return proto.SessionInfo{}, fmt.Errorf("fork: %w; %s is as it was", err, nonEmpty(info.Name, p.ID))
	}
	return fork.Info(), nil
}

// dropFork ends a fork that failed: a session, not a worktree, which it shares.
func (s *Server) dropFork(fork *session.Session) {
	if err := s.deleteSession(fork.ID()); err != nil {
		s.logger().Warn("fork: closing a failed fork", "session", fork.ID(), "err", err)
		return
	}
	s.broadcast(proto.EventSessionRemoved, proto.SessionRef{ID: fork.ID()})
}

// compactFork has session fork compact its copy, then take prompt.
func (s *Server) compactFork(fork *session.Session, preset agent.Preset, prompt string) error {
	id := fork.ID()
	if err := s.waitSessionReady(id); err != nil {
		return fmt.Errorf("it didn't start: %w", err)
	}
	// Enter on anything else answers it: an update offered, a question.
	if err := s.waitPromptShown(fork, preset.ID); err != nil {
		return err
	}
	if err := s.input(proto.SessionInputParams{ID: id, Text: "/compact", Enter: true}); err != nil {
		return err
	}
	start := time.Now()
	for time.Since(start) < forkCompactTime {
		if fork.Info().Exited {
			return errors.New("its agent ended as it compacted")
		}
		if failed := agent.CompactFailed(fork.Terminal().Text()); failed != "" {
			return errors.New("compacting: " + failed)
		}
		if transcript.Compacted(preset.ID, s.transcriptPath(id)) {
			if strings.TrimSpace(prompt) != "" {
				time.Sleep(promptQuiet)
				return s.input(proto.SessionInputParams{ID: id, Text: prompt, Paste: true, Enter: true})
			}
			return nil
		}
		time.Sleep(goaWakePoll)
	}
	return fmt.Errorf("compacting took over %s", forkCompactTime)
}

// waitPromptShown waits for session fork's agent to show its prompt, nothing
// typed until then.
func (s *Server) waitPromptShown(fork *session.Session, agentID string) error {
	for start := time.Now(); time.Since(start) < promptTimeout; time.Sleep(goaWakePoll) {
		if fork.Info().Exited {
			return errors.New("its agent ended as it started")
		}
		if agent.ShowsPrompt(agentID, fork.Terminal().Text()) {
			return nil
		}
	}
	return errors.New("its agent didn't show its prompt, so nothing was typed: it may be asking something, such as to update")
}

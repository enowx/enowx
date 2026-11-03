package daemon

import (
	"os"
	"regexp"
	"time"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/transcript"
	"github.com/enowx/enowx/proto"
)

// What a session's agent really runs on: what it writes in its transcript
// with each answer, so /model, a switch, a fallback and a config all show,
// never what was asked for at the start. An agent with no transcript enowX
// reads says nothing, and the session's model stays unknown.

const (
	modelEvery = 5 * time.Second  // how often the transcripts are looked at
	modelFind  = 30 * time.Second // how often a session's transcript is looked for
)

// modelFollow is what the loop knows of a session's transcript. Only
// watchModels touches it.
type modelFollow struct {
	path   string
	found  time.Time
	stamp  time.Time
	length int64
}

// watchModels keeps sessions' models up to date until done closes.
func (s *Server) watchModels(done <-chan struct{}) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	follows := map[string]*modelFollow{}
	tick := time.NewTicker(modelEvery)
	defer tick.Stop()
	for {
		select {
		case <-done:
			return
		case <-tick.C:
		}
		seen := map[string]bool{}
		for _, sess := range s.sessions.List() {
			info := sess.Info()
			if info.Exited || info.Queued || info.Parent != "" || !transcript.Supports(info.Agent) {
				continue
			}
			seen[info.ID] = true
			f := follows[info.ID]
			if f == nil {
				f = &modelFollow{}
				follows[info.ID] = f
			}
			if model, effort, mode, ok := s.readModel(home, info, f); ok {
				screen := sess.Terminal().Text()
				if effort == "" {
					effort = agentEffortShown(info.Agent, screen)
				}
				if effort == "" {
					effort = info.Effort // nothing says otherwise: it still has what it was given
				}
				if model != info.Model || effort != info.Effort {
					s.sessions.SetModel(sess, model, effort)
				}
				if m := agent.SettingsOf(info.Agent).ModeShown(screen); m != "" {
					mode = m // the screen says at once, the transcript with the next message
				}
				if meta := sess.Meta(); mode != "" && mode != meta.Mode {
					s.sessions.SetMode(sess, mode)
				}
			}
		}
		for id := range follows {
			if !seen[id] {
				delete(follows, id)
			}
		}
	}
}

// readModel is what session info's transcript says now; ok false when nothing
// changed since the last look.
func (s *Server) readModel(home string, info proto.SessionInfo, f *modelFollow) (model, effort, mode string, ok bool) {
	if own := s.transcripts.owned(info.ID); own != "" && own != f.path {
		f.path, f.stamp, f.length = own, time.Time{}, 0
	}
	// Files are found by folder alone, so a newer one may be another session's,
	// as a fork's: only a session at work can have started a new conversation.
	if f.path == "" && info.State != proto.StateDone || info.State == proto.StateWorking && time.Since(f.found) > modelFind {
		f.found = time.Now()
		for _, found := range transcript.FindIn(info.Agent, home, info.Dir, s.agentStore(info.Agent, info.ID, false)) {
			if found.Changed.Before(info.Created.Add(-time.Second)) {
				break
			}
			if found.Path == f.path {
				break
			}
			if s.transcripts.claim(info.ID, found.Path, f.path) {
				f.path, f.stamp, f.length = found.Path, time.Time{}, 0
				break
			}
		}
	}
	if f.path == "" {
		return "", "", "", false
	}
	st, err := os.Stat(f.path)
	if err != nil || st.ModTime().Equal(f.stamp) && st.Size() == f.length {
		return "", "", "", false
	}
	f.stamp, f.length = st.ModTime(), st.Size()
	model, effort, mode = transcript.LastSettings(info.Agent, f.path)
	return model, effort, mode, model != ""
}

var effortShown = regexp.MustCompile(`with (low|medium|high|xhigh|max) effort`)

// agentEffortShown is the effort an agent's screen says, for one whose
// transcript doesn't.
func agentEffortShown(id, screen string) string {
	if agent.Base(id) != "claude" {
		return ""
	}
	if m := effortShown.FindStringSubmatch(screen); m != nil {
		return m[1]
	}
	return ""
}

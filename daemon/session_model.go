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
// reads says nothing, and the session's model stays unknown. What it started on
// is kept apart, so that a move shows as one (SessionInfo.ModelStarted).

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
			if said, ok := s.readModel(home, info, f); ok {
				model, effort, mode := said.Model, said.Effort, said.Mode
				screen := sess.Terminal().Text()
				if effort == "" {
					effort = agentEffortShown(info.Agent, screen)
				}
				if effort == "" {
					effort = info.Effort // nothing says otherwise: it still has what it was given
				}
				meta := sess.Meta()
				started := transcript.ModelStart(meta.ModelStarted, said)
				moved := !transcript.SameModel(started, model)
				if moved && !meta.ModelMoved {
					s.logger().Info("a session's agent runs on another model than it started on", "session", info.ID, "started", started, "says", model)
				}
				if model != info.Model || effort != info.Effort || started != meta.ModelStarted || moved != meta.ModelMoved {
					s.sessions.SetModel(sess, model, effort, started, moved)
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
func (s *Server) readModel(home string, info proto.SessionInfo, f *modelFollow) (said transcript.Settings, ok bool) {
	if own := s.transcripts.owned(info.ID); own != "" && own != f.path {
		f.path, f.stamp, f.length = own, time.Time{}, 0
	}
	// Only a session at work can have started a new conversation.
	if f.path == "" && info.State != proto.StateDone || info.State == proto.StateWorking && time.Since(f.found) > modelFind {
		f.found = time.Now()
		if path := s.ownTranscript(info.ID, info, home, f.path); path != "" && path != f.path {
			f.path, f.stamp, f.length = path, time.Time{}, 0
		}
	}
	if f.path == "" {
		return said, false
	}
	st, err := os.Stat(f.path)
	if err != nil || st.ModTime().Equal(f.stamp) && st.Size() == f.length {
		return said, false
	}
	f.stamp, f.length = st.ModTime(), st.Size()
	said = transcript.LastSettingsSince(info.Agent, f.path, s.floorOf(info.ID, f.path))
	return said, said.Model != ""
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

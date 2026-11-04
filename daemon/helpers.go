package daemon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/session"
	"github.com/enowx/enowx/proto"
)

// Helpers are sessions enowX starts for one piece of work. They end by
// themselves once it is done, so that finished ones don't pile up: what they
// made is where it belongs already (the canvas, the review's result, the note).
// Each host's daemon sees to its own, so one on another host ends the same way.
//
//   - A reviewer and a region's unslop agent: their work has an end the daemon
//     hears of (helperFinished).
//   - A build's direction: it may be sent back once its designs are compared, so
//     it ends only after being left idle.
//   - A Canvas or Notes page's agent: a conversation, so it too ends only after
//     being left idle, and the next message goes on with it (takeHelperConvo).
//
// Not helpers: what the user or GOD of Agents works in, a teammate's session, a
// fan-out, whose results wait to be compared, a pipeline's and an automation's,
// whose result is the session itself, and tabs, which end with their session.

const (
	// helperIdle is how long one sits unused before it ends, its work's end
	// unheard or, on a page, the user perhaps coming back.
	helperIdle = 15 * time.Minute
	// helperGrace is how long a finished one has to end its turn.
	helperGrace = 5 * time.Second
	// helpersEvery is how often they're looked at.
	helpersEvery = time.Minute
	// regionFinishedMax is how long a region's agent that reported done has to settle: an
	// agent whose state enowX can't read may read as at work for good.
	regionFinishedMax = 2 * time.Minute
	// regionMax bounds a region's agent that never reports, a bugged or hung one: its few
	// passes take minutes, and nothing else would ever end it.
	regionMax = 2 * time.Hour
)

// helpersState is what the watch keeps. mu guards finished.
type helpersState struct {
	mu       sync.Mutex
	finished map[string]time.Time // sessions whose work is known to be done, and since when
	convos   sync.Mutex           // the file of conversations to go on with
}

// helperKind says what helper a session is, "" for none: a page's agent
// ("page") or one with a piece of work ("work").
func helperKind(info proto.SessionInfo) string {
	switch {
	case info.GOA || info.GOAWorker != "" || info.Member != nil || info.Parent != "":
		return ""
	case strings.HasPrefix(info.Group, "review:"), strings.HasPrefix(info.Group, proto.UnslopGroup), strings.HasPrefix(info.Group, "build:"):
		return "work"
	case info.CanvasFor != "" || info.NotesFor != "":
		return "page"
	}
	return ""
}

// helperEnds says a helper in this state ends now: never one at work or waiting
// on the user, nor one that may be either, its hooks gone quiet.
func helperEnds(info proto.SessionInfo, finished bool, quiet time.Duration) bool {
	if helperKind(info) == "" {
		return false
	}
	if info.Exited {
		return true
	}
	if info.State == proto.StateWorking || info.State == proto.StateWaiting || info.StateStale || info.Queued {
		return false
	}
	if finished {
		return quiet >= helperGrace
	}
	return quiet >= helperIdle
}

// helperFinished says session id's work is done: it ends once its turn does.
func (s *Server) helperFinished(id string) {
	if id == "" {
		return
	}
	s.helpers.mu.Lock()
	if s.helpers.finished == nil {
		s.helpers.finished = map[string]time.Time{}
	}
	s.helpers.finished[id] = time.Now()
	s.helpers.mu.Unlock()
}

// watchHelpers ends helpers as their work ends. Its first pass is also the once
// over of those left from before this existed.
func (s *Server) watchHelpers(done <-chan struct{}) {
	t := time.NewTicker(helpersEvery)
	defer t.Stop()
	for {
		select {
		case <-done:
			return
		case <-t.C:
			s.helpersPass(time.Now())
		}
	}
}

// helpersPass ends the helpers due to, and says which.
func (s *Server) helpersPass(now time.Time) []string {
	var ended []string
	for _, sess := range s.sessions.List() {
		info := sess.Info()
		if s.goa.board.Has(info.ID) {
			continue // GOA's, whatever else it looks like
		}
		s.helpers.mu.Lock()
		finishedAt, finished := s.helpers.finished[info.ID]
		s.helpers.mu.Unlock()
		region := strings.HasPrefix(info.Group, proto.UnslopGroup)
		if region && !info.Exited && !finished && now.Sub(info.Created) > regionMax {
			s.logger().Warn("helpers: a region's agent ran too long: ending its unslop's", "session", info.ID, "canvas", info.CanvasFor)
			s.endUnslopRegions(info.CanvasFor)
			ended = append(ended, info.ID)
			continue
		}
		overdue := region && finished && now.Sub(finishedAt) > regionFinishedMax
		if !overdue && !helperEnds(info, finished, sess.QuietFor(now)) {
			continue
		}
		if !info.Exited && s.regionsAtWork(info) {
			continue // what it leads goes with it: it ends once they are done
		}
		s.keepHelperConvo(info, sess.Meta())
		if err := s.deleteSession(info.ID); err != nil {
			s.logger().Warn("helpers: end", "session", info.ID, "err", err)
			continue
		}
		s.broadcast(proto.EventSessionRemoved, proto.SessionRef{ID: info.ID})
		s.logger().Info("helpers: ended", "session", info.ID, "kind", helperKind(info))
		s.helpers.mu.Lock()
		delete(s.helpers.finished, info.ID)
		s.helpers.mu.Unlock()
		ended = append(ended, info.ID)
	}
	return ended
}

func (s *Server) helperConvoFile() string { return filepath.Join(s.dataDir, "helper-convos.json") }

// helperConvoKey names a page's conversation with an agent.
func helperConvoKey(canvasFor, notesFor, agentID string) string {
	switch {
	case canvasFor != "":
		return "canvas:" + canvasFor + ":" + agentID
	case notesFor != "":
		return "note:" + notesFor + ":" + agentID
	}
	return ""
}

func (s *Server) readHelperConvos() map[string]string {
	out := map[string]string{}
	if data, err := os.ReadFile(s.helperConvoFile()); err == nil {
		_ = json.Unmarshal(data, &out)
	}
	return out
}

func (s *Server) writeHelperConvos(m map[string]string) {
	if data, err := json.MarshalIndent(m, "", "  "); err == nil {
		if err := os.WriteFile(s.helperConvoFile(), data, 0o600); err != nil {
			s.logger().Warn("helpers: keep a conversation", "err", err)
		}
	}
}

// keepHelperConvo remembers a page agent's conversation as it ends, for the
// next message on that page to go on with.
func (s *Server) keepHelperConvo(info proto.SessionInfo, meta session.Meta) {
	key := helperConvoKey(info.CanvasFor, info.NotesFor, info.Agent)
	if helperKind(info) != "page" || key == "" || meta.Conversation == "" {
		return
	}
	s.helpers.convos.Lock()
	defer s.helpers.convos.Unlock()
	m := s.readHelperConvos()
	m[key] = meta.Conversation
	s.writeHelperConvos(m)
}

// takeHelperConvo is the conversation a new page agent goes on with, "" for
// none, and forgets it. An agent that can't resume by ID starts anew: the page's
// chat log has what was said.
func (s *Server) takeHelperConvo(p proto.SessionCreateParams) string {
	key := helperConvoKey(p.CanvasFor, p.NotesFor, p.Agent)
	if key == "" || p.Group != "" || p.Resume != "" {
		return ""
	}
	s.helpers.convos.Lock()
	defer s.helpers.convos.Unlock()
	m := s.readHelperConvos()
	convo, ok := m[key]
	if !ok {
		return ""
	}
	delete(m, key)
	s.writeHelperConvos(m)
	if preset, found := agent.Find(p.Agent); !found || len(preset.Resume) == 0 {
		return ""
	}
	return convo
}

// forgetHelperConvo drops it: the user asked for a new conversation.
func (s *Server) forgetHelperConvo(canvasFor, notesFor, agentID string) {
	key := helperConvoKey(canvasFor, notesFor, agentID)
	s.helpers.convos.Lock()
	defer s.helpers.convos.Unlock()
	m := s.readHelperConvos()
	if _, ok := m[key]; ok {
		delete(m, key)
		s.writeHelperConvos(m)
	}
}

package daemon

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/decide"
	"github.com/enowx/enowx/core/session"
	"github.com/enowx/enowx/proto"
)

// Changing one session's model or effort must not change the user's other sessions. Claude
// Code's /model and /effort save what they set in the settings file all its sessions read
// (2.1.318: "saved as your default for new sessions"), so they are never typed for an agent
// that does that (Settings.SavesDefaults): it starts again on its conversation with its flags,
// which hold for that process alone. A restart needs a moment when nothing is lost, and says so.

// modelFloor is where session's transcript stood as its model was switched: what it says
// before that is of the model it left.
type modelFloor struct {
	path string
	size int64
}

// floorOf is how much of the transcript at path to pass over for session id.
func (s *Server) floorOf(id, path string) int64 {
	if v, ok := s.modelFloors.Load(id); ok {
		if f := v.(modelFloor); f.path == path {
			return f.size
		}
	}
	return 0
}

// keepModelFloor notes where session id's transcript stands, its agent stopped.
func (s *Server) keepModelFloor(id string) {
	path := s.transcriptPath(id)
	if st, err := os.Stat(path); path != "" && err == nil {
		s.modelFloors.Store(id, modelFloor{path: path, size: st.Size()})
	}
}

// switchSettings puts session p.ID's agent on p.Model and p.Effort, or says why not.
func (s *Server) switchSettings(p proto.SessionSettingsParams) error {
	if p.Model == "" && p.Effort == "" {
		return fmt.Errorf("session %s: a switch to which model or effort?", p.ID)
	}
	sess, err := s.sessions.Get(p.ID)
	if err != nil {
		return err
	}
	info := sess.Info()
	set := agent.SettingsOf(info.Agent)
	if p.Model != "" && set.ModelCommand == "" || p.Effort != "" && set.EffortCommand == "" {
		return fmt.Errorf("%s's model and effort can't be changed while it runs", agentName(info.Agent))
	}
	if !set.SavesDefaults {
		return s.typeSettings(p, info.Agent)
	}
	if strings.ContainsAny(p.Model, " <\n") {
		return fmt.Errorf("%q isn't a model's name", p.Model)
	}
	if p.Effort != "" && p.Effort != "default" && !slices.ContainsFunc(set.Efforts, func(c agent.Choice) bool { return c.ID == p.Effort }) {
		return fmt.Errorf("%s has no effort %q", agentName(info.Agent), p.Effort)
	}
	v, _ := s.modeSwitches.LoadOrStore(p.ID, new(sync.Mutex))
	turn := v.(*sync.Mutex)
	turn.Lock()
	defer turn.Unlock()
	info = sess.Info()
	meta := sess.Meta()
	refused := func(why string) error {
		return fmt.Errorf("session %s stays as it is: %s would save the change for all its sessions, so it changes by starting again on its conversation, and %s", p.ID, agentName(info.Agent), why)
	}
	switch {
	case meta.Config != nil:
		return fmt.Errorf("session %s runs on the config %s, which decides its model and effort: switch its config", p.ID, meta.Config.Name)
	case info.Queued:
		return refused("it hasn't started yet")
	case !comesBack(meta):
		return refused("it can't come back")
	}
	withSettings(&meta, p)
	if info.Exited {
		s.keepModelFloor(info.ID)
		s.sessions.SetModel(sess, meta.Model, meta.Effort, meta.ModelStarted, meta.ModelMoved)
		return nil // it comes back on them
	}
	if atWork(info) {
		return refused("its agent is at work or asking something: try again at its turn's end")
	}
	ports, _ := s.allPorts()
	if s.stoppingLoses("", info.ID, ports) {
		return refused("that would stop what it runs or serves")
	}
	s.procsLook()
	stopped := meta
	stopped.Hibernated = true // for a moment: to GOA's board it hasn't ended, its task hasn't stopped
	if _, err := s.sessions.Replace(stoppedSpec(sess, stopped)); err != nil {
		return err
	}
	s.wakes.undo(info.ID)
	s.reapSession(info.ID)
	s.keepModelFloor(info.ID)
	if _, err := s.wakeSessionIn(info.ID, ""); err != nil {
		return err
	}
	return s.waitSessionReady(info.ID)
}

// withSettings has meta say p's model and effort: given by the user, so no move.
func withSettings(meta *session.Meta, p proto.SessionSettingsParams) {
	if p.Model != "" {
		meta.Model = strings.TrimPrefix(p.Model, "default") // no flag: the agent's own
		meta.ModelStarted, meta.ModelMoved = meta.Model, false
	}
	if p.Effort != "" {
		meta.Effort = strings.TrimPrefix(p.Effort, "default")
	}
}

// typeSettings tells an agent whose commands hold for its session alone (Claude Platform), as
// its user would, bringing it back first if it stopped.
func (s *Server) typeSettings(p proto.SessionSettingsParams, id string) error {
	_, err := s.wakeThen(p.ID, func() error {
		for i, line := range (decide.Route{Model: p.Model, Effort: p.Effort}).Switch(id) {
			if i > 0 {
				time.Sleep(switchPause)
			}
			if err := s.input(proto.SessionInputParams{ID: p.ID, Text: line, Enter: true}); err != nil {
				return err
			}
		}
		return nil
	})
	return err
}

package daemon

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/session"
	"github.com/enowx/enowx/proto"
)

// How the mode a session's agent shows is waited for.
const (
	modeStep    = 50 * time.Millisecond
	modeRedraw  = 2 * time.Second  // after a key, or to read the mode at all
	modeRestart = 30 * time.Second // after starting it again on its conversation
)

// switchMode lands session p.ID's agent on mode p.Mode, or says why not. Its mode key only
// goes round, one step at a time, and Claude Code's goes through plan mode on the way to
// bypass (Settings.ModeReachable), which it has no way to skip: what can't be reached
// without it starts again on its conversation in that mode. Each key waits for the screen
// to say what it did, so a slow redraw never adds a press.
func (s *Server) switchMode(p proto.SessionModeParams) error {
	sess, err := s.sessions.Get(p.ID)
	if err != nil {
		return err
	}
	info := sess.Info()
	set := agent.SettingsOf(info.Agent)
	if set.ModeKey == "" {
		return fmt.Errorf("%s takes its permission mode only as it starts: skipping prompts needs a new session, or a restart with them skipped", agentName(info.Agent))
	}
	choice, ok := agent.FindChoice(set.Modes, p.Mode)
	if !ok {
		return fmt.Errorf("unknown mode %q", p.Mode)
	}
	if info.Exited || info.Queued {
		return fmt.Errorf("session %s isn't running, so its mode stays as it comes back", p.ID)
	}
	v, _ := s.modeSwitches.LoadOrStore(p.ID, new(sync.Mutex))
	turn := v.(*sync.Mutex)
	turn.Lock()
	defer turn.Unlock()
	shown := s.waitMode(sess, set, func(m string) bool { return m != "" }, modeRedraw)
	for range len(set.Modes) + 1 {
		switch {
		case shown == "":
			return fmt.Errorf("session %s's screen doesn't say its mode now (a dialog open?), so no key was pressed: try again", p.ID)
		case shown == choice.ID:
			s.sessions.SetMode(sess, choice.ID)
			return nil
		case agent.AlwaysSkips(info.Agent) && !set.ModeReachable(shown, choice.ID):
			return s.restartInMode(sess, set, choice.ID)
		}
		was := shown
		s.sessions.Input(sess, []byte(set.ModeKey))
		shown = s.waitMode(sess, set, func(m string) bool { return m != was }, modeRedraw)
		if shown == was {
			return fmt.Errorf("session %s stayed in %s: its agent didn't take the key", p.ID, was)
		}
	}
	return fmt.Errorf("session %s's agent didn't reach %s", p.ID, choice.ID)
}

// waitMode is the mode sess's screen shows, once done says it will do or after wait.
func (s *Server) waitMode(sess *session.Session, set agent.Settings, done func(string) bool, wait time.Duration) string {
	end := time.Now().Add(wait)
	for {
		m := set.ModeShown(sess.Terminal().Text())
		if done(m) || time.Now().After(end) {
			return m
		}
		time.Sleep(modeStep)
	}
}

// restartInMode starts sess's agent again on its conversation, in mode. It stops the program,
// so it waits for its agent's turn to end and keeps what stopping would lose.
func (s *Server) restartInMode(sess *session.Session, set agent.Settings, mode string) error {
	info, meta := sess.Info(), sess.Meta()
	preset, ok := agent.Find(info.Agent)
	refused := func(why string) error {
		return fmt.Errorf("session %s can reach %s only by starting again on its conversation, as its mode key would go through plan mode: %s", info.ID, mode, why)
	}
	switch {
	case !ok || !comesBack(meta):
		return refused("it can't come back")
	case mode == agent.ModeBypass && preset.SkipRefused():
		return refused(preset.Name + " refuses bypass as root")
	case atWork(info):
		return refused("its agent is at work or asking something, so try again at its turn's end")
	}
	ports, _ := s.allPorts()
	if s.stoppingLoses("", info.ID, ports) {
		return refused("that would stop what it runs or serves")
	}
	s.procsLook()
	if _, err := s.sessions.Replace(stoppedSpec(sess, meta)); err != nil {
		return err
	}
	s.wakes.undo(info.ID)
	s.reapSession(info.ID)
	once := mode
	if mode == agent.ModeBypass {
		once = "" // what it starts in anyway
	}
	next, err := s.wakeSessionIn(info.ID, once)
	if err != nil {
		return err
	}
	if s.waitMode(next, set, func(m string) bool { return m == mode }, modeRestart) != mode {
		return errors.New("it started again on its conversation, and its screen doesn't say " + mode)
	}
	s.sessions.SetMode(next, mode)
	return nil
}

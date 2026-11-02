package daemon

import (
	"fmt"
	"maps"
	"time"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/proto"
)

// A session GOA sent something to, with no task, reports like a task's: when no hook
// speaks for it, by its screen. Without this it told GOA nothing at all.

// toldQuiet is a told session's status once GOA heard it went quiet.
const toldQuiet = "quiet"

func toldFollow(id string) string { return "told:" + id }

// goaToldNow is the sessions GOA sent something to, none when there is no GOA.
func (s *Server) goaToldNow() map[string]goaTold {
	if _, ok := s.goaSession(); !ok {
		return nil
	}
	s.goa.mu.Lock()
	defer s.goa.mu.Unlock()
	return maps.Clone(s.goa.told)
}

// toldCheck tells what's new of session id, on host h. It has been heard of (Status)
// until GOA sends it something more, so nothing is read of its screen meanwhile.
func (s *Server) toldCheck(w *stallWatch, id string, was goaTold, h stallHost, lim proto.GOAStall) {
	if was.Status == proto.GOATaskTurnEnded || was.Status == proto.GOATaskWaiting {
		return
	}
	info, there := h.infos[id]
	if !there {
		s.goaToldEnd(id, was, "[enowX GOA] Session %s, which you sent something to, isn't on its host any more, and sent no report.")
		return
	}
	if info.Exited {
		if !info.Hibernated {
			s.goaToldChanged(info) // it ended unseen, while the daemon was down
		}
		return
	}
	shell := info.Agent == "" || info.Agent == agent.Shell
	text, err := h.screen(id)
	if err != nil {
		return
	}
	now := time.Now()
	f := w.follows[toldFollow(id)]
	switch {
	case f == nil:
		f = &stallFollow{screen: text, since: now}
		w.follows[toldFollow(id)] = f
	case text != f.screen:
		f.screen, f.since = text, now
		return // quiet stays heard: only what GOA sends it next makes it news again
	}
	look := stallLook{Info: info, NoCPU: !h.cpuKnown, CPU: h.cpuOf(id), Still: now.Sub(f.since)}
	said := proto.GOATask{Session: id, Host: was.Host}
	switch {
	case shell && shellBack(look, text):
		// Its command is over and nothing of it waits: GOA hears once, its turn left alone.
		if s.goaToldStatus(id, was.Status, proto.GOATaskTurnEnded) {
			s.goaTellLine(goaLine{Session: id, Soft: true, Text: fmt.Sprintf("[enowX GOA] Shell session %s, which you sent something to, is back at its prompt: %s", id, clip(oneLine(lastLines(text, 3)), 200))})
		}
	case turnOverOnScreen(look, text):
		if s.goaToldStatus(id, was.Status, proto.GOATaskTurnEnded) {
			s.goaTellAbout(id, fmt.Sprintf("[enowX GOA] Session %s, which you sent something to, finished a turn, as its screen shows: %s", id, clip(oneLine(s.goaResult(said)), goaResultMax)))
		}
	case was.Status != toldQuiet && toldGoneQuiet(look, text, lim):
		if s.goaToldStatus(id, was.Status, toldQuiet) {
			s.goaTellLine(goaLine{Session: id, Soft: true, Text: fmt.Sprintf("[enowX GOA] Session %s, which you sent something to, has shown nothing new for %s, and its screen doesn't say whether its turn ended: %s. Read it (read %s).", id, look.Still.Round(time.Second), clip(oneLine(lastLines(text, 3)), 200), id)})
		}
	}
}

// shellBack says a shell's command is over: still and idle, its prompt the last thing shown.
func shellBack(l stallLook, screen string) bool {
	return l.Still >= turnOverStill && (l.NoCPU || l.CPU < turnOverCPU) && agent.ShellPromptShown(screen)
}

// toldGoneQuiet says a told session has stopped with no sign why, so that GOA hears
// even from an agent whose screen nobody can read. The caller says it once, by Status.
func toldGoneQuiet(l stallLook, screen string, lim proto.GOAStall) bool {
	silent := l.Info.State == "" || l.Info.StateStale || l.Info.State == proto.StateDone
	if _, asks := agent.PromptShown(screen); asks {
		return false
	}
	return silent && l.Still >= stallSeconds(lim.Idle) && (l.NoCPU || l.CPU < turnOverCPU)
}

// goaToldStatus moves told session id from one status to another, unless something
// else moved it first, and says whether it did.
func (s *Server) goaToldStatus(id, from, to string) bool {
	s.goa.mu.Lock()
	t, told := s.goa.told[id]
	moved := told && t.Status == from && from != to
	if moved {
		t.Status = to
		s.goa.told[id] = t
	}
	s.goa.mu.Unlock()
	if moved {
		s.saveGOATold()
	}
	return moved
}

// goaToldEnd stops following session id, as was, telling GOA why.
func (s *Server) goaToldEnd(id string, was goaTold, line string) {
	s.goa.mu.Lock()
	t, told := s.goa.told[id]
	if told && t.Status == was.Status {
		delete(s.goa.told, id)
	} else {
		told = false
	}
	s.goa.mu.Unlock()
	if told {
		s.saveGOATold()
		s.goaTell(fmt.Sprintf(line, id))
	}
}

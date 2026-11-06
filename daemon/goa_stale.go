package daemon

import (
	"fmt"
	"time"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/goa"
	"github.com/enowx/enowx/proto"
)

// goaStaleWork is the task that holds session target's queue though the session sits
// idle (issue #45): its only open task, at work on the board, with the session not at
// work and its screen still past the idle limit. An agent with no hooks says nothing of
// its turns, so its screen's stillness alone decides, over the whole limit; one whose
// hooks said it is done needs only the quiet a queued task waits for. False when the
// session is busy, or still too short a while to tell.
func (s *Server) goaStaleWork(target proto.SessionInfo, now time.Time) (proto.GOATask, bool) {
	if goa.Busy(target) || target.Active || target.Exited || target.Queued || target.FinishedAt == 0 {
		return proto.GOATask{}, false
	}
	var open []proto.GOATask
	for _, t := range s.goa.board.List() {
		if t.Session == target.ID && !goa.Settled(t.Status) && t.Status != proto.GOATaskTurnEnded {
			open = append(open, t)
		}
	}
	if len(open) != 1 || open[0].Status != proto.GOATaskWorking && open[0].Status != proto.GOATaskStalled {
		return proto.GOATask{}, false
	}
	need := stallSeconds(s.goaStall().Idle)
	if target.State == proto.StateDone && agent.ReportsItself(target.Agent) {
		need = queuedQuiet
	}
	if need <= 0 || now.Sub(time.UnixMilli(target.FinishedAt)) < need {
		return proto.GOATask{}, false
	}
	return open[0], true
}

// goaEndStale marks stale task old turn-ended, as its session sits idle, for task next to
// go in, and tells GOA so: it is GOA's to settle.
func (s *Server) goaEndStale(old proto.GOATask, next string, idle time.Duration) {
	_, _ = s.goa.board.Update(old.ID, func(t *proto.GOATask) {
		if t.Status == proto.GOATaskWorking || t.Status == proto.GOATaskStalled {
			t.Status = proto.GOATaskTurnEnded
		}
	})
	s.goaTell(fmt.Sprintf("[enowX GOA] Task %s %q was still open though its session %s has sat idle for %s: it is marked turn-ended, and %s goes in now. Check what it did (chat %s), then settle it: goa task %s --status done|failed --result \"…\".",
		old.ID, old.Title, old.Session, idle.Round(time.Second), next, old.Session, old.ID))
	s.goaChanged()
}

// goaQueueHolder is what holds session id's queue, said so it can be settled.
func (s *Server) goaQueueHolder(id string) string {
	for _, t := range s.goa.board.List() {
		if t.Session == id && (t.Status == proto.GOATaskWorking || t.Status == proto.GOATaskWaiting || t.Status == proto.GOATaskStalled) {
			return fmt.Sprintf(" Queued behind task %s, still open on the board: once it is over, settle it (goa task %s --status done|failed|stopped) and this one goes in.", t.ID, t.ID)
		}
	}
	return " Queued: its session is busy, and gets it when its turn ends."
}

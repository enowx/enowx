package daemon

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/enowx/enowx/core/goa"
	"github.com/enowx/enowx/proto"
)

// A session counts as GOA's when it has a task on the board, or when GOA sent it
// something: session send gives no task, and such a session reports all the same.

// goaTold is what GOA last heard of a session it sent something to.
type goaTold struct {
	Status   string `json:"status,omitempty"`
	ReportAt int64  `json:"reportAt,omitempty"`
	Host     string `json:"host,omitempty"` // "" this one, or from before hosts were kept
}

func goaToldFile(dir string) string { return filepath.Join(dir, "goa-told.json") }

// loadGOATold reads the sessions GOA had sent something to before a restart.
func (s *Server) loadGOATold(dir string) {
	data, err := os.ReadFile(goaToldFile(dir))
	if err != nil {
		return
	}
	told := map[string]goaTold{}
	if err := json.Unmarshal(data, &told); err != nil {
		s.logger().Warn("goa: read the sessions it told", "err", err)
		return
	}
	s.goa.mu.Lock()
	s.goa.told = told
	s.goa.mu.Unlock()
}

// saveGOATold keeps them. It shares the file lock of what waits for GOA.
func (s *Server) saveGOATold() {
	s.goa.tellSave.Lock()
	defer s.goa.tellSave.Unlock()
	s.goa.mu.Lock()
	data, err := json.Marshal(s.goa.told)
	s.goa.mu.Unlock()
	if err == nil {
		err = os.WriteFile(goaToldFile(filepath.Dir(s.goa.tellFile)), data, 0o600)
	}
	if err != nil {
		s.logger().Warn("goa: keep the sessions it told", "err", err)
	}
}

// goaToldIDs lists them, for the board.
func (s *Server) goaToldIDs() []string {
	s.goa.mu.Lock()
	defer s.goa.mu.Unlock()
	ids := make([]string, 0, len(s.goa.told))
	for id := range s.goa.told {
		ids = append(ids, id)
	}
	return ids
}

// goaForgetTold stops following session id as one with no task.
func (s *Server) goaForgetTold(id string) {
	s.goa.mu.Lock()
	_, told := s.goa.told[id]
	delete(s.goa.told, id)
	s.goa.mu.Unlock()
	if told {
		s.saveGOATold()
	}
}

// goaCorrection tells GOA, once, of what a session reported after its last task was
// settled, marked a correction: it asks nothing to be settled, and a report sent before
// the task was settled, unheard until then, says nothing at all.
func (s *Server) goaCorrection(info proto.SessionInfo) {
	if info.ReportAt == 0 {
		return
	}
	var last *proto.GOATask
	tasks := s.goa.board.List()
	for i := range tasks {
		if tasks[i].Session == info.ID && (last == nil || tasks[i].Created.After(last.Created)) {
			last = &tasks[i]
		}
	}
	if last == nil || !goa.Settled(last.Status) || info.ReportAt <= last.ReportAt || info.ReportAt <= last.Updated.UnixMilli() {
		return
	}
	moved := false
	if _, err := s.goa.board.Update(last.ID, func(t *proto.GOATask) {
		moved = info.ReportAt > t.ReportAt
		t.ReportAt = max(t.ReportAt, info.ReportAt)
	}); err != nil || !moved {
		return
	}
	s.goaTell(fmt.Sprintf("[enowX GOA] Correction to settled task %s %q: session %s reported %s after it was settled: %s", last.ID, last.Title, info.ID, info.ReportOutcome, clip(oneLine(info.Report), goaResultMax)))
}

// goaCallTold follows input a session sends through host.call, to any host.
func (s *Server) goaCallTold(p proto.HostCallParams) {
	if p.Method != proto.MethodSessionInput || p.From == "" {
		return
	}
	var in proto.SessionInputParams
	if json.Unmarshal(p.Params, &in) == nil {
		s.goaMarkTold(p.From, p.Host, in.ID)
	}
}

// goaMarkTold follows what GOA (from) types into session id. One with a task whose
// turn had ended is at work again, for a screen with no hook to tell of the next
// turn too; one with no task now reports to GOA. Anyone else's input means nothing.
func (s *Server) goaMarkTold(from, host, id string) {
	g, ok := s.goaSession()
	if !ok || from != g.ID || id == "" || id == g.ID {
		return
	}
	if s.goa.board.Has(id) {
		changed := s.goa.board.Of(id, func(t *proto.GOATask) bool {
			if t.Status != proto.GOATaskTurnEnded {
				return false
			}
			t.Status = proto.GOATaskWorking
			return true
		})
		if len(changed) > 0 {
			s.goaChanged()
		}
		return
	}
	s.goa.mu.Lock()
	t := s.goa.told[id]
	t.Status, t.Host = "", host                          // what it does now is news, whatever it did before
	t.ReportAt = max(t.ReportAt, time.Now().UnixMilli()) // and what it reported before isn't
	s.goa.told[id] = t
	s.goa.mu.Unlock()
	s.saveGOATold()
	s.goaChanged()
}

// goaToldChanged tells GOA of a session it sent something to, with no task, as it
// is told of a worker: its report, its turn's end, what it asks, its end.
func (s *Server) goaToldChanged(info proto.SessionInfo) {
	s.goa.mu.Lock()
	was, told := s.goa.told[info.ID]
	if !told {
		s.goa.mu.Unlock()
		s.goaCorrection(info)
		return
	}
	status := goa.WorkerStatus(info)
	news := told && (info.ReportAt > was.ReportAt || status != "" && status != proto.GOATaskWorking && status != was.Status)
	ended := status == proto.GOATaskFailed || status == proto.GOATaskStopped
	if news {
		now := was
		now.ReportAt = max(was.ReportAt, info.ReportAt)
		if status != "" {
			now.Status = status
		}
		s.goa.told[info.ID] = now
		if ended {
			delete(s.goa.told, info.ID) // nothing more to come of it
		}
	}
	s.goa.mu.Unlock()
	if !news {
		return
	}
	s.saveGOATold()
	t := proto.GOATask{Session: info.ID}
	if info.ReportAt > was.ReportAt {
		s.goaTell(fmt.Sprintf("[enowX GOA] Session %s, which you sent something to, reports %s: %s", info.ID, info.ReportOutcome, clip(oneLine(info.Report), goaResultMax)))
		if status == was.Status || status == proto.GOATaskTurnEnded {
			return // that was its turn's end
		}
	}
	switch status {
	case proto.GOATaskTurnEnded:
		s.goaTell(fmt.Sprintf("[enowX GOA] Session %s, which you sent something to, finished a turn: %s", info.ID, clip(oneLine(s.goaResult(t)), goaResultMax)))
	case proto.GOATaskWaiting:
		s.goaTell(fmt.Sprintf("[enowX GOA] Session %s, which you sent something to, asks something: %s", info.ID, clip(oneLine(s.goaResult(t)), goaResultMax)))
	case proto.GOATaskFailed, proto.GOATaskStopped:
		s.goaTell(fmt.Sprintf("[enowX GOA] Session %s, which you sent something to, ended with exit code %d: %s", info.ID, info.ExitCode, clip(oneLine(s.goaResult(t)), goaResultMax)))
	}
}

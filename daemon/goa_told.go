package daemon

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/enowx/enowx/core/goa"
	"github.com/enowx/enowx/proto"
)

// A session counts as GOA's when it has a task on the board, or when GOA sent it
// something: session send gives no task, and such a session reports all the same.

// goaTold is what GOA last heard of a session it sent something to.
type goaTold struct {
	Status   string `json:"status,omitempty"`
	ReportAt int64  `json:"reportAt,omitempty"`
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

// goaCallTold follows input a session sends through host.call, to any host.
func (s *Server) goaCallTold(p proto.HostCallParams) {
	if p.Method != proto.MethodSessionInput || p.From == "" {
		return
	}
	var in proto.SessionInputParams
	if json.Unmarshal(p.Params, &in) == nil {
		s.goaMarkTold(p.From, in.ID)
	}
}

// goaMarkTold follows what GOA (from) types into session id. One with a task whose
// turn had ended is at work again, for a screen with no hook to tell of the next
// turn too; one with no task now reports to GOA. Anyone else's input means nothing.
func (s *Server) goaMarkTold(from, id string) {
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
	t.Status = "" // what it does now is news, whatever it did before
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

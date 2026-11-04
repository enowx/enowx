package daemon

import (
	"slices"
	"strings"
	"time"

	"github.com/enowx/enowx/proto"
)

// A worker's turn says two things of itself: its agent's goa report, a tool call made
// before the turn is over, and the turn's end. GOA gets them as one line. A report made
// at work waits for its turn's end, for a while, and the end joins it; a turn's end still
// waiting for GOA takes a report that comes after it. Lines never change places, a
// question or a failure is never merged, and a later turn has its own line.

const (
	goaLineReport = "report"
	goaLineTurn   = "turn"
	// goaReportHold is how long a report waits for its turn's end: past it, it goes alone,
	// as a turn may go on long after, or never be heard to end.
	goaReportHold = 30 * time.Second
)

// releaseHeld lets what lines hold back of session go. The caller holds goa.mu.
func releaseHeld(lines []goaLine, session string) {
	for i := range lines {
		if lines[i].Session == session {
			lines[i].HoldUntil = time.Time{}
		}
	}
}

// goaTellReport tells of a report of session info, as line: joined to its turn's end if
// that waits for GOA, else held for it if the session is at work.
func (s *Server) goaTellReport(info proto.SessionInfo, line goaLine) {
	line.Turn = goaLineReport
	s.goa.mu.Lock()
	i := s.goaOpenLine(line, goaLineTurn)
	if i >= 0 {
		said := s.goa.tell[i].Text
		s.goa.tell[i].Text, s.goa.tell[i].Turn, s.goa.tell[i].Closed = line.Text+lastMessageOf(said), goaLineTurn, true
	}
	s.goa.mu.Unlock()
	if i >= 0 {
		s.goaLinesChanged()
		return
	}
	if info.State == proto.StateWorking && !info.StateStale {
		line.HoldUntil = time.Now().Add(goaReportHold)
	} else {
		line.Closed = true
	}
	s.goaTellLine(line)
}

// goaTellTurnEnd tells of a turn's end, as line, saying said last: into its report if
// that waits for GOA. A report already gone leaves it a line of its own.
func (s *Server) goaTellTurnEnd(line goaLine, said string) {
	line.Turn = goaLineTurn
	s.goa.mu.Lock()
	i := s.goaOpenLine(line, goaLineReport)
	if i >= 0 {
		l := &s.goa.tell[i]
		l.HoldUntil, l.Closed = time.Time{}, true
		if text := strings.TrimSpace(said); text != "" && !strings.Contains(oneLine(l.Text), oneLine(text)) {
			l.Text += turnEndMark + oneLine(text)
		}
	}
	s.goa.mu.Unlock()
	if i >= 0 {
		s.goaLinesChanged()
		return
	}
	s.goaTellLine(line)
}

// turnEndMark comes before the turn's last message, in a line that holds its report.
const turnEndMark = " Its turn ended on: "

// lastMessageOf is a turn's end line as what follows its report in one line.
func lastMessageOf(line string) string {
	if _, said, ok := strings.Cut(line, ": "); ok {
		return turnEndMark + said
	}
	return ""
}

// goaOpenLine is the last line waiting for GOA of line's session and task, of kind turn,
// still open to joining; -1 for none. The caller holds goa.mu.
func (s *Server) goaOpenLine(line goaLine, turn string) int {
	return slices.IndexFunc(s.goa.tell, func(l goaLine) bool {
		return l.Turn == turn && !l.Closed && l.Session == line.Session && l.Task == line.Task
	})
}

// goaTurnStarted closes what waits of session's turns: what comes now is of another.
func (s *Server) goaTurnStarted(session string) {
	s.goa.mu.Lock()
	changed := false
	for i := range s.goa.tell {
		if l := &s.goa.tell[i]; l.Session == session && l.Turn == goaLineTurn && !l.Closed {
			l.Closed, changed = true, true
		}
	}
	s.goa.mu.Unlock()
	if changed {
		s.saveGOATell()
	}
}

// goaLinesChanged saves what waits, and gives it to an idle GOA.
func (s *Server) goaLinesChanged() {
	s.saveGOATell()
	if g, ok := s.goaSession(); ok && !g.Exited && g.State != proto.StateWorking {
		s.goaFlush(g.ID)
	}
	s.goaChanged()
}

// How long a turn's last message may take to reach its transcript, for an agent whose
// hook doesn't say it. Claude Code's transcript was seen behind its Stop hook by over a
// second (2.1.318).
const (
	turnEndWait  = 6 * time.Second
	turnEndEvery = 400 * time.Millisecond
)

// turnEndIn is the message transcript entries end on, once it holds the turn's end: the
// last there is but thinking and notices. What follows a tool's call or a prompt isn't
// written yet, and a message before them isn't the turn's last.
func turnEndIn(entries []proto.TranscriptEntry) (string, bool) {
	for i := len(entries) - 1; i >= 0; i-- {
		switch e := entries[i]; e.Kind {
		case proto.TranscriptThinking, proto.TranscriptNotice:
		case proto.TranscriptAssistant:
			if text := strings.TrimSpace(e.Text); text != "" {
				return text, true
			}
		default:
			return "", false
		}
	}
	return "", false
}

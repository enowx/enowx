package daemon

import "slices"

// A line may keep GOA's turn going at a Stop only once for what it is about, until GOA
// reads that session or sends it something: however many more come meanwhile, they wait
// for the next tool call or its idle moment, so no line left unanswered holds its turn.

// source is what line is about.
func (l goaLine) source() string {
	switch {
	case l.Session != "":
		return l.Session
	case l.Task != "":
		return l.Task
	}
	return l.Text
}

// goaTellAbout tells GOA line, about session id.
func (s *Server) goaTellAbout(id, line string) {
	s.goaTellLine(goaLine{Session: id, Text: line})
}

// goaTakeAtStop takes what may keep GOA's turn going, the rest staying for its next way in.
func (s *Server) goaTakeAtStop() string {
	s.goa.delivery.Lock()
	defer s.goa.delivery.Unlock()
	lines := s.goaLinesToSay()
	var may, wait []goaLine
	s.goa.mu.Lock()
	for _, l := range lines {
		if l.Soft || s.goa.stops[l.source()] {
			wait = append(wait, l)
		} else {
			may = append(may, l)
		}
	}
	s.goa.tell = uniqueGOALines(append(wait, s.goa.tell...))
	s.goa.mu.Unlock()
	text, n := s.goaTakeOf(may)
	s.goa.mu.Lock()
	defer s.goa.mu.Unlock()
	if s.goa.stops == nil || len(s.goa.stops) > goaTellMax {
		s.goa.stops = map[string]bool{}
	}
	for _, l := range may[:n] {
		s.goa.stops[l.source()] = true
	}
	return text
}

// goaHeardOf is GOA (from) having looked at session id, or sent it something: what only
// said to look is moot, and a new line about it may keep a turn going again.
func (s *Server) goaHeardOf(from, id string) {
	g, ok := s.goaSession()
	if !ok || from != g.ID || id == "" || id == g.ID {
		return
	}
	s.goa.mu.Lock()
	before := len(s.goa.tell)
	s.goa.tell = slices.DeleteFunc(s.goa.tell, func(l goaLine) bool { return l.Soft && l.Session == id })
	gone := len(s.goa.tell) != before
	s.goa.mu.Unlock()
	s.goaStopsClear(id)
	if gone {
		s.saveGOATell()
		s.goaChanged()
	}
}

// goaStopsClear lets a new line about session id keep a turn going again.
func (s *Server) goaStopsClear(id string) {
	s.goa.mu.Lock()
	delete(s.goa.stops, id)
	s.goa.mu.Unlock()
}

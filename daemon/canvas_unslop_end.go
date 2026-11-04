package daemon

import (
	"slices"
	"strings"

	"github.com/enowx/enowx/proto"
)

// An unslop's regions' agents are sessions of their own, so nothing but their own report ended
// them: stopping or closing the redesign's agent, or a stuck run, left up to six agents running
// with no control in reach. They end with the run: when the user stops it, when its agent is
// closed or stopped by hand, when it is done, and when a new run replaces it.

// unslopRegionSessions are the running sessions of canvas's regions' agents.
func (s *Server) unslopRegionSessions(canvas string) []proto.SessionInfo {
	var out []proto.SessionInfo
	for _, sess := range s.sessions.List() {
		if info := sess.Info(); info.CanvasFor == canvas && strings.HasPrefix(info.Group, proto.UnslopGroup) && !info.Exited {
			out = append(out, info)
		}
	}
	return out
}

// endUnslopRegions ends canvas's regions' agents, with all they started.
func (s *Server) endUnslopRegions(canvas string) {
	// Off the run first, so none of them can change the design from here on.
	if run, ok := s.unslopRuns.dropRegions(canvas); ok {
		s.broadcast(proto.EventCanvasUnslop, run)
	}
	for _, info := range s.unslopRegionSessions(canvas) {
		if err := s.deleteSession(info.ID); err != nil {
			s.logger().Warn("canvas: end a region's agent", "session", info.ID, "err", err)
			continue
		}
		s.broadcast(proto.EventSessionRemoved, proto.SessionRef{ID: info.ID})
	}
}

// endRegionsOfLead ends the regions' agents session lead fanned out, which has ended.
func (s *Server) endRegionsOfLead(lead proto.SessionInfo) {
	if canvas := s.ledCanvas(lead); canvas != "" {
		s.endUnslopRegions(canvas)
	}
}

// regionsAtWork says session lead has regions' agents still running.
func (s *Server) regionsAtWork(lead proto.SessionInfo) bool {
	canvas := s.ledCanvas(lead)
	return canvas != "" && len(s.unslopRegionSessions(canvas)) > 0
}

// ledCanvas is the canvas whose regions session lead leads, "" for none. A run is kept in memory,
// so after a daemon's restart its regions go by the canvas's agent they were started as.
func (s *Server) ledCanvas(lead proto.SessionInfo) string {
	if lead.ID == "" || strings.HasPrefix(lead.Group, proto.UnslopGroup) {
		return ""
	}
	if canvas := s.unslopRuns.ledBy(lead.ID); canvas != "" {
		return canvas
	}
	if lead.CanvasFor == "" {
		return ""
	}
	if run, ok := s.unslopRuns.status(lead.CanvasFor); ok && run.Lead != "" {
		return "" // another session leads it
	}
	if slices.ContainsFunc(s.unslopRegionSessions(lead.CanvasFor), func(r proto.SessionInfo) bool { return r.Agent == lead.Agent }) {
		return lead.CanvasFor
	}
	return ""
}

// ledBy is the canvas whose run session leads, "" for none.
func (u *unslopRuns) ledBy(session string) string {
	u.mu.Lock()
	defer u.mu.Unlock()
	for id, run := range u.runs {
		if run.Lead == session {
			return id
		}
	}
	return ""
}

// dropRegions takes the regions not done off id's run, as their agents are gone; false when
// there is no run or none to take.
func (u *unslopRuns) dropRegions(id string) (proto.CanvasUnslopRun, bool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	run := u.runs[id]
	if run == nil {
		return proto.CanvasUnslopRun{}, false
	}
	kept := slices.DeleteFunc(slices.Clone(run.Regions), func(r proto.UnslopRegion) bool { return !r.Done })
	if len(kept) == len(run.Regions) {
		return proto.CanvasUnslopRun{}, false
	}
	run.Regions = kept
	return copyRun(run), true
}

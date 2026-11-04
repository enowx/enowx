package daemon

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/enowx/enowx/core/canvas"
	"github.com/enowx/enowx/core/unslop"
	"github.com/enowx/enowx/proto"
)

// An unslop's later passes, fanned out: the redesign's agent, once the
// foundation (truth, structure, spacing, type, color) is set, names the
// design's parts that don't hold each other, and an agent starts on each,
// the same agent, as canvas agents start: the canvas's tools alone, no
// project's MCP servers to approve. Each may change only its part (its
// element and all in it), so they never write over each other. Each
// reports its passes; when the last is done, the redesign's agent is told
// to check the whole.

const maxRegions = 6

// regionPasses are the passes a region's agent does.
var regionPasses = []string{"06 Surfaces", "07 Icons", "08 Copy", "09 Data", "10 Motion"}

// regionID is a region as given: "#sidebar" or "sidebar".
func regionID(ref string) string { return strings.TrimPrefix(strings.TrimSpace(ref), "#") }

// inRegion says element id of page is region or inside it.
func inRegion(page, id, region string) bool {
	for n := 0; id != "" && n < 256; n++ {
		if id == region {
			return true
		}
		id = canvas.ParentID(page, id)
	}
	return false
}

// fanOutRegions starts an agent on each of regions of design f, for the run of
// canvas cv, as tabs of session lead.
func (s *Server) fanOutRegions(cv proto.Canvas, f proto.CanvasFrame, n int, regions []string, lead string) error {
	if len(regions) == 0 || len(regions) > maxRegions {
		return fmt.Errorf("canvas: fan out to 1 to %d regions", maxRegions)
	}
	var ids []string
	for _, r := range regions {
		id := regionID(r)
		if _, err := canvas.ElementHTML(f.HTML, id); err != nil || id == "" {
			return fmt.Errorf("canvas: no element #%s in design %d to give an agent", id, n)
		}
		if slices.Contains(ids, id) {
			continue
		}
		ids = append(ids, id)
	}
	for _, a := range ids {
		for _, b := range ids {
			if a != b && inRegion(f.HTML, a, b) {
				return fmt.Errorf("canvas: #%s is inside #%s: regions mustn't hold each other", a, b)
			}
		}
	}
	agentID := "claude"
	if sess, err := s.sessions.Get(lead); err == nil {
		agentID = sess.Meta().Agent
	} else {
		lead = ""
	}
	theme := s.unslopRuns.theme(cv.ID, f.ID)
	var started []proto.UnslopRegion
	for _, id := range ids {
		// A canvas's agent, grouped apart from its conversation's.
		p := proto.SessionCreateParams{Agent: agentID, CanvasFor: cv.ID, Group: proto.UnslopGroup + f.ID, Name: "Unslop #" + id, Cols: 100, Rows: 40,
			Prompt: regionAsk(cv, f, n, id, theme)}
		sess, err := s.createSession(p)
		if err != nil {
			return fmt.Errorf("canvas: start an agent on #%s: %w", id, err)
		}
		started = append(started, proto.UnslopRegion{ID: id, Session: sess.ID(), Agent: agentID})
	}
	run, ok := s.unslopRuns.addRegions(cv.ID, f.ID, started, lead)
	if !ok {
		return errors.New("canvas: no unslop under way on this design: start one first")
	}
	s.broadcast(proto.EventCanvasUnslop, run)
	return nil
}

// regionAsk is what a region's agent is told, in its canvas tools.
func regionAsk(cv proto.Canvas, f proto.CanvasFrame, n int, id, theme string) string {
	at := fmt.Sprintf(`canvas %q, frame %q`, cv.ID, f.ID)
	keep := "Keep its theme and brand colors."
	if theme != "" {
		keep = "It is a " + theme + " design and stays " + theme + "; keep its brand colors."
	}
	return fmt.Sprintf(`You are one of several agents unslopping design %d (“%s”) at once, the user watching live. Your part: the element #%s and everything in it. Change nothing else: changes outside it are refused.
The foundation is set (truth, structure, spacing, type, color): use its tokens (CSS variables, the spacing and type scales) and change none of them. %s Never invent data.
1. Read the design with canvas_show (%s, html true), and what's off with canvas_unslop (%s, region %q).
2. Do these passes in your part, one at a time: %s. Begin each with canvas_unslop (%s, region %q, pass N, its number).
3. Change it element by element with canvas_patch (%s, element …), so each change shows; CSS your part needs goes in a <style> element inside #%s (canvas_patch into %q), its selectors starting with #%s.
4. When your part is done: canvas_unslop (%s, region %q, pass "done"). The redesign's agent is told when all parts are; then stop.`,
		n, f.Name, id, keep, at, at, id, strings.Join(regionPasses, ", "), at, id, at, id, id, id, at, id)
}

// addRegions records regions started on the run of design frame of canvas
// id.
func (u *unslopRuns) addRegions(id, frame string, regions []proto.UnslopRegion, lead string) (proto.CanvasUnslopRun, bool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	run := u.runs[id]
	if run == nil || run.Frame != frame {
		return proto.CanvasUnslopRun{}, false
	}
	run.Regions, run.Lead = append(run.Regions, regions...), lead
	// The redesign's agent's pass is done; the regions' are under way.
	if run.Pass != "" && !slices.Contains(run.Finished, run.Pass) {
		run.Finished = append(run.Finished, run.Pass)
	}
	run.Pass = ""
	return copyRun(run), true
}

// regionReport records region's agent beginning pass ("done" when done).
func (u *unslopRuns) regionReport(id, frame, region, pass string) (proto.CanvasUnslopRun, error) {
	name := ""
	done := strings.EqualFold(strings.TrimSpace(pass), "done")
	if pass != "" && !done {
		var ok bool
		if name, ok = unslop.PassNamed(pass); !ok {
			return proto.CanvasUnslopRun{}, fmt.Errorf("canvas: no pass %q", pass)
		}
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	run := u.runs[id]
	if run == nil || run.Frame != frame {
		return proto.CanvasUnslopRun{}, errors.New("canvas: no unslop under way on this design")
	}
	i := slices.IndexFunc(run.Regions, func(r proto.UnslopRegion) bool { return r.ID == regionID(region) })
	if i < 0 {
		return proto.CanvasUnslopRun{}, fmt.Errorf("canvas: no region #%s in this unslop", regionID(region))
	}
	switch {
	case done:
		run.Regions[i].Pass, run.Regions[i].Done = "", true
	case name != "":
		run.Regions[i].Pass = name
	}
	return copyRun(run), nil
}

// allDone says every region of run is done.
func allDone(run proto.CanvasUnslopRun) bool {
	if len(run.Regions) == 0 {
		return false
	}
	for _, r := range run.Regions {
		if !r.Done {
			return false
		}
	}
	return true
}

// tellLead tells the redesign's agent its regions are all done, to go on.
func (s *Server) tellLead(run proto.CanvasUnslopRun, cv proto.Canvas) {
	if run.Lead == "" {
		return
	}
	n := slices.IndexFunc(cv.Frames, func(f proto.CanvasFrame) bool { return f.ID == run.Frame }) + 1
	at := fmt.Sprintf(`canvas %q, frame %q`, cv.ID, run.Frame)
	text := fmt.Sprintf(`All %d region agents are done; design %d scores %d/100. Go on: passes 11 (access) and 12 (mobile) on the whole design, each begun with canvas_unslop (%s, pass N); then canvas_unslop (%s, pass "done"), and show the original and design %d side by side (canvas_compare).`,
		len(run.Regions), n, run.Score, at, at, n)
	s.tellSession(proto.SessionInputParams{ID: run.Lead, Text: text, Paste: true, Enter: true}, "canvas: tell the unslop's agent its regions are done")
}

// lease is the region session may change on canvas id, when it is a
// region's agent.
func (u *unslopRuns) lease(id, session string) (frame, region string, ok bool) {
	if session == "" {
		return "", "", false
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	run := u.runs[id]
	if run == nil {
		return "", "", false
	}
	for _, r := range run.Regions {
		if r.Session == session && !r.Done {
			return run.Frame, r.ID, true
		}
	}
	return "", "", false
}

// status is the run of canvas id, if any.
func (u *unslopRuns) status(id string) (proto.CanvasUnslopRun, bool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if run := u.runs[id]; run != nil {
		return copyRun(run), true
	}
	return proto.CanvasUnslopRun{}, false
}

// checkLease refuses an edit of a region's agent outside its region.
func (s *Server) checkLease(e proto.CanvasEdit) error {
	frame, region, ok := s.unslopRuns.lease(e.ID, e.Session)
	if !ok {
		return nil
	}
	refuse := fmt.Errorf("canvas: you redesign #%s of design %s only: change elements in it (--frame %s), nothing else", region, frame, frame)
	cv, err := s.canvasFor(e.ID).Get(e.ID)
	if err != nil {
		return err
	}
	i, err := canvas.FrameIndex(cv, e.Frame)
	if err != nil || cv.Frames[i].ID != frame {
		return refuse
	}
	page := cv.Frames[i].HTML
	switch e.Op {
	case proto.CanvasOpHTMLPatch:
		// A new element goes into Into, in the region; one there is in it.
		if e.Into != "" && !inRegion(page, e.Into, region) {
			return refuse
		}
		if _, err := canvas.ElementHTML(page, e.Target); err == nil && !inRegion(page, e.Target, region) {
			return refuse
		}
		if e.Into == "" {
			if _, err := canvas.ElementHTML(page, e.Target); err != nil {
				return refuse // added where? Outside, by default
			}
		}
	case proto.CanvasOpHTMLText, proto.CanvasOpHTMLRemove:
		if e.Target == region && e.Op == proto.CanvasOpHTMLRemove || !inRegion(page, e.Target, region) {
			return refuse
		}
	default:
		return refuse
	}
	return nil
}

// copyRun is run as told: its slices its own.
func copyRun(run *proto.CanvasUnslopRun) proto.CanvasUnslopRun {
	out := *run
	out.Finished = slices.Clone(run.Finished)
	out.Regions = slices.Clone(run.Regions)
	out.Passed = slices.Clone(run.Passed)
	return out
}

// setBranch records the branch a run of id is made on, from base.
func (u *unslopRuns) setBranch(id, branch, base string) proto.CanvasUnslopRun {
	u.mu.Lock()
	defer u.mu.Unlock()
	run := u.runs[id]
	if run == nil {
		return proto.CanvasUnslopRun{}
	}
	run.Branch, run.Base = branch, base
	return copyRun(run)
}

// begin starts a run of id about frame that does steps before its first
// read.
func (u *unslopRuns) begin(id, frame string, steps []string) proto.CanvasUnslopRun {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.runs == nil {
		u.runs = map[string]*proto.CanvasUnslopRun{}
	}
	run := &proto.CanvasUnslopRun{ID: id, Frame: frame, Passes: unslop.Names(), Steps: steps}
	u.runs[id] = run
	delete(u.layouts, id)
	delete(u.shots, id)
	return copyRun(run)
}

// step marks step i of id's run under way.
func (u *unslopRuns) step(id string, i int) (proto.CanvasUnslopRun, bool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	run := u.runs[id]
	if run == nil {
		return proto.CanvasUnslopRun{}, false
	}
	run.Step = i
	return copyRun(run), true
}

// drop forgets id's run, which didn't start.
func (u *unslopRuns) drop(id string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	delete(u.runs, id)
	delete(u.layouts, id)
}

// setTheme records the theme id's run keeps.
func (u *unslopRuns) setTheme(id, theme string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if run := u.runs[id]; run != nil {
		run.Theme = theme
	}
}

// layoutOf is the findings of id's page at each width, as last measured.
func (u *unslopRuns) layoutOf(id string) ([]proto.UnslopFinding, bool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	l, ok := u.layouts[id]
	return l, ok
}

func (u *unslopRuns) setLayout(id string, l []proto.UnslopFinding) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.layouts == nil {
		u.layouts = map[string][]proto.UnslopFinding{}
	}
	u.layouts[id] = l
}

// addShot keeps a screenshot of id's run at a width, before it or after
// (when the run then counts the widths with both).
func (u *unslopRuns) addShot(id, label string, width int, png []byte, after bool) {
	if len(png) == 0 {
		return
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.shots == nil {
		u.shots = map[string][]proto.UnslopShot{}
	}
	list := u.shots[id]
	i := slices.IndexFunc(list, func(s proto.UnslopShot) bool { return s.Label == label })
	switch {
	case !after:
		if i < 0 {
			list = append(list, proto.UnslopShot{Label: label, Width: width, Before: png})
		}
	case i >= 0:
		list[i].After = png
	}
	u.shots[id] = list
	if run := u.runs[id]; run != nil {
		n := 0
		for _, s := range list {
			if s.After != nil {
				n++
			}
		}
		run.Shots = n
	}
}

// shotsOf are id's run's screenshots.
func (u *unslopRuns) shotsOf(id string) ([]proto.UnslopShot, bool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	list, ok := u.shots[id]
	return slices.Clone(list), ok
}

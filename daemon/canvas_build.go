package daemon

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/enowx/enowx/core/canvas"
	"github.com/enowx/enowx/core/designcompare"
	"github.com/enowx/enowx/core/designreview"
	"github.com/enowx/enowx/proto"
)

// The UI builder on a canvas (proto.CanvasBuildRun): what the app's Build
// dialog asks becomes what the canvas's agent is told; asked to explore,
// fresh agents build a direction each, a design of their own that only
// they may change; once all are built the daemon checks how distinct each
// pair is, sends the too-alike back once, and tells the lead; the build
// done, a fresh agent reviews the design picked.

// maxDirections is the most directions built at once.
const maxDirections = 4

// maxReworks is how many times a direction is sent back for being too
// alike another.
const maxReworks = 1

// buildRuns are the canvases' builds, by canvas. mu guards all.
type buildRuns struct {
	mu   sync.Mutex
	runs map[string]*proto.CanvasBuildRun
	// leases are the design each direction's agent builds, by session.
	leases map[string]buildLease
	// reworks counts each direction's send-backs, by design.
	reworks map[string]int
	reviews map[string]reviewing
}

type buildLease struct{ canvas, frame string }

func copyBuild(run *proto.CanvasBuildRun) proto.CanvasBuildRun {
	out := *run
	out.Directions = slices.Clone(run.Directions)
	out.Pairs = slices.Clone(run.Pairs)
	return out
}

// with changes canvas id's run with fn, answering it; false when none.
func (b *buildRuns) with(id string, fn func(run *proto.CanvasBuildRun)) (proto.CanvasBuildRun, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	run := b.runs[id]
	if run == nil {
		return proto.CanvasBuildRun{}, false
	}
	if fn != nil {
		fn(run)
	}
	return copyBuild(run), true
}

func (b *buildRuns) start(run proto.CanvasBuildRun) proto.CanvasBuildRun {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.runs == nil {
		b.runs, b.leases, b.reworks = map[string]*proto.CanvasBuildRun{}, map[string]buildLease{}, map[string]int{}
	}
	for s, l := range b.leases {
		if l.canvas == run.ID {
			delete(b.leases, s)
		}
	}
	b.runs[run.ID] = &run
	return copyBuild(&run)
}

// resume starts a run for canvas cv's build when the daemon has none,
// its directions the designs with cards.
func (b *buildRuns) resume(cv proto.Canvas) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.runs[cv.ID] != nil {
		return
	}
	if b.runs == nil {
		b.runs, b.leases, b.reworks = map[string]*proto.CanvasBuildRun{}, map[string]buildLease{}, map[string]int{}
	}
	run := &proto.CanvasBuildRun{ID: cv.ID, What: cv.Title, From: proto.BuildNew, Want: 1, Stage: proto.BuildBuilding}
	for _, f := range cv.Frames {
		if f.Direction != nil {
			run.Directions = append(run.Directions, proto.BuildDirection{Frame: f.ID, Done: true})
		}
	}
	b.runs[cv.ID] = run
}

func (b *buildRuns) lease(session string) (buildLease, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	l, ok := b.leases[session]
	return l, ok
}

func (b *buildRuns) addDirection(id string, d proto.BuildDirection) (proto.CanvasBuildRun, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	run := b.runs[id]
	if run == nil {
		return proto.CanvasBuildRun{}, false
	}
	run.Directions = append(run.Directions, d)
	if d.Session != "" {
		b.leases[d.Session] = buildLease{id, d.Frame}
	}
	return copyBuild(run), true
}

// directionDone marks session's direction done: the run, whether every
// direction now is, false when session builds none.
func (b *buildRuns) directionDone(id, session string) (proto.CanvasBuildRun, bool, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	run := b.runs[id]
	if run == nil {
		return proto.CanvasBuildRun{}, false, false
	}
	i := slices.IndexFunc(run.Directions, func(d proto.BuildDirection) bool { return d.Session == session && session != "" })
	if i < 0 {
		return proto.CanvasBuildRun{}, false, false
	}
	run.Directions[i].Done = true
	all := !slices.ContainsFunc(run.Directions, func(d proto.BuildDirection) bool { return !d.Done })
	return copyBuild(run), all, true
}

// rework sends design frame's direction back; false when it was already.
func (b *buildRuns) rework(id, frame string) (string, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	run := b.runs[id]
	if run == nil || b.reworks[frame] >= maxReworks {
		return "", false
	}
	i := slices.IndexFunc(run.Directions, func(d proto.BuildDirection) bool { return d.Frame == frame })
	if i < 0 || run.Directions[i].Session == "" {
		return "", false
	}
	b.reworks[frame]++
	run.Directions[i].Done = false
	return run.Directions[i].Session, true
}

func (b *buildRuns) reviewRound(id string, r reviewing) (int, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	run := b.runs[id]
	if run == nil || run.Stage != proto.BuildReviewing {
		return 0, false
	}
	round := 1
	if rv := run.Review; rv != nil {
		if !rv.Done || rv.Passed || rv.Round >= designreview.MaxRounds {
			return 0, false
		}
		round = rv.Round + 1
	}
	run.Review = &proto.DesignReview{Round: round}
	if b.reviews == nil {
		b.reviews = map[string]reviewing{}
	}
	b.reviews[id] = r
	return round, true
}

func (b *buildRuns) reviewer(id, session string) (any, bool) {
	return b.with(id, func(run *proto.CanvasBuildRun) {
		if run.Review != nil {
			rv := *run.Review
			rv.Session = session
			run.Review = &rv
		}
	})
}

func (b *buildRuns) verdict(session string, r proto.DesignReview) (any, proto.DesignReview, reviewing, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for id, run := range b.runs {
		if run.Review == nil || run.Review.Session != session || run.Review.Done {
			continue
		}
		r.Round, r.Session = run.Review.Round, session
		run.Review = &r
		if r.Passed || r.Round >= designreview.MaxRounds {
			run.Stage, run.Done = proto.BuildDone, true
		} else {
			run.Stage = proto.BuildRefining
		}
		return copyBuild(run), r, b.reviews[id], true
	}
	return nil, r, reviewing{}, false
}

func (b *buildRuns) dropReview(id string) (any, bool) {
	return b.with(id, func(run *proto.CanvasBuildRun) { run.Review = previousReview(*run.Review) })
}

// canvasBuild starts or advances the build on canvas p.ID.
func (s *Server) canvasBuild(p proto.CanvasBuildParams) (proto.CanvasBuild, error) {
	cv, err := s.canvasFor(p.ID).Get(p.ID)
	if err != nil {
		return proto.CanvasBuild{}, err
	}
	if p.Status {
		run, ok := s.canvasBuilds.with(cv.ID, nil)
		if !ok {
			return proto.CanvasBuild{}, errors.New("canvas: no build under way on this canvas")
		}
		return proto.CanvasBuild{Run: &run}, nil
	}
	if p.Start {
		return s.startBuild(cv, p)
	}
	if l, ok := s.canvasBuilds.lease(p.Session); ok && l.canvas == cv.ID {
		return s.directionReport(cv, p)
	}
	out := proto.CanvasBuild{}
	var run proto.CanvasBuildRun
	var ok bool
	if p.Done || p.Stage != "" {
		// Runs live in memory: after the daemon restarted (an update), the
		// lead goes on with a run taken up again from the canvas.
		s.canvasBuilds.resume(cv)
	}
	switch {
	case p.Fanout > 0:
		if err := s.fanOutDirections(cv, p.Fanout, p.Session, p.Concepts); err != nil {
			return out, err
		}
		run, ok = s.canvasBuilds.with(cv.ID, nil)
	case p.Done:
		i, err := canvas.FrameIndex(cv, p.Frame)
		if err != nil {
			return out, fmt.Errorf("canvas: the build is done with which design? %w", err)
		}
		run, ok = s.canvasBuilds.with(cv.ID, func(run *proto.CanvasBuildRun) {
			run.Stage, run.Lead = proto.BuildReviewing, p.Session
		})
		if ok && run.Work != proto.WorkReview {
			s.background.start(func() { s.reviewBuild(cv, cv.Frames[i], i+1, p.Session) })
		} else if ok {
			run, _ = s.canvasBuilds.with(cv.ID, func(run *proto.CanvasBuildRun) { run.Stage, run.Done = proto.BuildDone, true })
		}
	case p.Stage != "":
		if !slices.Contains([]string{proto.BuildBrief, proto.BuildDirections, proto.BuildBuilding, proto.BuildCompare, proto.BuildRefining}, p.Stage) {
			return out, fmt.Errorf("canvas: no stage %q: brief, directions, building, compare or refining", p.Stage)
		}
		if p.Stage != proto.BuildBrief && cv.Brief == nil {
			return out, errNoBrief(fmt.Sprintf("canvas_brief, canvas %q, with brief", cv.ID))
		}
		run, ok = s.canvasBuilds.with(cv.ID, func(run *proto.CanvasBuildRun) {
			run.Stage = p.Stage
			if p.Session != "" {
				run.Lead = p.Session
			}
		})
	}
	if !ok {
		return out, errors.New("canvas: no build under way on this canvas: the user starts one with Build")
	}
	s.broadcast(proto.EventCanvasBuild, run)
	out.Run = &run
	return out, nil
}

// startBuild begins a build from the app's Build dialog.
func (s *Server) startBuild(cv proto.Canvas, p proto.CanvasBuildParams) (proto.CanvasBuild, error) {
	if cv.Kind != proto.CanvasMockup {
		return proto.CanvasBuild{}, errors.New("canvas: the UI builder designs on a mockup canvas")
	}
	if strings.TrimSpace(p.What) == "" && p.From != proto.BuildScreenshot {
		return proto.CanvasBuild{}, errors.New("canvas: say what to build")
	}
	switch p.From {
	case "", proto.BuildNew:
		p.From = proto.BuildNew
	case proto.BuildChange:
		if _, err := canvas.FrameIndex(cv, p.Frame); err != nil {
			return proto.CanvasBuild{}, fmt.Errorf("canvas: which design to change? %w", err)
		}
	case proto.BuildProject:
		if p.Project == "" {
			return proto.CanvasBuild{}, errors.New("canvas: which project?")
		}
		if !slices.Contains([]string{proto.WorkRefresh, proto.WorkFlow, proto.WorkFeature, proto.WorkReview}, p.Work) {
			return proto.CanvasBuild{}, errors.New("canvas: a UI refresh, a flow fix, a new feature, or a review?")
		}
	case proto.BuildScreenshot:
		if _, err := s.canvasFor(cv.ID).Asset(cv.ID, p.Image); err != nil || p.Image == "" {
			return proto.CanvasBuild{}, errors.New("canvas: the screenshot to recreate isn't on the canvas")
		}
	default:
		return proto.CanvasBuild{}, fmt.Errorf("canvas: build from %q?", p.From)
	}
	if len(p.Devices) == 0 {
		p.Devices = []string{"desktop", "mobile"}
	}
	p.Directions = min(max(p.Directions, 1), maxDirections)
	if p.From == proto.BuildScreenshot || p.Work == proto.WorkReview || p.Work == proto.WorkFlow || p.Work == proto.WorkFeature {
		p.Directions = 1 // the look is set: one way, its own
	}
	run := s.canvasBuilds.start(proto.CanvasBuildRun{ID: cv.ID, What: strings.TrimSpace(p.What), From: p.From, Work: p.Work,
		Devices: p.Devices, Want: p.Directions, Stage: proto.BuildBrief})
	s.broadcast(proto.EventCanvasBuild, run)
	return proto.CanvasBuild{Ask: buildAsk(cv, p), Run: &run}, nil
}

// fanOutDirections starts n fresh agents, a direction each on a new
// design of its own, from the brief alone; concepts, its card only.
func (s *Server) fanOutDirections(cv proto.Canvas, n int, lead string, concepts bool) error {
	if n < 2 || n > maxDirections {
		return fmt.Errorf("canvas: fan out 2 to %d directions", maxDirections)
	}
	if cv.Brief == nil {
		return errNoBrief(fmt.Sprintf("canvas_brief, canvas %q, with brief", cv.ID))
	}
	stage, kind := proto.BuildBuilding, "Direction "
	if concepts {
		stage, kind = proto.BuildDirections, "Concept "
	}
	run, ok := s.canvasBuilds.with(cv.ID, func(run *proto.CanvasBuildRun) { run.Stage, run.Lead = stage, lead })
	if !ok {
		return errors.New("canvas: no build under way on this canvas: the user starts one with Build")
	}
	agentID := "claude"
	if sess, err := s.sessions.Get(lead); err == nil {
		agentID = sess.Meta().Agent
	}
	w, h := 1440, 900
	if len(cv.Frames) > 0 && cv.Frames[0].Width >= 900 {
		w, h = cv.Frames[0].Width, cv.Frames[0].Height
	}
	for i := range n {
		letter := string(rune('A' + len(run.Directions) + i))
		name := kind + letter
		desc := "Being built by its agent"
		if concepts {
			desc = "A concept: its card, from the brief alone"
		}
		c, err := s.canvasEdit(proto.CanvasEdit{ID: cv.ID, Op: proto.CanvasOpFrameAdd, FrameInfo: &proto.CanvasFrame{Name: name, Width: w, Height: h, Description: desc, Variant: letter, Page: run.What}})
		if err != nil {
			return err
		}
		f := c.Frames[len(c.Frames)-1]
		sess, err := s.createSession(proto.SessionCreateParams{Agent: agentID, CanvasFor: cv.ID, Group: "build:" + cv.ID, Name: name, Cols: 100, Rows: 40,
			Prompt: directionAsk(c, f, len(c.Frames), letter, sources[(len(run.Directions)+i)%len(sources)], run, concepts)})
		if err != nil {
			return fmt.Errorf("canvas: start the agent of %s: %w", name, err)
		}
		if r, ok := s.canvasBuilds.addDirection(cv.ID, proto.BuildDirection{Frame: f.ID, Session: sess.ID()}); ok {
			s.broadcast(proto.EventCanvasBuild, r)
		}
	}
	return nil
}

// directionReport is a direction's agent saying where it is: done, its
// design is compared with the others once all are.
func (s *Server) directionReport(cv proto.Canvas, p proto.CanvasBuildParams) (proto.CanvasBuild, error) {
	if !p.Done {
		return proto.CanvasBuild{}, errors.New("canvas: as a direction's agent, say done (canvas_build done) when your design is built")
	}
	l, _ := s.canvasBuilds.lease(p.Session)
	i, err := canvas.FrameIndex(cv, l.frame)
	if err != nil {
		return proto.CanvasBuild{}, err
	}
	if cv.Frames[i].Direction == nil {
		return proto.CanvasBuild{}, fmt.Errorf("canvas: write your direction's card first (canvas_direction, design %d), in this form:\n%s", i+1, designcompare.CardFormat)
	}
	run, all, ok := s.canvasBuilds.directionDone(cv.ID, p.Session)
	if !ok {
		return proto.CanvasBuild{}, errors.New("canvas: no build under way on this canvas")
	}
	s.broadcast(proto.EventCanvasBuild, run)
	if all {
		go s.compareDirections(cv.ID)
	}
	return proto.CanvasBuild{Run: &run}, nil
}

// compareDirections checks every pair of the build's directions, sends
// the later of a pair too alike back once, and when none is (or none may
// go back) tells the lead they're ready to compare.
func (s *Server) compareDirections(id string) {
	cv, err := s.canvasFor(id).Get(id)
	if err != nil {
		return
	}
	run, ok := s.canvasBuilds.with(id, nil)
	if !ok {
		return
	}
	frames := map[string]proto.CanvasFrame{}
	shots := map[string][]byte{}
	for _, d := range run.Directions {
		if i, err := canvas.FrameIndex(cv, d.Frame); err == nil {
			frames[d.Frame] = cv.Frames[i]
			if sh := s.canvasShots(cv, cv.Frames[i]); len(sh) > 0 {
				shots[d.Frame] = sh[0].png
			}
		}
	}
	var pairs []proto.DirectionPair
	sent := map[string]bool{}
	for i, a := range run.Directions {
		for _, b := range run.Directions[i+1:] {
			fa, fb := frames[a.Frame], frames[b.Frame]
			if fa.Direction == nil || fb.Direction == nil {
				continue
			}
			alike := -1
			if len(shots[a.Frame]) > 0 && len(shots[b.Frame]) > 0 {
				if n, err := designcompare.Squint(shots[a.Frame], shots[b.Frame]); err == nil {
					alike = n
				}
			}
			p := designcompare.Pair(a.Frame, b.Frame, *fa.Direction, *fb.Direction, alike)
			pairs = append(pairs, p)
			if designcompare.TooClose(p) && !sent[a.Frame] && !sent[b.Frame] {
				if session, ok := s.canvasBuilds.rework(id, b.Frame); ok {
					sent[b.Frame] = true
					s.tellDirection(session, fb, fa, p)
				}
			}
		}
	}
	run, _ = s.canvasBuilds.with(id, func(run *proto.CanvasBuildRun) {
		run.Pairs = pairs
		if len(sent) == 0 && run.Stage != proto.BuildDirections {
			run.Stage = proto.BuildCompare
		}
	})
	s.broadcast(proto.EventCanvasBuild, run)
	if len(sent) == 0 && run.Lead != "" {
		var names []string
		for _, d := range run.Directions {
			names = append(names, frames[d.Frame].Name)
		}
		if run.Stage == proto.BuildDirections {
			text := fmt.Sprintf("The concepts are in (%s), each a card written from the brief alone, their distinctness checked. Check each against the brief (fits the users and the task; its main action works; what it needs exists; it works on the devices), pick the one that fits best, and build it on its design (canvas_build stage building); keep the others as they are.", strings.Join(names, ", "))
			s.tellSession(proto.SessionInputParams{ID: run.Lead, Text: text, Paste: true, Enter: true}, "canvas build: tell the lead")
			return
		}
		text := fmt.Sprintf("All %d directions are built (%s), each pair checked for distinctness (the app shows the scores in Compare). Now: canvas_compare them side by side, the current design first if there is one; tell the user in a few lines what each is and which you recommend, and ask them to pick (Compare's Pick, with what to keep and change). Then refine the one picked, keeping the others.",
			len(run.Directions), strings.Join(names, ", "))
		s.tellSession(proto.SessionInputParams{ID: run.Lead, Text: text, Paste: true, Enter: true}, "canvas build: tell the lead")
	}
}

// tellDirection sends design f's agent back: too alike design g.
func (s *Server) tellDirection(session string, f, g proto.CanvasFrame, p proto.DirectionPair) {
	why := "its first screen's light and dark shapes read like " + g.Name + "'s"
	if len(p.Shared) > designcompare.MaxShared {
		why = "it shares " + strings.Join(p.Shared, ", ") + " with " + g.Name + " (at most one may be shared)"
	}
	text := fmt.Sprintf("Your direction is too close to %s: %s. Change your skeleton (the first screen's layout and where its focal point sits), not just colors, and whatever else you share with it; update your card (canvas_direction), look again (canvas_screenshot), then canvas_build done.", g.Name, why)
	s.tellSession(proto.SessionInputParams{ID: session, Text: text, Paste: true, Enter: true}, "canvas build: send a direction back")
}

// reviewBuild has the build's design f reviewed independently.
func (s *Server) reviewBuild(cv proto.Canvas, f proto.CanvasFrame, n int, lead string) {
	at := fmt.Sprintf(`canvas_build (canvas %q, done, design %d)`, cv.ID, n)
	task := fmt.Sprintf("design %d (“%s”) of the canvas “%s”, a mockup", n, f.Name, cv.Title)
	if run, ok := s.canvasBuilds.with(cv.ID, nil); ok && run.From == proto.BuildProject {
		task += ".\n\n" + designreview.Existing
	}
	s.startReview(&s.canvasBuilds, cv.ID, reviewing{lead: lead, again: at, event: proto.EventCanvasBuild}, task, cv.Brief, s.canvasShots(cv, f))
}

// checkBuildLease refuses a direction's agent changes to designs not its
// own.
func (s *Server) checkBuildLease(e proto.CanvasEdit) error {
	l, ok := s.canvasBuilds.lease(e.Session)
	if !ok || l.canvas != e.ID {
		return nil
	}
	cv, err := s.canvasFor(e.ID).Get(e.ID)
	if err != nil {
		return err
	}
	refuse := fmt.Errorf("canvas: you build your direction's design only: change it alone (--frame %s)", l.frame)
	switch e.Op {
	case proto.CanvasOpMeta, proto.CanvasOpFrameAdd, proto.CanvasOpFrameMove, proto.CanvasOpFrameCopy, proto.CanvasOpPick, proto.CanvasOpBrief:
		return refuse
	}
	if i, err := canvas.FrameIndex(cv, e.Frame); err != nil || cv.Frames[i].ID != l.frame {
		return refuse
	}
	return nil
}

// canvasMatch compares design p.Frame with the picture p.Image.
func (s *Server) canvasMatch(p proto.CanvasMatchParams) (proto.CanvasMatch, error) {
	cv, err := s.canvasFor(p.ID).Get(p.ID)
	if err != nil {
		return proto.CanvasMatch{}, err
	}
	i, err := canvas.FrameIndex(cv, p.Frame)
	if err != nil {
		return proto.CanvasMatch{}, err
	}
	ref, err := s.canvasFor(cv.ID).Asset(cv.ID, p.Image)
	if err != nil {
		return proto.CanvasMatch{}, fmt.Errorf("canvas: no picture %q on the canvas: %w", p.Image, err)
	}
	shots := s.canvasShots(cv, cv.Frames[i])
	if len(shots) == 0 || len(shots[0].png) == 0 {
		return proto.CanvasMatch{}, errors.New("canvas: the design couldn't be rendered: the app renders it, open it")
	}
	return designcompare.Match(shots[0].png, ref)
}

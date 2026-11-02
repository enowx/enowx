package daemon

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/enowx/enowx/core/canvas"
	"github.com/enowx/enowx/core/designbrief"
	"github.com/enowx/enowx/core/unslop"
	"github.com/enowx/enowx/proto"
)

// canvasUnslop reads a mockup's design for what makes it look AI-made
// (core/unslop); started, it copies the design first, so the redesign
// happens on the copy and the original stays to compare, and says what to
// ask the canvas's agent.
func (s *Server) canvasUnslop(p proto.CanvasUnslopParams) (proto.CanvasUnslop, error) {
	cv, err := s.canvasFor(p.ID).Get(p.ID)
	if err != nil {
		return proto.CanvasUnslop{}, err
	}
	if p.Status {
		run, ok := s.unslopRuns.status(cv.ID)
		if !ok {
			return proto.CanvasUnslop{}, errors.New("canvas: no unslop under way on this canvas")
		}
		return proto.CanvasUnslop{Frame: run.Frame, Score: run.Score, Findings: run.Findings, Run: &run}, nil
	}
	i, err := canvas.FrameIndex(cv, p.Frame)
	if err != nil {
		return proto.CanvasUnslop{}, err
	}
	from := cv.Frames[i]
	if from.HTML == "" {
		return proto.CanvasUnslop{}, errors.New("canvas: unslop reads a mockup's HTML, and this design has none")
	}
	f, n := from, i+1
	if p.Start {
		name := from.Name + " · unslop"
		desc := "Unslopped from design " + fmt.Sprint(i+1)
		c, err := s.canvasEdit(proto.CanvasEdit{ID: cv.ID, Op: proto.CanvasOpFrameCopy, Frame: from.ID, Name: &name, Description: &desc})
		if err != nil {
			return proto.CanvasUnslop{}, err
		}
		cv = c
		if i+1 >= len(cv.Frames) {
			return proto.CanvasUnslop{}, errors.New("canvas: the design's copy isn't there")
		}
		f, n = cv.Frames[i+1], i+2
	}
	score, findings := unslop.Scan(f.HTML, cv.Kind)
	// As rendered too: measured anew at the start, for the mobile pass and
	// at the end, else as last measured.
	rendered, measured := s.unslopRuns.layoutOf(cv.ID)
	var walk *proto.UnslopWalk
	if p.Start || p.Pass == "12" || strings.EqualFold(p.Pass, "done") || !measured && p.Region == "" {
		rendered = s.canvasRendered(cv, f)
		measured = false
		if len(rendered) > 0 {
			var states []proto.UnslopFinding
			walk, states = s.canvasWalk(cv, f, rendered)
			rendered = append(rendered, states...)
		}
	}
	if len(rendered) > 0 {
		findings = unslop.WithRendered(findings, rendered)
		score = unslop.ScoreStates(findings)
	}
	theme := unslop.Theme(from.HTML)
	if !p.Start {
		theme = s.unslopRuns.theme(cv.ID, f.ID)
	}
	// A redesign keeps its original's theme.
	if fd, turned := unslop.ThemeFinding(theme, f.HTML); turned {
		findings = append([]proto.UnslopFinding{fd}, findings...)
		score = max(score-15, 0)
	}
	out := proto.CanvasUnslop{Frame: f.ID, Score: score, Findings: findings, Walk: walk}
	if p.Start {
		out.Ask = unslopAsk(cv, i+1, n, f, score, len(findings), theme) + unslop.WalkReport(walk)
	}
	switch {
	case p.Region != "":
		// A region's agent: its pass, the run's score kept current.
		if _, _, err := s.unslopRuns.update(cv.ID, f.ID, proto.CanvasUnslopParams{}, score, findings, theme); err != nil {
			return out, err
		}
		was, _ := s.unslopRuns.status(cv.ID)
		run, err := s.unslopRuns.regionReport(cv.ID, f.ID, p.Region, p.Pass)
		if err != nil {
			return out, err
		}
		s.broadcast(proto.EventCanvasUnslop, run)
		if strings.EqualFold(p.Pass, "done") {
			for _, r := range run.Regions {
				if r.ID == p.Region {
					s.helperFinished(r.Session)
				}
			}
		}
		if allDone(run) && !allDone(was) {
			go s.tellLead(run, cv)
		}
		out.Run = &run
		return out, nil
	case len(p.Regions) > 0:
		if err := s.fanOutRegions(cv, f, n, p.Regions, p.Session); err != nil {
			return out, err
		}
	}
	if p.Pass != "" && !strings.EqualFold(p.Pass, "done") && cv.Brief == nil && !s.unslopRuns.hasBrief(cv.ID) {
		return out, errNoBrief(fmt.Sprintf("canvas_brief, canvas %q, with brief", cv.ID))
	}
	only, err := onlyPasses(p.Only)
	if err != nil {
		return out, err
	}
	was, _ := s.unslopRuns.status(cv.ID)
	run, ok, err := s.unslopRuns.update(cv.ID, f.ID, proto.CanvasUnslopParams{Start: p.Start, Pass: p.Pass, Frame: from.ID}, score, findings, theme)
	if err != nil {
		return out, err
	}
	switch {
	case ok && p.Start:
		s.startStory(cv, f, score)
		if len(only) > 0 {
			s.unslopRuns.keepOnly(cv.ID, only)
			run.Passes = only
			out.Ask += "\nRun only these passes, in this order, and skip the others: " + strings.Join(only, ", ") + "."
		}
	case ok && was.Frame == f.ID && was.Pass != "" && (run.Pass != was.Pass || run.Done):
		// A pass ended: the design as it left it is a step.
		steps := s.stepEnded(cv, f, passesBetween(was.Passes, was.Pass, run.Pass), score)
		run, _ = s.unslopRuns.setPassed(cv.ID, f.ID, steps)
	}
	if ok && cv.Brief != nil {
		run, _ = s.unslopRuns.setBrief(cv.ID, cv.Brief)
	}
	if ok && !measured {
		s.unslopRuns.setLayout(cv.ID, rendered)
	}
	if ok && run.Done && strings.EqualFold(p.Pass, "done") && p.Region == "" {
		s.background.start(func() { s.reviewCanvas(cv, f, n, p.Session) })
	}
	if ok {
		s.broadcast(proto.EventCanvasUnslop, run)
		out.Run = &run
	}
	return out, nil
}

// unslopRuns are the canvases' redesigns under way, by canvas: one each,
// the latest started. mu guards runs.
type unslopRuns struct {
	mu   sync.Mutex
	runs map[string]*proto.CanvasUnslopRun
	// layouts are a Browser page's findings at each width, and shots its
	// screenshots before and after, by session.
	layouts map[string][]proto.UnslopFinding
	shots   map[string][]proto.UnslopShot
	// reviews are what each run's independent review tells, by run.
	reviews map[string]reviewing
}

// update records a read of design frame of canvas id: a run started, or a
// pass begun, or the design read again in one under way; false when no
// run is about it.
func (u *unslopRuns) update(id, frame string, p proto.CanvasUnslopParams, score int, findings []proto.UnslopFinding, theme string) (proto.CanvasUnslopRun, bool, error) {
	pass, done := "", strings.EqualFold(strings.TrimSpace(p.Pass), "done")
	if p.Pass != "" && !done {
		name, ok := unslop.PassNamed(p.Pass)
		if !ok {
			return proto.CanvasUnslopRun{}, false, fmt.Errorf("canvas: no pass %q: a number from 1 to %d, or done", p.Pass, len(unslop.Passes))
		}
		pass = name
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.runs == nil {
		u.runs = map[string]*proto.CanvasUnslopRun{}
	}
	run := u.runs[id]
	switch {
	case p.Start:
		run = &proto.CanvasUnslopRun{ID: id, Frame: frame, From: score, Passes: unslop.Names(), Theme: theme}
		if p.Frame != frame {
			run.Original = p.Frame
		}
		u.runs[id] = run
	case run == nil || run.Frame != frame:
		return proto.CanvasUnslopRun{}, false, nil
	}
	if run.Scans == 0 {
		run.From = score // the first read, after a run's steps
	}
	run.Score, run.Findings = score, findings
	run.Scans++
	if (pass != "" || done) && run.Pass != "" && run.Pass != pass && !slices.Contains(run.Finished, run.Pass) {
		run.Finished = append(run.Finished, run.Pass)
	}
	switch {
	case done:
		run.Pass, run.Done = "", true
	case pass != "":
		run.Pass, run.Done = pass, false // a pass after done: done again later
	}
	return copyRun(run), true, nil
}

// theme is the original's theme of the run about design frame of canvas
// id, "" when none.
func (u *unslopRuns) theme(id, frame string) string {
	u.mu.Lock()
	defer u.mu.Unlock()
	if run := u.runs[id]; run != nil && run.Frame == frame {
		return run.Theme
	}
	return ""
}

// unslopAsk is what the canvas's agent is told to redesign design n, the
// copy of design from: the brief first, then the command telling it
// what's off, by pass, and how to go through them.
func unslopAsk(cv proto.Canvas, from, n int, f proto.CanvasFrame, score, found int, theme string) string {
	keep := "Keep its look's direction: its theme, brand color family and mood stay; unslop changes its care, not its style."
	if theme != "" {
		keep = "It is a " + theme + " design and stays " + theme + ": never turn its theme. " + keep
	}
	cmd := fmt.Sprintf(`canvas_unslop (canvas %q, frame %q)`, cv.ID, f.ID)
	brief := fmt.Sprintf("Write the canvas's brief with canvas_brief (canvas %q, brief: Markdown, a ## section a field: %s)", cv.ID, designbrief.Sections())
	if cv.Brief != nil {
		brief = fmt.Sprintf("The canvas has a brief (canvas_brief, canvas %q): read it, and write it again where it no longer holds", cv.ID)
	}
	return fmt.Sprintf(`Unslop design %d (“%s”), a copy of design %d. Its HTML reads %d/100 with %d findings of what makes a design look AI-made, but rules read only the code: what it looks like, and what it is for, you judge. The user watches it live.
0. Understand the product first: the canvas, its designs and their content, its project if it has one (project context), and how design %d looks (canvas_screenshot). %s. Tone as decisions (calm or lively, dense or airy, polished or raw), not adjectives; Remove names what repeats, explains the obvious or decorates without helping. Passes are refused until it is written.
1. Read what's off: %s — the findings by pass, in the order to fix them, each with what to avoid. Then look at the screenshot against the brief for what rules can't read (more than one focal point or main action, a label over every heading, a glow doing no work, boxes alike in a row, text saying what the layout shows, a look any product of its kind could have), each in the pass it belongs to. Then say honestly how much is off, and change that much: many signs across color, type, icons and copy, every pass in full; a designed screen with a few signs left, fix only those, its palette, type, layout and pictures kept, bigger ideas told as suggestions; nothing off, say so, suggest at most three refinements with exact values, and end with %s with pass "done" unless the user wants them.
2. Redesign design %d one pass at a time, in that order, keeping its content, purpose and layout's intent; never invent data. %s Passes with nothing found still get a designer's look (structure, mobile).
3. Begin each pass with %s with pass N (its number, 1 to 12): it reads the design again, and the app shows the pass under way. Change the design element by element (canvas_patch), not all at once, so each change shows.
4. Do passes 1 to 5 yourself (truth, structure, spacing, type, color): they set the foundation, with the spacing, type and color as CSS variables the rest uses.
5. Then fan passes 6 to 10 out, an agent per region at once: give each element with an id that is a part of its own (2 to 6 of them: the sidebar, the header, a row of cards, the chart…, none inside another, together most of the design) with %s with fanout "#a,#b,#c". Then end your turn and change nothing meanwhile: you get a message here when they are all done. A design too small to split, do 6 to 10 yourself.
6. Then do 11 (access) and 12 (mobile) on the whole design; after each pass, describe the design with what it changed (canvas_design, design %d, description …).
7. Take away once more: one focal point, one main action, the rest quiet; look at the screenshot again. When what it still finds is what you'd keep on purpose: %s with pass "done", then show both side by side (canvas_compare, designs %d and %d).`,
		n, f.Name, from, score, found, n, brief, cmd, cmd, n, keep, cmd, cmd, n, cmd, from, n)
}

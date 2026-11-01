package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/enowx/enowx/core/canvas"
	"github.com/enowx/enowx/core/envsync"
	"github.com/enowx/enowx/core/unslop"
	"github.com/enowx/enowx/proto"
)

// A canvas redesign's steps: the design as it was when it began and as
// each pass (or the passes region agents did at once) left it, its
// canvas version labelled with the pass, and what each changed as
// rendered; kept in the daemon's unslop/ folder, so a redesign plays back,
// compares and exports after a restart too, and one step can be left out.

// unslopStories keeps the steps. mu guards the files.
type unslopStories struct {
	mu sync.Mutex
}

// story is a redesign's steps on disk: HTML[0] the design it started
// from, HTML[k] as step k left it.
type story struct {
	Canvas string             `json:"canvas"`
	Frame  string             `json:"frame"`
	Name   string             `json:"name,omitempty"`
	From   int                `json:"from,omitempty"`
	Score  int                `json:"score,omitempty"`
	HTML   []string           `json:"html"`
	Steps  []proto.UnslopStep `json:"steps"`
}

func (s *Server) storyFile(canvasID, frame string) string {
	return filepath.Join(s.dataDir, "unslop", canvasID+"-"+frame+".json")
}

func (s *Server) loadStory(canvasID, frame string) (story, error) {
	var st story
	data, err := os.ReadFile(s.storyFile(canvasID, frame))
	if err != nil {
		return st, errors.New("canvas: this design wasn't redesigned pass by pass here")
	}
	return st, json.Unmarshal(data, &st)
}

func (s *Server) saveStory(st story) error {
	data, err := json.Marshal(st)
	if err != nil {
		return err
	}
	path := s.storyFile(st.Canvas, st.Frame)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return envsync.WriteFileAtomic(path, data, 0o600)
}

// startStory keeps the design a redesign starts from.
func (s *Server) startStory(cv proto.Canvas, f proto.CanvasFrame, score int) {
	s.stories.mu.Lock()
	defer s.stories.mu.Unlock()
	_ = s.saveStory(story{Canvas: cv.ID, Frame: f.ID, Name: f.Name, From: score, Score: score, HTML: []string{f.HTML}, Steps: []proto.UnslopStep{}})
}

// stepEnded keeps design f as passes left it, labels the canvas's version
// with them, and measures what they changed, in the background. It
// returns the steps so far.
func (s *Server) stepEnded(cv proto.Canvas, f proto.CanvasFrame, passes []string, score int) []proto.UnslopStep {
	s.stories.mu.Lock()
	st, err := s.loadStory(cv.ID, f.ID)
	if err != nil {
		s.stories.mu.Unlock()
		return nil
	}
	version, _ := s.canvasFor(cv.ID).Label(cv.ID, "Unslop · "+stepLabel(passes))
	st.HTML = append(st.HTML, f.HTML)
	st.Steps = append(st.Steps, proto.UnslopStep{Passes: passes, Version: version})
	st.Score = score
	k := len(st.Steps)
	_ = s.saveStory(st)
	steps := slices.Clone(st.Steps)
	s.stories.mu.Unlock()
	s.background.start(func() { s.measureStep(cv, f, k) })
	return steps
}

// stepLabel names a step by its passes: one, or the first to the last.
func stepLabel(passes []string) string {
	switch len(passes) {
	case 0:
		return ""
	case 1:
		return passes[0]
	}
	return passes[0] + " to " + passes[len(passes)-1]
}

// measureStep measures step k of the redesign of design f as rendered
// (in the app), against the step before, and tells the apps.
func (s *Server) measureStep(cv proto.Canvas, f proto.CanvasFrame, k int) {
	s.stories.mu.Lock()
	st, err := s.loadStory(cv.ID, f.ID)
	s.stories.mu.Unlock()
	if err != nil || k < 1 || k >= len(st.HTML) {
		return
	}
	assets := func(name string) ([]byte, error) { return s.canvasFor(cv.ID).Asset(cv.ID, name) }
	read := func(html string) string {
		g := f
		g.HTML = html
		r, err := s.appRender(proto.CanvasRenderRequest{HTML: canvas.FrameDocument(g, assets), Width: f.Width, Height: f.Height, Script: unslop.MeasureScript})
		if err != nil {
			return ""
		}
		return r.Text
	}
	before, after := read(st.HTML[k-1]), read(st.HTML[k])
	if before == "" || after == "" {
		return // no app to render it: measured when one asks (unslopStory)
	}
	changes := unslop.Changes(before, after)
	s.stories.mu.Lock()
	st, err = s.loadStory(cv.ID, f.ID)
	if err == nil && k <= len(st.Steps) {
		st.Steps[k-1].Changes, st.Steps[k-1].Measured = changes, true
		_ = s.saveStory(st)
	}
	s.stories.mu.Unlock()
	if err != nil {
		return
	}
	if run, ok := s.unslopRuns.setPassed(cv.ID, f.ID, st.Steps); ok {
		s.broadcast(proto.EventCanvasUnslop, run)
	}
}

// setPassed puts steps in the run about design frame of canvas id.
func (u *unslopRuns) setPassed(id, frame string, steps []proto.UnslopStep) (proto.CanvasUnslopRun, bool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	run := u.runs[id]
	if run == nil || run.Frame != frame {
		return proto.CanvasUnslopRun{}, false
	}
	run.Passed = slices.Clone(steps)
	return copyRun(run), true
}

// keepOnly narrows the run of canvas id to passes only.
func (u *unslopRuns) keepOnly(id string, only []string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if run := u.runs[id]; run != nil && len(only) > 0 {
		run.Passes = only
	}
}

// onlyPasses are the passes refs name (numbers or names), in order;
// none for none named.
func onlyPasses(refs []string) ([]string, error) {
	var names []string
	for _, r := range refs {
		name, ok := unslop.PassNamed(r)
		if !ok {
			return nil, fmt.Errorf("canvas: no pass %q", r)
		}
		names = append(names, name)
	}
	var out []string
	for _, n := range unslop.Names() {
		if slices.Contains(names, n) {
			out = append(out, n)
		}
	}
	return out, nil
}

// passesBetween are the passes of the run from prev up to, not with,
// next ("" for the run's end).
func passesBetween(all []string, prev, next string) []string {
	i, j := slices.Index(all, prev), slices.Index(all, next)
	if i < 0 {
		return []string{prev}
	}
	if j <= i {
		return []string{prev}
	}
	return slices.Clone(all[i:j])
}

// unslopStory is a redesign's steps as documents to play back.
func (s *Server) unslopStory(p proto.UnslopStepsParams) (proto.UnslopStory, error) {
	cv, err := s.canvasFor(p.ID).Get(p.ID)
	if err != nil {
		return proto.UnslopStory{}, err
	}
	i, err := canvas.FrameIndex(cv, p.Frame)
	if err != nil {
		return proto.UnslopStory{}, err
	}
	f := cv.Frames[i]
	s.stories.mu.Lock()
	st, err := s.loadStory(cv.ID, f.ID)
	s.stories.mu.Unlock()
	if err != nil {
		return proto.UnslopStory{}, err
	}
	for k, step := range st.Steps {
		if !step.Measured {
			s.measureStep(cv, f, k+1)
		}
	}
	s.stories.mu.Lock()
	st, _ = s.loadStory(cv.ID, f.ID)
	s.stories.mu.Unlock()
	assets := func(name string) ([]byte, error) { return s.canvasFor(cv.ID).Asset(cv.ID, name) }
	out := proto.UnslopStory{Frame: f.ID, Name: f.Name, Width: f.Width, Height: f.Height, Steps: st.Steps, From: st.From, Score: st.Score}
	for _, h := range st.HTML {
		g := f
		g.HTML = h
		out.Docs = append(out.Docs, canvas.FrameDocument(g, assets))
	}
	return out, nil
}

// leaveOut makes a copy of the redesigned design without step p.Step:
// what it changed put back, unless changed since.
func (s *Server) leaveOut(p proto.UnslopLeaveOutParams) (proto.UnslopLeftOut, error) {
	cv, err := s.canvasFor(p.ID).Get(p.ID)
	if err != nil {
		return proto.UnslopLeftOut{}, err
	}
	i, err := canvas.FrameIndex(cv, p.Frame)
	if err != nil {
		return proto.UnslopLeftOut{}, err
	}
	f := cv.Frames[i]
	s.stories.mu.Lock()
	st, err := s.loadStory(cv.ID, f.ID)
	s.stories.mu.Unlock()
	if err != nil {
		return proto.UnslopLeftOut{}, err
	}
	if p.Step < 1 || p.Step >= len(st.HTML) {
		return proto.UnslopLeftOut{}, fmt.Errorf("canvas: no step %d: 1 to %d", p.Step, len(st.HTML)-1)
	}
	page, back, kept, err := canvas.LeaveOut(f.HTML, st.HTML[p.Step-1], st.HTML[p.Step])
	if err != nil {
		return proto.UnslopLeftOut{}, err
	}
	if len(back) == 0 {
		return proto.UnslopLeftOut{Kept: kept}, errors.New("canvas: nothing of that step is left to take out: what it changed was changed again since")
	}
	name := f.Name + " · without " + strings.ToLower(stepLabel(st.Steps[p.Step-1].Passes))
	desc := "Design " + fmt.Sprint(i+1) + " with step " + fmt.Sprint(p.Step) + " left out"
	c, err := s.canvasEdit(proto.CanvasEdit{ID: cv.ID, Op: proto.CanvasOpFrameCopy, Frame: f.ID, Name: &name, Description: &desc})
	if err != nil {
		return proto.UnslopLeftOut{}, err
	}
	copied := c.Frames[i+1]
	if _, err := s.canvasEdit(proto.CanvasEdit{ID: cv.ID, Op: proto.CanvasOpHTMLSet, Frame: copied.ID, HTML: page}); err != nil {
		return proto.UnslopLeftOut{}, err
	}
	return proto.UnslopLeftOut{Frame: copied.ID, Back: back, Kept: kept}, nil
}

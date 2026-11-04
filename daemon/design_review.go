package daemon

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/enowx/enowx/core/canvas"
	"github.com/enowx/enowx/core/designbrief"
	"github.com/enowx/enowx/core/designreview"
	"github.com/enowx/enowx/proto"
)

// The independent review of a redesign once done: a fresh agent, in a
// folder holding only the screenshots and the brief, scores it on the
// rubric (core/designreview); the redesign's agent is told the verdict,
// and below the bar fixes it and says done again, up to MaxRounds.

// reviewing is what a review of a run needs once its verdict is in: the
// redesign's agent to tell, how it asks for the next round, the event the
// run goes out as.
type reviewing struct {
	lead, again, event string
}

// reviewStore keeps runs that get reviewed (unslops, builds): a round
// started, its reviewer, its verdict, a round that couldn't start; each
// answers the run to tell the apps.
type reviewStore interface {
	reviewRound(id string, r reviewing) (int, bool)
	reviewer(id, session string) (any, bool)
	verdict(session string, r proto.DesignReview) (any, proto.DesignReview, reviewing, bool)
	dropReview(id string) (any, bool)
}

// reviewShot is a screenshot a reviewer looks at.
type reviewShot struct {
	name string
	png  []byte
}

// reviewRound starts the next review of id's run: its round, false when
// the run isn't done or its reviews are over.
func (u *unslopRuns) reviewRound(id string, r reviewing) (int, bool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	run := u.runs[id]
	if run == nil || !run.Done {
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
	if u.reviews == nil {
		u.reviews = map[string]reviewing{}
	}
	u.reviews[id] = r
	return round, true
}

// dropReview forgets the review of id's run that couldn't start, so the
// next done starts it again.
func (u *unslopRuns) dropReview(id string) (any, bool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	run := u.runs[id]
	if run == nil || run.Review == nil {
		return nil, false
	}
	run.Review = previousReview(*run.Review)
	return copyRun(run), true
}

// previousReview is what a run's review was before round rv, which
// couldn't start.
func previousReview(rv proto.DesignReview) *proto.DesignReview {
	if rv.Round <= 1 {
		return nil
	}
	rv.Round, rv.Done, rv.Session = rv.Round-1, true, ""
	return &rv
}

// reviewer records session as the reviewer of id's run.
func (u *unslopRuns) reviewer(id, session string) (any, bool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	run := u.runs[id]
	if run == nil || run.Review == nil {
		return nil, false
	}
	rv := *run.Review
	rv.Session = session
	run.Review = &rv
	return copyRun(run), true
}

// verdict keeps reviewer session's verdict r on its run: the run, what to
// do with it, false when session reviews none.
func (u *unslopRuns) verdict(session string, r proto.DesignReview) (any, proto.DesignReview, reviewing, bool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	for id, run := range u.runs {
		if run.Review == nil || run.Review.Session != session || run.Review.Done {
			continue
		}
		r.Round, r.Session = run.Review.Round, session
		run.Review = &r
		return copyRun(run), r, u.reviews[id], true
	}
	return nil, r, reviewing{}, false
}

// startReview has a fresh agent review id's run in runs, done: task says
// what was designed, shots what it looks like now.
func (s *Server) startReview(runs reviewStore, id string, r reviewing, task string, brief *proto.DesignBrief, shots []reviewShot) {
	select {
	case <-s.done:
		return // its screenshots may be missing for that, and its agent wouldn't last
	default:
	}
	round, ok := runs.reviewRound(id, r)
	if !ok {
		return
	}
	fail := func(err error) {
		s.logger().Warn("design review: start", "run", id, "err", err)
		if run, ok := runs.dropReview(id); ok {
			s.broadcast(r.event, run)
		}
		if r.lead != "" {
			_ = s.input(proto.SessionInputParams{ID: r.lead, Text: "The independent review couldn't start (" + err.Error() + "): look at the screenshots yourself against the brief before you stop.", Paste: true, Enter: true})
		}
	}
	dir := filepath.Join(s.dataDir, "reviews", fmt.Sprintf("%s-%d", reviewFolder(id), round))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		fail(err)
		return
	}
	var names []string
	for _, sh := range shots {
		if len(sh.png) == 0 {
			continue
		}
		if err := os.WriteFile(filepath.Join(dir, sh.name), sh.png, 0o600); err != nil {
			fail(err)
			return
		}
		names = append(names, sh.name)
	}
	if len(names) == 0 {
		fail(fmt.Errorf("no screenshots"))
		return
	}
	meta, _ := json.Marshal(reviewMeta{ID: id, Round: round, Lead: r.lead, Again: r.again})
	if err := os.WriteFile(filepath.Join(dir, reviewMetaFile), meta, 0o600); err != nil {
		fail(err)
		return
	}
	md := "(none written)"
	if brief != nil {
		md = designbrief.Markdown(*brief)
	}
	agentID := "claude"
	if sess, err := s.sessions.Get(r.lead); err == nil {
		agentID = sess.Meta().Agent
	}
	sess, err := s.createSession(proto.SessionCreateParams{Agent: agentID, Dir: dir, Group: "review:" + id,
		Name: fmt.Sprintf("Review · round %d", round), Cols: 100, Rows: 40,
		Prompt: designreview.Ask(task, md, names, `"$ENOWX_BIN" review -`)})
	if err != nil {
		fail(err)
		return
	}
	if run, ok := runs.reviewer(id, sess.ID()); ok {
		s.broadcast(r.event, run)
	}
}

// reviewFolder is run id as a folder's name.
func reviewFolder(id string) string {
	return strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == ':' {
			return '-'
		}
		return r
	}, id)
}

// designReview takes a reviewer's verdict.
func (s *Server) designReview(p proto.DesignReviewParams) (proto.DesignReview, error) {
	r, err := designreview.Parse(p.Text)
	if err != nil {
		return r, fmt.Errorf("%w\n\nWrite it in this form:\n%s", err, designreview.Format)
	}
	for _, runs := range []reviewStore{&s.unslopRuns, &s.browserUnslops, &s.canvasBuilds} {
		run, rv, how, ok := runs.verdict(p.Session, r)
		if !ok {
			continue
		}
		s.broadcast(how.event, run)
		if how.lead != "" {
			s.tellSession(proto.SessionInputParams{ID: how.lead, Text: designreview.Told(rv, how.again), Paste: true, Enter: true}, "design review: tell the agent")
		}
		s.helperFinished(p.Session) // its verdict is its work: it stays on the run
		return rv, nil
	}
	// The daemon restarted since (an update): the review's folder says
	// which design it was and whom to tell.
	if sess, err := s.sessions.Get(p.Session); err == nil {
		if data, err := os.ReadFile(filepath.Join(sess.Info().Dir, reviewMetaFile)); err == nil {
			var m reviewMeta
			if json.Unmarshal(data, &m) == nil && m.ID != "" {
				r.Round, r.Session = m.Round, p.Session
				if m.Lead != "" {
					s.tellSession(proto.SessionInputParams{ID: m.Lead, Text: designreview.Told(r, m.Again), Paste: true, Enter: true}, "design review: tell the agent")
				}
				s.helperFinished(p.Session)
				return r, nil
			}
		}
	}
	return r, fmt.Errorf("review: this session reviews no design")
}

// reviewMetaFile, in a reviewer's folder, says what it reviews, so its
// verdict finds its way after the daemon restarted.
const reviewMetaFile = ".review.json"

type reviewMeta struct {
	ID    string `json:"id"`
	Round int    `json:"round"`
	Lead  string `json:"lead,omitempty"`
	Again string `json:"again,omitempty"`
}

// reviewCanvas has canvas cv's design f, its unslop done, reviewed.
func (s *Server) reviewCanvas(cv proto.Canvas, f proto.CanvasFrame, n int, lead string) {
	at := fmt.Sprintf(`canvas_unslop (canvas %q, frame %q, pass "done")`, cv.ID, f.ID)
	s.startReview(&s.unslopRuns, cv.ID, reviewing{lead: lead, again: at, event: proto.EventCanvasUnslop},
		fmt.Sprintf("design %d (“%s”) of the canvas “%s”, a mockup", n, f.Name, cv.Title), cv.Brief, s.canvasShots(cv, f))
}

// canvasShots are design f of canvas cv rendered at its width and, a wide
// one, a phone's.
func (s *Server) canvasShots(cv proto.Canvas, f proto.CanvasFrame) []reviewShot {
	assets := func(name string) ([]byte, error) { return s.canvasFor(cv.ID).Asset(cv.ID, name) }
	shot := func(w, h int) []byte {
		g := f
		g.Width, g.Height = w, h
		out, err := s.appRender(proto.CanvasRenderRequest{HTML: canvas.FrameDocument(g, assets), Width: w, Height: h, Format: proto.RenderPNG})
		if err != nil {
			return nil
		}
		return out.Data
	}
	shots := []reviewShot{{"1-desktop.png", shot(f.Width, f.Height)}}
	if f.Width <= 600 {
		return shots
	}
	// A canvas with a phone design of its own shows that one, not the
	// desktop design squeezed.
	for _, g := range cv.Frames {
		if g.ID != f.ID && g.Width > 0 && g.Width <= 500 && strings.TrimSpace(g.HTML) != "" {
			f = g
			return append(shots, reviewShot{"2-phone.png", shot(g.Width, g.Height)})
		}
	}
	return append(shots, reviewShot{"2-phone.png", shot(390, max(844, f.Height*390/max(f.Width, 1)))})
}

// reviewPage has session's page, its unslop done, reviewed from the
// screenshots taken when it was.
func (s *Server) reviewPage(session, u string) {
	shots, _ := s.browserUnslops.shotsOf(session)
	var rs []reviewShot
	for i, sh := range shots {
		rs = append(rs, reviewShot{fmt.Sprintf("%d-%s-%dpx.png", i+1, strings.ToLower(strings.ReplaceAll(sh.Label, " ", "-")), sh.Width), sh.After})
	}
	brief, _ := s.browserUnslops.briefOf(session)
	s.startReview(&s.browserUnslops, session, reviewing{lead: session, again: `"$ENOWX_BIN" browser unslop --pass done`, event: proto.EventBrowserUnslop},
		"the page "+u+" of a project, as it runs.\n\n"+designreview.Existing, brief, rs)
}

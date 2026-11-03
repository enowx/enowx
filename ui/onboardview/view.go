// Package onboardview is the first run: the app assembles itself, frames drawing
// themselves and each piece arriving where it belongs, while the user sets it up in the
// panel that holds the side panel's place. It can't be skipped, and nothing in it
// needs an agent or the network: what's optional has Later.
package onboardview

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
)

// Flow is who is going through it.
type Flow int

const (
	FlowNew       Flow = iota // everything
	FlowReturning             // their settings to confirm, filled in, then the community
	FlowDevice                // done on this account: a new device's own steps
)

// stop is where the clock waits.
type stop struct {
	id string
	at float32
	// step is its number of the seven the panel counts, 0 for none. The opening is the
	// first, so agents are the second; the project and a host are one step, as the design.
	step int
	name string // a detour's word in the panel's head
}

// stopsOf are where flow's clock waits.
func stopsOf(f Flow, _ bool) []stop {
	switch f {
	case FlowReturning:
		return []stop{{id: "back", at: tWhole}, {id: "community", at: tCommunity}, {id: "end", at: tEnd}}
	case FlowDevice:
		return []stop{{id: "agents", at: tAgents, step: 1}, {id: "host", at: tHost, step: 2}, {id: "hostdone", at: tHostDone, step: 2}, {id: "end", at: tEnd}}
	}
	return []stop{{id: "agents", at: tAgents, step: 2}, {id: "project", at: tProject, step: 3}, {id: "host", at: tHost, step: 3}, {id: "hostdone", at: tHostDone, step: 3},
		{id: "style", at: tStyle, step: 4}, {id: "goa", at: tGOA, step: 5}, {id: "task", at: tTask, step: 6}, {id: "running", at: tRunning, step: 6}, {id: "community", at: tCommunity, step: 7}, {id: "end", at: tEnd}}
}

// skipsOf are the stretches flow's clock doesn't run through: the design's tour of the
// styles, and what isn't this flow's.
func skipsOf(f Flow) []skip {
	switch f {
	case FlowReturning:
		return []skip{{tWhole, 45000}}
	case FlowDevice:
		return []skip{{13300, 14900}, {19250, 45650}}
	}
	return []skip{{tStyle, tStyleEnd}}
}

// startAt is when flow's clock starts: the opening, for everyone the first time they
// get this flow, returning users too; the frames on a new device, whose account has seen
// it. Less motion holds on the logo, whole.
func startAt(f Flow, reduce bool) float32 {
	switch {
	case f == FlowDevice:
		return tFramesFrom
	case reduce:
		return reduceOpening
	}
	return 0
}

const (
	// reduceOpening is the moment less motion holds on: the logo and its name, still.
	reduceOpening = 5950
	reduceHold    = 2200 * time.Millisecond
)

// The window's background, as the Style step offers it.
const (
	BackgroundDefault = "default"
	BackgroundPlain   = "plain"
	BackgroundImage   = "image" // an image of the user's
)

// Options are what onboarding gets of the app: it reads real settings, and acts through
// these.
type Options struct {
	Flow     Flow
	HostName string
	// Device is what the panel calls this computer ("this Mac").
	Device string
	// Reduce: less motion was asked for, by the system or the app.
	Reduce bool

	Agents       []proto.AgentInfo
	AgentsLoaded bool
	Projects     []proto.ProjectInfo
	OpenFolder   func()
	OpenClone    func()
	Hosts        []string // saved besides this one
	AddHost      func(address string)

	// Wallpaper draws the user's own image, when they chose one.
	Wallpaper func(c *ui.Context)
	// BackgroundMode is one of the Background constants; SetBackground asks for one,
	// BackgroundImage by choosing an image.
	BackgroundMode string
	SetBackground  func(mode string)
	// How opaque the panels over the user's image are, 0 to 1, and its blur.
	PanelOpacity    float32
	SetPanelOpacity func(float32)
	BlurValue       float32
	BlurMost        float32
	SetBlur         func(float32)
	StyleID         string
	SetStyle        func(id string)
	PaletteID       string
	SetPalette      func(id string) // "" the style's own

	GOAAgents []proto.GOAAgent
	StartGOA  func(agent string, done func(error))
	GOAOn     bool

	// StartTask starts a session on project with prompt.
	StartTask func(agent, project, prompt string, done func(error))
	OpenLink  func(url string)
	// Done ends onboarding; the app takes over.
	Done func()
}

// View is the flow's state. It belongs to the main thread.
type View struct {
	clock clock
	flow  Flow
	stops []stop
	at    int // the stop the clock is at or going to
	begun bool

	hostsAtStart    int
	projectsAtStart int
	projectPaths    map[string]bool
	projectMoved    bool
	hostAddress     string
	wireOn          bool // a host was added: its wire plays
	pick            int  // which card the project came from: its folder flies from there
	goaAgent        string
	goaBusy         bool
	goaErr          string
	goaChip         ui.Rect // the bar's chip, as last laid out: GOD of Agents' node leaves it
	task            taskState
	detour          *stop // a step "Your settings" sent the user to change

	morph  morph
	looks  map[string]look
	lookOf string // what looks were made for
	reduce bool   // less motion: what travels is cut to
	hold   time.Time
}

// New is onboarding, not begun.
func New() *View { return &View{} }

// begin starts the clock for o's flow.
func (v *View) begin(o Options) {
	v.begun, v.flow, v.stops, v.reduce = true, o.Flow, stopsOf(o.Flow, o.Reduce), o.Reduce
	v.clock = clock{t: startAt(o.Flow, o.Reduce), target: v.stops[0].at, skips: skipsOf(o.Flow)}
	if o.Flow == FlowReturning {
		v.clock.rush = [2]float32{tOpeningEnd, tWhole}
	}
	v.hostsAtStart, v.projectsAtStart = len(o.Hosts), len(o.Projects)
	v.projectPaths = map[string]bool{}
	for _, p := range o.Projects {
		v.projectPaths[p.Path] = true
	}
	if o.Reduce && o.Flow != FlowDevice {
		v.clock.target = v.clock.t
		v.hold = time.Now().Add(reduceHold)
	}
	if o.Reduce && o.Flow == FlowDevice {
		v.clock.t = v.clock.target
	}
}

// lookFor is style id's look with the palette and background as they are, kept until
// those change: a frame needn't work them out again.
func (v *View) lookFor(id string, o Options) look {
	key := fmt.Sprint(o.PaletteID, "|", o.BackgroundMode, "|", o.PanelOpacity, "|", o.BlurValue)
	if key != v.lookOf || v.looks == nil {
		v.lookOf, v.looks = key, map[string]look{}
	}
	l, ok := v.looks[id]
	if !ok {
		l = lookOf(id, o.PaletteID, o)
		v.looks[id] = l
	}
	return l
}

// project is the project this flow is about: the one the user just added, or their first.
func (v *View) project(o Options) (proto.ProjectInfo, bool) {
	for _, p := range o.Projects {
		if !v.projectPaths[p.Path] {
			return p, true
		}
	}
	if len(o.Projects) > 0 {
		return o.Projects[0], true
	}
	return proto.ProjectInfo{}, false
}

func (v *View) projectName(o Options) string {
	p, ok := v.project(o)
	if !ok {
		return ""
	}
	return nonEmpty(p.Name, p.Path)
}

func (v *View) current() stop {
	if v.detour != nil {
		return *v.detour
	}
	return v.stops[v.at]
}

// next goes on to the next stop.
func (v *View) next(o Options) {
	if v.detour != nil {
		v.detour = nil
		return
	}
	if v.at < len(v.stops)-1 {
		v.at++
	}
	v.goTo(o)
}

// back goes to the stop before.
func (v *View) back(o Options) {
	if v.detour != nil {
		v.detour = nil
		return
	}
	if v.at == 0 {
		return
	}
	left := v.stops[v.at]
	v.at--
	switch {
	case left.id == "hostdone" && !v.wireOn:
		// Nothing played there: straight back to the form it was skipped from.
		v.clock.t = v.stops[v.at].at
	case v.stops[v.at].id == "running" && !v.task.sent:
		v.at-- // no task ran: back to where it can be sent from
	}
	v.goTo(o)
}

func (v *View) goTo(o Options) {
	v.clock.target = v.current().at
	if o.Reduce {
		v.clock.t = v.clock.target
	}
}

// Covers says it hides the whole window, so nothing needs building under it: until it
// gives way to the app in its last moments.
func (v *View) Covers() bool { return !v.begun || v.clock.t < tEnd-handover }

// handover is how long onboarding fades into the app that's under it by then.
const handover = 300

// Draw draws it over the whole window.
func (v *View) Draw(c *ui.Context, o Options) {
	if !v.begun {
		v.begin(o)
	}
	now := time.Now()
	w, h := c.Size()
	v.morph.step(o, now)
	if !v.hold.IsZero() && now.After(v.hold) {
		v.hold = time.Time{}
		v.clock.t, v.clock.target = v.stops[0].at, v.stops[0].at
	}
	if v.clock.tick(now) || v.morph.running() || !v.hold.IsZero() {
		c.After(16 * time.Millisecond)
	}
	t := v.clock.t
	if t >= tEnd && v.current().id == "end" {
		o.Done()
	}
	s := v.sceneAt(o, w, h, t)
	root := ui.Box(c.Key("onboarding")).Absolute().Left(0).Top(0).Right(0).Bottom(0).Clip().Opacity(1 - seg(t, tEnd-handover, tEnd, easeOut))
	root.Children(func() {
		v.window(c, o, s)
		v.opening(c, o, w, h, t)
	})
}

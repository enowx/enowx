// Package onboardview is the first run: the app assembles itself, frames drawing
// themselves and each piece arriving where it belongs, while the user sets it up in the
// panel that holds the side panel's place. It can't be skipped, and nothing in it
// needs an agent or the network: what's optional has Later.
package onboardview

import (
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
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
	// step is its number of the seven the panel counts, 0 for none.
	step int
}

// stopsOf are where flow's clock waits.
func stopsOf(f Flow, _ bool) []stop {
	switch f {
	case FlowReturning:
		return []stop{{"back", tPanelTo + 400, 0}, {"community", tCommunity, 0}, {"end", tEnd, 0}}
	case FlowDevice:
		return []stop{{"agents", tAgents, 1}, {"host", tHost, 2}, {"end", tEnd, 0}}
	}
	return []stop{{"agents", tAgents, 1}, {"project", tProject, 2}, {"host", tHost, 3}, {"style", tStyle, 4}, {"goa", tGOA, 5}, {"task", tTask, 6}, {"community", tCommunity, 7}, {"end", tEnd, 0}}
}

func stepsShown(stops []stop) int {
	n := 0
	for _, s := range stops {
		if s.step > 0 {
			n++
		}
	}
	return n
}

// startAt is when flow's clock starts: the opening for a first run; the frames for one
// who has seen it. Less motion comes in where the opening holds still.
func startAt(f Flow, reduce bool) float32 {
	switch {
	case f != FlowNew:
		return tFramesFrom
	case reduce:
		return tOpeningEnd - reduceOpening
	}
	return 0
}

// reduceOpening is how long the opening stays with less motion: the logo and name still.
const reduceOpening = 1600

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
	Rescan       func()
	Projects     []proto.ProjectInfo
	OpenFolder   func()
	OpenClone    func()
	Hosts        []string // saved besides this one
	AddHost      func()

	// Wallpaper draws what lies under glass.
	Wallpaper  func(c *ui.Context)
	StyleID    string
	SetStyle   func(id string)
	PaletteID  string
	SetPalette func(id string) // "" the style's own

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

	hostsAtStart int
	wireOn       bool // a host was added: its wire plays
	goaAgent     string
	goaBusy      bool
	goaErr       string
	task         taskState
	detour       *stop // a step "Your settings" sent the user to change

	morph morph
}

// New is onboarding, not begun.
func New() *View { return &View{} }

// begin starts the clock for o's flow.
func (v *View) begin(o Options) {
	v.begun, v.flow, v.stops = true, o.Flow, stopsOf(o.Flow, o.Reduce)
	v.clock = clock{t: startAt(o.Flow, o.Reduce), target: v.stops[0].at}
	v.hostsAtStart = len(o.Hosts)
	if o.Reduce {
		// With less motion nothing travels: past the opening, each stop is cut to.
		v.clock.target = v.clock.t + reduceOpening
	}
}

func (v *View) current() stop {
	if v.detour != nil {
		return *v.detour
	}
	return v.stops[v.at]
}

// settle says the flow's first stop follows the opening, once that has held, with less
// motion.
func (v *View) settle(o Options) {
	if o.Reduce && v.at == 0 && v.clock.t == v.clock.target && v.clock.target != v.stops[0].at {
		v.clock.t, v.clock.target = v.stops[0].at, v.stops[0].at
	}
}

// skipEmpty has the clock not sit through stretches where nothing moves: once the
// window is whole, stops are a cut apart, and a returning user's window is whole at once.
func (v *View) skipEmpty() {
	k := &v.clock
	if k.t == k.target {
		return
	}
	from, to := float32(tMaterialTo+200), float32(tFold)
	if v.flow == FlowReturning {
		from = tPanelTo + 400
	}
	if k.t >= from && k.t <= to {
		k.t = clamp(k.target, from, to)
	}
}

// next goes on to the next stop.
func (v *View) next(o Options) {
	if v.detour != nil {
		v.detour = nil
	} else if v.at < len(v.stops)-1 {
		v.at++
	}
	v.goTo(o)
}

// back goes to the stop before.
func (v *View) back(o Options) {
	if v.detour != nil {
		v.detour = nil
	} else if v.at > 0 {
		v.at--
	}
	v.goTo(o)
}

func (v *View) goTo(o Options) {
	v.clock.target = v.current().at
	if o.Reduce {
		v.clock.t = v.clock.target
	}
}

// Draw draws it over the whole window.
func (v *View) Draw(c *ui.Context, o Options) {
	if !v.begun {
		v.begin(o)
	}
	v.skipEmpty()
	now := time.Now()
	w, h := c.Size()
	v.morph.step(o, w, h, now)
	if v.clock.tick(now) || v.morph.running() {
		c.After(16 * time.Millisecond)
	}
	v.settle(o)
	t := v.clock.t
	if t >= tEnd && v.current().id == "end" {
		o.Done()
	}
	g := layoutFor(w, h, theme.StyleByID(o.StyleID))
	root := ui.Box(c.Key("onboarding")).Absolute().Left(0).Top(0).Right(0).Bottom(0).Clip().Opacity(1 - seg(t, tEnd-300, tEnd, easeOut))
	root.Children(func() {
		v.ground(c, o, t)
		v.shell(c, o, g, t)
		v.opening(c, o, w, h, t)
	})
}

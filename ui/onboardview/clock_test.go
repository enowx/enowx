package onboardview

import (
	"testing"
	"time"
)

// The clock runs to its stop and waits there, and back faster than forward.
func TestClockStops(t *testing.T) {
	now := time.Now()
	k := clock{target: 100, last: now}
	if !k.tick(now.Add(50*time.Millisecond)) || k.t != 50 {
		t.Fatalf("forward: %v", k.t)
	}
	k.tick(now.Add(100 * time.Millisecond))
	if k.t != 100 || k.tick(now.Add(150*time.Millisecond)) {
		t.Fatalf("at its stop: %v", k.t)
	}
	k.target = 0
	k.tick(now.Add(170 * time.Millisecond))
	if want := float32(100 - 20*backSpeed); k.t != want {
		t.Fatalf("back: %v, want %v", k.t, want)
	}
}

// Each flow's stops rise, end at the end, and a new user sees all seven steps.
func TestFlows(t *testing.T) {
	for _, f := range []Flow{FlowNew, FlowReturning, FlowDevice} {
		stops := stopsOf(f, true)
		for i := 1; i < len(stops); i++ {
			if stops[i].at <= stops[i-1].at {
				t.Errorf("flow %d: stop %d at %v", f, i, stops[i].at)
			}
		}
		if stops[len(stops)-1].at != tEnd {
			t.Errorf("flow %d ends at %v", f, stops[len(stops)-1].at)
		}
	}
	if n := stepsShown(stopsOf(FlowNew, true)); n != 7 {
		t.Errorf("a new user sees %d steps", n)
	}
	// No Discord link yet: its card hides, the others stay.
	if got := communityLinks(); len(got) != 2 {
		t.Errorf("links: %+v", got)
	}
}

// The opening starts a first run; a returning user comes in at the frames.
func TestStartTimes(t *testing.T) {
	if startAt(FlowNew, false) != 0 {
		t.Error("a new user skipped the opening")
	}
	if startAt(FlowReturning, false) < tOpeningEnd {
		t.Error("a returning user sat through the opening")
	}
}

// Once the window is whole, going on to a stop doesn't sit through empty time.
func TestSkipEmpty(t *testing.T) {
	v := New()
	v.begin(Options{Flow: FlowNew})
	v.clock.t, v.clock.target = tStyle, tGOA
	v.skipEmpty()
	if v.clock.t != tGOA {
		t.Errorf("at %v", v.clock.t)
	}
	v.clock.t, v.clock.target = tGOA, tEnd
	v.skipEmpty()
	if v.clock.t != tFold {
		t.Errorf("the fold was skipped: at %v", v.clock.t)
	}
	v.clock.t, v.clock.target = tHost, tStyle
	v.skipEmpty()
	if v.clock.t != tHost {
		t.Errorf("the regions' material was skipped: at %v", v.clock.t)
	}
}

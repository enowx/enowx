package onboardview

import (
	"math"
	"time"
)

// The whole flow is a function of one clock, as in the design: what's on screen follows
// from the time alone, so going back is the same film run backward. A click moves the
// clock to the next stop, where it waits for the user.

// Where the clock waits for the user, in milliseconds, as in the design. The stretch
// from the Style stop to tStyleEnd is the design's tour of the styles: the app plays a
// change only when one is picked, so the clock jumps over it.
const (
	tOpeningEnd = 7000 // the opening has given way to frames drawing themselves
	tFramesFrom = 7050
	tAgents     = 11800
	tProject    = 13800
	tHost       = 15300
	tHostDone   = 17100
	tStyle      = 20000
	tStyleEnd   = 36800
	tGOA        = 39400
	tTask       = 41200
	tRunning    = 44600
	tCommunity  = 45500
	tEnd        = 47000
	// A returning user's window is whole here, and the community is what's left.
	tWhole = 19999
	// What going back runs at, faster than forward.
	backSpeed = 1.8
)

// skip is a stretch the clock doesn't run through, either way, on its way past it.
type skip struct{ from, to float32 }

// clock is the flow's time. It belongs to the main thread.
type clock struct {
	t, target float32
	last      time.Time
	skips     []skip
	rush      [2]float32 // a stretch that runs at rushSpeed
}

// rushSpeed is how fast a returning user's window assembles.
const rushSpeed = 3

// tick moves the clock to now, and says whether it's still moving.
func (k *clock) tick(now time.Time) bool {
	if k.last.IsZero() {
		k.last = now
	}
	d := float32(min(now.Sub(k.last).Seconds(), 0.1) * 1000)
	k.last = now
	switch {
	case k.t < k.target:
		if k.t >= k.rush[0] && k.t < k.rush[1] {
			d *= rushSpeed
		}
		k.t = min(k.t+d, k.target)
		for _, s := range k.skips {
			if k.t >= s.from && k.t < s.to && k.target >= s.to {
				k.t = s.to
			}
		}
	case k.t > k.target:
		k.t = max(k.t-d*backSpeed, k.target)
		for _, s := range k.skips {
			if k.t <= s.to && k.t > s.from && k.target <= s.from {
				k.t = s.from
			}
		}
	}
	return k.t != k.target
}

func clamp(x, a, b float32) float32 { return max(a, min(x, b)) }

func lerp(a, b, x float32) float32 { return a + (b-a)*x }

// The design's curves: what enters, what crosses, what leaves, what settles with weight.
func easeOut(x float32) float32 { return 1 - pow(1-x, 3) }

func easeOutQuint(x float32) float32 { return 1 - pow(1-x, 5) }

func easeInOut(x float32) float32 {
	if x < .5 {
		return 4 * x * x * x
	}
	return 1 - pow(-2*x+2, 3)/2
}

func easeIn(x float32) float32 { return x * x * x }

func easeBack(x float32) float32 {
	const c = 1.15
	y := x - 1
	return 1 + (c+1)*y*y*y + c*y*y
}

func pow(x float32, n float64) float32 { return float32(math.Pow(float64(x), n)) }

// seg is how far along time a to b the clock t is, on a curve.
func seg(t, a, b float32, curve func(float32) float32) float32 {
	return curve(clamp((t-a)/(b-a), 0, 1))
}

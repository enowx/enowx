package onboardview

import (
	"testing"
	"time"
)

// The app takes the style at once while the window crosses to it, region after region,
// and a pick made meanwhile follows.
func TestMorphWave(t *testing.T) {
	var set []string
	o := Options{StyleID: "ledger", SetStyle: func(id string) { set = append(set, id) }}
	var m morph
	now := time.Now()
	m.begin(o, "tonal", now)
	if len(set) != 1 || set[0] != "tonal" {
		t.Fatalf("the app has %v", set)
	}
	m.queued = "bento"
	m.step(o, now.Add(morphLead+morphMove/2))
	from, to, p := m.at("tonal")
	if from != "ledger" || to != "tonal" || p < .4 || p > .6 {
		t.Fatalf("halfway: %s %s %v", from, to, p)
	}
	o.StyleID = "tonal"
	m.step(o, now.Add(morphLead+morphMove+morphRest))
	if from, to, p := m.at("bento"); from != "tonal" || to != "bento" || p != 0 {
		t.Fatalf("what was picked meanwhile: %s %s %v", from, to, p)
	}
}

// Regions cross one after another: the first is further along than the last.
func TestWaveOrder(t *testing.T) {
	v := New()
	o := pictureOptions("")
	v.begin(o)
	now := time.Now()
	v.morph.begin(Options{StyleID: "liquid-glass", SetStyle: func(string) {}}, "ledger", now)
	v.morph.step(o, now.Add(morphLead+morphMove*2/5))
	o.StyleID = "ledger"
	s := v.sceneAt(o, 1440, 900, tStyle)
	if s.ctx.r.y >= s.rail.r.y || s.ctx.v.m >= s.rail.v.m {
		t.Fatalf("the bar's %v, the dock's %v", s.ctx.v.m, s.rail.v.m)
	}
}

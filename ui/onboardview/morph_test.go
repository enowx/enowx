package onboardview

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// The style changes only once what's in the regions has left, the regions start one
// after another, and a pick made meanwhile follows.
func TestMorphWave(t *testing.T) {
	var set []string
	o := Options{StyleID: "liquid", SetStyle: func(id string) { set = append(set, id) }}
	var m morph
	m.start(o, "ledger")
	now := m.began
	m.step(o, 1440, 900, now.Add(morphFade/2))
	if len(set) != 0 || m.content(now.Add(morphFade/2)) >= 1 {
		t.Fatalf("changed early: %v", set)
	}
	m.step(o, 1440, 900, now.Add(morphFade))
	if len(set) != 1 || set[0] != "ledger" {
		t.Fatalf("not changed: %v", set)
	}
	m.step(o, 1440, 900, now.Add(morphFade+morphMove/4))
	if first, last := m.x(delayCtx), m.x(delayRail); first <= last {
		t.Errorf("no wave: first %v, last %v", first, last)
	}
	m.start(o, "tonal")
	m.step(o, 1440, 900, now.Add(morphFade+morphMove+time.Millisecond))
	if m.to != "tonal" || !m.running() {
		t.Errorf("the pick made meanwhile: %q", m.to)
	}
}

// With less motion a style just changes.
func TestMorphReduced(t *testing.T) {
	var set string
	var m morph
	m.start(Options{Reduce: true, SetStyle: func(id string) { set = id }}, "bento")
	if set != "bento" || m.running() {
		t.Fatalf("%q, running %v", set, m.running())
	}
}

// Colors cross without going muddy: halfway from one to itself is itself.
func TestMixColor(t *testing.T) {
	c := ui.RGBA(47, 111, 237, 1)
	got := mixColor(c, c, .5)
	if d := int(got.R) - 47; d < -2 || d > 2 {
		t.Errorf("%+v", got)
	}
	if mixColor(c, ui.RGBA(0, 0, 0, 1), 0) != c {
		t.Error("not where it starts")
	}
}

package onboardview

import "time"

// The hero: picking a style plays one change across the window, a wave, each region
// starting a beat after the last (scene.go) with its place, corners, colors and font
// crossing together. Everything here is painted from looks, so it runs on its own clock.

const (
	morphLead = 150 * time.Millisecond // before the first region moves
	morphMove = 1100 * time.Millisecond
	morphRest = 150 * time.Millisecond // after, before another may start
)

// morph is a change of style under way. It belongs to the main thread.
type morph struct {
	began    time.Time // zero for none
	from, to string
	queued   string
	now      time.Time
}

func (m *morph) running() bool { return !m.began.IsZero() }

// start changes to style id: the app takes it at once, what's drawn crosses to it.
func (m *morph) start(o Options, id string) {
	switch {
	case o.Reduce:
		o.SetStyle(id)
	case m.running():
		m.queued = id
	default:
		m.begin(o, id, time.Now())
	}
}

func (m *morph) begin(o Options, id string, now time.Time) {
	m.began, m.from, m.to, m.now = now, o.StyleID, id, now
	o.SetStyle(id)
}

// step moves it on to now.
func (m *morph) step(o Options, now time.Time) {
	m.now = now
	if !m.running() || now.Sub(m.began) < morphLead+morphMove+morphRest {
		return
	}
	m.began = time.Time{}
	if m.queued != "" {
		id := m.queued
		m.queued = ""
		m.begin(o, id, now)
	}
}

// at is what the window is changing between and how far along it is, current being
// the style the app has.
func (m *morph) at(current string) (from, to string, p float32) {
	if !m.running() {
		return current, current, 0
	}
	return m.from, m.to, clamp(float32(m.now.Sub(m.began)-morphLead)/float32(morphMove), 0, 1)
}

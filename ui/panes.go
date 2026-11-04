package ui

import (
	"cmp"
	"errors"
	"slices"
	"time"
)

// A session's terminal in the app holds its whole scrollback, up to about 10 MB, and
// parses all it prints. The daemon holds all of that anyway, so the app attaches to a
// session when something shows or types into it, and leaves those out of sight for a
// while: coming back shows them again from the daemon.

const (
	// paneKeep is how long a terminal stays after it was last drawn.
	paneKeep = 3 * time.Minute
	// panesKept are the terminals last drawn that stay however long ago: going back to
	// them shouldn't take attaching.
	panesKept = 3
	// paneWait is how long something typed waits for a terminal being attached.
	paneWait = 5 * time.Second
)

// wantPane is session id's terminal, nil until attached, and starts attaching it. It runs
// on the main thread, in the frames showing it, which is what keeps it from unloadPanes.
func (a *App) wantPane(id string) *pane {
	if a.paneSeen == nil {
		a.paneSeen = map[string]time.Time{}
	}
	a.paneSeen[id] = time.Now()
	p, ok := a.panes[id]
	if !ok && a.client != nil {
		a.panes[id] = nil // attaching
		go a.attach(a.client, a.gen, id)
	}
	return p
}

// withPane has use called on the main thread once session id has a terminal, or failed
// if it can't soon: what is typed into a session out of sight waits for attaching.
func (a *App) withPane(id string, use func(*pane), failed func()) {
	if p := a.wantPane(id); p != nil {
		use(p)
		return
	}
	deadline := time.Now().Add(paneWait)
	var again func()
	again = func() {
		p, attaching := a.panes[id]
		switch {
		case p != nil:
			use(p)
		case !attaching || time.Now().After(deadline):
			failed()
		default:
			time.AfterFunc(50*time.Millisecond, func() { a.onMain(again) })
		}
	}
	time.AfterFunc(50*time.Millisecond, func() { a.onMain(again) })
}

// canvasKeep is how long the Canvas page's web view and pictures stay once it is left.
const canvasKeep = 2 * time.Minute

// unloadCanvasPage lets go of the Canvas page's heavy parts once it has been left a while.
// It runs on the main thread.
func (a *App) unloadCanvasPage(now time.Time) {
	switch {
	case a.page == pageCanvas:
		a.canvasLeft = time.Time{}
	case a.canvasLeft.IsZero():
		a.canvasLeft = now
	case now.Sub(a.canvasLeft) > canvasKeep:
		a.canvases.Unload()
	}
}

// errNotAttached is what withPane's failure comes to.
var errNotAttached = errors.New("the session isn't attached yet")

// unloadPanes leaves the terminals out of sight. It runs on the main thread, and what
// asks for it is followed by a frame, so a terminal on screen was seen just now however
// still the window has been.
func (a *App) unloadPanes(now time.Time) {
	for _, id := range panesToUnload(a.panes, a.paneSeen, now) {
		a.panes[id].close()
		delete(a.panes, id)
		delete(a.paneSeen, id)
	}
	for id := range a.paneSeen {
		if _, ok := a.panes[id]; !ok {
			delete(a.paneSeen, id)
		}
	}
}

// panesToUnload are the terminals past paneKeep beyond the panesKept last drawn. One still
// attaching stays: it isn't there to close yet.
func panesToUnload(panes map[string]*pane, seen map[string]time.Time, now time.Time) []string {
	var ids []string
	for id, p := range panes {
		if p != nil {
			ids = append(ids, id)
		}
	}
	slices.SortFunc(ids, func(a, b string) int {
		if c := seen[b].Compare(seen[a]); c != 0 {
			return c
		}
		return cmp.Compare(a, b)
	})
	var out []string
	for i, id := range ids {
		if i >= panesKept && now.Sub(seen[id]) > paneKeep {
			out = append(out, id)
		}
	}
	return out
}

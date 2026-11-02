package usersync

import "testing"

// A canvas's versions stay on their host; the canvas itself and an asset named alike go.
func TestCanvasHistoryIsntSynced(t *testing.T) {
	for p, want := range map[string]bool{
		"canvas/0123456789ab/canvas.json":        true,
		"canvas/0123456789ab/assets/history.png": true,
		"canvas/0123456789ab/assets/history/a":   true,
		"canvas/0123456789ab/history.json":       false,
		"canvas/0123456789ab/history/12.json":    false,
		"canvas/0123456789ab/history/frames/a":   false,
	} {
		if got := Synced(p); got != want {
			t.Errorf("%s: %v", p, got)
		}
	}
}

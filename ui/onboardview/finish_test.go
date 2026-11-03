package onboardview

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// With no agents, no projects, no hosts and nothing else to be had, every step still has a
// way on, and it finishes.
func TestFinishesWithNothing(t *testing.T) {
	done := false
	o := Options{Flow: FlowNew, HostName: "this Mac", Device: "this Mac", Reduce: true, AgentsLoaded: true,
		BackgroundMode: BackgroundDefault, PanelOpacity: 1, BlurMost: 40,
		OpenLink: func(string) {}, OpenFolder: func() {}, OpenClone: func() {}, AddHost: func(string) {}, SetStyle: func(string) {}, SetPalette: func(string) {},
		SetBackground: func(string) {}, SetPanelOpacity: func(float32) {}, SetBlur: func(float32) {}, Done: func() { done = true }}
	v := New()
	v.begin(o)
	v.hold = time.Time{}
	v.clock.t, v.clock.target = tAgents, tAgents
	tt := ui.NewTester(func(c *ui.Context) { v.Draw(c, o) }, 1440, 900)
	for _, step := range []string{"Continue", "Later", "Later", "Continue", "Continue", "Later", "Later", "Continue"} {
		tt.Frame()
		if err := tt.Click(step); err != nil {
			t.Fatalf("at %q (%s): %v in %q", step, v.current().id, err, tt.Texts())
		}
		tt.Frame()
		if done {
			break
		}
	}
	tt.Advance(16 * time.Millisecond)
	if !done {
		t.Fatalf("stuck at %s, t %v: %q", v.current().id, v.clock.t, tt.Texts())
	}
}

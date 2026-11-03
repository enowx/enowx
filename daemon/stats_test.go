package daemon

import (
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/enowx/enowx/core/session"
	"github.com/enowx/enowx/proto"
)

func TestStatsMeasureEachSession(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("not measured on " + runtime.GOOS)
	}
	srv := startServer(t)
	c := connect(t, srv, nil)
	busy, err := srv.sessions.Create(session.Spec{Command: []string{"sh", "-c", "while :; do :; done"}, Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	idle, err := srv.sessions.Create(session.Spec{Command: []string{"sleep", "30"}, Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	// The first sample to see the busy session's processes has no CPU for
	// them yet: wait for one.
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(minSample) {
		st, err := c.Stats()
		if err != nil {
			t.Fatal(err)
		}
		if slices.ContainsFunc(st.Sessions, func(u proto.SessionUsage) bool { return u.ID == busy.ID() && u.Procs >= 2 }) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the busy session's processes never showed: %+v", st.Sessions)
		}
	}
	// A machine busy with other tests can starve the session for a moment, or wake the idle
	// one: at some look, busy uses a core and idle next to nothing.
	for deadline := time.Now().Add(30 * time.Second); ; {
		time.Sleep(minSample + 200*time.Millisecond)
		st, err := c.Stats()
		if err != nil {
			t.Fatal(err)
		}
		if !st.Supported || st.Host.MemTotal == 0 || st.Daemon.RSS == 0 {
			t.Fatalf("stats = %+v", st)
		}
		got := map[string]float64{}
		for _, s := range st.Sessions {
			if s.RSS == 0 || s.Procs == 0 {
				t.Fatalf("session %s measured nothing: %+v", s.ID, s)
			}
			got[s.ID] = s.CPU
		}
		cpu, measured := got[idle.ID()]
		if got[busy.ID()] >= 50 && measured && cpu <= 10 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("busy session CPU = %.0f%%, want a core; idle = %.0f%% (measured %v): %+v", got[busy.ID()], cpu, measured, st.Sessions)
		}
	}
}

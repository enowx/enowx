package session

import (
	"testing"
	"time"

	"github.com/enowx/enowx/proto"
)

func TestWorkingGoesStaleWhenTheScreenStandsStill(t *testing.T) {
	m := NewManager()
	s, err := m.Create(Spec{Command: []string{"sleep", "30"}, Cols: 80, Rows: 24})
	if err != nil {
		t.Skip(err) // no terminal emulator
	}
	defer m.CloseAll()
	if err := m.SetState(s, proto.StateWorking); err != nil {
		t.Fatal(err)
	}
	if got := s.Info().State; got != proto.StateWorking {
		t.Fatalf("state %q", got)
	}
	past := time.Now().Add(-2 * staleWorking)
	s.mu.Lock()
	s.stateAt, s.lastChange = past, past
	s.mu.Unlock()
	if got := s.Info(); got.State != "" || !got.StateStale {
		t.Fatalf("a still screen since %v: state %q, stale %v", staleWorking*2, got.State, got.StateStale)
	}
	// A screen that keeps changing is still working.
	s.mu.Lock()
	s.lastChange = time.Now()
	s.mu.Unlock()
	if got := s.Info(); got.State != proto.StateWorking || got.StateStale {
		t.Fatalf("a changing screen: state %q, stale %v", got.State, got.StateStale)
	}
}

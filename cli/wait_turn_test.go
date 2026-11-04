package cli

import (
	"testing"
	"time"

	"github.com/enowx/enowx/proto"
)

// A wait ends only when the agent truly ended its turn or asks: not at a Stop followed by
// more, not on an agent long at one step with its screen still.
func TestWaitOnlyAtTheTurnsEnd(t *testing.T) {
	start := time.Now()
	at := func(d time.Duration) time.Time { return start.Add(d) }
	done := proto.SessionInfo{Agent: "claude", State: proto.StateDone}
	busy := proto.SessionInfo{Agent: "claude", State: proto.StateWorking, Active: true}

	var w turnWait
	if w.over(done, at(0)) || w.over(done, at(doneSettles-time.Second)) {
		t.Error("done ended it at once")
	}
	if w.over(busy, at(doneSettles-time.Millisecond)) {
		t.Error("working ended it")
	}
	if w.over(done, at(doneSettles)) {
		t.Error("the first done counted, across a step at work")
	}
	if !w.over(done, at(2*doneSettles)) {
		t.Error("done that held didn't end it")
	}

	// Done with the screen moving: more is coming.
	w = turnWait{}
	moving := done
	moving.Active = true
	if w.over(moving, at(0)) || w.over(moving, at(time.Hour)) {
		t.Error("done with its screen moving ended it")
	}

	// A long quiet step: its state went stale, and it never ends the wait.
	w = turnWait{}
	stale := proto.SessionInfo{Agent: "claude", StateStale: true}
	if w.over(stale, at(0)) || w.over(stale, at(time.Hour)) {
		t.Error("a long step ended it")
	}

	// Quiet with hooks takes longer than quiet without.
	w = turnWait{}
	hooked := proto.SessionInfo{Agent: "claude"}
	if w.over(hooked, at(0)) || w.over(hooked, at(quietFor)) {
		t.Error("an agent with hooks was done by a moment's quiet")
	}
	if !w.over(hooked, at(hookedQuietFor)) {
		t.Error("it never ended")
	}
	w = turnWait{}
	plain := proto.SessionInfo{Agent: "aider"}
	if w.over(plain, at(0)) || !w.over(plain, at(quietFor)) {
		t.Error("an agent without hooks goes by its screen")
	}

	// Asking or gone ends it at once.
	w = turnWait{}
	if !w.over(proto.SessionInfo{Agent: "claude", State: proto.StateWaiting}, at(0)) {
		t.Error("asking")
	}
	if !w.over(proto.SessionInfo{Agent: "claude", Exited: true}, at(0)) {
		t.Error("exited")
	}
}

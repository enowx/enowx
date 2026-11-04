package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/enowx/enowx/proto"
)

// A session whose turn ended with an agent in its background doesn't read as done or idle, and its
// row says what it runs, for how long, and what it uses.
func TestSessionWithABackgroundAgent(t *testing.T) {
	now := time.Now()
	s := proto.SessionInfo{State: proto.StateDone, Background: []proto.BackgroundAgent{{PID: 9, Name: "codex", Command: "codex exec x",
		Since: now.Add(-3 * time.Minute), CPUPercent: 12, Memory: 100 << 20, Stuck: true}}}
	if got := statusOf(s).label; got != "background" {
		t.Fatalf("state: %q", got)
	}
	short, full := backgroundLine(s, now)
	for _, want := range []string{"1 in background", "codex", "12%", "100"} {
		if !strings.Contains(short, want) {
			t.Errorf("row %q lacks %q", short, want)
		}
	}
	if !strings.Contains(full, "codex exec x") || !strings.Contains(full, "no CPU") {
		t.Errorf("hover: %q", full)
	}
	if s.State, s.Background = proto.StateDone, nil; statusOf(s).label != "done" {
		t.Fatal("none left isn't done")
	}
}

// An update says which background agents its restart stops.
func TestUpdateNamesBackgroundAgents(t *testing.T) {
	if got := backgroundAtRisk([]proto.SessionInfo{{State: proto.StateDone}}); got != "" {
		t.Fatalf("with none: %q", got)
	}
	got := backgroundAtRisk([]proto.SessionInfo{{Background: []proto.BackgroundAgent{{Name: "codex"}}}, {Background: []proto.BackgroundAgent{{Name: "kilo"}}}})
	if !strings.Contains(got, "2 background agents") || !strings.Contains(got, "codex, kilo") || !strings.Contains(got, "lost") {
		t.Fatalf("%q", got)
	}
}

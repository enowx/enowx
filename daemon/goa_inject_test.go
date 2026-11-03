package daemon

import (
	"encoding/json"
	"strings"
	"testing"
)

// An agent's hook gets lines in the shape its documentation gives.
func TestHookOutputs(t *testing.T) {
	var out struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
		Hook     struct {
			Event   string `json:"hookEventName"`
			Context string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	read := func(s string) {
		t.Helper()
		out.Decision, out.Reason, out.Hook.Event, out.Hook.Context = "", "", "", ""
		if err := json.Unmarshal([]byte(s), &out); err != nil {
			t.Fatal(err)
		}
	}
	read(contextOutput([]byte(`{"hook_event_name":"PostToolUseFailure"}`), "a line"))
	if out.Hook.Event != "PostToolUseFailure" || out.Hook.Context != "a line" {
		t.Errorf("a tool's end: %+v", out)
	}
	read(continueOutput([]byte(`{"hook_event_name":"Stop"}`), "a line"))
	if out.Hook.Event != "Stop" || out.Hook.Context != "a line" || out.Decision != "" {
		t.Errorf("Claude Code's turn's end: %+v", out)
	}
	read(continueOutput([]byte(`{"hook_event_name":"Stop","turn_id":"t1"}`), "a line"))
	if out.Decision != "block" || out.Reason != "a line" {
		t.Errorf("Codex's turn's end: %+v", out)
	}
}

// A later hook takes over from one before, which ends with nothing to say; lines go to
// the latest and none is lost if it has gone.
func TestWakerTakesOver(t *testing.T) {
	srv := startServer(t)
	w := &goaWaker{text: make(chan string, 1)}
	srv.goa.mu.Lock()
	srv.goa.waker = w
	srv.goa.mu.Unlock()
	lines := srv.goaWaitingLines()
	if lines != nil {
		t.Fatalf("waiting: %v", lines)
	}
	srv.goa.mu.Lock()
	srv.goa.tell = []goaLine{{Text: "first"}, {Text: "second"}}
	srv.goa.mu.Unlock()
	if got := srv.goaTake(); !strings.Contains(got, "first") || !strings.Contains(got, "second") {
		t.Fatalf("took %q", got)
	}
	if srv.goaWaitingLines() != nil {
		t.Fatal("what was taken still waits")
	}
}

// What doesn't fit an agent's context stays for the next chance.
func TestGoaTakeKeepsTheRest(t *testing.T) {
	srv := startServer(t)
	long := strings.Repeat("x", goaContextMost-5)
	srv.goa.mu.Lock()
	srv.goa.tell = []goaLine{{Text: long}, {Text: "the next"}}
	srv.goa.mu.Unlock()
	if got := srv.goaTake(); got != long {
		t.Fatalf("took %d", len(got))
	}
	if left := srv.goaWaitingLines(); len(left) != 1 || left[0] != "the next" {
		t.Fatalf("left %v", left)
	}
}

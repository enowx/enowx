package daemon

import (
	"strings"
	"testing"
	"time"

	"github.com/enowx/enowx/proto"
)

// A finished turn on a screen no hook speaks for is a report, not a stall; a
// session only quiet stays one.
func TestTurnOverOnScreen(t *testing.T) {
	const over = "The report is in.\n✻ Baked for 27s\n"
	still := 30 * time.Second
	for name, c := range map[string]struct {
		look   stallLook
		screen string
		want   bool
	}{
		"in a shell tab":   {stallLook{Still: still}, over, true},
		"its hooks quiet":  {stallLook{Info: proto.SessionInfo{StateStale: true}, Still: still}, over, true},
		"its hooks at it":  {stallLook{Info: proto.SessionInfo{State: proto.StateWorking}, Still: still}, over, false},
		"too soon":         {stallLook{Still: time.Second}, over, false},
		"something runs":   {stallLook{Still: still, CPU: 80}, over, false},
		"only quiet":       {stallLook{Still: still}, "compiling the thing\n", false},
		"cannot see a CPU": {stallLook{Still: still, NoCPU: true, CPU: 80}, over, true},
	} {
		if got := turnOverOnScreen(c.look, c.screen); got != c.want {
			t.Errorf("%s: %v", name, got)
		}
	}
}

// A report's outcome is one of those there are; it goes on the task once, and
// reaches GOA by the session's own change.
func TestGOAReportGoesOnTheTask(t *testing.T) {
	srv := startServer(t)
	task, err := srv.goa.board.Add(proto.GOATask{Title: "x", Session: "s1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.goaReport(proto.GOAReportParams{Session: "s1", Outcome: "maybe"}); err == nil {
		t.Fatal("an outcome there isn't")
	}
	info := proto.SessionInfo{ID: "s1", Report: "no access", ReportOutcome: "blocked", ReportAt: 5}
	srv.goaReportSeen(info)
	srv.goaReportSeen(info)
	found := false
	for _, x := range srv.goa.board.List() {
		if x.ID == task.ID {
			found = x.Outcome == "blocked" && strings.Contains(x.Said, "no access") && x.ReportAt == 5
		}
	}
	if !found {
		t.Fatal("not on the task")
	}
}

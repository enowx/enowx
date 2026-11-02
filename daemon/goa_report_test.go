package daemon

import (
	"strings"
	"testing"
	"time"

	"github.com/enowx/enowx/core/goa"
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
		"in a shell tab":    {stallLook{Still: still}, over, true},
		"its hooks quiet":   {stallLook{Info: proto.SessionInfo{StateStale: true}, Still: still}, over, true},
		"its hooks at it":   {stallLook{Info: proto.SessionInfo{State: proto.StateWorking}, Still: still}, over, false},
		"its hooks dead":    {stallLook{Info: proto.SessionInfo{State: proto.StateDone}, Still: still}, over, true},
		"at rest, not at 0": {stallLook{Still: still, CPU: 1.6}, over, true},
		"too soon":          {stallLook{Still: time.Second}, over, false},
		"something runs":    {stallLook{Still: still, CPU: 80}, over, false},
		"only quiet":        {stallLook{Still: still}, "compiling the thing\n", false},
		"cannot see a CPU":  {stallLook{Still: still, NoCPU: true, CPU: 80}, over, true},
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

// A session GOA sent something to with no task reports like a worker, once per
// thing to say, and one nothing was sent to isn't followed.
func TestToldSessionReports(t *testing.T) {
	srv := startServer(t)
	done := proto.SessionInfo{ID: "s2", State: proto.StateDone}
	srv.goaToldChanged(done)
	srv.goa.mu.Lock()
	followed := len(srv.goa.told) != 0
	srv.goa.told["s2"] = goaTold{}
	srv.goa.mu.Unlock()
	if followed {
		t.Fatal("followed unasked")
	}
	srv.goaToldChanged(done)
	srv.goa.mu.Lock()
	heard := srv.goa.told["s2"].Status
	srv.goa.mu.Unlock()
	if heard != proto.GOATaskTurnEnded {
		t.Fatalf("heard %q", heard)
	}
	srv.goaToldChanged(proto.SessionInfo{ID: "s2", Exited: true, ExitCode: 3})
	srv.goa.mu.Lock()
	_, still := srv.goa.told["s2"]
	srv.goa.mu.Unlock()
	if still {
		t.Fatal("still followed after its end")
	}
}

// A session that ended with no report is marked so, and said to be; one that
// reported keeps its words.
func TestEndedWithNoReportIsMarked(t *testing.T) {
	srv := startServer(t)
	task, err := srv.goa.board.Add(proto.GOATask{Title: "x", Session: "gone-session"})
	if err != nil {
		t.Fatal(err)
	}
	got := srv.goaNoteEnded(task)
	if !got.NoReport || !strings.Contains(goaReportWord(got), "no report") {
		t.Fatalf("%+v", got)
	}
	reported, err := srv.goa.board.Add(proto.GOATask{Title: "y", Session: "other", Outcome: "failed", Said: "no access"})
	if err != nil {
		t.Fatal(err)
	}
	got = srv.goaNoteEnded(reported)
	if got.NoReport || got.Said != "no access" {
		t.Fatalf("%+v", got)
	}
}

// A host out of reach is told of once its idle limit has passed.
func TestHostLostIsTold(t *testing.T) {
	srv := startServer(t)
	task, err := srv.goa.board.Add(proto.GOATask{Title: "x", Session: "s", Host: "far", Status: proto.GOATaskWorking})
	if err != nil {
		t.Fatal(err)
	}
	w := &stallWatch{lost: map[string]time.Time{"far": time.Now().Add(-time.Hour)}}
	srv.stallHostLost(w, "far", task, proto.GOAStall{Idle: 60})
	for _, x := range srv.goa.board.List() {
		if x.ID == task.ID && x.Status != proto.GOATaskStalled {
			t.Fatalf("%+v", x)
		}
	}
}

// A session GOA sent something to, with no task and no hook, is told of by its screen:
// its turn's end once, then nothing until GOA sends it more.
func TestToldSessionReportsByItsScreen(t *testing.T) {
	info := proto.SessionInfo{ID: "s3"}
	const over = "The report is in.\n✻ Baked for 27s\n"
	h := stallHost{infos: map[string]proto.SessionInfo{"s3": info}, cpuKnown: true,
		cpuOf:  func(string) float64 { return 1.6 },
		screen: func(string) (string, error) { return over, nil }}
	lim := goa.StallOr(proto.GOAStall{})
	srv := startServer(t)
	srv.goa.mu.Lock()
	srv.goa.told["s3"] = goaTold{}
	srv.goa.mu.Unlock()
	status := func() string {
		srv.goa.mu.Lock()
		defer srv.goa.mu.Unlock()
		return srv.goa.told["s3"].Status
	}
	w := &stallWatch{follows: map[string]*stallFollow{}, sampled: map[string]bool{}}
	srv.toldCheck(w, "s3", goaTold{}, h, lim)
	if got := status(); got != "" {
		t.Fatalf("at first sight: %q", got)
	}
	w.follows[toldFollow("s3")].since = time.Now().Add(-time.Minute)
	srv.toldCheck(w, "s3", goaTold{}, h, lim)
	if got := status(); got != proto.GOATaskTurnEnded {
		t.Fatalf("after its turn's end: %q", got)
	}
}

// One whose screen says nothing of its turn is told of once it has been quiet for
// the idle limit; one asking something isn't.
func TestToldGoneQuiet(t *testing.T) {
	lim := goa.StallOr(proto.GOAStall{})
	long := stallSeconds(lim.Idle) + time.Second
	for name, c := range map[string]struct {
		look   stallLook
		screen string
		want   bool
	}{
		"quiet":        {stallLook{Still: long}, "something\n", true},
		"not for long": {stallLook{Still: time.Second}, "something\n", false},
		"at work":      {stallLook{Still: long, CPU: 90}, "something\n", false},
		"hooks at it":  {stallLook{Still: long, Info: proto.SessionInfo{State: proto.StateWorking}}, "something\n", false},
		"it asks":      {stallLook{Still: long}, "Do you want to proceed?\n", false},
	} {
		if got := toldGoneQuiet(c.look, c.screen, lim); got != c.want {
			t.Errorf("%s: %v", name, got)
		}
	}
}

// A task given to a session that has reported already doesn't take that report: only
// what comes after it. One queued meanwhile takes none, and starts clean when given.
func TestNewTaskDoesntTakeAnOldReport(t *testing.T) {
	srv := startServer(t)
	old := proto.SessionInfo{ID: "s4", ReportAt: 1000, ReportOutcome: "done", Report: "the task before's"}
	task, err := srv.goaAddTask(proto.GOADelegateParams{}, proto.GOATask{Session: "s4", Status: proto.GOATaskWorking, Title: "new"}, old)
	if err != nil {
		t.Fatal(err)
	}
	srv.goaReportSeen(old)
	if got := boardTask(t, srv, task.ID); got.Said != "" || got.Outcome != "" {
		t.Fatalf("took the old report: %+v", got)
	}
	queued, err := srv.goaAddTask(proto.GOADelegateParams{}, proto.GOATask{Session: "s4", Status: proto.GOATaskQueued, Title: "next"}, old)
	if err != nil {
		t.Fatal(err)
	}
	fresh := old
	fresh.ReportAt, fresh.Report = 2000, "this task's"
	srv.goaReportSeen(fresh)
	if got := boardTask(t, srv, task.ID); got.Said != "this task's" {
		t.Fatalf("missed its own report: %+v", got)
	}
	if got := boardTask(t, srv, queued.ID); got.Said != "" || got.ReportAt == 2000 {
		t.Fatalf("a queued task took a report: %+v", got)
	}
}

func boardTask(t *testing.T, srv *Server, id string) proto.GOATask {
	t.Helper()
	for _, task := range srv.goa.board.List() {
		if task.ID == id {
			return task
		}
	}
	t.Fatalf("no task %s", id)
	return proto.GOATask{}
}

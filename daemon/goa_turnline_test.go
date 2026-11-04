package daemon

import (
	"strings"
	"testing"
	"time"

	"github.com/enowx/enowx/core/session"
	"github.com/enowx/enowx/proto"
)

// turnFixture is a server with a busy GOA, so lines wait to be read, and a worker's
// session with task id's.
func turnFixture(t *testing.T) (*Server, *session.Session, proto.GOATask) {
	t.Helper()
	srv := startServer(t)
	g, err := srv.sessions.Create(session.Spec{Command: []string{"sh", "-c", "cat"}, Dir: srv.goaDir(), Cols: 80, Rows: 24, Meta: session.Meta{Agent: "codex", GOA: true}})
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.sessions.SetState(g, proto.StateWorking); err != nil {
		t.Fatal(err)
	}
	w, err := srv.sessions.Create(session.Spec{Command: []string{"sh", "-c", "cat"}, Dir: t.TempDir(), Cols: 80, Rows: 24, Meta: session.Meta{Agent: "claude"}})
	if err != nil {
		t.Fatal(err)
	}
	task, err := srv.goa.board.Add(proto.GOATask{Title: "Totals", Session: w.ID(), Status: proto.GOATaskWorking})
	if err != nil {
		t.Fatal(err)
	}
	return srv, w, task
}

func waitingOf(srv *Server) []string { return srv.goaWaitingLines() }

func report(srv *Server, w *session.Session, state, text string, at int64) {
	srv.goaReportSeen(proto.SessionInfo{ID: w.ID(), State: state, Report: text, ReportOutcome: "done", ReportAt: at})
}

func turnDone(t *testing.T, srv *Server, w *session.Session, said string) {
	t.Helper()
	time.Sleep(goaTurnLeast)
	srv.sessions.NoteTurnText(w, said)
	if err := srv.sessions.SetState(w, proto.StateDone); err != nil {
		t.Fatal(err)
	}
	srv.goaWorkerChanged(w.Info())
}

func oneLineWith(t *testing.T, lines []string, want ...string) {
	t.Helper()
	if len(lines) != 1 {
		t.Fatalf("%d lines: %q", len(lines), lines)
	}
	for _, w := range want {
		if !strings.Contains(lines[0], w) {
			t.Errorf("no %q in %q", w, lines[0])
		}
	}
}

// The usual order: it reports as its last act, then its turn ends. One line, with its
// real last message; held meanwhile, not said alone.
func TestReportThenTurnEndIsOneLine(t *testing.T) {
	srv, w, _ := turnFixture(t)
	report(srv, w, proto.StateWorking, "All done.", 1)
	if said := srv.goaLinesToSay(); len(said) != 0 {
		t.Fatalf("said before its turn ended: %+v", said)
	}
	turnDone(t, srv, w, "FINAL message")
	oneLineWith(t, waitingOf(srv), "reports done: All done.", "FINAL message")
	if said := srv.goaLinesToSay(); len(said) != 1 {
		t.Fatalf("not free to go: %+v", said)
	}
}

// Its turn's end first, as another host's updates can come: one line, while it waits.
func TestTurnEndThenReportIsOneLine(t *testing.T) {
	srv, w, _ := turnFixture(t)
	turnDone(t, srv, w, "FINAL message")
	report(srv, w, proto.StateDone, "All done.", 1)
	oneLineWith(t, waitingOf(srv), "reports done: All done.", "FINAL message")
}

// A report saying what the message says doesn't say it twice.
func TestSameTextIsSaidOnce(t *testing.T) {
	srv, w, _ := turnFixture(t)
	report(srv, w, proto.StateWorking, "All done.", 1)
	turnDone(t, srv, w, "All done.")
	lines := waitingOf(srv)
	if len(lines) != 1 || strings.Count(lines[0], "All done.") != 1 {
		t.Fatalf("%q", lines)
	}
}

// The turn's last message is the transcript's only once it ends on one: what comes
// before a tool's call, or a later prompt, is older, and waits.
func TestTurnEndInTranscript(t *testing.T) {
	a := func(text string) proto.TranscriptEntry {
		return proto.TranscriptEntry{Kind: proto.TranscriptAssistant, Text: text}
	}
	tool := proto.TranscriptEntry{Kind: proto.TranscriptTool}
	user := proto.TranscriptEntry{Kind: proto.TranscriptUser}
	thinking := proto.TranscriptEntry{Kind: proto.TranscriptThinking}
	for name, c := range map[string]struct {
		entries []proto.TranscriptEntry
		want    string
		ok      bool
	}{
		"written":             {[]proto.TranscriptEntry{user, a("interim"), tool, a("final")}, "final", true},
		"thinking after":      {[]proto.TranscriptEntry{a("final"), thinking}, "final", true},
		"late: after a tool":  {[]proto.TranscriptEntry{user, a("interim"), tool}, "", false},
		"late: only a prompt": {[]proto.TranscriptEntry{a("the turn before's"), user}, "", false},
		"none yet":            {nil, "", false},
	} {
		if got, ok := turnEndIn(c.entries); got != c.want || ok != c.ok {
			t.Errorf("%s: %q, %v", name, got, ok)
		}
	}
}

// A transcript written late doesn't give an old message: the hook's says, for a session
// whose agent gives it, whatever the transcript holds.
func TestHookTextGoesPastALateTranscript(t *testing.T) {
	srv, w := hookServer(t)
	t.Setenv(session.EnvSession, w.ID())
	if _, _, _ = hookOf(`{"last_assistant_message":"FINAL message"}`, "--event", proto.HookTurnDone); w.Info().TurnText != "FINAL message" {
		t.Fatalf("%q", w.Info().TurnText)
	}
	if _, _, _ = hookOf(`{"prompt":"next"}`, "--event", proto.HookPromptStart); w.Info().TurnText != "" {
		t.Fatalf("still %q as a turn started", w.Info().TurnText)
	}
	_ = srv
}

// While GOA is busy, each real turn keeps its own line; the same turn's report joins.
func TestTwoRealTurnsHaveTwoLines(t *testing.T) {
	srv, w, _ := turnFixture(t)
	report(srv, w, proto.StateWorking, "First done.", 1)
	turnDone(t, srv, w, "first message")
	srv.goaTurnStarted(w.ID())
	_, _ = srv.goa.board.Update(srv.goa.board.List()[0].ID, func(x *proto.GOATask) { x.Status = proto.GOATaskWorking })
	report(srv, w, proto.StateWorking, "Second done.", 2)
	turnDone(t, srv, w, "second message")
	lines := waitingOf(srv)
	if len(lines) != 2 || !strings.Contains(lines[0], "first message") || !strings.Contains(lines[1], "second message") {
		t.Fatalf("%q", lines)
	}
}

// A session with no task goes the same way.
func TestSessionWithNoTaskIsOneLine(t *testing.T) {
	srv := startServer(t)
	g, err := srv.sessions.Create(session.Spec{Command: []string{"sh", "-c", "cat"}, Dir: srv.goaDir(), Cols: 80, Rows: 24, Meta: session.Meta{Agent: "codex", GOA: true}})
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.sessions.SetState(g, proto.StateWorking); err != nil {
		t.Fatal(err)
	}
	const id = "worker-no-task"
	srv.goaMarkTold(g.ID(), "", id)
	at := time.Now().UnixMilli() + 1000 // after it was sent something
	srv.goaToldChanged(proto.SessionInfo{ID: id, State: proto.StateWorking, Report: "All done.", ReportOutcome: "done", ReportAt: at})
	srv.goaToldChanged(proto.SessionInfo{ID: id, State: proto.StateDone, TurnText: "FINAL message", Report: "All done.", ReportOutcome: "done", ReportAt: at})
	oneLineWith(t, waitingOf(srv), "reports done: All done.", "FINAL message")
}

// A report goes alone if its turn's end is long in coming, and a question or failure is
// never merged: it comes after what was held, in order.
func TestHeldReportIsNotLost(t *testing.T) {
	srv, w, _ := turnFixture(t)
	report(srv, w, proto.StateWorking, "All done.", 1)
	srv.goa.mu.Lock()
	srv.goa.tell[0].HoldUntil = time.Now().Add(-time.Second)
	srv.goa.mu.Unlock()
	if said := srv.goaLinesToSay(); len(said) != 1 || !strings.Contains(said[0].Text, "All done.") {
		t.Fatalf("%+v", said)
	}
	report(srv, w, proto.StateWorking, "Second.", 2)
	srv.goaTellLine(goaLine{Session: w.ID(), Text: "[enowX GOA] it asks something"})
	said := srv.goaLinesToSay()
	if len(said) != 2 || !strings.Contains(said[0].Text, "Second.") || !strings.Contains(said[1].Text, "asks something") {
		t.Fatalf("%+v", said)
	}
}

// A line held back for one session doesn't hold another's.
func TestHoldIsPerSession(t *testing.T) {
	srv, w, _ := turnFixture(t)
	report(srv, w, proto.StateWorking, "All done.", 1)
	srv.goaTellLine(goaLine{Session: "another", Text: "[enowX GOA] another's"})
	said := srv.goaLinesToSay()
	if len(said) != 1 || !strings.Contains(said[0].Text, "another's") {
		t.Fatalf("%+v", said)
	}
	if lines := waitingOf(srv); len(lines) != 1 || !strings.Contains(lines[0], "All done.") {
		t.Fatalf("%q", lines)
	}
}

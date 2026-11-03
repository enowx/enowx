package daemon

import (
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/enowx/enowx/core/session"
	"github.com/enowx/enowx/proto"
)

// A queued fixture has a headless terminal but starts no program or daemon.
// Its screen can look empty or misleading without touching a real composer.
func goaComposerFixture(t *testing.T, srv *Server, screen string) *session.Session {
	t.Helper()
	g, err := srv.sessions.Create(session.Spec{ID: "isolated-goa", Cols: 120, Rows: 40,
		History: []byte(screen), Meta: session.Meta{Agent: "codex", GOA: true, Queued: true}})
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func TestGOAComposerNeverReceivesBackgroundReports(t *testing.T) {
	for name, screen := range map[string]string{
		"empty":           "Done.\r\n› Ask Codex to do anything\r\n",
		"draft":           "› my unfinished sentence\r\nsecond line\r\n",
		"misleading mark": "› my unfinished sentence\r\nquoted prompt:\r\n$ \r\n",
		"mouse fragment":  "› draft 5;38;39M;35;12;24M\r\n",
	} {
		t.Run(name, func(t *testing.T) {
			srv := startServer(t)
			g := goaComposerFixture(t, srv, screen)
			before := g.Terminal().Text()
			idleBefore := g.TypedFor(time.Now().Add(time.Minute))
			var wg sync.WaitGroup
			for range 20 {
				wg.Go(func() {
					srv.goaTellLine(goaLine{Session: "worker-a", Text: "[enowX GOA] identical report"})
					srv.goaFlush(g.ID())
				})
			}
			wg.Wait()
			if got := g.Terminal().Text(); got != before {
				t.Fatalf("draft changed: %q -> %q", before, got)
			}
			if g.Info().Prompt != "" || g.TypedFor(time.Now().Add(time.Minute)) < idleBefore-time.Second {
				t.Fatal("background delivery typed or submitted input")
			}
			if got := srv.goaBoard().Waiting; len(got) != 1 || got[0] != "[enowX GOA] identical report" {
				t.Fatalf("Tasks waiting: %v", got)
			}
			data, err := os.ReadFile(srv.goa.tellFile)
			var kept []goaLine
			if err != nil || json.Unmarshal(data, &kept) != nil || len(kept) != 1 {
				t.Fatalf("queue wasn't preserved: %s (%v)", data, err)
			}
		})
	}
}

func TestGOAReportsDeduplicateAcrossNewTimestamps(t *testing.T) {
	srv := startServer(t)
	g := goaComposerFixture(t, srv, "› my draft\r\n")
	task, err := srv.goa.board.Add(proto.GOATask{Title: "Worker", Session: "worker-a"})
	if err != nil {
		t.Fatal(err)
	}
	info := proto.SessionInfo{ID: "worker-a", Report: "The requested changes are ready for review.", ReportOutcome: "partly"}
	for at := int64(1); at <= 5; at++ {
		info.ReportAt = at
		srv.goaReportSeen(info)
	}
	if got := srv.goaWaitingLines(); len(got) != 1 {
		t.Fatalf("repeated reports: %v", got)
	}
	var decision proto.HookDecision
	srv.goaHookLines(g, g.Info(), proto.HookEvent{Event: proto.HookPromptStart}, &decision)
	if got := decision.Context; strings.Count(got, "[enowX GOA]") != 1 {
		t.Fatalf("hook received %q", got)
	}
	info.ReportAt = 6
	srv.goaReportSeen(info)
	if len(srv.goaWaitingLines()) != 0 {
		t.Fatal("already delivered report was requeued with a newer timestamp")
	}
	info.ReportAt, info.Report = 7, "A changed report"
	srv.goaReportSeen(info)
	if got := srv.goaWaitingLines(); len(got) != 1 || !strings.Contains(got[0], "A changed report") {
		t.Fatalf("changed report lost: %v", got)
	}
	for _, got := range srv.goa.board.List() {
		if got.ID == task.ID && (got.ReportAt != 7 || got.Said != info.Report) {
			t.Fatalf("board didn't retain latest report: %+v", got)
		}
	}
}

func TestGOAQueueRestartAndCancelledWakePreserveIdentity(t *testing.T) {
	srv := startServer(t)
	g := goaComposerFixture(t, srv, "› draft\r\n")
	one, _ := srv.goa.board.Add(proto.GOATask{Title: "one", Session: "worker-a"})
	two, _ := srv.goa.board.Add(proto.GOATask{Title: "two", Session: "other"})
	a := goaLine{Task: one.ID, Session: "worker-a", Text: "same report"}
	b := goaLine{Task: two.ID, Session: "other", Text: "same report"}
	legacy, _ := json.Marshal([]goaLine{a, a, b, a})
	if err := os.WriteFile(srv.goa.tellFile, legacy, 0o600); err != nil {
		t.Fatal(err)
	}
	srv.loadGOATell()
	if got := srv.goaWaitingLines(); len(got) != 2 {
		t.Fatalf("restart: %v", got)
	}
	w := &goaWaker{text: make(chan string, 1)}
	srv.goa.mu.Lock()
	srv.goa.waker = w
	srv.goa.mu.Unlock()
	srv.goaFlush(g.ID())
	srv.goaReturnWake(w) // disconnected after handoff, before reading the channel
	srv.goa.mu.Lock()
	kept := append([]goaLine(nil), srv.goa.tell...)
	srv.goa.mu.Unlock()
	if len(kept) != 2 || kept[0] != a || kept[1] != b {
		t.Fatalf("cancelled hook lost task metadata/order: %+v", kept)
	}
	if got := srv.goaTake(); strings.Count(got, "same report") != 2 {
		t.Fatalf("hook delivery: %q", got)
	}
	if len(srv.goaWaitingLines()) != 0 {
		t.Fatal("delivered lines still queued")
	}
}

func TestGOAConcurrentHookRoutesClaimEachReportOnce(t *testing.T) {
	srv := startServer(t)
	g := goaComposerFixture(t, srv, "› draft\r\n")
	for _, text := range []string{"report-one", "report-two", "report-three"} {
		srv.goaTellLine(goaLine{Text: text})
	}
	w := &goaWaker{text: make(chan string, 1)}
	srv.goa.mu.Lock()
	srv.goa.waker = w
	srv.goa.mu.Unlock()
	context := make(chan string, 1)
	var wg sync.WaitGroup
	wg.Go(func() { srv.goaFlush(g.ID()) })
	wg.Go(func() { context <- srv.goaTake() })
	wg.Wait()
	got := <-context
	select {
	case wake := <-w.text:
		got += wake
	default:
	}
	for _, text := range []string{"report-one", "report-two", "report-three"} {
		if strings.Count(got, text) != 1 {
			t.Fatalf("lost/duplicated report %s: %q", text, got)
		}
	}
	if len(srv.goaWaitingLines()) != 0 {
		t.Fatal("reports were requeued")
	}
}

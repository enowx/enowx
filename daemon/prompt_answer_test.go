package daemon

import (
	"encoding/json"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/session"
	"github.com/enowx/enowx/proto"
)

const dangerPrompt = `Bash command
  "$S/go.sh" build ./ui
Dangerous rm operation not ruled out
Claude Code will automatically block this request in 0:30
Do you want to proceed?
 1. Yes
 2. No
`

// promptOnScreen is a session showing the prompt, its program echoing what it
// is sent, and the prompt as the watch reads it.
func promptOnScreen(t *testing.T, srv *Server, meta session.Meta) (*session.Session, agent.ScreenPrompt) {
	t.Helper()
	sess, err := srv.sessions.Create(session.Spec{Command: []string{"cat"}, Dir: t.TempDir(), Cols: 100, Rows: 24, Meta: meta})
	if err != nil {
		t.Fatal(err)
	}
	srv.sessions.Input(sess, []byte(strings.ReplaceAll(dangerPrompt, "\n", "\r")))
	var p agent.ScreenPrompt
	waitFor(t, "the prompt on screen", func() bool {
		var ok bool
		p, ok = agent.PromptShown(sess.Terminal().Text())
		return ok
	})
	return sess, p
}

// The judge answers what it is sure of, and logs it with what was asked; what it
// isn't sure of reaches the owner, with the time the prompt gives.
func TestJudgeAnswersAPermissionPrompt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses cat")
	}
	srv := startServer(t)
	var mu sync.Mutex
	var told []string
	connect(t, srv, func(ev proto.Event) {
		if ev.Name == proto.EventNotifySound {
			var p proto.NotifySoundParams
			json.Unmarshal(ev.Data, &p)
			mu.Lock()
			told = append(told, p.Text)
			mu.Unlock()
		}
	})
	set := func(min float64) {
		t.Helper()
		if err := srv.judge.Settings().Set(proto.DecideSettings{Policy: proto.DecideLocalOnly,
			Own:  []proto.Judge{{Kind: proto.JudgeLocal, URL: judgeServer(t), Model: "judge"}},
			Uses: map[string]proto.DecideUse{proto.DecideApprove: {On: true, MinConfidence: min}}}); err != nil {
			t.Fatal(err)
		}
	}
	set(0.9)
	sess, p := promptOnScreen(t, srv, session.Meta{Agent: "claude"})
	srv.promptSeen(sess.Info(), p, sess.Terminal().Text())
	log := srv.judge.Log(proto.DecideLogParams{Use: proto.DecideApprove, Session: sess.ID()})
	if len(log) != 1 || log[0].Acted != proto.ActedAllowed || !strings.Contains(log[0].Input, "Dangerous rm") {
		t.Fatalf("log: %+v", log)
	}
	mu.Lock()
	n := len(told)
	mu.Unlock()
	if n != 0 {
		t.Fatalf("the owner was told: %q", told)
	}

	set(0.99) // the test judge says 0.95
	sess, p = promptOnScreen(t, srv, session.Meta{Agent: "claude"})
	srv.promptSeen(sess.Info(), p, sess.Terminal().Text())
	waitFor(t, "the owner told, with the time", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(told) == 1 && strings.Contains(told[0], "0:30")
	})
}

// With no judge, GOA answers its worker's prompt, told at once, unless it is the
// user's; a session of no GOA goes to the owner.
func TestGOATakesAWorkersPromptWithNoJudge(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses cat")
	}
	srv := startServer(t)
	g, err := srv.sessions.Create(session.Spec{Command: []string{"cat"}, Dir: t.TempDir(), Cols: 200, Rows: 40, Meta: session.Meta{Agent: "claude", GOA: true}})
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var told []string
	connect(t, srv, func(ev proto.Event) {
		if ev.Name == proto.EventNotifySound {
			var p proto.NotifySoundParams
			json.Unmarshal(ev.Data, &p)
			mu.Lock()
			told = append(told, p.Text)
			mu.Unlock()
		}
	})
	worker, p := promptOnScreen(t, srv, session.Meta{Agent: "claude", GOAWorker: g.ID()})
	srv.promptSeen(worker.Info(), p, worker.Terminal().Text())
	waitFor(t, "GOA told", func() bool { return strings.Contains(g.Terminal().Text(), "shows a permission prompt") })
	if srv.goaUrgentOf(worker.ID()) != "" {
		t.Fatal("marked the user's")
	}
	outside, p := promptOnScreen(t, srv, session.Meta{Agent: "claude"})
	srv.promptSeen(outside.Info(), p, outside.Terminal().Text())
	waitFor(t, "the owner told", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(told) == 1
	})
	time.Sleep(10 * time.Millisecond)
}

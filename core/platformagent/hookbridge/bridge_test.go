package hookbridge

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/enowx/enowx/core/platformagent"
	"github.com/enowx/enowx/core/platformagent/record"
	"github.com/enowx/enowx/proto"
)

// fakeDaemon answers calls as a test says and keeps them.
type fakeDaemon struct {
	mu      sync.Mutex
	calls   []call
	answer  proto.HookDecision
	failAll error
	hang    chan struct{} // session.hook blocks until it closes
	refuse  bool          // session.hook unknown, as on an older daemon
}

type call struct {
	method string
	hook   proto.SessionHookParams
	state  proto.SessionStateParams
}

func (d *fakeDaemon) dial() (Conn, error) {
	if d.failAll != nil {
		return nil, d.failAll
	}
	return d, nil
}

func (d *fakeDaemon) Close() error { return nil }

func (d *fakeDaemon) Call(method string, params, out any) error {
	c := call{method: method}
	b, _ := json.Marshal(params)
	if method == proto.MethodSessionHook {
		_ = json.Unmarshal(b, &c.hook)
	} else {
		_ = json.Unmarshal(b, &c.state)
	}
	d.mu.Lock()
	d.calls = append(d.calls, c)
	answer, hang, refuse := d.answer, d.hang, d.refuse
	d.mu.Unlock()
	if method == proto.MethodSessionHook {
		if hang != nil {
			<-hang
			return errors.New("closed")
		}
		if refuse {
			return errors.New("unknown method")
		}
		if out != nil {
			*out.(*proto.HookDecision) = answer
		}
	}
	return nil
}

func (d *fakeDaemon) seen() []call {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]call(nil), d.calls...)
}

func TestToolBeforeAsksTheDaemonAndReadsItsAnswer(t *testing.T) {
	d := &fakeDaemon{answer: proto.HookDecision{Block: true, Reason: "delegate it"}}
	b := NewOver("s1", false, "/work", d.dial)
	got := b.ToolBefore(context.Background(), "Bash", []byte(`{"command":"git status"}`))
	if !got.Deny || got.Why != "delegate it" {
		t.Fatalf("answer %+v", got)
	}
	c := d.seen()[0]
	if c.method != proto.MethodSessionHook || c.hook.ID != "s1" || c.hook.Event.Event != proto.HookToolBefore ||
		c.hook.Event.Tool != "Bash" || c.hook.Event.Command != "git status" || c.hook.State != "" {
		t.Fatalf("call %+v", c)
	}

	d.answer = proto.HookDecision{Approve: true}
	if got := b.ToolBefore(context.Background(), "Read", []byte(`{"file_path":"/work/a/b.go"}`)); !got.Allow || got.Deny {
		t.Fatalf("answer %+v", got)
	}
	if c := d.seen()[1]; !reflect.DeepEqual(c.hook.Event.Paths, []string{"a/b.go"}) || c.hook.Event.Command != "" {
		t.Fatalf("call %+v", c)
	}

	d.answer = proto.HookDecision{Context: "from a hook"}
	if got := b.ToolBefore(context.Background(), "Glob", []byte(`{"pattern":"*"}`)); got.Context != "from a hook" || got.Allow {
		t.Fatalf("answer %+v", got)
	}
	if c := d.seen()[2]; c.hook.Event.Command != `{"pattern":"*"}` {
		t.Fatalf("call %+v", c)
	}
}

// With no daemon, GOD of Agents' rules can't be checked: it stops, everyone else has no
// decision, and asks the user.
func TestToolBeforeWhenTheDaemonIsGone(t *testing.T) {
	d := &fakeDaemon{failAll: errors.New("no socket")}
	if got := NewOver("s1", false, "", d.dial).ToolBefore(context.Background(), "Bash", nil); got != (platformagent.HookAnswer{}) {
		t.Fatalf("a session: %+v", got)
	}
	got := NewOver("s1", true, "", d.dial).ToolBefore(context.Background(), "Bash", nil)
	if !got.Deny || got.Why == "" {
		t.Fatalf("GOA: %+v", got)
	}
}

func TestToolBeforeEndsWithItsContextEvenInGOA(t *testing.T) {
	d := &fakeDaemon{hang: make(chan struct{})}
	defer close(d.hang)
	ctx, cancel := context.WithCancel(context.Background())
	got := make(chan platformagent.HookAnswer, 1)
	go func() { got <- NewOver("s1", true, "", d.dial).ToolBefore(ctx, "Bash", nil) }()
	time.Sleep(10 * time.Millisecond)
	cancel()
	select {
	case a := <-got:
		if !a.Deny {
			t.Fatalf("answer %+v", a)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("it blocked")
	}
}

func TestEventsCarryTheirState(t *testing.T) {
	d := &fakeDaemon{}
	b := NewOver("s1", false, "", d.dial)
	b.SetMode(record.ModePlan)
	ctx := context.Background()
	b.Note(record.Record{Type: record.Assistant}, "the answer")
	b.Prompt(ctx, "hello")
	b.ToolAfter(ctx, "Bash", []byte(`{"command":"false"}`), true)
	b.TurnDone(ctx)
	b.Start(ctx)
	b.End()
	b.State(proto.StateWaiting)

	got := d.seen()
	want := []struct{ event, state string }{
		{proto.HookPromptStart, proto.StateWorking},
		{proto.HookToolAfter, proto.StateWorking},
		{proto.HookTurnDone, proto.StateDone},
		{proto.HookSessionStart, ""},
		{proto.HookSessionEnd, ""},
	}
	for i, w := range want {
		if got[i].hook.Event.Event != w.event || got[i].hook.State != w.state || got[i].hook.Event.PermissionMode != record.ModePlan {
			t.Errorf("%d: %+v, want %v", i, got[i].hook, w)
		}
	}
	if got[0].hook.Event.Prompt != "hello" || !got[1].hook.Event.Failed || got[2].hook.Event.LastMessage != "the answer" {
		t.Errorf("fields: %+v", got[:3])
	}
	if s := got[5]; s.method != proto.MethodSessionState || s.state.State != proto.StateWaiting || s.state.ID != "s1" {
		t.Errorf("state: %+v", s)
	}
}

func TestAnOlderDaemonGetsTheStateAlone(t *testing.T) {
	d := &fakeDaemon{refuse: true}
	NewOver("s1", false, "", d.dial).TurnDone(context.Background())
	got := d.seen()
	if len(got) != 2 || got[1].method != proto.MethodSessionState || got[1].state.State != proto.StateDone {
		t.Fatalf("calls %+v", got)
	}
}

func TestAnswersBeyondTheInterface(t *testing.T) {
	d := &fakeDaemon{answer: proto.HookDecision{Block: true, Reason: "no", Context: "rag", Continue: "more lines"}}
	b := NewOver("s1", false, "", d.dial)
	a, err := b.PromptAnswer(context.Background(), "x")
	if err != nil || !a.Deny || a.Why != "no" || a.Context != "rag" {
		t.Fatalf("%+v %v", a, err)
	}
	if more, err := b.TurnAnswer(context.Background()); err != nil || more != "more lines" {
		t.Fatalf("%q %v", more, err)
	}
}

func TestNothingHappensOutsideASession(t *testing.T) {
	dialed := false
	b := NewOver("", true, "", func() (Conn, error) { dialed = true; return nil, errors.New("no") })
	ctx := context.Background()
	b.State(proto.StateWorking)
	if got := b.ToolBefore(ctx, "Bash", nil); got != (platformagent.HookAnswer{}) {
		t.Fatalf("%+v", got)
	}
	b.ToolAfter(ctx, "Bash", nil, false)
	b.Prompt(ctx, "x")
	b.TurnDone(ctx)
	b.Start(ctx)
	b.End()
	if dialed {
		t.Fatal("it dialled")
	}
}

// fakeFront answers as a test says.
type fakeFront struct {
	platformagent.Front
	asked    int
	decision platformagent.Decision
	events   []string
}

func (f *fakeFront) Event(r record.Record) { f.events = append(f.events, r.Type) }

func (f *fakeFront) Permission(context.Context, platformagent.PermissionRequest) (platformagent.Decision, error) {
	f.asked++
	return f.decision, nil
}

func (f *fakeFront) Questions(context.Context, []record.Question) ([]string, error) {
	f.asked++
	return []string{"x"}, nil
}

func (f *fakeFront) OverBudget(context.Context, float64, float64) (bool, error) {
	f.asked++
	return true, nil
}

// A permission the daemon answers (a session in bypass, the judge) never reaches the
// user; otherwise the session waits while the user answers.
func TestFrontTellsTheDaemonOfAsks(t *testing.T) {
	d := &fakeDaemon{answer: proto.HookDecision{Approve: true}}
	front := &fakeFront{decision: platformagent.AllowAlways}
	f := Front{Front: front, Bridge: NewOver("s1", false, "", d.dial)}
	ctx := context.Background()
	got, err := f.Permission(ctx, platformagent.PermissionRequest{Tool: "Bash", Detail: "ls"})
	if err != nil || got != platformagent.AllowOnce || front.asked != 0 {
		t.Fatalf("%v %v, asked %d", got, err, front.asked)
	}
	if c := d.seen()[0]; c.hook.Event.Event != proto.HookPermission || c.hook.Event.Tool != "Bash" || c.hook.Event.Command != "ls" {
		t.Fatalf("call %+v", c)
	}

	d.mu.Lock()
	d.answer = proto.HookDecision{}
	d.calls = nil
	d.mu.Unlock()
	got, err = f.Permission(ctx, platformagent.PermissionRequest{Tool: "Bash"})
	if err != nil || got != platformagent.AllowAlways || front.asked != 1 {
		t.Fatalf("%v %v, asked %d", got, err, front.asked)
	}
	calls := d.seen()
	var said []string
	for _, c := range calls {
		said = append(said, c.hook.Event.Event+"/"+c.hook.State+c.state.State)
	}
	if want := "permission/ waiting/waiting /working"; strings.Join(said, " ") != want {
		t.Fatalf("calls %q, want %q", strings.Join(said, " "), want)
	}
}

func TestFrontAsksTheUserWithNoDaemon(t *testing.T) {
	d := &fakeDaemon{failAll: errors.New("no socket")}
	front := &fakeFront{decision: platformagent.AllowOnce}
	f := Front{Front: front, Bridge: NewOver("s1", false, "", d.dial)}
	got, err := f.Permission(context.Background(), platformagent.PermissionRequest{Tool: "Bash"})
	if err != nil || got != platformagent.AllowOnce || front.asked != 1 {
		t.Fatalf("%v %v, asked %d", got, err, front.asked)
	}
}

func TestFrontNotesRecords(t *testing.T) {
	d := &fakeDaemon{}
	front := &fakeFront{}
	b := NewOver("s1", false, "", d.dial)
	f := Front{Front: front, Bridge: b, Text: func(record.Record) string { return "said" }}
	f.Event(record.Record{Type: record.Meta, Mode: record.ModeAcceptEdits})
	f.Event(record.Record{Type: record.Assistant})
	f.Event(record.Record{Type: record.Assistant, Sidechain: "t1"})
	b.TurnDone(context.Background())
	c := d.seen()[0].hook.Event
	if c.PermissionMode != record.ModeAcceptEdits || c.LastMessage != "said" {
		t.Fatalf("event %+v", c)
	}
	if len(front.events) != 3 {
		t.Fatalf("events %v", front.events)
	}
}

package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/hosts"
	"github.com/enowx/enowx/proto"
)

// A watched host says how it is, and its running sessions list under the
// shown host's, an agent waiting for input lighting its dot amber.
func TestOtherHostsShowTheirSessions(t *testing.T) {
	a := New(Options{})
	vps := hosts.Host{ID: "vps", Name: "enowxlabs", Address: "root@10.0.0.1"}
	a.hostList = []hosts.Host{vps}
	a.watches = map[string]*hostWatch{"vps": {host: vps, status: hostStatus{
		state: hostOnline, latency: 42 * time.Millisecond,
		sessions: []proto.SessionInfo{
			{ID: "a", Name: "Fix login", Agent: "claude", State: proto.StateWaiting},
			{ID: "b", Name: "Old one", Exited: true},
			{ID: "c", Name: "A tab", Parent: "a"},
		},
	}}}
	st := a.watches["vps"].status
	if got := hostSummary(st); got != "online · 42 ms · 1 need input" {
		t.Fatalf("summary %q", got)
	}
	if hostColor(st) == hostColor(hostStatus{state: hostOnline}) {
		t.Fatal("a waiting agent doesn't show")
	}
	tt := ui.NewTester(func(c *ui.Context) { a.otherHostsSessions(c) }, 260, 300)
	tt.Frame()
	for _, want := range []string{"On other hosts", "enowxlabs", "Fix login", "needs input"} {
		if !tt.HasText(want) {
			t.Fatalf("missing %q in %q", want, tt.Texts())
		}
	}
	if tt.HasText("Old one") || tt.HasText("A tab") {
		t.Fatalf("listed an ended session or a tab: %q", tt.Texts())
	}
}

// A session whose agent finishes or asks unseen is unread until shown.
func TestSessionsTurnUnreadUntilShown(t *testing.T) {
	a := New(Options{})
	s := proto.SessionInfo{ID: "s1", Agent: "claude", State: proto.StateWorking}
	tab := proto.SessionInfo{ID: "s2", Agent: "codex", Parent: "s1", State: proto.StateWorking}
	a.sessions = []proto.SessionInfo{s, tab}
	done := s
	done.State = proto.StateDone
	a.markUnread(s, done)
	if !a.unread["s1"] {
		t.Fatal("not unread after finishing unseen")
	}
	a.selectSession(done)
	if a.unread["s1"] {
		t.Fatal("still unread once shown")
	}
	// A tab asking marks its session.
	asking := tab
	asking.State = proto.StateWaiting
	a.markUnread(tab, asking)
	if !a.unread["s1"] {
		t.Fatalf("a tab's question didn't mark its session: %v", a.unread)
	}
	// Working again is no news.
	delete(a.unread, "s1")
	a.markUnread(done, s)
	if a.unread["s1"] {
		t.Fatal("unread for starting to work")
	}
}

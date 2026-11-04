package ui

import (
	"net"
	"slices"
	"strings"
	"testing"
	"time"

	mygoui "github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/daemon"
	"github.com/enowx/enowx/proto"
)

// A session's terminal attaches when it shows or is typed into, not for being there, and
// goes once out of sight; the session itself goes on.
func TestTerminalsAttachWhenWantedAndUnload(t *testing.T) {
	srv, err := daemon.NewServer(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	a1, b1 := net.Pipe()
	go srv.ServeConn(a1)
	cl, err := client.New(b1, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cl.Close() })
	dir := t.TempDir()
	shown, err := cl.CreateSession(proto.SessionCreateParams{Command: []string{"sh"}, Dir: dir, Cols: 80, Rows: 10})
	if err != nil {
		t.Fatal(err)
	}
	hidden, err := cl.CreateSession(proto.SessionCreateParams{Command: []string{"sh"}, Dir: dir, Cols: 80, Rows: 10})
	if err != nil {
		t.Fatal(err)
	}
	app := New(Options{})
	posted := make(chan func(), 64)
	app.onMain = func(fn func()) { posted <- fn }
	app.client = cl
	app.upsert(shown)
	app.upsert(hidden)
	if len(app.panes) != 0 {
		t.Fatalf("attached for being there: %d", len(app.panes))
	}
	tt := mygoui.NewTester(func(c *mygoui.Context) {
		mygoui.Column(c).Width(800).Height(600).Children(func() { app.sessionBody(c, shown) })
	}, 800, 600)
	wait := func(what string, until func() bool) {
		t.Helper()
		deadline := time.Now().Add(20 * time.Second)
		for !until() {
			if time.Now().After(deadline) {
				t.Fatal("never: " + what)
			}
			select {
			case fn := <-posted:
				fn()
			case <-time.After(20 * time.Millisecond):
			}
			tt.Frame()
		}
	}
	wait("the session shown attached", func() bool { return app.panes[shown.ID] != nil })
	if _, ok := app.panes[hidden.ID]; ok {
		t.Fatal("the session out of sight attached")
	}
	app.sendToSession(hidden.ID, "echo heard-$((40+2))")
	wait("what was typed reached the session out of sight", func() bool {
		screen, _ := cl.Screen(hidden.ID)
		return strings.Contains(screen, "heard-42")
	})
	// Past the time and beyond the kept ones: more terminals than panesKept, all old.
	old := time.Now().Add(-2 * paneKeep)
	for _, id := range []string{"x1", "x2", "x3"} {
		app.panes[id], app.paneSeen[id] = nil, time.Now()
	}
	for _, id := range []string{shown.ID, hidden.ID} {
		app.paneSeen[id] = old
	}
	got := panesToUnload(app.panes, app.paneSeen, time.Now())
	if len(got) != 0 {
		t.Fatalf("two terminals are within panesKept: %v", got)
	}
}

func TestPanesToUnloadKeepsThoseLastDrawn(t *testing.T) {
	now := time.Now()
	old := now.Add(-2 * paneKeep)
	panes := map[string]*pane{"a": {}, "b": {}, "c": {}, "d": {}, "e": {}, "attaching": nil}
	seen := map[string]time.Time{
		"a": now.Add(-time.Minute), // drawn lately
		"b": old.Add(3 * time.Minute),
		"c": old.Add(2 * time.Minute),
		"d": old.Add(time.Minute),
		"e": old,
	}
	got := panesToUnload(panes, seen, now)
	slices.Sort(got)
	// a, b and c are the last three drawn; d and e are past the time and beyond them.
	if !slices.Equal(got, []string{"d", "e"}) {
		t.Fatalf("%v", got)
	}
	if got := panesToUnload(map[string]*pane{"a": {}}, map[string]time.Time{"a": old}, now); len(got) != 0 {
		t.Fatalf("a lone terminal went: %v", got)
	}
}

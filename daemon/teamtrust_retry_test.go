package daemon

import (
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/enowx/enowx/cloud/api"
)

// An empty folder holds no canvas: it doesn't keep a team's out. One with something in
// it does.
func TestEmptyFolderHoldsNothing(t *testing.T) {
	srv := startServer(t)
	dir := filepath.Join(srv.dataDir, "canvas", "abc123")
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o700); err != nil {
		t.Fatal(err)
	}
	if got := srv.spacesHolding("canvas", "abc123"); len(got) != 0 {
		t.Fatalf("an empty folder held it: %q", got)
	}
	if srv.heldElsewhere("team", "canvas/abc123/canvas.json") {
		t.Fatal("kept a team's out")
	}
	if err := os.WriteFile(filepath.Join(dir, "canvas.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := srv.spacesHolding("canvas", "abc123"); !slices.Contains(got, "") {
		t.Fatalf("a real one wasn't held: %q", got)
	}
}

// A host past items it never took asks for them all once, and only once.
func TestHostPastItemsRecoversOnce(t *testing.T) {
	ts := &teamSync{srv: startServer(t), state: teamSyncState{Seq: 2103}}
	if got := ts.pullFrom(); got != 0 {
		t.Fatalf("asked from %d", got)
	}
	ts.state.Recheck = teamRecheck
	if got := ts.pullFrom(); got != 2103 {
		t.Fatalf("asked from %d, after", got)
	}
	// Cut off partway, it carries on from where it stopped: asking for the whole
	// space again is what read it all, every few seconds.
	ts.state.Recheck, ts.state.RecheckAt = 0, 869
	if got := ts.pullFrom(); got != 869 {
		t.Fatalf("a recheck cut off asked from %d, not from 869", got)
	}
}

// What waits is asked for again, and past the first day still, less often: what was seen
// isn't seen again otherwise.
func TestRetryIsNeverGivenUp(t *testing.T) {
	ts := &teamSync{srv: startServer(t), state: teamSyncState{Seq: 100, Retry: 50, RetrySince: time.Now(), Recheck: teamRecheck}}
	if got := ts.pullFrom(); got != 49 {
		t.Fatalf("asked from %d", got)
	}
	ts.state.Retry, ts.state.RetrySince = 50, time.Now().Add(-2*teamRetryFor)
	if got := ts.pullFrom(); got != 49 {
		t.Fatalf("past a day, first: %d", got)
	}
	ts.state.Retry = 50
	if got := ts.pullFrom(); got != 100 || ts.state.Retry != 50 {
		t.Fatalf("past a day, again at once: %d, retry %d", got, ts.state.Retry)
	}
	ts.state.RetryAt = time.Now().Add(-2 * teamRetryLate)
	if got := ts.pullFrom(); got != 49 {
		t.Fatalf("past a day, an hour on: %d", got)
	}
}

// The case a host met: an empty folder under a canvas's ID kept a team's canvas out,
// and a failed pull then lost that it waited. It still comes, when the host syncs again.
func TestTeamItemKeptOutSurvivesAFailedPull(t *testing.T) {
	sc := newHookedCloud(t)
	acme, hs := teamOfHosts(t, sc, []string{"ada", "bob"}, []string{"", api.RoleMember})
	ada, bob := hs[0], hs[1]
	ada.sync(t, acme)
	bob.sync(t, acme)

	// Bob's host holds it for real elsewhere, so it waits; then a failed pull comes.
	own := filepath.Join(bob.srv.dataDir, "canvas", "abc123")
	if err := os.MkdirAll(own, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(own, "x"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeInSpace(t, ada, acme, "canvas/abc123/canvas.json", []byte(`{"id":"abc123"}`))
	ada.sync(t, acme)
	bob.sync(t, acme)
	there := filepath.Join(bob.srv.teamsDir(), acme, "canvas", "abc123", "canvas.json")
	if _, err := os.Stat(there); err == nil {
		t.Fatal("taken under an ID held here")
	}

	sc.setHook(func(w http.ResponseWriter, r *http.Request, next http.Handler) {
		if strings.Contains(r.URL.Path, "/items") {
			http.Error(w, "down", http.StatusBadGateway)
			return
		}
		next.ServeHTTP(w, r)
	})
	if _, err := bob.srv.syncSpace(acme); err == nil {
		t.Fatal("the sync didn't fail")
	}
	sc.setHook(nil)

	// All that's left of it is an empty folder now: nothing keeps it out.
	if err := os.Remove(filepath.Join(own, "x")); err != nil {
		t.Fatal(err)
	}
	bob.sync(t, acme)
	if _, err := os.Stat(there); err != nil {
		t.Fatalf("the team's canvas was lost: %v", err)
	}
}

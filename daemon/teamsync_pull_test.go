package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/enowx/enowx/cloud/api"
)

// readSyncState is what a host's copy of team's space last agreed with the
// cloud on.
func readSyncState(t *testing.T, h teamHost, team string) teamSyncState {
	t.Helper()
	var st teamSyncState
	data, err := os.ReadFile(filepath.Join(h.srv.teamsDir(), team, "sync", "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &st); err != nil {
		t.Fatal(err)
	}
	return st
}

// seedItems puts n items in team's space, from h's device: they don't open
// with the team's key, and a pull passes them by, page after page.
func seedItems(t *testing.T, sc *hookedCloud, team string, h teamHost, n int) {
	t.Helper()
	ctx := context.Background()
	dev := deviceOf(t, h)
	for i := 1; i <= n; i++ {
		p := api.PutItemParams{Kind: api.ItemFile, Version: 1, Epoch: 1, Sealed: []byte("sealed")}
		if _, err := sc.db.PutItem(ctx, team, h.acc, dev, fmt.Sprintf("%016x", i), p); err != nil {
			t.Fatal(err)
		}
	}
}

// A pull cut off partway carries on from where it stopped, not from the
// start of the space: asking for the whole space again is what made a chat's
// refresh read it all, every 5 seconds, for a billion bytes a start.
func TestAPullCutOffResumesWhereItStopped(t *testing.T) {
	sc := newHookedCloud(t)
	acme, hs := teamOfHosts(t, sc, []string{"ada"}, []string{""})
	ada := hs[0]
	seedItems(t, sc, acme, ada, 201) // two pages: 200, then 1

	var pages atomic.Int32
	sc.setHook(func(w http.ResponseWriter, r *http.Request, next http.Handler) {
		if strings.HasSuffix(r.URL.Path, "/items") && pages.Add(1) == 2 {
			http.Error(w, "down", http.StatusBadGateway)
			return
		}
		next.ServeHTTP(w, r)
	})
	if _, err := ada.srv.syncSpace(acme); err == nil {
		t.Fatal("the sync didn't fail")
	}
	sc.setHook(nil)

	st := readSyncState(t, ada, acme)
	if st.Recheck >= teamRecheck {
		t.Fatal("a pull cut off marks the recheck done: the rest is never asked for")
	}
	if st.RecheckAt == 0 {
		t.Fatal("a pull cut off keeps no place to carry on from")
	}

	asked := atomic.Int64{}
	asked.Store(-1)
	sc.setHook(func(w http.ResponseWriter, r *http.Request, next http.Handler) {
		if strings.HasSuffix(r.URL.Path, "/items") {
			if n, err := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64); err == nil {
				asked.CompareAndSwap(-1, n)
			}
		}
		next.ServeHTTP(w, r)
	})
	ada.sync(t, acme)
	sc.setHook(nil)
	if got := asked.Load(); got != st.RecheckAt {
		t.Fatalf("asked from %d, not from %d where it stopped", got, st.RecheckAt)
	}
}

// A chat's two views ask a second or two apart: the second is answered from
// the sync that just ended, not with a read of the whole space again.
func TestAChatRefreshIsAnsweredFromTheLastSync(t *testing.T) {
	sc := newHookedCloud(t)
	acme, hs := teamOfHosts(t, sc, []string{"ada", "bob"}, []string{"", api.RoleMember})
	ada, bob := hs[0], hs[1]
	ada.sync(t, acme) // the team's key
	bob.sync(t, acme)
	time.Sleep(pullEvery + 100*time.Millisecond) // past the sync just done

	var reads atomic.Int32
	sc.setHook(func(w http.ResponseWriter, r *http.Request, next http.Handler) {
		if strings.HasSuffix(r.URL.Path, "/items") {
			reads.Add(1)
		}
		next.ServeHTTP(w, r)
	})
	for range 2 {
		if err := bob.c.RefreshChat(acme); err != nil {
			t.Fatal(err)
		}
	}
	sc.setHook(nil)
	if got := reads.Load(); got != 1 {
		t.Fatalf("%d reads of the space for two refreshes of one chat", got)
	}
}

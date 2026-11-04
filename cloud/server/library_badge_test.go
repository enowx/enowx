package server

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/enowx/enowx/cloud/api"
	"github.com/enowx/enowx/cloud/store/storetest"
)

// The badge is the Cloud's to give: nothing a creator sends sets it, and it's there once
// the Cloud found their GitHub account contributed.
func TestLibraryContributorBadge(t *testing.T) {
	e := newLibEnv(t, Config{})
	ctx := context.Background()
	ada, bob := e.signIn(77, "ada", ""), e.signIn(78, "bob", "")
	e.live(ada, "A")
	e.live(bob, "B")
	e.mustProfile(ada.token, api.LibraryProfileParams{OpenForWork: true, ContactURL: "https://ada.example"})
	e.db.LibraryProfile(ctx, bob.acc.ID)
	if _, p := e.profile("ada", ""); len(p.Badges) != 0 {
		t.Fatalf("a badge for nothing: %+v", p.Badges)
	}
	if err := e.db.MarkGitHubContributors(ctx, "test", 77, 77, 0); err != nil {
		t.Fatal(err)
	}
	if _, p := e.profile("ada", ""); !slices.Equal(p.Badges, []string{api.BadgeContributor}) {
		t.Fatalf("badges: %+v", p.Badges)
	}
	if _, p := e.profile("bob", ""); len(p.Badges) != 0 {
		t.Fatalf("someone else's: %+v", p.Badges)
	}
	var list api.LibraryOpenCreators
	send(t, "GET", e.url+api.PathLibraryCreators+"?open=1", "", nil, &list)
	if len(list.Creators) != 1 || !slices.Equal(list.Creators[0].Badges, []string{api.BadgeContributor}) {
		t.Fatalf("on the list: %+v", list)
	}
}

// What the Contributor role finds is kept, for the badge: a merged pull request by
// signed webhook or by its sign-in, not a delivery for what didn't merge.
func TestContributorRoleKeepsTheBadgesBasis(t *testing.T) {
	rig := newContributorRig(t)
	ctx := context.Background()
	has := func(id int64) bool {
		t.Helper()
		var n int
		if err := rig.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM github_contributors WHERE github_id = $1`, id).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n > 0
	}
	// No merged pull request: linked, and no claim.
	rig.signIn(t, browser(t), "m1")
	if has(42) {
		t.Fatal("kept with no merged pull request")
	}
	// Someone linked to nothing: only their GitHub ID comes into it, and no role call.
	hook := func(payload string) {
		t.Helper()
		if code := signedHook(t, rig, payload); code != http.StatusNoContent {
			t.Fatalf("webhook: %d", code)
		}
	}
	hook(`{"action":"closed","pull_request":{"merged":false,"user":{"id":500}},"repository":{"full_name":"enowdev/enowx"}}`)
	hook(`{"action":"closed","pull_request":{"merged":true,"user":{"id":501}},"repository":{"full_name":"someone/else"}}`)
	if has(500) || has(501) {
		t.Fatal("kept what didn't merge here")
	}
	hook(`{"action":"closed","pull_request":{"merged":true,"user":{"id":502}},"repository":{"full_name":"enowdev/enowx"}}`)
	if !has(502) {
		t.Fatal("a merged pull request wasn't kept")
	}
	if err := rig.db.UnlinkDiscord(ctx, "m1"); err != nil {
		t.Fatal(err)
	}
	rig.merged = true
	rig.signIn(t, browser(t), "m2")
	if !has(42) {
		t.Fatal("a merged pull request found at sign-in wasn't kept")
	}
}

// signedHook sends a pull_request delivery signed with the rig's webhook secret.
func signedHook(t *testing.T, rig *contributorRig, payload string) int {
	t.Helper()
	req, _ := http.NewRequest("POST", rig.srv.URL+pathGitHubWebhook, strings.NewReader(payload))
	req.Header.Set("X-GitHub-Event", "pull_request")
	req.Header.Set("X-Hub-Signature-256", hookSignature(payload, "hook"))
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	return res.StatusCode
}

// The default team's repository's list counts too, when the Cloud reads it anyway.
func TestContributorListKeepsTheBadgesBasis(t *testing.T) {
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{{"id": 900}})
	}))
	t.Cleanup(gh.Close)
	db := storetest.Open(t)
	s := New(Config{URL: "https://cloud.test", GitHubAPIURL: gh.URL, DefaultTeamRepo: "enowx/enowx"}, db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if !s.isContributor(t.Context(), 900) {
		t.Fatal("not read")
	}
	var n int
	if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM github_contributors WHERE github_id = 900`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("kept: %d %v", n, err)
	}
}

func hookSignature(payload, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

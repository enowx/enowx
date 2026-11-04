package server

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/enowx/enowx/cloud/api"
)

// An account's email is in no answer of the library's: to the public, to its owner, or
// in its export.
func TestLibraryEmailNeverAppears(t *testing.T) {
	e := newLibEnv(t, Config{})
	const mail, otherMail = "ada.private@example.com", "bob.private@example.com"
	ada, bob := e.signIn(1, "ada", mail), e.signIn(2, "bob", otherMail)
	id := e.live(ada, "Mine")
	web, bobWeb := e.webSession(ada.acc), e.webSession(bob.acc)
	e.mustProfile(ada.token, api.LibraryProfileParams{ShowGitHub: true, OpenForWork: true, ContactURL: "https://ada.example", WorkKinds: []string{"freelance"}})
	var coll api.LibraryCollection
	send(t, "POST", e.url+api.PathLibraryCollections, ada.token, api.LibraryCollectionParams{Title: "Mine", Public: true}, &coll)
	send(t, "PUT", e.url+collectionItemPath(coll.ID, id), ada.token, nil, nil)
	send(t, "PUT", e.url+savePath(id), bob.token, nil, nil)
	send(t, "PUT", e.url+creatorPath(api.PathLibraryFollow, "ada"), bob.token, nil, nil)
	send(t, "POST", e.url+api.PathLibraryReports, bobWeb, api.LibraryReportParams{AccountID: ada.acc.ID, Reason: "spam"}, nil)

	for _, c := range []struct{ path, token string }{
		{api.PathLibraryItems, ""},
		{itemPath(api.PathLibraryItem, id), ""},
		{api.PathLibraryCreators + "?open=1", ""},
		{creatorPath(api.PathLibraryCreator, "ada"), ""},
		{creatorPath(api.PathLibraryCreator, "ada"), bob.token},
		{creatorPath(api.PathLibraryCreator, "ada"), web},
		{creatorPath(api.PathLibraryCreatorItems, "ada"), ""},
		{creatorPath(api.PathLibraryCreatorCollections, "ada"), ""},
		{itemPath(api.PathLibraryCollection, coll.ID), ""},
		{itemPath(api.PathLibraryCollection, coll.ID), web},
		{api.PathLibraryCollections, web},
		{api.PathLibrarySaves, bobWeb},
		{api.PathLibraryFollowing, bobWeb},
		{api.PathLibraryFeed, bobWeb},
		{api.PathLibraryMe, web},
		{api.PathLibraryExport, web},
		{api.PathLibraryExport, bobWeb},
		{api.PathLibraryQueue, e.webSession(e.admin.acc)},
	} {
		raw, code, _ := doRaw(t, "GET", e.url+c.path, c.token)
		if code != http.StatusOK {
			t.Errorf("%s: %d %s", c.path, code, raw)
		}
		if strings.Contains(raw, mail) || strings.Contains(raw, otherMail) || strings.Contains(raw, "@example.com") {
			t.Errorf("%s has an email: %s", c.path, raw)
		}
	}
}

// Export holds what the library keeps; delete takes it and its objects, by the purge,
// after a typed confirmation, and touches neither the Cloud account nor its teams. The
// log keeps its rows, and adds one.
func TestLibraryExportAndDelete(t *testing.T) {
	e := newLibEnv(t, Config{})
	ctx := context.Background()
	ada, bob := e.signIn(1, "ada", ""), e.signIn(2, "bob", "")
	team := e.team(ada, "ada's team")
	id := e.live(ada, "Mine")
	bobs := e.live(bob, "Bob's")
	web := e.webSession(ada.acc)
	e.mustProfile(web, api.LibraryProfileParams{Bio: "Hello", OpenForWork: true, ContactURL: "https://ada.example"})
	var coll api.LibraryCollection
	send(t, "POST", e.url+api.PathLibraryCollections, ada.token, api.LibraryCollectionParams{Title: "Private one"}, &coll)
	send(t, "PUT", e.url+collectionItemPath(coll.ID, bobs), ada.token, nil, nil)
	send(t, "PUT", e.url+savePath(bobs), ada.token, nil, nil)
	send(t, "PUT", e.url+savePath(id), bob.token, nil, nil)
	send(t, "PUT", e.url+creatorPath(api.PathLibraryFollow, "bob"), ada.token, nil, nil)
	send(t, "PUT", e.url+creatorPath(api.PathLibraryFollow, "ada"), bob.token, nil, nil)
	send(t, "POST", e.url+api.PathLibraryReports, web, api.LibraryReportParams{ItemID: bobs, Reason: "spam", Detail: "ada's report"}, nil)
	var bobColl api.LibraryCollection
	send(t, "POST", e.url+api.PathLibraryCollections, bob.token, api.LibraryCollectionParams{Title: "Bob keeps", Public: true}, &bobColl)
	send(t, "PUT", e.url+collectionItemPath(bobColl.ID, id), bob.token, nil, nil)

	if code, _ := send(t, "GET", e.url+api.PathLibraryExport, "", nil, nil); code != http.StatusUnauthorized {
		t.Fatalf("export signed out: %d", code)
	}
	_, code, hdr := doRaw(t, "GET", e.url+api.PathLibraryExport, ada.token)
	if code != http.StatusOK || !strings.Contains(hdr.Get("Content-Disposition"), "attachment") {
		t.Fatalf("export: %d %v", code, hdr)
	}
	raw, _, _ := doRaw(t, "GET", e.url+api.PathLibraryExport, web)
	var ex api.LibraryExport
	if err := json.Unmarshal([]byte(raw), &ex); err != nil {
		t.Fatal(err)
	}
	if ex.Format != api.LibraryExportFormat || ex.Profile.Handle != "ada" || ex.Profile.Settings == nil || ex.Profile.Settings.ContactURL != "https://ada.example" ||
		len(ex.Items) != 1 || ex.Items[0].ID != id || ex.Items[0].Latest.Prompt == "" ||
		len(ex.Likes) != 1 || ex.Likes[0] != bobs || len(ex.Collections) != 1 || ex.Collections[0].Title != "Private one" || !slices.Equal(ex.Collections[0].ItemIDs, []string{bobs}) ||
		!slices.Equal(ex.Following, []string{"bob"}) || ex.Followers != 1 || len(ex.Reports) != 1 || ex.Reports[0].Detail != "ada's report" || len(ex.Decisions) == 0 {
		t.Fatalf("export holds: %s", raw)
	}
	if strings.Contains(raw, "boss") || strings.Contains(raw, e.admin.acc.ID) {
		t.Fatalf("an export names a moderator: %s", raw)
	}
	raw, code, _ = doRaw(t, "GET", e.url+api.PathLibraryExport, ada.token)
	if code != http.StatusTooManyRequests {
		t.Fatalf("export's limit: %d %s", code, raw)
	}

	var logBefore api.LibraryLog
	boss := e.webSession(e.admin.acc)
	send(t, "GET", e.url+api.PathLibraryLog, boss, nil, &logBefore)
	if code, _ := send(t, "DELETE", e.url+api.PathLibraryMe, "", api.LibraryDeleteMe{Confirm: "ada"}, nil); code != http.StatusUnauthorized {
		t.Fatalf("delete signed out: %d", code)
	}
	for _, confirm := range []string{"", "bob", "ADA"} {
		if code, _ := send(t, "DELETE", e.url+api.PathLibraryMe, web, api.LibraryDeleteMe{Confirm: confirm}, nil); code != http.StatusBadRequest {
			t.Fatalf("confirm %q: %d", confirm, code)
		}
	}
	if _, err := e.blobs.Get(ctx, "library/items/"+id+"/v1/doc"); err != nil {
		t.Fatalf("refused deletes took something: %v", err)
	}
	if code, msg := send(t, "DELETE", e.url+api.PathLibraryMe, web, api.LibraryDeleteMe{Confirm: "ada"}, nil); code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", code, msg)
	}

	// What it kept is gone, its objects too.
	for _, part := range []string{"source", "doc", "thumb"} {
		if _, err := e.blobs.Get(ctx, "library/items/"+id+"/v1/"+part); err == nil {
			t.Errorf("its %s stayed", part)
		}
	}
	if code, _ := send(t, "GET", e.url+itemPath(api.PathLibraryItem, id), "", nil, nil); code != http.StatusGone {
		t.Fatalf("its address: %d", code)
	}
	if code, _ := e.profile("ada", ""); code != http.StatusNotFound {
		t.Fatalf("its profile: %d", code)
	}
	if code, _ := send(t, "GET", e.url+api.PathLibraryMe, web, nil, nil); code != http.StatusUnauthorized {
		t.Fatalf("its web session: %d", code)
	}
	if code, _ := send(t, "GET", e.url+itemPath(api.PathLibraryCollection, coll.ID), ada.token, nil, nil); code != http.StatusNotFound {
		t.Fatalf("its collection: %d", code)
	}
	var row struct{ title, prompt string }
	if err := e.db.QueryRowContext(ctx, `SELECT title FROM library_items WHERE id = $1`, id).Scan(&row.title); err != nil || row.title != "" {
		t.Fatalf("the row of its design: %q %v", row.title, err)
	}
	for table, where := range map[string]string{"library_profiles": "account_id", "library_saves": "account_id", "library_collections": "owner_id", "library_sessions": "account_id"} {
		var n int
		if err := e.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table+` WHERE `+where+` = $1`, ada.acc.ID).Scan(&n); err != nil || n != 0 {
			t.Errorf("%s: %d %v", table, n, err)
		}
	}
	var follows, versions int
	e.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM library_follows WHERE follower_id = $1 OR creator_id = $1`, ada.acc.ID).Scan(&follows)
	e.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM library_versions WHERE item_id = $1`, id).Scan(&versions)
	if follows != 0 || versions != 0 {
		t.Fatalf("follows %d, versions %d", follows, versions)
	}

	// Others' are as they were, less ada's like and ada's design.
	var it api.LibraryItem
	send(t, "GET", e.url+itemPath(api.PathLibraryItem, bobs), "", nil, &it)
	if it.Saves != 0 {
		t.Fatalf("ada's like still counts: %d", it.Saves)
	}
	var page api.LibraryCollectionPage
	send(t, "GET", e.url+itemPath(api.PathLibraryCollection, bobColl.ID), bob.token, nil, &page)
	if len(page.Items) != 0 {
		t.Fatalf("in someone's collection: %+v", page.Items)
	}
	var who api.LibraryFollowing
	send(t, "GET", e.url+api.PathLibraryFollowing, bob.token, nil, &who)
	if len(who.Creators) != 0 {
		t.Fatalf("bob still follows: %+v", who)
	}

	// The Cloud account, its device and its team stay.
	if acc, err := e.db.Account(ctx, ada.acc.ID); err != nil || acc.GitHub != "ada" {
		t.Fatalf("the account: %+v %v", acc, err)
	}
	var teams []api.Team
	if code, _ := send(t, "GET", e.url+api.PathTeams, ada.token, nil, &teams); code != http.StatusOK || !slices.ContainsFunc(teams, func(tm api.Team) bool { return tm.ID == team }) {
		t.Fatalf("its team: %d %+v", code, teams)
	}

	// The log keeps every row, by the account's ID, with one more saying so.
	var log api.LibraryLog
	send(t, "GET", e.url+api.PathLibraryLog, boss, nil, &log)
	if len(log.Entries) != len(logBefore.Entries)+1 || log.Entries[0].Action != "account.deleted" || log.Entries[0].AccountID != ada.acc.ID || log.Entries[0].Actor != ada.acc.ID {
		t.Fatalf("the log: %+v", log.Entries)
	}
	// A report it made stays for moderators, no longer naming it; one about it closes.
	var q api.LibraryQueue
	send(t, "GET", e.url+api.PathLibraryQueue, boss, nil, &q)
	if len(q.Reports) != 1 || q.Reports[0].Reporter != "" || q.Reports[0].Detail != "ada's report" {
		t.Fatalf("reports: %+v", q.Reports)
	}

	// The device signs in to the library again: a new, empty profile, with nothing back.
	var mine api.LibraryMine
	if code, _ := send(t, "GET", e.url+api.PathLibraryMine, ada.token, nil, &mine); code != http.StatusOK || len(mine.Items) != 0 || mine.Trust != api.TrustNew {
		t.Fatalf("again: %d %+v", code, mine)
	}
}

// Deleting isn't a way out of a block, and those who moderate give that up first.
func TestLibraryDeleteKeepsABlock(t *testing.T) {
	e := newLibEnv(t, Config{})
	ctx := context.Background()
	ada := e.signIn(1, "ada", "")
	id := e.live(ada, "Bad")
	boss := e.webSession(e.admin.acc)
	if code, msg := send(t, "POST", e.url+itemPath(api.PathLibraryBlock, ada.acc.ID), boss, api.LibraryReason{Reason: "phishing"}, nil); code != http.StatusNoContent {
		t.Fatalf("block: %d %s", code, msg)
	}
	var ex api.LibraryExport
	if code, _ := send(t, "GET", e.url+api.PathLibraryExport, ada.token, nil, &ex); code != http.StatusOK || ex.Trust != api.TrustBlocked {
		t.Fatalf("a blocked account's export: %d %+v", code, ex.Trust)
	}
	if code, msg := send(t, "DELETE", e.url+api.PathLibraryMe, ada.token, api.LibraryDeleteMe{Confirm: "ada"}, nil); code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", code, msg)
	}
	if _, err := e.blobs.Get(ctx, "library/items/"+id+"/v1/doc"); err == nil {
		t.Fatal("its objects stayed")
	}
	if code, _ := send(t, "POST", e.url+api.PathLibraryItems, ada.token, publishParams("Again"), nil); code != http.StatusForbidden {
		t.Fatalf("publishing after deleting: %d", code)
	}
	me, err := e.db.LibraryProfile(ctx, ada.acc.ID)
	if err != nil || me.Trust != api.TrustBlocked {
		t.Fatalf("after deleting: %+v %v", me, err)
	}
	if code, _ := e.profile("ada", ""); code != http.StatusNotFound {
		t.Fatalf("the handle: %d", code)
	}
	if code, _ := e.profile(me.Account.Handle, ""); code != http.StatusNotFound {
		t.Fatalf("what stays: %d", code)
	}
	// One who moderates is refused.
	if code, _ := send(t, "DELETE", e.url+api.PathLibraryMe, boss, api.LibraryDeleteMe{Confirm: "boss"}, nil); code != http.StatusConflict {
		t.Fatalf("a moderator: %d", code)
	}
}

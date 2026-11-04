package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/enowx/enowx/cloud/api"
)

// Why a later version was rejected is between its creator and the moderators: the
// item goes on showing to everyone else with nothing of it.
func TestLibraryRejectReasonIsPrivate(t *testing.T) {
	e := newLibEnv(t, Config{})
	boss := e.webSession(e.admin.acc)
	ada := e.signIn(1, "ada", "")
	bob := e.signIn(2, "bob", "")
	id := e.live(ada, "First")
	p := publishParams("Second")
	p.ItemID, p.Kind = id, api.LibraryComponent // another kind waits for a moderator
	sub := e.publish(ada, p, `<p>2</p>`)
	if sub.Status != api.LibraryReview {
		t.Fatalf("v2: %+v", sub)
	}
	const why = "looks copied from someone private"
	if code, msg := e.decide(boss, id, sub.Version, api.DecisionReject, why); code != http.StatusNoContent {
		t.Fatalf("reject: %d %s", code, msg)
	}
	item := e.url + itemPath(api.PathLibraryItem, id)
	for name, token := range map[string]string{"anyone": "", "another account": bob.token} {
		body, code, _ := doRaw(t, "GET", item, token)
		if code != http.StatusOK || strings.Contains(body, why) {
			t.Errorf("%s reads it: %d %s", name, code, body)
		}
	}
	if body, _, _ := doRaw(t, "GET", e.url+api.PathLibraryItems, ""); strings.Contains(body, why) {
		t.Errorf("listed with it: %s", body)
	}
	for name, token := range map[string]string{"its creator": ada.token, "a moderator": boss} {
		if body, _, _ := doRaw(t, "GET", item, token); !strings.Contains(body, why) {
			t.Errorf("%s doesn't read it: %s", name, body)
		}
	}
	if body, _, _ := doRaw(t, "GET", e.url+api.PathLibraryMine, ada.token); !strings.Contains(body, why) {
		t.Errorf("not in the app's list: %s", body)
	}
}

// Making room for a new version takes all of the oldest's objects, its video too.
func TestLibraryDroppedVersionTakesItsVideo(t *testing.T) {
	e := newLibEnv(t, Config{})
	ada := e.signIn(1, "ada", "")
	e.live(ada, "First")
	made, video := e.movingItem(ada, "Moving")
	if code, msg := e.put(ada.token, partPath(made.ID, 1, api.PartVideo), video); code != http.StatusNoContent {
		t.Fatalf("video: %d %s", code, msg)
	}
	e.put(ada.token, partPath(made.ID, 1, api.PartSource), bundle(t, `<p>1</p>`))
	e.put(ada.token, partPath(made.ID, 1, api.PartThumb), thumb(t))
	if code, msg := send(t, "POST", e.url+submitPath(made.ID, 1), ada.token, nil, nil); code != http.StatusOK {
		t.Fatalf("submit: %d %s", code, msg)
	}
	key := "library/items/" + made.ID + "/v1/video"
	if _, err := e.blobs.Get(context.Background(), key); err != nil {
		t.Fatal(err)
	}
	for n := 2; n <= api.MaxLibraryVersions+1; n++ {
		p := publishParams(fmt.Sprintf("Moving %d", n))
		p.ItemID = made.ID
		e.publish(ada, p, `<p>more</p>`)
	}
	if _, err := e.blobs.Get(context.Background(), key); err == nil {
		t.Fatal("its video stayed in the bucket")
	}
}

// Frames saying they moved are told once per item for hours, whatever is asked, and
// never push what people reported out of the moderators' list.
func TestLibraryEscapedCantBuryTheQueue(t *testing.T) {
	e := newLibEnv(t, Config{})
	boss := e.webSession(e.admin.acc)
	ada, bob := e.signIn(1, "ada", ""), e.signIn(2, "bob", "")
	id := e.live(ada, "First")
	escaped := func(item string) int {
		t.Helper()
		code, _ := sendWith(t, "POST", e.url+itemPath(api.PathLibraryEscaped, item), "", map[string]string{api.HeaderLibraryWeb: libSecret}, nil, nil)
		return code
	}
	open := func() []api.LibraryReport {
		t.Helper()
		var q api.LibraryQueue
		if code, msg := send(t, "GET", e.url+api.PathLibraryQueue, boss, nil, &q); code != http.StatusOK {
			t.Fatalf("queue: %d %s", code, msg)
		}
		return q.Reports
	}
	for range 3 {
		if code := escaped(id); code != http.StatusNoContent {
			t.Fatalf("escaped: %d", code)
		}
	}
	reports := open()
	if len(reports) != 1 {
		t.Fatalf("one item, reported %d times", len(reports))
	}
	// Dismissed, the same flood doesn't bring it straight back.
	if code, msg := send(t, "POST", e.url+itemPath(api.PathLibraryRepDecide, reports[0].ID), boss, api.LibraryDecision{Action: api.DecisionDismiss, Reason: "a false alarm"}, nil); code != http.StatusNoContent {
		t.Fatalf("dismiss: %d %s", code, msg)
	}
	escaped(id)
	if got := open(); len(got) != 0 {
		t.Fatalf("back at once: %+v", got)
	}
	if code := escaped("0123456789abcdef"); code != http.StatusNotFound {
		t.Fatalf("what isn't there: %d", code)
	}
	// Taken down, there's nothing left to look at.
	gone := e.live(ada, "Second")
	e.decide(boss, gone, 0, api.DecisionRemove, "rules")
	escaped(gone)
	if got := open(); len(got) != 0 {
		t.Fatalf("about what's gone: %+v", got)
	}
	// Many frames, older than anyone's report: the person's report still shows.
	ctx := context.Background()
	for i := range 120 {
		if _, err := e.db.ExecContext(ctx, `INSERT INTO library_reports (id, item_id, reason, detail, created) VALUES ($1, $2, 'escaped-preview', 'x', '2001-01-01T00:00:00.000000000Z')`, fmt.Sprintf("flood%d", i), id); err != nil {
			t.Fatal(err)
		}
	}
	if code, msg := send(t, "POST", e.url+api.PathLibraryReports, bob.token, api.LibraryReportParams{ItemID: id, Reason: "spam"}, nil); code != http.StatusNoContent {
		t.Fatalf("report: %d %s", code, msg)
	}
	if got := open(); len(got) == 0 || got[0].Reason != "spam" {
		t.Fatalf("what a person reported is under the flood: %d reports", len(got))
	}
}

// Another moderator decides on a moderator's own design, the owner's too: approving
// also makes its account trusted. So does whether a report on it stands.
func TestLibraryModeratorDoesntJudgeTheirOwn(t *testing.T) {
	e := newLibEnv(t, Config{})
	boss := e.webSession(e.admin.acc)
	ada, bob := e.signIn(1, "ada", ""), e.signIn(2, "bob", "")
	if _, err := e.db.LibraryProfile(context.Background(), ada.acc.ID); err != nil {
		t.Fatal(err)
	}
	if code, msg := send(t, "POST", e.url+api.PathLibraryModerators, boss, api.LibraryAppoint{AccountID: ada.acc.ID, Reason: "helps"}, nil); code != http.StatusNoContent {
		t.Fatalf("appoint: %d %s", code, msg)
	}
	mine := e.publish(ada, publishParams("Mine"), `<p>x</p>`)
	if code, _ := e.decide(ada.token, mine.ID, mine.Version, api.DecisionApprove, "I like it"); code != http.StatusForbidden {
		t.Fatalf("approving one's own: %d", code)
	}
	if code, msg := e.decide(boss, mine.ID, mine.Version, api.DecisionApprove, "fine"); code != http.StatusNoContent {
		t.Fatalf("another's approval: %d %s", code, msg)
	}
	own := e.publish(e.admin, publishParams("The owner's"), `<p>x</p>`)
	if code, _ := e.decide(boss, own.ID, own.Version, api.DecisionApprove, "mine"); code != http.StatusForbidden {
		t.Fatalf("the owner approving their own: %d", code)
	}
	if code, msg := e.decide(ada.token, own.ID, own.Version, api.DecisionApprove, "fine"); code != http.StatusNoContent {
		t.Fatalf("a moderator approving the owner's: %d %s", code, msg)
	}
	if code, msg := send(t, "POST", e.url+api.PathLibraryReports, bob.token, api.LibraryReportParams{ItemID: mine.ID, Reason: "spam"}, nil); code != http.StatusNoContent {
		t.Fatalf("report: %d %s", code, msg)
	}
	var q api.LibraryQueue
	send(t, "GET", e.url+api.PathLibraryQueue, boss, nil, &q)
	if len(q.Reports) != 1 {
		t.Fatalf("reports: %+v", q.Reports)
	}
	decide := e.url + itemPath(api.PathLibraryRepDecide, q.Reports[0].ID)
	if code, _ := send(t, "POST", decide, ada.token, api.LibraryDecision{Action: api.DecisionDismiss, Reason: "not so"}, nil); code != http.StatusForbidden {
		t.Fatalf("dismissing a report on one's own: %d", code)
	}
	if code, msg := send(t, "POST", decide, boss, api.LibraryDecision{Action: api.DecisionDismiss, Reason: "not so"}, nil); code != http.StatusNoContent {
		t.Fatalf("another's: %d %s", code, msg)
	}
}

// A first sign-in can't take a handle kept for the site or its staff, whatever the
// GitHub login is: it gets another one and can choose its own later.
func TestLibraryFirstHandleKeepsToTheRules(t *testing.T) {
	e := newLibEnv(t, Config{})
	for i, login := range []string{"admin", "Support", "enowx", "deleted-ada", "x", "new", strings.Repeat("a", 39)} {
		m := e.signIn(int64(100+i), login, "")
		me, err := e.db.LibraryProfile(context.Background(), m.acc.ID)
		if err != nil {
			t.Fatalf("%s: %v", login, err)
		}
		if why := api.LibraryHandleProblem(me.Account.Handle); why != "" {
			t.Errorf("%s got %q: %s", login, me.Account.Handle, why)
		}
	}
	m := e.signIn(200, "ada-lovelace", "")
	if me, _ := e.db.LibraryProfile(context.Background(), m.acc.ID); me.Account.Handle != "ada-lovelace" {
		t.Errorf("a fair login: %q", me.Account.Handle)
	}
}

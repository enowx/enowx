package server

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/enowx/enowx/cloud/api"
	"github.com/enowx/enowx/cloud/blobstore"
	"github.com/enowx/enowx/cloud/client"
	"github.com/enowx/enowx/cloud/library"
)

// newLibEnvWith is a library set up further by set.
func newLibEnvWith(t *testing.T, set func(*library.Config)) *libEnv {
	t.Helper()
	dir := blobstore.Dir(t.TempDir())
	e := newTeamEnv(t, Config{})
	admin := e.signIn(1000, "boss", "boss@example.com")
	cfg := Config{URL: "https://cloud.example.com", Library: &library.Config{Blobs: dir, PublicURL: "https://ui.example.com", ReturnURL: "https://ui.example.com/auth/callback", WebSecret: libSecret, AdminTeam: e.team(admin, "moderators")}}
	if set != nil {
		set(cfg.Library)
	}
	e.url = e.on(cfg, e.db)
	return &libEnv{teamEnv: e, blobs: dir, admin: admin}
}

// live publishes a design and has it approved, by a new account.
func (e *libEnv) live(m signedInMember, title string) string {
	e.t.Helper()
	sub := e.publish(m, publishParams(title), `<p>x</p>`)
	if sub.Status == api.LibraryReview {
		if code, msg := e.decide(e.webSession(e.admin.acc), sub.ID, sub.Version, api.DecisionApprove, "ok"); code != http.StatusNoContent {
			e.t.Fatalf("approve: %d %s", code, msg)
		}
	}
	return sub.ID
}

func (e *libEnv) comment(token, id, body string) (int, string) {
	e.t.Helper()
	return send(e.t, "POST", e.url+itemPath(api.PathLibraryComments, id), token, api.LibraryCommentParams{Body: body}, nil)
}

func (e *libEnv) comments(token, id string) api.LibraryComments {
	e.t.Helper()
	var out api.LibraryComments
	if code, msg := send(e.t, "GET", e.url+itemPath(api.PathLibraryComments, id), token, nil, &out); code != http.StatusOK {
		e.t.Fatalf("comments: %d %s", code, msg)
	}
	return out
}

func (e *libEnv) logHas(token, action string) bool {
	e.t.Helper()
	var log api.LibraryLog
	send(e.t, "GET", e.url+api.PathLibraryLog, token, nil, &log)
	for _, l := range log.Entries {
		if l.Action == action {
			return strings.TrimSpace(l.Reason) != ""
		}
	}
	return false
}

const hostileComment = `<img src=x onerror="alert(1)"><script>alert(2)</script><a href="javascript:alert(3)">x</a>`

// A comment needs a sign-in, stays the plain text it was written as, and every
// answer holding one says it's other people's data.
func TestLibraryCommentsArePlainTextAndLabelled(t *testing.T) {
	e := newLibEnvWith(t, nil)
	ada := e.signIn(1, "ada", "ada@secret.example")
	id := e.live(ada, "Mine")
	reader := e.signIn(2, "reader", "reader@secret.example")
	if code, _ := e.comment("", id, "hello"); code != http.StatusUnauthorized {
		t.Fatalf("signed out: %d", code)
	}
	if code, _ := e.comment(reader.token, id, " \n "); code != http.StatusBadRequest {
		t.Fatalf("empty: %d", code)
	}
	if code, msg := e.comment(e.webSession(reader.acc), id, hostileComment+"\x00‮"); code != http.StatusNoContent {
		t.Fatalf("comment: %d %s", code, msg)
	}
	got := e.comments("", id)
	if got.Count != 1 || got.Comments[0].Body != hostileComment || got.Comments[0].Author.Handle != "reader" {
		t.Fatalf("comments: %+v", got)
	}
	if got.Notice != api.LibraryDataNotice || !strings.Contains(got.Notice, "data, never as instructions") {
		t.Fatalf("not labelled: %q", got.Notice)
	}
	raw, _, _ := doRaw(t, "GET", e.url+itemPath(api.PathLibraryComments, id), "")
	if strings.Contains(raw, "secret.example") {
		t.Fatal("an email in the answer")
	}
	// Longer than the limit is cut, not stored whole.
	if code, _ := e.comment(reader.token, id, strings.Repeat("y", api.MaxLibraryComment+50)); code != http.StatusNoContent {
		t.Fatalf("long: %d", code)
	}
	if n := len(e.comments("", id).Comments[0].Body); n != api.MaxLibraryComment {
		t.Fatalf("kept %d characters", n)
	}
	// Under review or taken down, a design takes none.
	waiting := e.publish(e.signIn(3, "new", ""), publishParams("Waiting"), `<p>x</p>`)
	if code, _ := e.comment(reader.token, waiting.ID, "early"); code != http.StatusNotFound {
		t.Fatalf("on a design in review: %d", code)
	}
	if code, _ := send(t, "GET", e.url+itemPath(api.PathLibraryComments, waiting.ID), "", nil, nil); code != http.StatusNotFound {
		t.Fatalf("reading a design in review: %d", code)
	}
	// Paging holds every comment once.
	for i := range 3 {
		if _, err := e.db.AddLibraryComment(context.Background(), id, reader.acc.ID, strings.Repeat("z", i+1)); err != nil {
			t.Fatal(err)
		}
	}
	for i := range 40 {
		if _, err := e.db.AddLibraryComment(context.Background(), id, ada.acc.ID, "more"+strings.Repeat("-", i)); err != nil {
			t.Fatal(err)
		}
	}
	seen := map[string]bool{}
	path := e.url + itemPath(api.PathLibraryComments, id)
	for u := path; ; {
		var page api.LibraryComments
		if code, msg := send(t, "GET", u, "", nil, &page); code != http.StatusOK {
			t.Fatalf("page: %d %s", code, msg)
		}
		for _, c := range page.Comments {
			if seen[c.ID] {
				t.Fatalf("twice: %s", c.ID)
			}
			seen[c.ID] = true
		}
		if page.Next == "" {
			break
		}
		u = path + "?cursor=" + page.Next
	}
	if len(seen) != 45 {
		t.Fatalf("paged through %d of 45", len(seen))
	}
}

// Limits per account on one design, per account everywhere, and per design.
func TestLibraryCommentLimits(t *testing.T) {
	e := newLibEnvWith(t, nil)
	ctx := context.Background()
	ada := e.signIn(1, "ada", "")
	first, second := e.live(ada, "First"), e.live(ada, "Second")
	reader := e.signIn(2, "reader", "")
	for i := range api.MaxLibraryCommentsHere {
		if code, msg := e.comment(reader.token, first, "c"+strings.Repeat("!", i)); code != http.StatusNoContent {
			t.Fatalf("comment %d: %d %s", i, code, msg)
		}
	}
	if code, _ := e.comment(reader.token, first, "too many here"); code != http.StatusTooManyRequests {
		t.Fatalf("past the limit on a design: %d", code)
	}
	if code, msg := e.comment(reader.token, second, "elsewhere"); code != http.StatusNoContent {
		t.Fatalf("on another design: %d %s", code, msg)
	}
	// Deleting gives no turn back.
	list := e.comments("", first)
	if code, msg := send(t, "DELETE", e.url+strings.NewReplacer("{id}", first, "{comment}", list.Comments[0].ID).Replace(api.PathLibraryComment), reader.token, nil, nil); code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", code, msg)
	}
	if code, _ := e.comment(reader.token, first, "again"); code != http.StatusTooManyRequests {
		t.Fatalf("after deleting: %d", code)
	}
	busy := e.signIn(3, "busy", "")
	third := e.live(ada, "Third")
	for range api.MaxLibraryCommentsHour {
		if _, err := e.db.AddLibraryComment(ctx, second, busy.acc.ID, "x"); err != nil {
			t.Fatal(err)
		}
	}
	if code, _ := e.comment(busy.token, third, "one more"); code != http.StatusTooManyRequests {
		t.Fatalf("past an account's limit: %d", code)
	}
	for range api.MaxLibraryCommentsItem {
		if _, err := e.db.AddLibraryComment(ctx, third, busy.acc.ID, "x"); err != nil {
			t.Fatal(err)
		}
	}
	if code, _ := e.comment(e.signIn(4, "fresh", "").token, third, "a stranger"); code != http.StatusTooManyRequests {
		t.Fatalf("past a design's limit: %d", code)
	}
}

// Its creator closes comments, nobody else can; closed, they're for the creator
// and moderators. A blocked account can't comment and what it said goes quiet.
func TestLibraryCommentsOffAndBlocked(t *testing.T) {
	e := newLibEnvWith(t, nil)
	boss := e.webSession(e.admin.acc)
	ada := e.signIn(1, "ada", "")
	id := e.live(ada, "Mine")
	reader := e.signIn(2, "reader", "")
	if code, _ := e.comment(reader.token, id, "first"); code != http.StatusNoContent {
		t.Fatalf("comment: %d", code)
	}
	path := e.url + itemPath(api.PathLibraryCommentsOff, id)
	if code, _ := send(t, "PUT", path, reader.token, api.LibraryCommentsOffParams{Off: true}, nil); code != http.StatusForbidden {
		t.Fatalf("closed by a stranger: %d", code)
	}
	if code, msg := send(t, "PUT", path, e.webSession(ada.acc), api.LibraryCommentsOffParams{Off: true}, nil); code != http.StatusNoContent {
		t.Fatalf("closed: %d %s", code, msg)
	}
	if code, _ := e.comment(reader.token, id, "closed now"); code != http.StatusForbidden {
		t.Fatalf("on closed comments: %d", code)
	}
	if got := e.comments("", id); !got.Off || got.Count != 0 || len(got.Comments) != 0 {
		t.Fatalf("closed, to anyone: %+v", got)
	}
	if got := e.comments(reader.token, id); len(got.Comments) != 0 {
		t.Fatalf("closed, to who wrote there: %+v", got)
	}
	if got := e.comments(ada.token, id); !got.Off || len(got.Comments) != 1 {
		t.Fatalf("closed, to its creator: %+v", got)
	}
	if got := e.comments(boss, id); len(got.Comments) != 1 {
		t.Fatalf("closed, to a moderator: %+v", got)
	}
	if !e.logHas(boss, "item.comments-off") {
		t.Fatal("closing isn't in the log")
	}
	if code, _ := send(t, "PUT", path, ada.token, api.LibraryCommentsOffParams{}, nil); code != http.StatusNoContent {
		t.Fatalf("opened: %d", code)
	}
	if code, _ := send(t, "POST", e.url+itemPath(api.PathLibraryBlock, reader.acc.ID), boss, api.LibraryReason{Reason: "abuse"}, nil); code != http.StatusNoContent {
		t.Fatalf("block: %d", code)
	}
	if code, _ := e.comment(reader.token, id, "blocked"); code != http.StatusForbidden {
		t.Fatalf("blocked: %d", code)
	}
	if got := e.comments("", id); got.Count != 0 || len(got.Comments) != 0 {
		t.Fatalf("a blocked account's words: %+v", got)
	}
}

// A comment is reported into the same queue. Upheld, it goes and the design stays;
// a moderator can also remove one, with a reason in the log; its author takes it
// back themselves, the text gone.
func TestLibraryCommentReportsAndRemoval(t *testing.T) {
	e := newLibEnvWith(t, nil)
	boss := e.webSession(e.admin.acc)
	ada := e.signIn(1, "ada", "")
	id := e.live(ada, "Mine")
	writer := e.signIn(2, "writer", "")
	reader := e.signIn(3, "reader", "")
	e.comment(writer.token, id, hostileComment)
	c := e.comments("", id).Comments[0]
	report := api.LibraryReportParams{CommentID: c.ID, Reason: "abuse", Detail: "rude"}
	if code, _ := send(t, "POST", e.url+api.PathLibraryReports, "", report, nil); code != http.StatusUnauthorized {
		t.Fatalf("signed out: %d", code)
	}
	if code, _ := send(t, "POST", e.url+api.PathLibraryReports, reader.token, api.LibraryReportParams{CommentID: c.ID, ItemID: id, Reason: "abuse"}, nil); code != http.StatusBadRequest {
		t.Fatalf("two things at once: %d", code)
	}
	if code, msg := send(t, "POST", e.url+api.PathLibraryReports, reader.token, report, nil); code != http.StatusNoContent {
		t.Fatalf("report: %d %s", code, msg)
	}
	send(t, "POST", e.url+api.PathLibraryReports, reader.token, report, nil)
	var q api.LibraryQueue
	send(t, "GET", e.url+api.PathLibraryQueue, boss, nil, &q)
	if len(q.Reports) != 1 || q.Reports[0].CommentID != c.ID || q.Reports[0].Comment != hostileComment || q.Reports[0].Reporter != "reader" || q.Reports[0].AccountID != writer.acc.ID || q.Reports[0].ItemID != id {
		t.Fatalf("queue: %+v", q.Reports)
	}
	rep := e.url + itemPath(api.PathLibraryRepDecide, q.Reports[0].ID)
	if code, _ := send(t, "POST", rep, boss, api.LibraryDecision{Action: api.DecisionUphold}, nil); code != http.StatusBadRequest {
		t.Fatalf("no reason: %d", code)
	}
	if code, msg := send(t, "POST", rep, boss, api.LibraryDecision{Action: api.DecisionUphold, Reason: "it is rude"}, nil); code != http.StatusNoContent {
		t.Fatalf("uphold: %d %s", code, msg)
	}
	if got := e.comments("", id); got.Count != 0 {
		t.Fatalf("an upheld comment: %+v", got)
	}
	if code, _ := send(t, "GET", e.url+itemPath(api.PathLibraryItem, id), "", nil, nil); code != http.StatusOK {
		t.Fatalf("the design a comment was on: %d", code)
	}
	if !e.logHas(boss, "comment.report-upheld") {
		t.Fatal("not in the log")
	}
	if me, _ := e.db.LibraryProfile(context.Background(), ada.acc.ID); me.Trust != api.TrustTrusted {
		t.Fatalf("the creator's trust changed: %s", me.Trust)
	}

	e.comment(writer.token, id, "another")
	c = e.comments("", id).Comments[0]
	remove := e.url + itemPath(api.PathLibraryCommentRemove, c.ID)
	if code, _ := send(t, "POST", remove, reader.token, api.LibraryReason{Reason: "no"}, nil); code != http.StatusForbidden {
		t.Fatalf("a stranger removing: %d", code)
	}
	if code, _ := send(t, "POST", remove, boss, api.LibraryReason{}, nil); code != http.StatusBadRequest {
		t.Fatalf("no reason: %d", code)
	}
	if code, msg := send(t, "POST", remove, boss, api.LibraryReason{Reason: "off topic"}, nil); code != http.StatusNoContent {
		t.Fatalf("remove: %d %s", code, msg)
	}
	if !e.logHas(boss, "comment.removed") {
		t.Fatal("not in the log")
	}
	if code, _ := send(t, "POST", remove, boss, api.LibraryReason{Reason: "again"}, nil); code != http.StatusNotFound {
		t.Fatalf("removed twice: %d", code)
	}

	e.comment(writer.token, id, "mine to take back")
	c = e.comments("", id).Comments[0]
	send(t, "POST", e.url+api.PathLibraryReports, reader.token, api.LibraryReportParams{CommentID: c.ID, Reason: "spam"}, nil)
	del := e.url + strings.NewReplacer("{id}", id, "{comment}", c.ID).Replace(api.PathLibraryComment)
	if code, _ := send(t, "DELETE", del, reader.token, nil, nil); code != http.StatusNotFound {
		t.Fatalf("someone else's: %d", code)
	}
	if code, msg := send(t, "DELETE", del, writer.token, nil, nil); code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", code, msg)
	}
	send(t, "GET", e.url+api.PathLibraryQueue, boss, nil, &q)
	if len(q.Reports) != 0 {
		t.Fatalf("a report on deleted words waits: %+v", q.Reports)
	}
	var body string
	if err := e.db.QueryRowContext(context.Background(), `SELECT body FROM library_comments WHERE id = $1`, c.ID).Scan(&body); err != nil || body != "" {
		t.Fatalf("deleted text stayed: %q %v", body, err)
	}
	if code, _ := send(t, "POST", e.url+api.PathLibraryReports, reader.token, api.LibraryReportParams{CommentID: c.ID, Reason: "spam"}, nil); code != http.StatusNotFound {
		t.Fatalf("reporting deleted words: %d", code)
	}
}

// fakeMailer stands in for SMTP.
type fakeMailer struct {
	mu   sync.Mutex
	sent []string
	got  chan struct{}
}

func (f *fakeMailer) Send(_ context.Context, to, subject, text string) error {
	f.mu.Lock()
	f.sent = append(f.sent, to+"|"+subject+"|"+text)
	f.mu.Unlock()
	f.got <- struct{}{}
	return nil
}

// What happens to a creator reaches them on the website and through the API, a
// kind at a time; nobody else reads it, and it never says who reported.
func TestLibraryNotifications(t *testing.T) {
	e := newLibEnvWith(t, nil)
	boss := e.webSession(e.admin.acc)
	ada := e.signIn(1, "ada", "ada@secret.example")
	reader := e.signIn(2, "reader", "reader@secret.example")
	ctx := context.Background()
	c, err := client.New(e.url, ada.token)
	if err != nil {
		t.Fatal(err)
	}
	kinds := func() []string {
		t.Helper()
		got, err := c.LibraryNotifications(ctx, "")
		if err != nil {
			t.Fatal(err)
		}
		if got.Notice != api.LibraryDataNotice {
			t.Fatalf("not labelled: %q", got.Notice)
		}
		out := []string{}
		for _, n := range got.Notifications {
			out = append(out, n.Kind)
		}
		return out
	}
	rejected := e.publish(ada, publishParams("Rejected"), `<p>x</p>`)
	if code, _ := e.decide(boss, rejected.ID, 1, api.DecisionReject, "too plain"); code != http.StatusNoContent {
		t.Fatalf("reject: %d", code)
	}
	first := e.publish(ada, publishParams("First"), `<p>x</p>`)
	if code, _ := e.decide(boss, first.ID, 1, api.DecisionApprove, "fine"); code != http.StatusNoContent {
		t.Fatalf("approve: %d", code)
	}
	if got := strings.Join(kinds(), ","); got != "approved,rejected" {
		t.Fatalf("after two decisions: %s", got)
	}
	e.comment(reader.token, first.ID, "nice")
	e.comment(ada.token, first.ID, "my own")
	if got := strings.Join(kinds(), ","); got != "comment,approved,rejected" {
		t.Fatalf("a comment, and one's own: %s", got)
	}
	report := api.LibraryReportParams{ItemID: first.ID, Reason: "spam", Detail: "ads"}
	send(t, "POST", e.url+api.PathLibraryReports, reader.token, report, nil)
	var q api.LibraryQueue
	send(t, "GET", e.url+api.PathLibraryQueue, boss, nil, &q)
	if code, _ := send(t, "POST", e.url+itemPath(api.PathLibraryRepDecide, q.Reports[0].ID), boss, api.LibraryDecision{Action: api.DecisionUphold, Reason: "it is spam"}, nil); code != http.StatusNoContent {
		t.Fatalf("uphold: %d", code)
	}
	list, err := c.LibraryNotifications(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	top := list.Notifications[0]
	if top.Kind != api.NotifyReportUpheld || top.Reason != "it is spam" || top.Item != "First" || top.Read || list.Unread != 4 {
		t.Fatalf("upheld: %+v unread %d", top, list.Unread)
	}
	raw, _, _ := doRaw(t, "GET", e.url+api.PathLibraryNotifications, ada.token)
	for _, secret := range []string{"secret.example", "boss", q.Reports[0].ID} {
		if strings.Contains(raw, secret) {
			t.Fatalf("%s in a notification: %s", secret, raw)
		}
	}
	// "reader" did comment, and says so; it's the upheld one that names nobody.
	if strings.Contains(strings.Split(raw, `"kind":"comment"`)[0], "reader") {
		t.Fatalf("who reported, in a notification: %s", raw)
	}
	var me api.LibraryMe
	send(t, "GET", e.url+api.PathLibraryMe, ada.token, nil, &me)
	if me.Unread != 4 {
		t.Fatalf("unread in me: %d", me.Unread)
	}
	// Nobody else's.
	var other api.LibraryNotifications
	send(t, "GET", e.url+api.PathLibraryNotifications, reader.token, nil, &other)
	if len(other.Notifications) != 0 {
		t.Fatalf("someone else's: %+v", other)
	}
	if code, _ := send(t, "GET", e.url+api.PathLibraryNotifications, "", nil, nil); code != http.StatusUnauthorized {
		t.Fatalf("signed out: %d", code)
	}
	if err := c.LibraryNotificationsRead(ctx, []string{top.ID}); err != nil {
		t.Fatal(err)
	}
	send(t, "POST", e.url+api.PathLibraryNotifyRead, reader.token, api.LibraryNotifyRead{}, nil)
	if list, _ = c.LibraryNotifications(ctx, ""); list.Unread != 3 || !list.Notifications[0].Read {
		t.Fatalf("one read: %+v", list)
	}
	if err := c.LibraryNotificationsRead(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if list, _ = c.LibraryNotifications(ctx, ""); list.Unread != 0 {
		t.Fatalf("all read: %+v", list)
	}
	// Turned off, a kind stops; no cloud with a mailer here, so no email on offer.
	set, err := c.LibraryNotifySettings(ctx)
	if err != nil || set.EmailAvailable || set.Email || len(set.Off) != 0 {
		t.Fatalf("settings: %+v %v", set, err)
	}
	if code, _ := send(t, "PUT", e.url+api.PathLibraryNotifySet, ada.token, api.LibraryNotifySettings{Off: []string{"nonsense"}}, nil); code != http.StatusBadRequest {
		t.Fatalf("no such kind: %d", code)
	}
	if err := c.SetLibraryNotifySettings(ctx, api.LibraryNotifySettings{Off: []string{api.NotifyComment}}); err != nil {
		t.Fatal(err)
	}
	second := e.publish(ada, publishParams("Second"), `<p>x</p>`)
	if second.Status != api.LibraryReview {
		t.Fatalf("after an upheld report: %+v", second)
	}
	e.decide(boss, second.ID, 1, api.DecisionApprove, "fine")
	e.comment(reader.token, second.ID, "quiet")
	if list, _ = c.LibraryNotifications(ctx, ""); list.Notifications[0].Kind != api.NotifyApproved || list.Unread != 1 {
		t.Fatalf("a kind turned off: %+v", list)
	}
	e.decide(boss, second.ID, 0, api.DecisionRemove, "broke a rule")
	if list, _ = c.LibraryNotifications(ctx, ""); list.Notifications[0].Kind != api.NotifyRemoved || list.Notifications[0].Reason != "broke a rule" {
		t.Fatalf("taken down: %+v", list.Notifications[0])
	}
}

// Email is off with no mailer; with one (a fake), it goes only to who asked, once
// for comments unread, holding neither a comment's text nor a line break in its
// subject. It never reaches an answer.
func TestLibraryNotificationEmail(t *testing.T) {
	mail := &fakeMailer{got: make(chan struct{}, 8)}
	e := newLibEnvWith(t, func(c *library.Config) { c.Mailer = mail })
	ada := e.signIn(1, "ada", "ada@secret.example")
	reader := e.signIn(2, "reader", "reader@secret.example")
	id := e.live(ada, "Mine\nBcc: evil@example.com")
	select {
	case <-mail.got:
		t.Fatal("email without asking for it")
	case <-time.After(150 * time.Millisecond):
	}
	var set api.LibraryNotifySettings
	send(t, "GET", e.url+api.PathLibraryNotifySet, ada.token, nil, &set)
	if !set.EmailAvailable || set.Email {
		t.Fatalf("settings: %+v", set)
	}
	if code, _ := send(t, "PUT", e.url+api.PathLibraryNotifySet, ada.token, api.LibraryNotifySettings{Email: true, EmailAvailable: false}, nil); code != http.StatusNoContent {
		t.Fatalf("set: %d", code)
	}
	e.comment(reader.token, id, "SECRET WORDS")
	select {
	case <-mail.got:
	case <-time.After(5 * time.Second):
		t.Fatal("no email")
	}
	e.comment(reader.token, id, "a second one")
	select {
	case <-mail.got:
		t.Fatal("a second email for comments still unread")
	case <-time.After(150 * time.Millisecond):
	}
	mail.mu.Lock()
	parts := strings.SplitN(mail.sent[0], "|", 3)
	mail.mu.Unlock()
	to, subject, text := parts[0], parts[1], parts[2]
	if to != "ada@secret.example" || strings.ContainsAny(subject, "\r\n") {
		t.Fatalf("email: %q", parts)
	}
	if strings.Contains(text, "SECRET WORDS") || strings.Contains(text, "Mine\nBcc") || !strings.Contains(text, "https://ui.example.com/notifications") {
		t.Fatalf("what the email holds: %q", text)
	}
	raw, _, _ := doRaw(t, "GET", e.url+api.PathLibraryNotifications, ada.token)
	if strings.Contains(raw, "@secret.example") {
		t.Fatal("an email in an answer")
	}
}

// Views come from the website alone, as numbers; reuses are written in a batch;
// the numbers are their creator's alone.
func TestLibraryStats(t *testing.T) {
	dir := blobstore.Dir(t.TempDir())
	e := newTeamEnv(t, Config{})
	admin := e.signIn(1000, "boss", "")
	cfg := Config{URL: "https://cloud.example.com", Library: &library.Config{Blobs: dir, PublicURL: "https://ui.example.com", WebSecret: libSecret, AdminTeam: e.team(admin, "moderators")}}
	srv := New(cfg, e.db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	web := httptest.NewServer(srv)
	t.Cleanup(web.Close)
	e.url = web.URL
	lib := &libEnv{teamEnv: e, blobs: dir, admin: admin}
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	done := make(chan struct{})
	go func() { srv.RunLibrary(ctx); close(done) }()

	ada := e.signIn(1, "ada", "")
	id := lib.live(ada, "Mine")
	other := e.signIn(2, "other", "")
	views := e.url + api.PathLibraryViews
	body := api.LibraryViews{Views: map[string]int{id: 3, "0000000000000000": 9}}
	if code, _ := send(t, "POST", views, other.token, body, nil); code != http.StatusForbidden {
		t.Fatalf("views from an account: %d", code)
	}
	if code, _ := send(t, "POST", views, "", body, nil); code != http.StatusForbidden {
		t.Fatalf("views from anyone: %d", code)
	}
	web3 := map[string]string{api.HeaderLibraryWeb: libSecret}
	if code, msg := sendWith(t, "POST", views, "", web3, body, nil); code != http.StatusNoContent {
		t.Fatalf("views: %d %s", code, msg)
	}
	if code, _ := sendWith(t, "POST", views, "", web3, api.LibraryViews{Views: map[string]int{id: 2, "bad": -5}}, nil); code != http.StatusNoContent {
		t.Fatalf("more views: %d", code)
	}
	tooMany := map[string]int{}
	for i := range 1001 {
		tooMany[strings.Repeat("a", 4)+string(rune(i+0x100))] = 1
	}
	if code, _ := sendWith(t, "POST", views, "", web3, api.LibraryViews{Views: tooMany}, nil); code != http.StatusBadRequest {
		t.Fatalf("too many: %d", code)
	}
	if code, _ := send(t, "GET", e.url+itemPath(api.PathLibraryPrompt, id), "", nil, nil); code != http.StatusOK {
		t.Fatalf("prompt: %d", code)
	}
	if _, err := e.db.ExecContext(context.Background(), `UPDATE library_items SET saves = 7 WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	send(t, "POST", e.url+itemPath(api.PathLibraryComments, id), other.token, api.LibraryCommentParams{Body: "nice"}, nil)
	// A running library writes what it holds as it stops.
	stop()
	<-done
	var mine api.LibraryStats
	for range 100 {
		send(t, "GET", e.url+api.PathLibraryStats, ada.token, nil, &mine)
		if len(mine.Items) == 1 && mine.Items[0].Reuses == 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(mine.Items) != 1 || mine.Items[0].Views != 5 || mine.Items[0].Reuses != 1 || mine.Items[0].Saves != 7 || mine.Items[0].Comments != 1 {
		t.Fatalf("stats: %+v", mine)
	}
	var theirs api.LibraryStats
	send(t, "GET", e.url+api.PathLibraryStats, other.token, nil, &theirs)
	if len(theirs.Items) != 0 {
		t.Fatalf("someone else's numbers: %+v", theirs)
	}
	if code, _ := send(t, "GET", e.url+api.PathLibraryStats, "", nil, nil); code != http.StatusUnauthorized {
		t.Fatalf("signed out: %d", code)
	}
	raw, _, _ := doRaw(t, "GET", e.url+itemPath(api.PathLibraryItem, id), "")
	if strings.Contains(raw, "views") || strings.Contains(raw, "reuses") {
		t.Fatalf("a creator's numbers in public: %s", raw)
	}
	var cols int
	if err := e.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM library_stats WHERE item_id = $1`, "0000000000000000").Scan(&cols); err != nil || cols != 0 {
		t.Fatalf("a count for no item: %d %v", cols, err)
	}
}

// mp4 is an MP4 of the boxes the library reads, its tracks of the handlers given.
func mp4(seconds uint32, handlers ...string) []byte {
	box := func(kind string, parts ...[]byte) []byte {
		size := 8
		for _, p := range parts {
			size += len(p)
		}
		out := binary.BigEndian.AppendUint32(nil, uint32(size))
		out = append(out, kind...)
		for _, p := range parts {
			out = append(out, p...)
		}
		return out
	}
	mvhd := make([]byte, 20)
	binary.BigEndian.PutUint32(mvhd[12:], 1000)
	binary.BigEndian.PutUint32(mvhd[16:], seconds*1000)
	moov := [][]byte{box("mvhd", mvhd)}
	for _, h := range handlers {
		hdlr := append(make([]byte, 8), h...)
		hdlr = append(hdlr, make([]byte, 12)...)
		moov = append(moov, box("trak", box("mdia", box("hdlr", hdlr))))
	}
	return append(box("ftyp", []byte("isom\x00\x00\x02\x00")), box("moov", moov...)...)
}

// A moving design may carry a short video, its size counted; nothing else is
// taken as one, and only the website and moderators get it.
func TestLibraryVideoPart(t *testing.T) {
	e := newLibEnvWith(t, nil)
	boss := e.webSession(e.admin.acc)
	ada := e.signIn(1, "ada", "")
	params := publishParams("Moving")
	params.HasMotion = true
	var made api.LibraryPublished
	if code, msg := send(t, "POST", e.url+api.PathLibraryItems, ada.token, params, &made); code != http.StatusOK {
		t.Fatalf("create: %d %s", code, msg)
	}
	put := func(data []byte) int {
		t.Helper()
		code, _ := e.put(ada.token, partPath(made.ID, made.Version, api.PartVideo), data)
		return code
	}
	for name, data := range map[string][]byte{
		"nothing like one": []byte("<script>alert(1)</script>"),
		"with sound":       mp4(5, "vide", "soun"),
		"sound alone":      mp4(5, "soun"),
		"too long":         mp4(api.MaxLibraryVideoSeconds+1, "vide"),
		"no length":        mp4(0, "vide"),
		"no tracks":        mp4(5),
	} {
		if got := put(data); got != http.StatusBadRequest {
			t.Fatalf("%s: %d", name, got)
		}
	}
	if got := put(make([]byte, api.MaxLibraryVideo+1)); got != http.StatusRequestEntityTooLarge {
		t.Fatalf("too big: %d", got)
	}
	video := mp4(5, "vide")
	if got := put(video); got != http.StatusNoContent {
		t.Fatalf("video: %d", got)
	}
	if code, msg := e.put(ada.token, partPath(made.ID, made.Version, api.PartSource), bundle(t, `<p>x</p>`)); code != http.StatusNoContent {
		t.Fatalf("source: %d %s", code, msg)
	}
	if code, msg := e.put(ada.token, partPath(made.ID, made.Version, api.PartThumb), thumb(t)); code != http.StatusNoContent {
		t.Fatalf("thumb: %d %s", code, msg)
	}
	var sub api.LibrarySubmitted
	if code, msg := send(t, "POST", e.url+strings.NewReplacer("{id}", made.ID, "{version}", "1").Replace(api.PathLibrarySubmit), ada.token, nil, &sub); code != http.StatusOK {
		t.Fatalf("submit: %d %s", code, msg)
	}
	adminPath := strings.NewReplacer("{id}", made.ID, "{version}", "1", "{part}", api.PartVideo).Replace(api.PathLibraryAdminPart)
	got, status, h := doRaw(t, "GET", e.url+adminPath, boss)
	if status != http.StatusOK || got != string(video) || h.Get("Content-Type") != "video/mp4" || h.Get("Content-Security-Policy") == "" {
		t.Fatalf("a moderator's look: %d %s", status, h)
	}
	if _, status, _ := doRaw(t, "GET", e.url+adminPath, ada.token); status != http.StatusForbidden {
		t.Fatalf("anyone else's look: %d", status)
	}
	object := e.url + partPath(made.ID, 1, api.PartVideo)
	web := map[string]string{api.HeaderLibraryWeb: libSecret}
	if code, _ := sendWith(t, "GET", object, "", web, nil, nil); code != http.StatusNotFound {
		t.Fatalf("under review: %d", code)
	}
	e.decide(boss, made.ID, 1, api.DecisionApprove, "fine")
	var it api.LibraryItem
	send(t, "GET", e.url+itemPath(api.PathLibraryItem, made.ID), "", nil, &it)
	if !it.HasVideo {
		t.Fatalf("no video said: %+v", it)
	}
	if code, _ := send(t, "GET", object, ada.token, nil, nil); code != http.StatusForbidden {
		t.Fatalf("the object from an account: %d", code)
	}
	if code, _ := sendWith(t, "GET", object, "", web, nil, nil); code != http.StatusOK {
		t.Fatalf("the object for the website: %d", code)
	}
	var mine api.LibraryMine
	send(t, "GET", e.url+api.PathLibraryMine, ada.token, nil, &mine)
	if mine.Items[0].Latest.Size < int64(len(video)) || !mine.Items[0].Latest.HasVideo {
		t.Fatalf("not counted: %+v", mine.Items[0].Latest)
	}
	// A design that doesn't move has no video.
	var still api.LibraryPublished
	send(t, "POST", e.url+api.PathLibraryItems, ada.token, publishParams("Still"), &still)
	if code, _ := e.put(ada.token, partPath(still.ID, 1, api.PartVideo), video); code != http.StatusBadRequest {
		t.Fatalf("on a still design: %d", code)
	}
	// It goes with the rest.
	send(t, "DELETE", e.url+itemPath(api.PathLibraryItem, made.ID), ada.token, nil, nil)
	if _, err := e.db.ExecContext(context.Background(), `UPDATE library_items SET purge_at = '2000-01-01T00:00:00.000000000Z' WHERE id = $1`, made.ID); err != nil {
		t.Fatal(err)
	}
	svc, err := library.New(library.Config{Blobs: e.blobs}, library.Host{}, e.db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	svc.SweepOnce(context.Background())
	if _, err := e.blobs.Get(context.Background(), "library/items/"+made.ID+"/v1/video"); err == nil {
		t.Fatal("its video stayed")
	}
}

// Off unless set up; only admins, only a live pick, once, in the log; a title can't
// mention anyone or make a link; a message that didn't go can be tried again.
func TestLibraryAnnounce(t *testing.T) {
	type said struct {
		Path, Auth string
		Body       map[string]any
	}
	var mu sync.Mutex
	var heard []said
	answer := http.StatusOK
	discord := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		defer mu.Unlock()
		heard = append(heard, said{r.URL.Path, r.Header.Get("Authorization"), body})
		w.WriteHeader(answer)
	}))
	defer discord.Close()

	off := newLibEnvWith(t, nil)
	offBoss := off.webSession(off.admin.acc)
	offID := off.live(off.signIn(1, "ada", ""), "Pick")
	off.decide(offBoss, offID, 0, api.DecisionFeature, "good")
	if code, _ := off.decide(offBoss, offID, 0, api.DecisionAnnounce, "say it"); code != http.StatusConflict {
		t.Fatalf("with no channel: %d", code)
	}
	var offPicks api.LibraryPicks
	send(t, "GET", off.url+api.PathLibraryPicks, offBoss, nil, &offPicks)
	if offPicks.AnnounceOn || len(offPicks.Picks) != 1 {
		t.Fatalf("picks: %+v", offPicks)
	}
	if len(heard) != 0 {
		t.Fatalf("heard while off: %+v", heard)
	}

	e := newLibEnvWith(t, func(c *library.Config) {
		c.Announce = library.Announce{BotToken: "bot-token-value", ChannelID: "channel-1", DiscordURL: discord.URL}
	})
	boss := e.webSession(e.admin.acc)
	ada := e.signIn(1, "ada", "")
	id := e.live(ada, "@everyone [click](https://evil.example) **loud**")
	if code, _ := e.decide(boss, id, 0, api.DecisionAnnounce, "say it"); code != http.StatusConflict {
		t.Fatalf("not a pick: %d", code)
	}
	if code, _ := e.decide(boss, id, 0, api.DecisionFeature, "good"); code != http.StatusNoContent {
		t.Fatalf("pick: %d", code)
	}
	mod := e.signIn(2, "mod", "")
	send(t, "POST", e.url+api.PathLibraryModerators, boss, api.LibraryAppoint{AccountID: mod.acc.ID, Reason: "helps"}, nil)
	if code, _ := e.decide(e.webSession(mod.acc), id, 0, api.DecisionAnnounce, "say it"); code != http.StatusForbidden {
		t.Fatalf("a moderator: %d", code)
	}
	if code, _ := e.decide(boss, id, 0, api.DecisionAnnounce, ""); code != http.StatusBadRequest {
		t.Fatalf("no reason: %d", code)
	}
	if len(heard) != 0 {
		t.Fatalf("heard too soon: %+v", heard)
	}
	answer = http.StatusForbidden
	code, msg := e.decide(boss, id, 0, api.DecisionAnnounce, "say it")
	if code != http.StatusBadGateway || strings.Contains(msg, "bot-token-value") || strings.Contains(msg, discord.URL) {
		t.Fatalf("Discord refusing: %d %s", code, msg)
	}
	if !e.logHas(boss, "item.announce-failed") {
		t.Fatal("the failure isn't in the log")
	}
	answer = http.StatusOK
	if code, msg := e.decide(boss, id, 0, api.DecisionAnnounce, "say it"); code != http.StatusNoContent {
		t.Fatalf("announce: %d %s", code, msg)
	}
	if code, _ := e.decide(boss, id, 0, api.DecisionAnnounce, "say it again"); code != http.StatusConflict {
		t.Fatalf("twice: %d", code)
	}
	if len(heard) != 2 {
		t.Fatalf("heard %d", len(heard))
	}
	got := heard[1]
	content, _ := got.Body["content"].(string)
	if got.Path != "/channels/channel-1/messages" || got.Auth != "Bot bot-token-value" {
		t.Fatalf("how it was said: %+v", got)
	}
	if mentions, _ := got.Body["allowed_mentions"].(map[string]any); mentions == nil || len(mentions["parse"].([]any)) != 0 {
		t.Fatalf("mentions: %+v", got.Body)
	}
	for _, bad := range []string{"@everyone", "](https", "https://evil", "**loud**"} {
		if strings.Contains(strings.ReplaceAll(content, `\@`, ""), bad) {
			t.Fatalf("%q in %q", bad, content)
		}
	}
	if !strings.Contains(content, "https://ui.example.com/design/"+id+"/") {
		t.Fatalf("no address: %q", content)
	}
	if !e.logHas(boss, "item.announced") {
		t.Fatal("not in the log")
	}
	var picks api.LibraryPicks
	send(t, "GET", e.url+api.PathLibraryPicks, boss, nil, &picks)
	if !picks.AnnounceOn || picks.Picks[0].Announced == "" {
		t.Fatalf("picks: %+v", picks)
	}
	if code, _ := send(t, "GET", e.url+api.PathLibraryPicks, ada.token, nil, nil); code != http.StatusForbidden {
		t.Fatalf("picks to a creator: %d", code)
	}
}

// A copyright report says what, where the original is, who claims it and that it's
// true; moderators read that and the creator doesn't; upheld, the log says what kind.
func TestLibraryCopyrightReport(t *testing.T) {
	e := newLibEnvWith(t, nil)
	boss := e.webSession(e.admin.acc)
	ada := e.signIn(1, "ada", "")
	id := e.live(ada, "Mine")
	reader := e.signIn(2, "reader", "")
	claim := api.LibraryCopyright{Original: "https://example.com/original", Claimant: "Rights Holder Ltd", Statement: true}
	for name, c := range map[string]api.LibraryCopyright{
		"no address":   {Claimant: "x", Statement: true},
		"not a link":   {Original: "javascript:alert(1)", Claimant: "x", Statement: true},
		"no claimant":  {Original: claim.Original, Statement: true},
		"no statement": {Original: claim.Original, Claimant: "x"},
	} {
		if code, _ := send(t, "POST", e.url+api.PathLibraryReports, reader.token, api.LibraryReportParams{ItemID: id, Reason: "copyright", Detail: "copied", Copyright: &c}, nil); code != http.StatusBadRequest {
			t.Fatalf("%s: %d", name, code)
		}
	}
	if code, msg := send(t, "POST", e.url+api.PathLibraryReports, reader.token, api.LibraryReportParams{ItemID: id, Reason: "copyright", Detail: "copied my page", Copyright: &claim}, nil); code != http.StatusNoContent {
		t.Fatalf("report: %d %s", code, msg)
	}
	// What the fields don't apply to doesn't keep them.
	send(t, "POST", e.url+api.PathLibraryReports, reader.token, api.LibraryReportParams{ItemID: id, Reason: "spam", Copyright: &claim}, nil)
	var q api.LibraryQueue
	send(t, "GET", e.url+api.PathLibraryQueue, boss, nil, &q)
	if len(q.Reports) != 2 {
		t.Fatalf("queue: %+v", q.Reports)
	}
	var copyright api.LibraryReport
	for _, r := range q.Reports {
		if r.Reason == "copyright" {
			copyright = r
		} else if r.Copyright != nil {
			t.Fatalf("a spam report kept a claim: %+v", r)
		}
	}
	if copyright.Copyright == nil || *copyright.Copyright != claim || copyright.Detail != "copied my page" {
		t.Fatalf("what moderators read: %+v", copyright)
	}
	if code, _ := send(t, "POST", e.url+itemPath(api.PathLibraryRepDecide, copyright.ID), boss, api.LibraryDecision{Action: api.DecisionUphold, Reason: "the original is older and the same"}, nil); code != http.StatusNoContent {
		t.Fatalf("uphold: %d", code)
	}
	if !e.logHas(boss, "report.upheld.copyright") {
		t.Fatal("the log doesn't say what kind")
	}
	if code, _ := send(t, "GET", e.url+itemPath(api.PathLibraryItem, id), "", nil, nil); code != http.StatusGone {
		t.Fatalf("taken down: %d", code)
	}
	raw, _, _ := doRaw(t, "GET", e.url+api.PathLibraryNotifications, ada.token)
	if !strings.Contains(raw, api.NotifyReportUpheld) || strings.Contains(raw, "Rights Holder") || strings.Contains(raw, "reader") {
		t.Fatalf("what the creator reads: %s", raw)
	}
	// A website not yet sending the fields still reports with its detail.
	other := e.live(e.signIn(3, "cleo", ""), "Other")
	if code, _ := send(t, "POST", e.url+api.PathLibraryReports, reader.token, api.LibraryReportParams{ItemID: other, Reason: "copyright", Detail: "copied, see https://example.com/x"}, nil); code != http.StatusNoContent {
		t.Fatalf("with its detail alone: %d", code)
	}
}

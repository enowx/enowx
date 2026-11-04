package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"image"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/gen2brain/webp"

	"github.com/enowx/enowx/cloud/api"
	"github.com/enowx/enowx/cloud/blobstore"
	"github.com/enowx/enowx/cloud/library"
	"github.com/enowx/enowx/core/designshare"
	"github.com/enowx/enowx/proto"
)

const libSecret = "test-web-secret"

type libEnv struct {
	*teamEnv
	blobs blobstore.Dir
	admin signedInMember
}

func newLibEnv(t *testing.T, cfg Config) *libEnv {
	t.Helper()
	return newLibEnvOver(t, cfg, blobstore.Dir(t.TempDir()))
}

// newLibEnvOver is a library in store dir.
func newLibEnvOver(t *testing.T, cfg Config, dir blobstore.Dir) *libEnv {
	t.Helper()
	cfg.URL = "https://cloud.example.com"
	cfg.Library = &library.Config{Blobs: dir, PublicURL: "https://ui.example.com", ReturnURL: "https://ui.example.com/auth/callback", WebSecret: libSecret}
	e := newTeamEnv(t, Config{}) // to make the admin team first, then a server knowing it
	admin := e.signIn(1000, "boss", "boss@example.com")
	team := e.team(admin, "moderators")
	cfg.Library.AdminTeam = team
	e.url = e.on(cfg, e.db)
	return &libEnv{teamEnv: e, blobs: dir, admin: admin}
}

func thumb(t *testing.T) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := webp.Encode(&out, image.NewRGBA(image.Rect(0, 0, 40, 20)), webp.Options{Quality: 50}); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func bundle(t *testing.T, page string) []byte {
	t.Helper()
	data, err := designshare.Build(proto.CanvasFrame{Name: "Home", Width: 800, Height: 600, HTML: page}, nil, nil).Marshal()
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func publishParams(title string) api.LibraryPublishParams {
	return api.LibraryPublishParams{Kind: api.LibraryPage, License: api.LicenseMIT, Title: title, Description: "About it", Category: "dashboard", Tags: []string{"Dark Mode", "x"},
		Prompt: "Build this", PromptApproved: true, Rights: true, Format: designshare.Format, Width: 800, Height: 600}
}

// put sends a raw body as token.
func (e *libEnv) put(token, path string, data []byte) (int, string) {
	e.t.Helper()
	req, _ := http.NewRequest("PUT", e.url+path, bytes.NewReader(data))
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(body)
}

func partPath(id string, n int, part string) string {
	v := "1"
	if n != 1 {
		v = string(rune('0' + n))
	}
	return strings.NewReplacer("{id}", id, "{version}", v, "{part}", part).Replace(api.PathLibraryPart)
}

func itemPath(pattern, id string) string { return strings.NewReplacer("{id}", id).Replace(pattern) }

// publish sends a whole design as m, and says how it stands.
func (e *libEnv) publish(m signedInMember, p api.LibraryPublishParams, page string) api.LibrarySubmitted {
	e.t.Helper()
	var made api.LibraryPublished
	if code, msg := send(e.t, "POST", e.url+api.PathLibraryItems, m.token, p, &made); code != http.StatusOK {
		e.t.Fatalf("create: %d %s", code, msg)
	}
	if code, msg := e.put(m.token, partPath(made.ID, made.Version, api.PartSource), bundle(e.t, page)); code != http.StatusNoContent {
		e.t.Fatalf("source: %d %s", code, msg)
	}
	if code, msg := e.put(m.token, partPath(made.ID, made.Version, api.PartThumb), thumb(e.t)); code != http.StatusNoContent {
		e.t.Fatalf("thumb: %d %s", code, msg)
	}
	var out api.LibrarySubmitted
	if code, msg := send(e.t, "POST", e.url+strings.NewReplacer("{id}", made.ID, "{version}", string(rune('0'+made.Version))).Replace(api.PathLibrarySubmit), m.token, nil, &out); code != http.StatusOK {
		e.t.Fatalf("submit: %d %s", code, msg)
	}
	return out
}

// webSession signs account acc in as the website does, and returns its token.
func (e *libEnv) webSession(acc api.Account) string {
	e.t.Helper()
	verifier := strings.Repeat("v", 50)
	sum := sha256.Sum256([]byte(verifier))
	code, err := e.db.AddLibraryCode(context.Background(), acc.ID, base64.RawURLEncoding.EncodeToString(sum[:]))
	if err != nil {
		e.t.Fatal(err)
	}
	var s api.LibrarySession
	if status, msg := sendWith(e.t, "POST", e.url+api.PathLibraryExchange, "", map[string]string{api.HeaderLibraryWeb: libSecret}, api.LibraryExchange{Code: code, Verifier: verifier}, &s); status != http.StatusOK {
		e.t.Fatalf("exchange: %d %s", status, msg)
	}
	return s.Token
}

func (e *libEnv) decide(token, id string, version int, action, reason string) (int, string) {
	e.t.Helper()
	return send(e.t, "POST", e.url+itemPath(api.PathLibraryItemDecide, id), token, api.LibraryDecision{Action: action, Version: version, Reason: reason}, nil)
}

// Off unless its bucket is set: no routes, nothing in info.
func TestLibraryOffUnlessSetUp(t *testing.T) {
	e := newTeamEnv(t, Config{})
	if code, _ := send(t, "GET", e.url+api.PathLibraryItems, "", nil, nil); code != http.StatusNotFound {
		t.Fatalf("items without a bucket: %d", code)
	}
	var in api.Info
	send(t, "GET", e.url+api.PathInfo, "", nil, &in)
	if len(in.Features) != 0 {
		t.Fatalf("features: %v", in.Features)
	}
	lib := newLibEnv(t, Config{})
	send(t, "GET", lib.url+api.PathInfo, "", nil, &in)
	if !slices.Contains(in.Features, api.FeatureLibrary) {
		t.Fatalf("features: %v", in.Features)
	}
}

// A new account's design waits for a moderator and isn't public until then;
// approved, it shows and its account is trusted, its next goes live at once.
func TestLibraryPublishReviewAndTrust(t *testing.T) {
	e := newLibEnv(t, Config{})
	ada := e.signIn(1, "ada", "ada@secret.example")
	sub := e.publish(ada, publishParams("First"), `<h1>hi</h1>`)
	if sub.Status != api.LibraryReview {
		t.Fatalf("a new account's: %+v", sub)
	}
	item := itemPath(api.PathLibraryItem, sub.ID)
	if code, _ := send(t, "GET", e.url+item, "", nil, nil); code != http.StatusNotFound {
		t.Fatalf("in review, to anyone: %d", code)
	}
	var list api.LibraryItems
	send(t, "GET", e.url+api.PathLibraryItems, "", nil, &list)
	if len(list.Items) != 0 {
		t.Fatalf("listed in review: %+v", list)
	}
	if code, _ := send(t, "GET", e.url+item, ada.token, nil, nil); code != http.StatusOK {
		t.Fatalf("in review, to its owner: %d", code)
	}
	boss := e.webSession(e.admin.acc)
	var q api.LibraryQueue
	if code, msg := send(t, "GET", e.url+api.PathLibraryQueue, boss, nil, &q); code != http.StatusOK || len(q.Items) != 1 || q.Role != api.RoleLibraryOwner {
		t.Fatalf("queue: %d %s %+v", code, msg, q)
	}
	if q.Items[0].Version.Prompt == "" {
		t.Fatal("a moderator can't read the prompt")
	}
	if code, _ := e.decide(boss, sub.ID, 1, api.DecisionApprove, ""); code != http.StatusBadRequest {
		t.Fatalf("a decision without a reason: %d", code)
	}
	if code, msg := e.decide(boss, sub.ID, 1, api.DecisionApprove, "looks fine"); code != http.StatusNoContent {
		t.Fatalf("approve: %d %s", code, msg)
	}
	var it api.LibraryItem
	if code, msg := send(t, "GET", e.url+item, "", nil, &it); code != http.StatusOK || it.Status != api.LibraryLive || it.LiveVersion != 1 {
		t.Fatalf("live: %d %s %+v", code, msg, it)
	}
	raw, _ := io.ReadAll(mustGet(t, e.url+item))
	if strings.Contains(string(raw), "ada@secret.example") {
		t.Fatal("a public answer holds an email")
	}
	if !slices.Equal(it.Tags, []string{"dark-mode", "x"}) || it.Creator.Handle != "ada" {
		t.Fatalf("shown as: %+v", it)
	}
	if next := e.publish(ada, publishParams("Second"), `<h1>2</h1>`); next.Status != api.LibraryLive {
		t.Fatalf("a trusted account's: %+v", next)
	}
	var log api.LibraryLog
	send(t, "GET", e.url+api.PathLibraryLog, boss, nil, &log)
	if !slices.ContainsFunc(log.Entries, func(l api.LibraryLogEntry) bool {
		return l.Action == "version.approved" && l.Reason == "looks fine" && l.Actor == "boss"
	}) {
		t.Fatalf("log: %+v", log.Entries)
	}
}

func mustGet(t *testing.T, u string) io.Reader {
	t.Helper()
	res, err := http.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { res.Body.Close() })
	return res.Body
}

// The server doesn't take the host's word: a bundle with a secret, a tracker,
// or a "component" with a script is refused at submit. What it answers holds
// no value.
func TestLibraryServerChecksAgain(t *testing.T) {
	e := newLibEnv(t, Config{})
	ada := e.signIn(1, "ada", "")
	secret := "ghp_" + strings.Repeat("a1B2", 10)
	for name, c := range map[string]struct {
		kind, page string
	}{
		"secret":    {api.LibraryPage, `<p>` + secret + `</p>`},
		"tracker":   {api.LibraryPage, `<script src="https://www.googletagmanager.com/gtag/js"></script>`},
		"component": {api.LibraryComponent, `<button onclick="x()">Go</button>`},
	} {
		p := publishParams(name)
		p.Kind = c.kind
		var made api.LibraryPublished
		if code, msg := send(t, "POST", e.url+api.PathLibraryItems, ada.token, p, &made); code != http.StatusOK {
			t.Fatalf("%s: %d %s", name, code, msg)
		}
		e.put(ada.token, partPath(made.ID, 1, api.PartSource), bundle(t, c.page))
		e.put(ada.token, partPath(made.ID, 1, api.PartThumb), thumb(t))
		code, msg := send(t, "POST", e.url+strings.NewReplacer("{id}", made.ID, "{version}", "1").Replace(api.PathLibrarySubmit), ada.token, nil, nil)
		if code != http.StatusUnprocessableEntity {
			t.Errorf("%s: %d %s", name, code, msg)
		}
		if strings.Contains(msg, secret) {
			t.Errorf("%s: the answer holds the secret", name)
		}
		if _, err := e.blobs.Get(context.Background(), "library/items/"+made.ID+"/v1/doc"); err == nil {
			t.Errorf("%s: a document was made", name)
		}
	}
	p := publishParams("t")
	p.Prompt = secret
	if code, msg := send(t, "POST", e.url+api.PathLibraryItems, ada.token, p, nil); code != http.StatusUnprocessableEntity || strings.Contains(msg, secret) {
		t.Fatalf("a secret in the prompt: %d %s", code, msg)
	}
}

// Rights and the prompt's approval are a must; a thumbnail is a small WebP.
func TestLibraryPublishMusts(t *testing.T) {
	e := newLibEnv(t, Config{})
	ada := e.signIn(1, "ada", "")
	p := publishParams("t")
	p.Rights = false
	if code, _ := send(t, "POST", e.url+api.PathLibraryItems, ada.token, p, nil); code != http.StatusBadRequest {
		t.Fatalf("without rights: %d", code)
	}
	p = publishParams("t")
	p.PromptApproved = false
	if code, _ := send(t, "POST", e.url+api.PathLibraryItems, ada.token, p, nil); code != http.StatusBadRequest {
		t.Fatalf("without the prompt's approval: %d", code)
	}
	var made api.LibraryPublished
	send(t, "POST", e.url+api.PathLibraryItems, ada.token, publishParams("t"), &made)
	if code, _ := e.put(ada.token, partPath(made.ID, 1, api.PartThumb), []byte("<svg>not a picture</svg>")); code != http.StatusBadRequest {
		t.Fatalf("a thumbnail not WebP: %d", code)
	}
	if code, _ := e.put(ada.token, partPath(made.ID, 1, api.PartThumb), make([]byte, api.MaxLibraryThumb+1)); code != http.StatusRequestEntityTooLarge {
		t.Fatalf("a thumbnail too big: %d", code)
	}
	if code, _ := e.put(ada.token, partPath(made.ID, 1, api.PartSource), make([]byte, api.MaxLibraryBundle+1)); code != http.StatusRequestEntityTooLarge {
		t.Fatalf("a bundle too big: %d", code)
	}
	if code, _ := e.put(ada.token, partPath(made.ID, 1, api.PartDoc), []byte("<script>")); code != http.StatusNotFound {
		t.Fatalf("a document from a host: %d", code)
	}
	other := e.signIn(2, "bob", "")
	if code, _ := e.put(other.token, partPath(made.ID, 1, api.PartThumb), thumb(t)); code != http.StatusNotFound {
		t.Fatalf("someone else's item: %d", code)
	}
}

// A new account has at most 3 designs waiting.
func TestLibraryNewAccountLimit(t *testing.T) {
	e := newLibEnv(t, Config{})
	ada := e.signIn(1, "ada", "")
	for i := range 3 {
		e.publish(ada, publishParams("t"+string(rune('a'+i))), `<p>x</p>`)
	}
	if code, msg := send(t, "POST", e.url+api.PathLibraryItems, ada.token, publishParams("more"), nil); code != http.StatusConflict {
		t.Fatalf("a fourth: %d %s", code, msg)
	}
}

// A web session opens the library and nothing else, can't publish, and needs
// its PKCE verifier.
func TestLibrarySessionIsConfined(t *testing.T) {
	e := newLibEnv(t, Config{})
	ada := e.signIn(1, "ada", "")
	token := e.webSession(ada.acc)
	if !strings.HasPrefix(token, "enxl_") {
		t.Fatal("not a library token")
	}
	for _, path := range []string{api.PathMe, api.PathTeams, api.PathDevices} {
		if code, _ := send(t, "GET", e.url+path, token, nil, nil); code != http.StatusUnauthorized {
			t.Errorf("%s with a web session: %d", path, code)
		}
	}
	if code, _ := send(t, "POST", e.url+api.PathLibraryItems, token, publishParams("x"), nil); code != http.StatusForbidden {
		t.Errorf("publish with a web session: %d", code)
	}
	var me api.LibraryMe
	if code, msg := send(t, "GET", e.url+api.PathLibraryMe, token, nil, &me); code != http.StatusOK || me.Account.Handle != "ada" || me.Role != "" {
		t.Errorf("me: %d %s %+v", code, msg, me)
	}
	if code, _ := send(t, "POST", e.url+api.PathLibrarySignOut, token, nil, nil); code != http.StatusNoContent {
		t.Errorf("sign out: %d", code)
	}
	if code, _ := send(t, "GET", e.url+api.PathLibraryMe, token, nil, nil); code != http.StatusUnauthorized {
		t.Errorf("after signing out: %d", code)
	}

	sum := sha256.Sum256([]byte("right"))
	code, _ := e.db.AddLibraryCode(context.Background(), ada.acc.ID, base64.RawURLEncoding.EncodeToString(sum[:]))
	web := map[string]string{api.HeaderLibraryWeb: libSecret}
	if status, _ := sendWith(t, "POST", e.url+api.PathLibraryExchange, "", nil, api.LibraryExchange{Code: code, Verifier: "right"}, nil); status != http.StatusForbidden {
		t.Errorf("exchange without the website's secret: %d", status)
	}
	if status, _ := sendWith(t, "POST", e.url+api.PathLibraryExchange, "", web, api.LibraryExchange{Code: code, Verifier: "wrong"}, nil); status != http.StatusBadRequest {
		t.Errorf("exchange with the wrong verifier: %d", status)
	}
	if status, _ := sendWith(t, "POST", e.url+api.PathLibraryExchange, "", web, api.LibraryExchange{Code: code, Verifier: "right"}, nil); status != http.StatusBadRequest {
		t.Errorf("a code used up by a wrong try: %d", status)
	}
}

// Signing in to the library doesn't join the default team, unlike the cloud's
// own sign-in, and starts no session of its pages.
func TestLibrarySignInJoinsNoTeam(t *testing.T) {
	gh := fakeGitHub(t)
	e := newTeamEnv(t, Config{})
	seed := e.signIn(1, "seed", "")
	defaultTeam := e.team(seed, "everyone")
	cfg := Config{URL: "https://cloud.example.com", GitHubClientID: "cid", GitHubClientSecret: "csecret", GitHubURL: gh.URL, GitHubAPIURL: gh.URL, DefaultTeam: defaultTeam,
		Library: &library.Config{Blobs: blobstore.Dir(t.TempDir()), ReturnURL: "https://ui.example.com/auth/callback", WebSecret: libSecret}}
	base := e.on(cfg, e.db)

	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	state, challenge := strings.Repeat("s", 20), strings.Repeat("c", 43)
	authorize := api.PathLibraryAuthorize + "?state=" + state + "&code_challenge=" + challenge + "&code_challenge_method=S256"
	res, err := client.Get(base + authorize)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusFound || !strings.HasPrefix(res.Header.Get("Location"), pathGitHub) {
		t.Fatalf("authorize: %d %s", res.StatusCode, res.Header.Get("Location"))
	}
	res, err = client.Get(base + res.Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	gotState := mustURL(t, res.Header.Get("Location")).Query().Get("state")
	req, _ := http.NewRequest("GET", base+pathGitHubCallback+"?code=good&state="+gotState, nil)
	for _, c := range res.Cookies() {
		req.AddCookie(c)
	}
	res, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	to := mustURL(t, res.Header.Get("Location"))
	if !strings.HasPrefix(to.String(), "https://ui.example.com/auth/callback?") || to.Query().Get("state") != state || to.Query().Get("code") == "" {
		t.Fatalf("back to: %s", to)
	}
	if slices.ContainsFunc(res.Cookies(), func(c *http.Cookie) bool { return c.Name == sessionCookie && c.MaxAge >= 0 }) {
		t.Fatal("signing in to the library started a session of the cloud's pages")
	}
	acc, err := e.db.GitHubAccount(context.Background(), 42, "ada", "Ada Lovelace", "ada@example.com", "")
	if err != nil {
		t.Fatal(err)
	}
	if role, err := e.db.Role(context.Background(), defaultTeam, acc.ID); err == nil {
		t.Fatalf("joined the default team as %q", role)
	}
	// A request can't say where the code goes.
	res, err = client.Get(base + authorize + "&redirect_uri=https://evil.example&return=https://evil.example")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if strings.Contains(res.Header.Get("Location"), "evil.example") && !strings.HasPrefix(res.Header.Get("Location"), pathGitHub) {
		t.Fatalf("sent to: %s", res.Header.Get("Location"))
	}
	res, _ = client.Get(base + api.PathLibraryAuthorize + "?state=short&code_challenge=x")
	res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("a bad sign-in: %d", res.StatusCode)
	}
}

func mustURL(t *testing.T, s string) *url.URL {
	t.Helper()
	u, err := url.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

// A view-only item's prompt answers 403 at the API, not just without a button.
func TestLibraryLicenseGate(t *testing.T) {
	e := newLibEnv(t, Config{})
	ada := e.signIn(1, "ada", "")
	boss := e.webSession(e.admin.acc)
	p := publishParams("Look only")
	p.License = api.LicenseViewOnly
	sub := e.publish(ada, p, `<p>x</p>`)
	e.decide(boss, sub.ID, 1, api.DecisionApprove, "ok")
	var it api.LibraryItem
	send(t, "GET", e.url+itemPath(api.PathLibraryItem, sub.ID), "", nil, &it)
	if it.Actions != (api.LibraryActions{}) {
		t.Fatalf("view only allows: %+v", it.Actions)
	}
	if code, _ := send(t, "GET", e.url+itemPath(api.PathLibraryPrompt, sub.ID), "", nil, nil); code != http.StatusForbidden {
		t.Fatalf("a view-only prompt: %d", code)
	}
	page := e.publish(ada, publishParams("Full"), `<p>x</p>`)
	var got api.LibraryPrompt
	if code, msg := send(t, "GET", e.url+itemPath(api.PathLibraryPrompt, page.ID), "", nil, &got); code != http.StatusOK {
		t.Fatalf("a prompt: %d %s", code, msg)
	}
	for _, want := range []string{"Build this", "Full by ada", "https://ui.example.com/d/" + page.ID, "License: mit"} {
		if !strings.Contains(got.Prompt, want) {
			t.Errorf("no %q in %q", want, got.Prompt)
		}
	}
	send(t, "GET", e.url+itemPath(api.PathLibraryItem, page.ID), "", nil, &it)
	if !it.Actions.CopyPrompt || it.Actions.UseComponent || !it.Actions.UseReference || !it.Actions.OpenInEnowX {
		t.Fatalf("a full page allows: %+v", it.Actions)
	}
}

// Only moderators reach the queue, only the owner picks moderators, and each
// appointment and removal is in the log with its reason.
func TestLibraryModerators(t *testing.T) {
	e := newLibEnv(t, Config{})
	boss := e.webSession(e.admin.acc)
	mod := e.signIn(2, "mod", "mod@secret.example")
	modWeb := e.webSession(mod.acc)
	plain := e.webSession(e.signIn(3, "plain", "").acc)
	if code, _ := send(t, "GET", e.url+api.PathLibraryQueue, plain, nil, nil); code != http.StatusForbidden {
		t.Fatalf("queue, as nobody: %d", code)
	}
	if code, _ := send(t, "GET", e.url+api.PathLibraryQueue, "", nil, nil); code != http.StatusUnauthorized {
		t.Fatalf("queue, signed out: %d", code)
	}
	var found []api.LibraryAccount
	send(t, "GET", e.url+api.PathLibraryAccounts+"?q=mo", boss, nil, &found)
	if len(found) != 1 || found[0].GitHub != "mod" {
		t.Fatalf("picker: %+v", found)
	}
	raw, _, _ := doRaw(t, "GET", e.url+api.PathLibraryAccounts+"?q=mo", boss)
	if strings.Contains(raw, "mod@secret.example") {
		t.Fatal("the picker shows an email")
	}
	if code, _ := send(t, "POST", e.url+api.PathLibraryModerators, boss, api.LibraryAppoint{AccountID: mod.acc.ID}, nil); code != http.StatusBadRequest {
		t.Fatalf("appointed without a reason: %d", code)
	}
	if code, msg := send(t, "POST", e.url+api.PathLibraryModerators, boss, api.LibraryAppoint{AccountID: mod.acc.ID, Reason: "trusted helper"}, nil); code != http.StatusNoContent {
		t.Fatalf("appoint: %d %s", code, msg)
	}
	var q api.LibraryQueue
	if code, _ := send(t, "GET", e.url+api.PathLibraryQueue, modWeb, nil, &q); code != http.StatusOK || q.Role != api.RoleLibraryModerator {
		t.Fatalf("a moderator's queue: %d %+v", code, q)
	}
	third := e.signIn(4, "third", "")
	for name, c := range map[string]struct{ method, path string }{
		"list":    {"GET", api.PathLibraryModerators},
		"pick":    {"GET", api.PathLibraryAccounts + "?q=th"},
		"appoint": {"POST", api.PathLibraryModerators},
		"remove":  {"DELETE", itemPath(api.PathLibraryModerator, third.acc.ID)},
	} {
		if code, _ := send(t, c.method, e.url+c.path, modWeb, api.LibraryAppoint{AccountID: third.acc.ID, Reason: "x"}, nil); code != http.StatusForbidden {
			t.Errorf("%s, as a moderator: %d", name, code)
		}
	}
	if code, _ := send(t, "POST", e.url+itemPath(api.PathLibraryBlock, mod.acc.ID), boss, api.LibraryReason{Reason: "x"}, nil); code != http.StatusConflict {
		t.Errorf("a moderator blocked: %d", code)
	}
	if code, msg := send(t, "DELETE", e.url+itemPath(api.PathLibraryModerator, mod.acc.ID), boss, api.LibraryReason{Reason: "stepped down"}, nil); code != http.StatusNoContent {
		t.Fatalf("remove: %d %s", code, msg)
	}
	if code, _ := send(t, "GET", e.url+api.PathLibraryQueue, modWeb, nil, nil); code != http.StatusForbidden {
		t.Fatalf("queue, once removed: %d", code)
	}
	var log api.LibraryLog
	send(t, "GET", e.url+api.PathLibraryLog, boss, nil, &log)
	for _, want := range []string{"moderator.appointed:trusted helper", "moderator.removed:stepped down"} {
		if !slices.ContainsFunc(log.Entries, func(l api.LibraryLogEntry) bool {
			return l.Action+":"+l.Reason == want && l.AccountID == mod.acc.ID && l.Actor == "boss"
		}) {
			t.Errorf("no %s in %+v", want, log.Entries)
		}
	}
}

func doRaw(t *testing.T, method, u, token string) (string, int, http.Header) {
	t.Helper()
	req, _ := http.NewRequest(method, u, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(res.Body)
	return string(data), res.StatusCode, res.Header
}

// Reports reach moderators, never the creator; upheld, the item goes and its
// owner is reviewed again. Blocked, what they show goes and they can't publish.
func TestLibraryReportsAndBlocks(t *testing.T) {
	e := newLibEnv(t, Config{})
	boss := e.webSession(e.admin.acc)
	ada := e.signIn(1, "ada", "")
	sub := e.publish(ada, publishParams("Mine"), `<p>x</p>`)
	e.decide(boss, sub.ID, 1, api.DecisionApprove, "ok")
	reader := e.signIn(2, "reader", "")
	readerWeb := e.webSession(reader.acc)
	if code, _ := send(t, "POST", e.url+api.PathLibraryReports, "", api.LibraryReportParams{ItemID: sub.ID, Reason: "spam"}, nil); code != http.StatusUnauthorized {
		t.Fatalf("a report signed out: %d", code)
	}
	if code, _ := send(t, "POST", e.url+api.PathLibraryReports, readerWeb, api.LibraryReportParams{ItemID: sub.ID, Reason: "copyright"}, nil); code != http.StatusBadRequest {
		t.Fatalf("a copyright report saying nothing: %d", code)
	}
	if code, msg := send(t, "POST", e.url+api.PathLibraryReports, readerWeb, api.LibraryReportParams{ItemID: sub.ID, Reason: "spam", Detail: "ads"}, nil); code != http.StatusNoContent {
		t.Fatalf("report: %d %s", code, msg)
	}
	send(t, "POST", e.url+api.PathLibraryReports, readerWeb, api.LibraryReportParams{ItemID: sub.ID, Reason: "spam", Detail: "again"}, nil)
	var q api.LibraryQueue
	send(t, "GET", e.url+api.PathLibraryQueue, boss, nil, &q)
	if len(q.Reports) != 1 || q.Reports[0].Reporter != "reader" {
		t.Fatalf("reports: %+v", q.Reports)
	}
	raw, _, _ := doRaw(t, "GET", e.url+api.PathLibraryMine, ada.token)
	if strings.Contains(raw, "reader") {
		t.Fatal("a creator sees who reported them")
	}
	repPath := itemPath(api.PathLibraryRepDecide, q.Reports[0].ID)
	if code, msg := send(t, "POST", e.url+repPath, boss, api.LibraryDecision{Action: api.DecisionUphold, Reason: "it is spam"}, nil); code != http.StatusNoContent {
		t.Fatalf("uphold: %d %s", code, msg)
	}
	if code, _ := send(t, "GET", e.url+itemPath(api.PathLibraryItem, sub.ID), "", nil, nil); code != http.StatusGone {
		t.Fatalf("taken down: %d", code)
	}
	if next := e.publish(ada, publishParams("After"), `<p>x</p>`); next.Status != api.LibraryReview {
		t.Fatalf("after an upheld report: %+v", next)
	}
	if code, _ := send(t, "POST", e.url+itemPath(api.PathLibraryBlock, ada.acc.ID), boss, api.LibraryReason{}, nil); code != http.StatusBadRequest {
		t.Fatalf("blocked without a reason: %d", code)
	}
	if code, msg := send(t, "POST", e.url+itemPath(api.PathLibraryBlock, ada.acc.ID), boss, api.LibraryReason{Reason: "repeated spam"}, nil); code != http.StatusNoContent {
		t.Fatalf("block: %d %s", code, msg)
	}
	if code, _ := send(t, "POST", e.url+api.PathLibraryItems, ada.token, publishParams("Again"), nil); code != http.StatusForbidden {
		t.Fatalf("a blocked account publishing: %d", code)
	}
	// The website alone can say a preview tried to leave.
	if code, _ := send(t, "POST", e.url+itemPath(api.PathLibraryEscaped, sub.ID), readerWeb, nil, nil); code != http.StatusForbidden {
		t.Fatalf("escaped, from anyone: %d", code)
	}
	if code, _ := sendWith(t, "POST", e.url+itemPath(api.PathLibraryEscaped, sub.ID), "", map[string]string{api.HeaderLibraryWeb: libSecret}, nil, nil); code != http.StatusNoContent {
		t.Fatalf("escaped: %d", code)
	}
}

// Unpublished, an address answers 410 and its objects stay until purged; what
// a version says of the item only shows once it's live.
func TestLibraryWithdrawAndVersions(t *testing.T) {
	e := newLibEnv(t, Config{})
	boss := e.webSession(e.admin.acc)
	ada := e.signIn(1, "ada", "")
	sub := e.publish(ada, publishParams("One"), `<p>x</p>`)
	e.decide(boss, sub.ID, 1, api.DecisionApprove, "ok")

	p := publishParams("One, again")
	p.ItemID = sub.ID
	p.Kind = api.LibraryComponent
	next := e.publish(ada, p, `<p>y</p>`)
	if next.ID != sub.ID || next.Version != 2 || next.Status != api.LibraryReview {
		t.Fatalf("a new kind on a new version goes through review: %+v", next)
	}
	var it api.LibraryItem
	send(t, "GET", e.url+itemPath(api.PathLibraryItem, sub.ID), "", nil, &it)
	if it.Title != "One" || it.LiveVersion != 1 || it.Kind != api.LibraryPage {
		t.Fatalf("what shows changed before review: %+v", it)
	}
	e.decide(boss, sub.ID, 2, api.DecisionApprove, "ok")
	send(t, "GET", e.url+itemPath(api.PathLibraryItem, sub.ID), "", nil, &it)
	if it.Title != "One, again" || it.LiveVersion != 2 || it.Kind != api.LibraryComponent {
		t.Fatalf("live: %+v", it)
	}

	other := e.webSession(e.signIn(2, "bob", "").acc)
	if code, _ := send(t, "DELETE", e.url+itemPath(api.PathLibraryItem, sub.ID), other, nil, nil); code != http.StatusForbidden {
		t.Fatalf("someone else's: %d", code)
	}
	if code, msg := send(t, "DELETE", e.url+itemPath(api.PathLibraryItem, sub.ID), ada.token, nil, nil); code != http.StatusNoContent {
		t.Fatalf("withdraw: %d %s", code, msg)
	}
	if code, _ := send(t, "GET", e.url+itemPath(api.PathLibraryItem, sub.ID), "", nil, nil); code != http.StatusGone {
		t.Fatalf("after unpublishing: %d", code)
	}
	if _, err := e.blobs.Get(context.Background(), "library/items/"+sub.ID+"/v1/doc"); err != nil {
		t.Fatalf("objects stay for a while: %v", err)
	}
}

// Past its time a taken-back item's objects go and its account gets the room
// back, while its address still answers 410; an upload left unfinished goes whole.
func TestLibrarySweep(t *testing.T) {
	e := newLibEnv(t, Config{})
	ctx := context.Background()
	ada := e.signIn(1, "ada", "")
	sub := e.publish(ada, publishParams("Old"), `<p>x</p>`)
	var left api.LibraryPublished
	send(t, "POST", e.url+api.PathLibraryItems, ada.token, publishParams("Left"), &left)
	if code, _ := send(t, "DELETE", e.url+itemPath(api.PathLibraryItem, sub.ID), ada.token, nil, nil); code != http.StatusNoContent {
		t.Fatalf("withdraw: %d", code)
	}
	svc, err := library.New(library.Config{Blobs: e.blobs}, library.Host{}, e.db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	svc.SweepOnce(ctx)
	if _, err := e.blobs.Get(ctx, "library/items/"+sub.ID+"/v1/doc"); err != nil {
		t.Fatalf("gone before its time: %v", err)
	}
	if _, err := e.db.ExecContext(ctx, `UPDATE library_items SET purge_at = '2000-01-01T00:00:00.000000000Z' WHERE id = $1`, sub.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.ExecContext(ctx, `UPDATE library_versions SET created = '2000-01-01T00:00:00.000000000Z' WHERE item_id = $1`, left.ID); err != nil {
		t.Fatal(err)
	}
	svc.SweepOnce(ctx)
	if _, err := e.blobs.Get(ctx, "library/items/"+sub.ID+"/v1/doc"); err == nil {
		t.Fatal("its document stayed")
	}
	if code, _ := send(t, "GET", e.url+itemPath(api.PathLibraryItem, sub.ID), "", nil, nil); code != http.StatusGone {
		t.Fatalf("its address after the purge: %d", code)
	}
	if _, err := e.db.LibraryItem(ctx, left.ID); err == nil {
		t.Fatal("an upload left unfinished stayed")
	}
	var mine api.LibraryMine
	send(t, "GET", e.url+api.PathLibraryMine, ada.token, nil, &mine)
	if mine.Used != 0 {
		t.Fatalf("still counted: %d", mine.Used)
	}
}

// A moderator reads a version under review, sandboxed; nobody else does.
func TestLibraryAdminPart(t *testing.T) {
	e := newLibEnv(t, Config{})
	boss := e.webSession(e.admin.acc)
	ada := e.signIn(1, "ada", "")
	sub := e.publish(ada, publishParams("One"), `<p>x</p>`)
	path := strings.NewReplacer("{id}", sub.ID, "{version}", "1", "{part}", api.PartDoc).Replace(api.PathLibraryAdminPart)
	if code, _ := send(t, "GET", e.url+path, e.webSession(ada.acc), nil, nil); code != http.StatusForbidden {
		t.Fatalf("not a moderator: %d", code)
	}
	body, code, hdr := doRaw(t, "GET", e.url+path, boss)
	if code != http.StatusOK || !strings.Contains(body, "<p>x</p>") {
		t.Fatalf("doc: %d %.80s", code, body)
	}
	if !strings.Contains(hdr.Get("Content-Security-Policy"), "sandbox") {
		t.Fatalf("csp: %q", hdr.Get("Content-Security-Policy"))
	}
}

// Objects reach the website, not anyone else, and only a live version's: other
// people's HTML is never served from this server's address to a browser.
func TestLibraryObjectsGoToTheWebsiteAlone(t *testing.T) {
	e := newLibEnv(t, Config{})
	boss := e.webSession(e.admin.acc)
	ada := e.signIn(1, "ada", "")
	sub := e.publish(ada, publishParams("One"), `<p>x</p>`)
	part := func(id string, v int, p string) string { return partPath(id, v, p) }
	web := map[string]string{api.HeaderLibraryWeb: libSecret}
	if code, _ := sendWith(t, "GET", e.url+part(sub.ID, 1, api.PartThumb), "", web, nil, nil); code != http.StatusNotFound {
		t.Fatalf("in review, to the website: %d", code)
	}
	e.decide(boss, sub.ID, 1, api.DecisionApprove, "ok")
	for name, c := range map[string]struct {
		token string
		hdr   map[string]string
	}{"nobody": {}, "a session": {token: boss}, "a device": {token: ada.token}, "a wrong secret": {hdr: map[string]string{api.HeaderLibraryWeb: "wrong"}}} {
		if code, _ := sendWith(t, "GET", e.url+part(sub.ID, 1, api.PartDoc), c.token, c.hdr, nil, nil); code != http.StatusForbidden {
			t.Errorf("%s: %d", name, code)
		}
	}
	req, _ := http.NewRequest("GET", e.url+part(sub.ID, 1, api.PartDoc), nil)
	req.Header.Set(api.HeaderLibraryWeb, libSecret)
	res, err := (&http.Client{Transport: &http.Transport{DisableCompression: true}}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	doc, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK || !bytes.HasPrefix(doc, []byte{0x1f, 0x8b}) || !strings.Contains(res.Header.Get("Content-Security-Policy"), "sandbox") {
		t.Fatalf("a document, as it's kept: %d %.20q %v", res.StatusCode, doc, res.Header)
	}
	if code, _ := sendWith(t, "GET", e.url+part(sub.ID, 1, api.PartThumb), "", web, nil, nil); code != http.StatusOK {
		t.Fatalf("a picture: %d", code)
	}
	if code, _ := sendWith(t, "GET", e.url+part(sub.ID, 1, api.PartSource), "", web, nil, nil); code != http.StatusNotFound {
		t.Fatalf("a bundle: %d", code)
	}
	for _, id := range []string{"..%2F..%2Fteams", "teams"} {
		if code, _ := sendWith(t, "GET", e.url+"/v1/library/items/"+id+"/versions/1/doc", "", web, nil, nil); code != http.StatusNotFound {
			t.Errorf("%s: %d", id, code)
		}
	}
	if code, _ := send(t, "DELETE", e.url+itemPath(api.PathLibraryItem, sub.ID), ada.token, nil, nil); code != http.StatusNoContent {
		t.Fatalf("withdraw: %d", code)
	}
	if code, _ := sendWith(t, "GET", e.url+part(sub.ID, 1, api.PartDoc), "", web, nil, nil); code != http.StatusGone {
		t.Fatalf("unpublished: %d", code)
	}
}

// Sharing a store with teams: a team's file stays where it is, isn't counted
// against the library's limits, and the library's objects aren't among a team's.
func TestLibrarySharesTheStoreWithTeams(t *testing.T) {
	dir := blobstore.Dir(t.TempDir())
	e := newLibEnvOver(t, Config{Blobs: dir}, dir)
	ada := e.signIn(1, "ada", "")
	team := e.team(ada, "t")
	blob := strings.Repeat("a", 32)
	if code, msg := e.put(ada.token, "/v1/teams/"+team+"/blobs/"+blob, []byte("sealed")); code != http.StatusNoContent {
		t.Fatalf("a team's file: %d %s", code, msg)
	}
	boss := e.webSession(e.admin.acc)
	var before api.LibraryUsage
	send(t, "GET", e.url+api.PathLibraryUsage, boss, nil, &before)
	if before.BytesStored != 0 || before.BytesWritten != 0 {
		t.Fatalf("a team's file counts against the library: %+v", before)
	}
	sub := e.publish(ada, publishParams("One"), `<p>x</p>`)
	ctx := context.Background()
	if got, err := dir.Get(ctx, "teams/"+team+"/"+blob); err != nil || string(got) != "sealed" {
		t.Fatalf("a team's file changed: %q %v", got, err)
	}
	if _, err := dir.Get(ctx, "library/items/"+sub.ID+"/v1/doc"); err != nil {
		t.Fatalf("not under its prefix: %v", err)
	}
	if _, err := dir.Get(ctx, "items/"+sub.ID+"/v1/doc"); err == nil {
		t.Fatal("outside its prefix")
	}
	var mine api.LibraryMine
	send(t, "GET", e.url+api.PathLibraryMine, ada.token, nil, &mine)
	if mine.Used == 0 {
		t.Fatal("the library's own isn't counted")
	}
}

// Over an account's storage the next part is refused and nothing written.
func TestLibraryQuota(t *testing.T) {
	e := newLibEnv(t, Config{})
	ada := e.signIn(1, "ada", "")
	var made api.LibraryPublished
	send(t, "POST", e.url+api.PathLibraryItems, ada.token, publishParams("t"), &made)
	if _, err := e.db.ExecContext(context.Background(), `UPDATE library_profiles SET used_bytes = $1 WHERE account_id = $2`, int64(api.MaxLibraryAccount-10), ada.acc.ID); err != nil {
		t.Fatal(err)
	}
	if code, msg := e.put(ada.token, partPath(made.ID, 1, api.PartThumb), thumb(t)); code != http.StatusInsufficientStorage {
		t.Fatalf("over quota: %d %s", code, msg)
	}
	if _, err := e.blobs.Get(context.Background(), "library/items/"+made.ID+"/v1/thumb"); err == nil {
		t.Fatal("written over quota")
	}
}

// Filters and search find what they say; a %, in a search, is a %.
func TestLibraryBrowse(t *testing.T) {
	e := newLibEnv(t, Config{})
	ada := e.signIn(1, "ada", "")
	boss := e.webSession(e.admin.acc)
	first := e.publish(ada, publishParams("Alpha dashboard"), `<p>x</p>`)
	e.decide(boss, first.ID, 1, api.DecisionApprove, "ok")
	c := publishParams("Beta 100% card")
	c.Kind, c.Category, c.License = api.LibraryComponent, "component", api.LicenseViewOnly
	e.publish(ada, c, `<p>x</p>`)
	for query, want := range map[string]int{"": 2, "?kind=component": 1, "?category=dashboard": 1, "?q=alpha": 1, "?q=100%25": 1, "?q=%25": 1, "?q=nothing": 0, "?license=view-only": 1, "?tag=dark-mode": 2, "?sort=featured": 0, "?limit=1": 1} {
		var got api.LibraryItems
		if code, msg := send(t, "GET", e.url+api.PathLibraryItems+query, "", nil, &got); code != http.StatusOK {
			t.Fatalf("%q: %d %s", query, code, msg)
		}
		if len(got.Items) != want {
			t.Errorf("%q: %d items, want %d", query, len(got.Items), want)
		}
	}
	var page api.LibraryItems
	send(t, "GET", e.url+api.PathLibraryItems+"?limit=1", "", nil, &page)
	if page.Next == "" {
		t.Fatal("no next page")
	}
	var second api.LibraryItems
	send(t, "GET", e.url+api.PathLibraryItems+"?limit=1&cursor="+page.Next, "", nil, &second)
	if len(second.Items) != 1 || second.Items[0].ID == page.Items[0].ID {
		t.Fatalf("second page: %+v", second)
	}
	if code, _ := e.decide(boss, first.ID, 0, api.DecisionFeature, "a pick"); code != http.StatusNoContent {
		t.Fatalf("feature: %d", code)
	}
	var picks api.LibraryItems
	send(t, "GET", e.url+api.PathLibraryItems+"?sort=featured", "", nil, &picks)
	if len(picks.Items) != 1 || !picks.Items[0].Featured {
		t.Fatalf("picks: %+v", picks)
	}
}

// Past 70% moderators are warned; a full bucket stops publishing, not reading.
func TestLibraryUsageWarns(t *testing.T) {
	e := newLibEnv(t, Config{})
	boss := e.webSession(e.admin.acc)
	var u api.LibraryUsage
	send(t, "GET", e.url+api.PathLibraryUsage, boss, nil, &u)
	if u.Warn {
		t.Fatalf("warned at nothing: %+v", u)
	}
	ada := e.signIn(1, "ada", "")
	e.publish(ada, publishParams("t"), `<p>x</p>`)
	if _, err := e.db.ExecContext(context.Background(), `UPDATE library_profiles SET used_bytes = $1 WHERE account_id = $2`, int64(api.MaxLibraryBucket)*71/100, e.admin.acc.ID); err != nil {
		t.Fatal(err)
	}
	send(t, "GET", e.url+api.PathLibraryUsage, boss, nil, &u)
	if !u.Warn || u.WriteOps == 0 || u.BytesWritten == 0 {
		t.Fatalf("usage: %+v", u)
	}
	if code, _ := send(t, "POST", e.url+api.PathLibraryItems, ada.token, publishParams("over"), nil); code == http.StatusInsufficientStorage {
		t.Fatal("at 71% publishing stops")
	}
	if _, err := e.db.ExecContext(context.Background(), `UPDATE library_profiles SET used_bytes = $1 WHERE account_id = $2`, int64(api.MaxLibraryBucket), e.admin.acc.ID); err != nil {
		t.Fatal(err)
	}
	if code, _ := send(t, "POST", e.url+api.PathLibraryItems, ada.token, publishParams("full"), nil); code != http.StatusInsufficientStorage {
		t.Fatalf("a full bucket: %d", code)
	}
	if code, _ := send(t, "GET", e.url+api.PathLibraryItems, "", nil, nil); code != http.StatusOK {
		t.Fatalf("reading a full bucket: %d", code)
	}
}

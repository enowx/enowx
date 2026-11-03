package server

import (
	"context"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/enowx/enowx/cloud/store"
	"github.com/enowx/enowx/cloud/store/storetest"
)

// contributorRig is a server with the role on, and stand-ins for GitHub and Discord:
// nothing here reaches either.
type contributorRig struct {
	srv    *httptest.Server
	s      *Server
	db     *store.DB
	priv   ed25519.PrivateKey
	mu     sync.Mutex
	roles  map[string]bool
	merged bool
	scope  []string
	search string
}

func (r *contributorRig) hasRole(member string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.roles[member]
}

func newContributorRig(t *testing.T) *contributorRig {
	rig := &contributorRig{roles: map[string]bool{}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /login/oauth/authorize", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		rig.mu.Lock()
		rig.scope = append(rig.scope, q.Get("scope"))
		rig.mu.Unlock()
		http.Redirect(w, r, q.Get("redirect_uri")+"?code=good&state="+url.QueryEscape(q.Get("state")), http.StatusFound)
	})
	mux.HandleFunc("POST /login/oauth/access_token", func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"access_token": "gho_ada"})
	})
	mux.HandleFunc("GET /user", func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"id": 42, "login": "ada"})
	})
	mux.HandleFunc("GET /search/issues", func(w http.ResponseWriter, r *http.Request) {
		rig.mu.Lock()
		rig.search = r.URL.Query().Get("q")
		total := 0
		if rig.merged {
			total = 1
		}
		rig.mu.Unlock()
		json.NewEncoder(w).Encode(map[string]int{"total_count": total})
	})
	mux.HandleFunc("/guilds/", func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(r.URL.Path, "/")
		rig.mu.Lock()
		rig.roles[parts[4]] = r.Method == http.MethodPut
		rig.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})
	outside := httptest.NewServer(mux)
	t.Cleanup(outside.Close)
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	rig.priv = priv
	var handler http.Handler
	rig.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { handler.ServeHTTP(w, r) }))
	t.Cleanup(rig.srv.Close)
	rig.db = storetest.Open(t)
	rig.s = New(Config{URL: rig.srv.URL, GitHubClientID: "cid", GitHubClientSecret: "csecret", GitHubURL: outside.URL, GitHubAPIURL: outside.URL,
		Contributor: ContributorConfig{DiscordPublicKey: hex.EncodeToString(pub), DiscordBotToken: "bot", DiscordGuildID: "g1", DiscordRoleID: "r1",
			WebhookSecret: "hook", LinkSecret: "link", Repo: "enowdev/enowx", DiscordURL: outside.URL}},
		rig.db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	handler = rig.s
	return rig
}

// browser follows a link through sign-in, as one browser with its cookies.
func browser(t *testing.T) *http.Client {
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{Jar: jar}
}

func (r *contributorRig) signIn(t *testing.T, c *http.Client, member string) (int, string) {
	t.Helper()
	start := r.srv.URL + pathContributorStart + "?" + url.Values{"link": {r.s.contributor.makeLink(member, "ada")}}.Encode()
	res, err := c.Get(start)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(body)
}

// A member with a merged pull request gets the role, for no scope, and two IDs are kept.
func TestContributorConnects(t *testing.T) {
	rig := newContributorRig(t)
	rig.merged = true
	code, body := rig.signIn(t, browser(t), "m1")
	if code != http.StatusOK || !strings.Contains(body, "Contributor") {
		t.Fatalf("%d %s", code, body)
	}
	if !rig.hasRole("m1") {
		t.Fatal("no role")
	}
	if len(rig.scope) != 1 || rig.scope[0] != "" {
		t.Fatalf("asked for a scope: %q", rig.scope)
	}
	if !strings.Contains(rig.search, "repo:enowdev/enowx") || !strings.Contains(rig.search, "is:merged") || !strings.Contains(rig.search, "author:ada") {
		t.Fatalf("searched %q", rig.search)
	}
	if id, err := rig.db.DiscordOfGitHub(context.Background(), 42); err != nil || id != "m1" {
		t.Fatalf("link: %q %v", id, err)
	}
}

// One without is told so and gets no role, then gets it when a pull request merges;
// what isn't merged here, or isn't signed, does nothing.
func TestContributorRoleOnMerge(t *testing.T) {
	rig := newContributorRig(t)
	if code, body := rig.signIn(t, browser(t), "m1"); code != http.StatusOK || !strings.Contains(body, "no merged pull request") {
		t.Fatalf("%d %s", code, body)
	}
	if rig.hasRole("m1") {
		t.Fatal("a role with no merged pull request")
	}
	send := func(payload, secret string) int {
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(payload))
		req, _ := http.NewRequest("POST", rig.srv.URL+pathGitHubWebhook, strings.NewReader(payload))
		req.Header.Set("X-GitHub-Event", "pull_request")
		req.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res.StatusCode
	}
	merged := `{"action":"closed","pull_request":{"merged":true,"user":{"id":42}},"repository":{"full_name":"enowdev/enowx"}}`
	if code := send(merged, "wrong"); code != http.StatusUnauthorized || rig.hasRole("m1") {
		t.Fatalf("a forged delivery: %d", code)
	}
	send(`{"action":"closed","pull_request":{"merged":false,"user":{"id":42}},"repository":{"full_name":"enowdev/enowx"}}`, "hook")
	send(`{"action":"closed","pull_request":{"merged":true,"user":{"id":42}},"repository":{"full_name":"someone/else"}}`, "hook")
	if rig.hasRole("m1") {
		t.Fatal("a role for what didn't merge here")
	}
	if code := send(merged, "hook"); code != http.StatusNoContent || !rig.hasRole("m1") {
		t.Fatalf("merged: %d, role %v", code, rig.hasRole("m1"))
	}
}

// One GitHub account links to one Discord account.
func TestContributorOneAccount(t *testing.T) {
	rig := newContributorRig(t)
	rig.merged = true
	rig.signIn(t, browser(t), "m1")
	if code, _ := rig.signIn(t, browser(t), "m2"); code != http.StatusConflict || rig.hasRole("m2") {
		t.Fatalf("%d, role %v", code, rig.hasRole("m2"))
	}
}

// Disconnect removes the role and the link, and the role with no link too.
func TestContributorDisconnect(t *testing.T) {
	rig := newContributorRig(t)
	rig.merged = true
	rig.signIn(t, browser(t), "m1")
	got := rig.s.contributorDisconnect(context.Background(), "m1")
	if !strings.Contains(got, "removed") || rig.hasRole("m1") {
		t.Fatalf("%q, role %v", got, rig.hasRole("m1"))
	}
	if on, _ := rig.db.DiscordLinked(context.Background(), "m1"); on {
		t.Fatal("still linked")
	}
	rig.mu.Lock()
	rig.roles["m1"] = true // given by hand
	rig.mu.Unlock()
	if got := rig.s.contributorDisconnect(context.Background(), "m1"); !strings.Contains(got, "no GitHub link") || rig.hasRole("m1") {
		t.Fatalf("%q, role %v", got, rig.hasRole("m1"))
	}
}

// A link can't be changed to another member's, doesn't last, and works once; and a
// sign-in begun elsewhere can't be finished here.
func TestContributorLinkCantBeForged(t *testing.T) {
	rig := newContributorRig(t)
	role := rig.s.contributor
	token := role.makeLink("m1", "ada")
	l, ok := role.readLink(token)
	if !ok || l.Discord != "m1" {
		t.Fatalf("%+v %v", l, ok)
	}
	body, sig, _ := strings.Cut(token, ".")
	if _, ok := role.readLink(body + "x." + sig); ok {
		t.Fatal("a changed link read")
	}
	if !role.spend(l) || role.spend(l) {
		t.Fatal("not once")
	}
	if _, ok := role.readLink(token); ok {
		t.Fatal("a used link read")
	}
	old := role.makeLink("m2", "")
	role.now = func() time.Time { return time.Now().Add(linkFor + time.Minute) }
	if _, ok := role.readLink(old); ok {
		t.Fatal("an old link read")
	}
	role.now = time.Now
	// The GitHub step of someone else's flow, opened in a browser that didn't begin it.
	victim := role.makeLink("attacker", "attacker")
	res, err := browser(t).Get(rig.srv.URL + pathContributorCallback + "?" + url.Values{"code": {"good"}, "state": {"bind~" + victim}}.Encode())
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("%d", res.StatusCode)
	}
	if id, _ := rig.db.DiscordOfGitHub(context.Background(), 42); id != "" {
		t.Fatalf("linked %q", id)
	}
}

// Only what Discord signed is taken; a ping is answered as it asks; Connect replies to
// the member who clicked alone with their own link; Disconnect replies to them alone,
// linked or not; another server gets neither; a slash command is no longer answered.
func TestContributorInteractions(t *testing.T) {
	rig := newContributorRig(t)
	send := func(body string, signed bool) (int, string) {
		req, _ := http.NewRequest("POST", rig.srv.URL+pathDiscordInteractions, strings.NewReader(body))
		req.Header.Set("X-Signature-Timestamp", "1")
		sig := make([]byte, ed25519.SignatureSize)
		if signed {
			sig = ed25519.Sign(rig.priv, append([]byte("1"), body...))
		}
		req.Header.Set("X-Signature-Ed25519", hex.EncodeToString(sig))
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		data, _ := io.ReadAll(res.Body)
		return res.StatusCode, string(data)
	}
	if code, _ := send(`{"type":1}`, false); code != http.StatusUnauthorized {
		t.Fatalf("unsigned: %d", code)
	}
	if code, body := send(`{"type":1}`, true); code != http.StatusOK || !strings.Contains(body, `"type":1`) {
		t.Fatalf("ping: %d %s", code, body)
	}
	click := func(guild, member, button string) string {
		t.Helper()
		code, body := send(fmt.Sprintf(`{"type":3,"guild_id":%q,"member":{"user":{"id":%q,"username":"ada"}},"data":{"custom_id":%q,"component_type":2}}`, guild, member, button), true)
		if code != http.StatusOK || !strings.Contains(body, `"flags":64`) {
			t.Fatalf("%s: %d %s", button, code, body)
		}
		return body
	}
	if code, _ := send(`{"type":3,"guild_id":"g1","member":{"user":{"id":"m1"}},"data":{"custom_id":"contributor:connect"}}`, false); code != http.StatusUnauthorized {
		t.Fatalf("an unsigned click: %d", code)
	}
	body := click("g1", "m1", buttonConnect)
	var reply struct {
		Data struct {
			Content string `json:"content"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &reply); err != nil {
		t.Fatal(err)
	}
	_, after, ok := strings.Cut(reply.Data.Content, "?link=")
	if !ok {
		t.Fatalf("connect: %s", body)
	}
	token, _, _ := strings.Cut(after, ")")
	token, err := url.QueryUnescape(token)
	if err != nil {
		t.Fatal(err)
	}
	if l, ok := rig.s.contributor.readLink(token); !ok || l.Discord != "m1" {
		t.Fatalf("the link is for %q, good %v", l.Discord, ok)
	}
	if body := click("other", "m1", buttonConnect); strings.Contains(body, "link=") {
		t.Fatalf("another server: %s", body)
	}
	rig.mu.Lock()
	rig.roles["m1"] = true
	rig.mu.Unlock()
	if body := click("other", "m1", buttonDisconnect); !rig.hasRole("m1") {
		t.Fatalf("another server took the role: %s", body)
	}
	if body := click("g1", "m1", buttonDisconnect); !strings.Contains(body, "no GitHub link") || rig.hasRole("m1") {
		t.Fatalf("disconnect: %s, role %v", body, rig.hasRole("m1"))
	}
	if body := click("g1", "m1", "something-else"); strings.Contains(body, "link=") {
		t.Fatalf("an unknown button: %s", body)
	}
	if code, body := send(`{"type":2,"guild_id":"g1","member":{"user":{"id":"m1"}},"data":{"name":"connect"}}`, true); code != http.StatusBadRequest || strings.Contains(body, "link=") {
		t.Fatalf("a slash command: %d %s", code, body)
	}
}

// Without its configuration its routes aren't there, and the server is as ever; a part
// of it missing is off too, and says what.
func TestContributorOff(t *testing.T) {
	db := storetest.Open(t)
	s := New(Config{URL: "http://cloud.test", GitHubClientID: "cid", GitHubClientSecret: "csecret"}, db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if s.contributor != nil {
		t.Fatal("on with nothing set")
	}
	for _, path := range []string{pathContributorPage, pathContributorStart, pathContributorCallback} {
		w := httptest.NewRecorder()
		s.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != http.StatusNotFound {
			t.Errorf("%s: %d", path, w.Code)
		}
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest("GET", pathGitHub, nil))
	if w.Code != http.StatusFound {
		t.Errorf("sign-in changed: %d", w.Code)
	}
	part := New(Config{URL: "http://cloud.test", GitHubClientID: "cid", GitHubClientSecret: "csecret", Contributor: ContributorConfig{DiscordBotToken: "x"}}, db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if part.contributor != nil {
		t.Fatal("on with a part set")
	}
}

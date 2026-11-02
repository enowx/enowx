package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakes stands for Discord and GitHub: nothing here reaches either.
type fakes struct {
	mu     sync.Mutex
	roles  map[string]bool
	merged bool
	id     int64
	login  string
	search string
	srv    *httptest.Server
}

func newFakes(t *testing.T) *fakes {
	f := &fakes{roles: map[string]bool{}, id: 42, login: "ada"}
	mux := http.NewServeMux()
	mux.HandleFunc("/login/oauth/access_token", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]string{"access_token": "gho_test"})
	})
	mux.HandleFunc("/user", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"id": f.id, "login": f.login})
	})
	mux.HandleFunc("/search/issues", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.search = r.URL.Query().Get("q")
		merged := f.merged
		f.mu.Unlock()
		total := 0
		if merged {
			total = 1
		}
		writeJSON(w, map[string]int{"total_count": total})
	})
	mux.HandleFunc("/guilds/", func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(r.URL.Path, "/")
		f.mu.Lock()
		f.roles[parts[4]] = r.Method == http.MethodPut
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakes) hasRole(member string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.roles[member]
}

func newTestBot(t *testing.T, f *fakes) (*bot, ed25519.PrivateKey) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config{BaseURL: "https://bot.test", PublicKey: hex.EncodeToString(pub), GuildID: "g1", RoleID: "r1", ClientID: "c", ClientSecret: "s",
		WebhookSecret: "hook", StateSecret: "state", Repo: "enowdev/enowx", LinksFile: filepath.Join(t.TempDir(), "links.json")}
	l, err := openLinks(cfg.LinksFile)
	if err != nil {
		t.Fatal(err)
	}
	d := newDiscord(cfg)
	d.api = f.srv.URL
	g := newGitHub(cfg)
	g.web, g.api = f.srv.URL, f.srv.URL
	b, err := newBot(cfg, l, d, g)
	if err != nil {
		t.Fatal(err)
	}
	return b, priv
}

func signIn(t *testing.T, b *bot, member string) (int, string) {
	t.Helper()
	q := url.Values{"state": {b.makeState(member)}, "code": {"c"}}
	w := httptest.NewRecorder()
	b.routes().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/github/callback?"+q.Encode(), nil))
	return w.Code, w.Body.String()
}

// A member with a merged pull request gets the role at once.
func TestConnectWithMergedPR(t *testing.T) {
	f := newFakes(t)
	f.merged = true
	b, _ := newTestBot(t, f)
	if code, body := signIn(t, b, "m1"); code != http.StatusOK || !strings.Contains(body, "Contributor role") {
		t.Fatalf("%d %s", code, body)
	}
	if !f.hasRole("m1") {
		t.Fatal("no role")
	}
	if !strings.Contains(f.search, "repo:enowdev/enowx") || !strings.Contains(f.search, "is:merged") || !strings.Contains(f.search, "author:ada") {
		t.Fatalf("searched %q", f.search)
	}
}

// One without is told so, gets no role, and gets it when a pull request merges.
func TestRoleOnMergeAfterLinking(t *testing.T) {
	f := newFakes(t)
	b, _ := newTestBot(t, f)
	if _, body := signIn(t, b, "m1"); !strings.Contains(body, "no merged pull request") {
		t.Fatal(body)
	}
	if f.hasRole("m1") {
		t.Fatal("a role with no merged pull request")
	}
	send := func(payload string) int {
		mac := hmac.New(sha256.New, []byte("hook"))
		mac.Write([]byte(payload))
		r := httptest.NewRequest(http.MethodPost, "/github/webhook", strings.NewReader(payload))
		r.Header.Set("X-GitHub-Event", "pull_request")
		r.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
		w := httptest.NewRecorder()
		b.routes().ServeHTTP(w, r)
		return w.Code
	}
	closed := `{"action":"closed","pull_request":{"merged":false,"user":{"id":42}},"repository":{"full_name":"enowdev/enowx"}}`
	other := `{"action":"closed","pull_request":{"merged":true,"user":{"id":42}},"repository":{"full_name":"someone/else"}}`
	merged := `{"action":"closed","pull_request":{"merged":true,"user":{"id":42}},"repository":{"full_name":"enowdev/enowx"}}`
	send(closed)
	send(other)
	if f.hasRole("m1") {
		t.Fatal("a role for what didn't merge here")
	}
	if code := send(merged); code != http.StatusNoContent || !f.hasRole("m1") {
		t.Fatalf("merged: %d, role %v", code, f.hasRole("m1"))
	}
}

// A delivery not signed with the secret does nothing.
func TestWebhookSignature(t *testing.T) {
	f := newFakes(t)
	b, _ := newTestBot(t, f)
	r := httptest.NewRequest(http.MethodPost, "/github/webhook", strings.NewReader(`{}`))
	r.Header.Set("X-Hub-Signature-256", "sha256=00")
	w := httptest.NewRecorder()
	b.routes().ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("%d", w.Code)
	}
}

// One GitHub account links to one Discord account.
func TestOneAccountOneMember(t *testing.T) {
	f := newFakes(t)
	f.merged = true
	b, _ := newTestBot(t, f)
	signIn(t, b, "m1")
	if code, _ := signIn(t, b, "m2"); code != http.StatusConflict {
		t.Fatalf("%d", code)
	}
	if f.hasRole("m2") {
		t.Fatal("a second member got the role")
	}
}

// /disconnect removes the role and the link; only the two IDs were ever kept.
func TestDisconnect(t *testing.T) {
	f := newFakes(t)
	f.merged = true
	b, _ := newTestBot(t, f)
	signIn(t, b, "m1")
	data, err := os.ReadFile(b.cfg.LinksFile)
	if err != nil {
		t.Fatal(err)
	}
	var kept map[string]int64
	if err := json.Unmarshal(data, &kept); err != nil || len(kept) != 1 || kept["m1"] != 42 {
		t.Fatalf("kept %s: %v", data, err)
	}
	if strings.Contains(string(data), "ada") || strings.Contains(string(data), "gho_") {
		t.Fatalf("kept more than IDs: %s", data)
	}
	got := b.disconnect(t.Context(), "m1")
	if !strings.Contains(got, "removed") || f.hasRole("m1") || b.links.has("m1") {
		t.Fatalf("%q, role %v", got, f.hasRole("m1"))
	}
	if got := b.disconnect(t.Context(), "m1"); !strings.Contains(got, "no GitHub link") {
		t.Fatal(got)
	}
}

// A link can't be forged, and doesn't last.
func TestState(t *testing.T) {
	f := newFakes(t)
	b, _ := newTestBot(t, f)
	state := b.makeState("m1")
	if m, ok := b.readState(state); !ok || m != "m1" {
		t.Fatalf("%q %v", m, ok)
	}
	if _, ok := b.readState(strings.Replace(state, "m1", "m2", 1)); ok {
		t.Fatal("another member's state read")
	}
	b.now = func() time.Time { return time.Now().Add(stateFor + time.Minute) }
	if _, ok := b.readState(state); ok {
		t.Fatal("an old state read")
	}
}

// Only what Discord signed is taken, and a ping is answered as it asks.
func TestInteractions(t *testing.T) {
	f := newFakes(t)
	b, priv := newTestBot(t, f)
	send := func(body string, sign bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/discord/interactions", bytes.NewReader([]byte(body)))
		r.Header.Set("X-Signature-Timestamp", "1")
		sig := make([]byte, ed25519.SignatureSize)
		if sign {
			sig = ed25519.Sign(priv, append([]byte("1"), body...))
		}
		r.Header.Set("X-Signature-Ed25519", hex.EncodeToString(sig))
		w := httptest.NewRecorder()
		b.routes().ServeHTTP(w, r)
		return w
	}
	if w := send(`{"type":1}`, false); w.Code != http.StatusUnauthorized {
		t.Fatalf("unsigned: %d", w.Code)
	}
	if w := send(`{"type":1}`, true); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"type":1`) {
		t.Fatalf("ping: %d %s", w.Code, w.Body)
	}
	w := send(`{"type":2,"guild_id":"g1","member":{"user":{"id":"m1"}},"data":{"name":"connect"}}`, true)
	if !strings.Contains(w.Body.String(), "/connect?state=") || !strings.Contains(w.Body.String(), `"flags":64`) {
		t.Fatalf("connect: %s", w.Body)
	}
	w = send(`{"type":2,"guild_id":"other","member":{"user":{"id":"m1"}},"data":{"name":"connect"}}`, true)
	if strings.Contains(w.Body.String(), "state=") {
		t.Fatalf("another server: %s", w.Body)
	}
}

// Sign-in asks for no scope: the identity alone.
func TestNoScopeAsked(t *testing.T) {
	g := newGitHub(config{ClientID: "c"})
	if u := g.authorizeURL("s", "https://bot.test/github/callback"); strings.Contains(u, "scope") {
		t.Fatal(u)
	}
}

// All that's missing is named at once; a secret's value never is.
func TestConfigMissing(t *testing.T) {
	_, err := readConfig(func(name string) string {
		if name == "BASE_URL" {
			return "https://bot.test/"
		}
		return ""
	})
	if err == nil || !strings.Contains(err.Error(), "DISCORD_BOT_TOKEN") || !strings.Contains(err.Error(), "GITHUB_CLIENT_SECRET") {
		t.Fatalf("%v", err)
	}
}

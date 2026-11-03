package server

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/enowx/enowx/cloud/store"
)

// The Contributor role on Discord (docs/cloud.md): a member clicks Connect in a channel, signs in with
// GitHub on a page here, and gets the role for a merged pull request in one repository,
// then or when one merges. It's off unless wholly configured: its routes aren't there,
// and nothing else here depends on it.
//
// It takes the GitHub OAuth app of the cloud's sign-in, but not that sign-in: that makes
// an account, a session and a member of the default team, keeps a name and an email, and
// asks for more than an identity. This asks for no scope and keeps two IDs.

const (
	pathContributorPage     = "/contributor"
	pathContributorStart    = pathGitHub + "/contributor"
	pathContributorCallback = pathGitHubCallback + "/contributor" // a subfolder of the OAuth app's callback
	pathDiscordInteractions = "/v1/discord/interactions"
	pathGitHubWebhook       = "/v1/github/webhook"

	contributorCookie = "enowx_contributor"
	// linkFor is how long a Connect link works.
	linkFor = 10 * time.Minute
	// discordAnswer is what Discord waits for an interaction's reply, a little under.
	discordAnswer = 2500 * time.Millisecond
	// The buttons' custom_id, in the message docs/cloud.md says to post.
	buttonConnect    = "contributor:connect"
	buttonDisconnect = "contributor:disconnect"
	maxHook          = 8 << 20 // a pull request's delivery can be large
)

// ContributorConfig is the feature's configuration; all of it, or it stays off.
type ContributorConfig struct {
	DiscordPublicKey string // hex, Discord's to sign interactions with
	DiscordBotToken  string
	DiscordGuildID   string
	DiscordRoleID    string
	WebhookSecret    string // GitHub's webhook's
	LinkSecret       string // signs Connect links
	Repo             string // owner/name: only a merged pull request here counts
	// DiscordURL is Discord's API, its real one when empty (tests give their own).
	DiscordURL string
}

// Missing names what isn't set, by the field's use. All of them empty is simply off.
func (c ContributorConfig) Missing() []string {
	var out []string
	for _, f := range []struct{ name, v string }{
		{"discord public key", c.DiscordPublicKey}, {"discord bot token", c.DiscordBotToken}, {"discord guild", c.DiscordGuildID},
		{"discord role", c.DiscordRoleID}, {"webhook secret", c.WebhookSecret}, {"link secret", c.LinkSecret}, {"repository", c.Repo},
	} {
		if strings.TrimSpace(f.v) == "" {
			out = append(out, f.name)
		}
	}
	return out
}

// contributorRole is the feature's state; nil on a server without it.
type contributorRole struct {
	cfg  ContributorConfig
	key  ed25519.PublicKey
	http *http.Client
	api  string
	now  func() time.Time

	// used are the links used up, until they'd have expired anyway. Kept here, not in
	// the database, which holds the two IDs alone: a restart forgets them, within the
	// few minutes a link lives.
	mu   sync.Mutex
	used map[string]time.Time
}

// newContributorRole is the feature, nil and a reason when it can't run. It does
// nothing on the network: a server without Discord starts as ever.
func (s *Server) newContributorRole() (*contributorRole, string) {
	c := s.cfg.Contributor
	missing := c.Missing()
	switch {
	case len(missing) == 7:
		return nil, ""
	case len(missing) > 0:
		return nil, "not set: " + strings.Join(missing, ", ")
	case !s.githubOn():
		return nil, "it needs GitHub sign-in set up, for its OAuth app"
	case strings.Count(strings.Trim(c.Repo, "/"), "/") != 1:
		return nil, "the repository is owner/name"
	}
	key, err := hex.DecodeString(strings.TrimSpace(c.DiscordPublicKey))
	if err != nil || len(key) != ed25519.PublicKeySize {
		return nil, "the Discord public key isn't one, in hex"
	}
	api := strings.TrimSuffix(c.DiscordURL, "/")
	if api == "" {
		api = "https://discord.com/api/v10"
	}
	return &contributorRole{cfg: c, key: key, api: api, http: &http.Client{Timeout: 10 * time.Second}, now: time.Now, used: map[string]time.Time{}}, ""
}

// contributorRoutes adds the feature's routes, when it's configured.
func (s *Server) contributorRoutes() {
	role, why := s.newContributorRole()
	if why != "" {
		s.log.Warn("the Contributor role stays off", "why", why)
	}
	if role == nil {
		return
	}
	s.contributor = role
	s.mux.HandleFunc("POST "+pathDiscordInteractions, s.discordInteractions)
	s.mux.HandleFunc("GET "+pathContributorPage, s.contributorPage)
	s.mux.HandleFunc("GET "+pathContributorStart, s.contributorStart)
	s.mux.HandleFunc("GET "+pathContributorCallback, s.contributorCallback)
	s.mux.HandleFunc("POST "+pathGitHubWebhook, s.githubWebhook)
}

// A link names the member Discord said clicked Connect, signed, so that nobody can make
// one for another member. What it holds goes no further than the member's own browser.
type contributorLink struct {
	Discord string `json:"d"`
	Name    string `json:"n"` // shown on the page, for them to see whose it is
	Until   int64  `json:"u"`
	ID      string `json:"i"` // makes it usable once
}

func (r *contributorRole) sign(body string) string {
	mac := hmac.New(sha256.New, []byte(r.cfg.LinkSecret))
	mac.Write([]byte("enowx contributor link\x00" + body))
	return hex.EncodeToString(mac.Sum(nil))
}

// makeLink is a link's token for Discord account discord.
func (r *contributorRole) makeLink(discord, name string) string {
	var id [16]byte
	rand.Read(id[:])
	data, _ := json.Marshal(contributorLink{Discord: discord, Name: name, Until: r.now().Add(linkFor).Unix(), ID: hex.EncodeToString(id[:])})
	body := base64.RawURLEncoding.EncodeToString(data)
	return body + "." + r.sign(body)
}

// readLink is what token holds; false when forged, expired or used.
func (r *contributorRole) readLink(token string) (contributorLink, bool) {
	body, sig, ok := strings.Cut(token, ".")
	if !ok || !hmac.Equal([]byte(sig), []byte(r.sign(body))) {
		return contributorLink{}, false
	}
	data, err := base64.RawURLEncoding.DecodeString(body)
	var l contributorLink
	if err != nil || json.Unmarshal(data, &l) != nil || l.Discord == "" || r.now().Unix() > l.Until {
		return contributorLink{}, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	_, spent := r.used[l.ID]
	return l, !spent
}

// spend uses link l up, and says whether it was still good: of two at once, one wins.
func (r *contributorRole) spend(l contributorLink) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	for id, until := range r.used {
		if now.After(until) {
			delete(r.used, id)
		}
	}
	if _, spent := r.used[l.ID]; spent {
		return false
	}
	r.used[l.ID] = time.Unix(l.Until, 0)
	return true
}

// discordInteractions takes clicks on the channel's buttons, once Discord's signature holds.
func (s *Server) discordInteractions(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		s.fail(w, http.StatusRequestEntityTooLarge, "too large")
		return
	}
	if !verifyDiscord(s.contributor.key, r.Header.Get("X-Signature-Ed25519"), r.Header.Get("X-Signature-Timestamp"), body) {
		s.fail(w, http.StatusUnauthorized, "bad signature")
		return
	}
	var in struct {
		Type    int    `json:"type"`
		GuildID string `json:"guild_id"`
		Member  struct {
			User struct {
				ID       string `json:"id"`
				Username string `json:"username"`
			} `json:"user"`
		} `json:"member"`
		Data struct {
			CustomID string `json:"custom_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &in); err != nil {
		s.fail(w, http.StatusBadRequest, "bad request")
		return
	}
	const ping, click = 1, 3
	switch in.Type {
	case ping:
		s.reply(w, map[string]any{"type": 1})
	case click:
		s.reply(w, discordReply(s.contributorClick(r.Context(), in.GuildID, in.Member.User.ID, in.Member.User.Username, in.Data.CustomID)))
	default:
		s.fail(w, http.StatusBadRequest, "unknown interaction")
	}
}

// verifyDiscord says an interaction is signed with Discord's key.
func verifyDiscord(key ed25519.PublicKey, signature, timestamp string, body []byte) bool {
	sig, err := hex.DecodeString(signature)
	if err != nil || len(sig) != ed25519.SignatureSize || timestamp == "" {
		return false
	}
	return ed25519.Verify(key, append([]byte(timestamp), body...), sig)
}

// discordReply is a message only the member sees, mentioning nobody.
func discordReply(text string) map[string]any {
	const message, ephemeral = 4, 1 << 6
	return map[string]any{"type": message, "data": map[string]any{"content": text, "flags": ephemeral, "allowed_mentions": map[string]any{"parse": []string{}}}}
}

// contributorClick answers a click on one of the buttons, for the member who clicked.
func (s *Server) contributorClick(ctx context.Context, guild, member, name, button string) string {
	if guild != s.contributor.cfg.DiscordGuildID || member == "" {
		return "These buttons work on the enowX server."
	}
	switch button {
	case buttonConnect:
		link := s.base() + pathContributorPage + "?" + url.Values{"link": {s.contributor.makeLink(member, name)}}.Encode()
		return fmt.Sprintf("[Link your GitHub account](%s) to get Contributor for a merged pull request. This link is yours alone: don't share it. It works once, for 10 minutes.", link)
	case buttonDisconnect:
		return s.contributorDisconnect(ctx, member)
	}
	return "I don't know that button."
}

// contributorDisconnect removes the role, then the link: a role left with no link
// couldn't be taken away by asking again.
func (s *Server) contributorDisconnect(ctx context.Context, member string) string {
	ctx, cancel := context.WithTimeout(ctx, discordAnswer)
	defer cancel()
	linked, err := s.db.DiscordLinked(ctx, member)
	if err != nil {
		s.log.Warn("contributor: read the link", "err", err)
		return "I couldn't read your link just now. Please try again in a moment."
	}
	// With no link too: Disconnect takes the role away however it came.
	if err := s.contributor.setRole(ctx, member, false); err != nil {
		s.log.Warn("contributor: remove the role", "err", err)
		return "I couldn't remove the role just now, so nothing changed. Please try again in a moment."
	}
	if !linked {
		return "Your Discord account has no GitHub link, and it no longer has the Contributor role."
	}
	if err := s.db.UnlinkDiscord(ctx, member); err != nil {
		s.log.Warn("contributor: remove the link", "err", err)
		return "The role is gone, but I couldn't remove your link. Please try again in a moment."
	}
	return "Your GitHub link and the Contributor role are removed."
}

// setRole gives member the role, or takes it away.
func (r *contributorRole) setRole(ctx context.Context, member string, on bool) error {
	method := http.MethodDelete
	if on {
		method = http.MethodPut
	}
	path := "/guilds/" + url.PathEscape(r.cfg.DiscordGuildID) + "/members/" + url.PathEscape(member) + "/roles/" + url.PathEscape(r.cfg.DiscordRoleID)
	req, err := http.NewRequestWithContext(ctx, method, r.api+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bot "+r.cfg.DiscordBotToken)
	res, err := r.http.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err // no address in a log
		}
		return fmt.Errorf("discord: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		return fmt.Errorf("discord answered %d", res.StatusCode)
	}
	return nil
}

// contributorPage says whose Discord account the link is for, before anything happens,
// so that someone sent another member's link sees it isn't theirs.
func (s *Server) contributorPage(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("link")
	l, ok := s.contributor.readLink(token)
	if !ok {
		s.page(w, http.StatusBadRequest, "This link has expired or was used. Click Connect in Discord again.", nil)
		return
	}
	who := "the Discord account that clicked Connect"
	if l.Name != "" {
		who = "the Discord account " + l.Name
	}
	s.page(w, http.StatusOK, "", &pageData{Title: "Link your GitHub account",
		Lead:      "This links the GitHub account you sign in with to " + who + ". Only your public identity is read, and only the two accounts' IDs are kept.",
		SignInURL: pathContributorStart + "?" + url.Values{"link": {token}}.Encode(),
		Note:      "Continue only if you clicked Connect yourself. If someone sent you this link, close this page: it would give them credit for your work."})
}

// contributorStart sends the browser to GitHub, tying the sign-in to this browser: a
// sign-in begun elsewhere and sent here to finish doesn't count.
func (s *Server) contributorStart(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("link")
	if _, ok := s.contributor.readLink(token); !ok {
		s.page(w, http.StatusBadRequest, "This link has expired or was used. Click Connect in Discord again.", nil)
		return
	}
	var b [24]byte
	rand.Read(b[:])
	bind := base64.RawURLEncoding.EncodeToString(b[:])
	http.SetCookie(w, &http.Cookie{Name: contributorCookie, Value: bind, Path: pathGitHub, MaxAge: int(linkFor.Seconds()), HttpOnly: true, Secure: s.secure(), SameSite: http.SameSiteLaxMode})
	// No scope: the identity alone.
	q := url.Values{"client_id": {s.cfg.GitHubClientID}, "redirect_uri": {s.base() + pathContributorCallback}, "state": {bind + "~" + token}, "allow_signup": {"false"}}
	http.Redirect(w, r, s.githubWeb()+"/login/oauth/authorize?"+q.Encode(), http.StatusFound)
}

// contributorCallback finishes a sign-in: the two IDs linked, a merged pull request
// looked for, the role given. GitHub's token goes when this returns.
func (s *Server) contributorCallback(w http.ResponseWriter, r *http.Request) {
	again := "This link has expired or was used. Click Connect in Discord again."
	q := r.URL.Query()
	bind, token, _ := strings.Cut(q.Get("state"), "~")
	c, err := r.Cookie(contributorCookie)
	http.SetCookie(w, &http.Cookie{Name: contributorCookie, Path: pathGitHub, MaxAge: -1, HttpOnly: true, Secure: s.secure(), SameSite: http.SameSiteLaxMode})
	if err != nil || bind == "" || subtle.ConstantTimeCompare([]byte(c.Value), []byte(bind)) != 1 {
		s.page(w, http.StatusBadRequest, "This sign-in didn't start in this browser. Click Connect in Discord again.", nil)
		return
	}
	role := s.contributor
	l, ok := role.readLink(token)
	if !ok {
		s.page(w, http.StatusBadRequest, again, nil)
		return
	}
	if q.Get("error") != "" || q.Get("code") == "" {
		s.page(w, http.StatusForbidden, "GitHub didn't sign you in, so nothing changed.", nil)
		return
	}
	if !role.spend(l) {
		s.page(w, http.StatusBadRequest, again, nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	access, err := s.githubToken(ctx, q.Get("code"), s.base()+pathContributorCallback)
	if err != nil {
		s.log.Warn("contributor: sign-in", "err", err)
		s.page(w, http.StatusBadGateway, "GitHub didn't answer as expected, so nothing changed. Click Connect in Discord again.", nil)
		return
	}
	var user githubUser
	if err := s.githubGet(ctx, access, "/user", &user); err != nil || user.ID == 0 || user.Login == "" {
		s.log.Warn("contributor: identity", "err", err)
		s.page(w, http.StatusBadGateway, "GitHub didn't answer as expected, so nothing changed. Click Connect in Discord again.", nil)
		return
	}
	switch err := s.db.LinkDiscord(ctx, l.Discord, user.ID); {
	case errors.Is(err, store.ErrGitHubLinked):
		s.page(w, http.StatusConflict, "That GitHub account is already linked to another Discord account, which has to click Disconnect first.", nil)
		return
	case err != nil:
		s.log.Warn("contributor: keep the link", "err", err)
		s.page(w, http.StatusInternalServerError, "I couldn't keep the link, so nothing changed. Click Connect in Discord again.", nil)
		return
	}
	merged, err := s.mergedPR(ctx, access, user.Login)
	if err != nil {
		s.log.Warn("contributor: look for a merged pull request", "err", err)
		s.page(w, http.StatusBadGateway, "Your account is linked, but I couldn't look for a merged pull request. Click Connect in Discord again in a moment.", nil)
		return
	}
	if !merged {
		s.page(w, http.StatusOK, "", &pageData{Title: "Linked", Lead: fmt.Sprintf("%s has no merged pull request in %s yet, so there's no role for now. You'll get Contributor when one merges.", user.Login, role.cfg.Repo)})
		return
	}
	if err := role.setRole(ctx, l.Discord, true); err != nil {
		s.log.Warn("contributor: give the role", "err", err)
		s.page(w, http.StatusBadGateway, "Your account is linked and has a merged pull request, but I couldn't give the role. Click Connect in Discord again in a moment.", nil)
		return
	}
	s.page(w, http.StatusOK, "", &pageData{Title: "You're a Contributor", Done: "The role is yours on the server. You can close this page."})
}

// mergedPR says login has a merged pull request in the repository: public data.
func (s *Server) mergedPR(ctx context.Context, token, login string) (bool, error) {
	q := url.Values{"q": {fmt.Sprintf("repo:%s is:pr is:merged author:%s", strings.Trim(s.contributor.cfg.Repo, "/"), login)}, "per_page": {"1"}}
	var out struct {
		Total int `json:"total_count"`
	}
	err := s.githubGet(ctx, token, "/search/issues?"+q.Encode(), &out)
	return out.Total > 0, err
}

// githubWebhook gives the role when a pull request by someone linked merges.
func (s *Server) githubWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxHook))
	if err != nil {
		s.fail(w, http.StatusRequestEntityTooLarge, "too large")
		return
	}
	role := s.contributor
	if !verifyWebhook(role.cfg.WebhookSecret, r.Header.Get("X-Hub-Signature-256"), body) {
		s.fail(w, http.StatusUnauthorized, "bad signature")
		return
	}
	if r.Header.Get("X-GitHub-Event") != "pull_request" {
		w.WriteHeader(http.StatusNoContent) // its ping, and what else: nothing to do
		return
	}
	author, err := mergedAuthor(role.cfg.Repo, body)
	if err != nil {
		s.fail(w, http.StatusBadRequest, "bad request")
		return
	}
	if author == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	member, err := s.db.DiscordOfGitHub(ctx, author)
	if err != nil {
		s.log.Warn("contributor: read the link", "err", err)
		s.fail(w, http.StatusInternalServerError, "couldn't read the link")
		return
	}
	if member == "" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err := role.setRole(ctx, member, true); err != nil {
		// Failed on GitHub's side too, to deliver again from its page.
		s.log.Warn("contributor: give the role", "err", err)
		s.fail(w, http.StatusBadGateway, "couldn't give the role")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// verifyWebhook says a delivery is signed with the webhook's secret.
func verifyWebhook(secret, signature string, body []byte) bool {
	got, ok := strings.CutPrefix(signature, "sha256=")
	if !ok {
		return false
	}
	want, err := hex.DecodeString(got)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal(mac.Sum(nil), want)
}

// mergedAuthor reads a pull_request delivery: who wrote a pull request just merged in
// the repository, 0 for any other.
func mergedAuthor(repo string, body []byte) (int64, error) {
	var ev struct {
		Action      string `json:"action"`
		PullRequest struct {
			Merged bool `json:"merged"`
			User   struct {
				ID int64 `json:"id"`
			} `json:"user"`
		} `json:"pull_request"`
		Repository struct {
			FullName string `json:"full_name"`
		} `json:"repository"`
	}
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&ev); err != nil {
		return 0, err
	}
	if ev.Action != "closed" || !ev.PullRequest.Merged || !strings.EqualFold(ev.Repository.FullName, strings.Trim(repo, "/")) {
		return 0, nil
	}
	return ev.PullRequest.User.ID, nil
}

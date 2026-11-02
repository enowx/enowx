package main

import (
	"context"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	// stateFor is how long a /connect link works.
	stateFor = 10 * time.Minute
	// discordAnswer is what Discord waits for an interaction's reply, a little under.
	discordAnswer = 2500 * time.Millisecond
)

// bot is the service. Its links guard themselves; the rest doesn't change.
type bot struct {
	cfg     config
	links   *links
	discord *discord
	github  *github
	key     ed25519.PublicKey
	now     func() time.Time
}

func newBot(cfg config, l *links, d *discord, g *github) (*bot, error) {
	key, err := hex.DecodeString(cfg.PublicKey)
	if err != nil || len(key) != ed25519.PublicKeySize {
		return nil, errors.New("DISCORD_PUBLIC_KEY isn't a public key in hex")
	}
	return &bot{cfg: cfg, links: l, discord: d, github: g, key: key, now: time.Now}, nil
}

func (b *bot) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /discord/interactions", b.interactions)
	mux.HandleFunc("GET /connect", b.connect)
	mux.HandleFunc("GET /github/callback", b.callback)
	mux.HandleFunc("POST /github/webhook", b.webhook)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	return mux
}

func (b *bot) callbackURL() string { return b.cfg.BaseURL + "/github/callback" }

// makeState is a /connect link's proof of which member asked, and until when: signed,
// so that nothing is kept between asking and signing in.
func (b *bot) makeState(member string) string {
	body := member + "." + strconv.FormatInt(b.now().Add(stateFor).Unix(), 10)
	return body + "." + b.stateMAC(body)
}

func (b *bot) stateMAC(body string) string {
	mac := hmac.New(sha256.New, []byte(b.cfg.StateSecret))
	mac.Write([]byte(body))
	return hex.EncodeToString(mac.Sum(nil))
}

// readState is the member a state names; false when forged or too old.
func (b *bot) readState(state string) (string, bool) {
	cut := strings.LastIndexByte(state, '.')
	if cut < 0 || !hmac.Equal([]byte(state[cut+1:]), []byte(b.stateMAC(state[:cut]))) {
		return "", false
	}
	member, until, ok := strings.Cut(state[:cut], ".")
	end, err := strconv.ParseInt(until, 10, 64)
	if !ok || err != nil || b.now().Unix() > end {
		return "", false
	}
	return member, true
}

// interactions takes Discord's slash commands.
func (b *bot) interactions(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		http.Error(w, "too large", http.StatusRequestEntityTooLarge)
		return
	}
	if !verifyDiscord(b.key, r.Header.Get("X-Signature-Ed25519"), r.Header.Get("X-Signature-Timestamp"), body) {
		http.Error(w, "bad signature", http.StatusUnauthorized)
		return
	}
	var in interaction
	if err := json.Unmarshal(body, &in); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	switch in.Type {
	case interactionPing:
		writeJSON(w, map[string]any{"type": replyPong})
	case interactionCommand:
		writeJSON(w, ephemeral(b.command(r.Context(), in)))
	default:
		http.Error(w, "unknown interaction", http.StatusBadRequest)
	}
}

// command answers a slash command, in words only the member sees.
func (b *bot) command(ctx context.Context, in interaction) string {
	member := in.Member.User.ID
	if in.GuildID != b.cfg.GuildID || member == "" {
		return "This command works on the enowX server."
	}
	switch in.Data.Name {
	case "connect":
		link := b.cfg.BaseURL + "/connect?" + url.Values{"state": {b.makeState(member)}}.Encode()
		return fmt.Sprintf("[Sign in with GitHub](%s) to link your account. The link works for 10 minutes. Only your identity is read: no access to your repositories.", link)
	case "disconnect":
		return b.disconnect(ctx, member)
	}
	return "I don't know that command."
}

// disconnect removes the role, then the link: a role left with no link couldn't be
// taken back by asking again.
func (b *bot) disconnect(ctx context.Context, member string) string {
	if !b.links.has(member) {
		return "Your Discord account has no GitHub link."
	}
	ctx, cancel := context.WithTimeout(ctx, discordAnswer)
	defer cancel()
	if err := b.discord.removeRole(ctx, member); err != nil {
		slog.Error("disconnect: remove the role", "err", err)
		return "I couldn't remove the role just now, so your link stays. Please try again in a moment."
	}
	if _, err := b.links.unlink(member); err != nil {
		slog.Error("disconnect: remove the link", "err", err)
		return "The role is gone, but I couldn't remove your link. Please try again in a moment."
	}
	return "Your GitHub link and the Contributor role are removed."
}

// connect sends a member with a valid link on to GitHub.
func (b *bot) connect(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	if _, ok := b.readState(state); !ok {
		page(w, http.StatusBadRequest, "This link has expired. Run /connect in Discord again.")
		return
	}
	http.Redirect(w, r, b.github.authorizeURL(state, b.callbackURL()), http.StatusFound)
}

// callback finishes a sign-in: links the two IDs, checks for a merged pull request, and
// gives the role. The token goes when this returns.
func (b *bot) callback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	member, ok := b.readState(q.Get("state"))
	if !ok {
		page(w, http.StatusBadRequest, "This link has expired. Run /connect in Discord again.")
		return
	}
	code := q.Get("code")
	if code == "" {
		page(w, http.StatusBadRequest, "GitHub didn't sign you in, so nothing changed.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	token, err := b.github.exchange(ctx, code, b.callbackURL())
	if err != nil {
		slog.Error("callback: exchange", "err", err)
		page(w, http.StatusBadGateway, "GitHub didn't answer as expected. Run /connect again in a moment.")
		return
	}
	id, login, err := b.github.identity(ctx, token)
	if err != nil {
		slog.Error("callback: identity", "err", err)
		page(w, http.StatusBadGateway, "GitHub didn't answer as expected. Run /connect again in a moment.")
		return
	}
	if err := b.links.link(member, id); errors.Is(err, errTaken) {
		page(w, http.StatusConflict, "That GitHub account is already linked to another Discord account. It has to /disconnect first.")
		return
	} else if err != nil {
		slog.Error("callback: keep the link", "err", err)
		page(w, http.StatusInternalServerError, "I couldn't keep the link. Please try again in a moment.")
		return
	}
	merged, err := b.github.hasMergedPR(ctx, token, login)
	if err != nil {
		slog.Error("callback: check", "err", err)
		page(w, http.StatusBadGateway, "Your account is linked, but I couldn't check for a merged pull request. Run /connect again in a moment.")
		return
	}
	if !merged {
		page(w, http.StatusOK, fmt.Sprintf("Linked. %s has no merged pull request in %s yet, so there's no role for now. You'll get Contributor when one merges.", login, b.cfg.Repo))
		return
	}
	if err := b.discord.addRole(ctx, member); err != nil {
		slog.Error("callback: give the role", "err", err)
		page(w, http.StatusBadGateway, "Your account is linked and has a merged pull request, but I couldn't give the role. Run /connect again in a moment.")
		return
	}
	page(w, http.StatusOK, "Linked. You have the Contributor role now. You can close this page.")
}

// webhook gives the role when a pull request by someone linked merges.
func (b *bot) webhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 8*maxBody))
	if err != nil {
		http.Error(w, "too large", http.StatusRequestEntityTooLarge)
		return
	}
	if !verifyWebhook(b.cfg.WebhookSecret, r.Header.Get("X-Hub-Signature-256"), body) {
		http.Error(w, "bad signature", http.StatusUnauthorized)
		return
	}
	if r.Header.Get("X-GitHub-Event") != "pull_request" {
		w.WriteHeader(http.StatusNoContent) // ping and whatever else: nothing to do
		return
	}
	author, err := mergedAuthor(b.cfg.Repo, body)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	member, linked := b.links.discordOf(author)
	if author == 0 || !linked {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := b.discord.addRole(ctx, member); err != nil {
		// Failed on GitHub's side too, to deliver it again from its page.
		slog.Error("webhook: give the role", "err", err)
		http.Error(w, "couldn't give the role", http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// page is a plain page a member reads in their browser.
func page(w http.ResponseWriter, status int, text string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
	w.WriteHeader(status)
	fmt.Fprintf(w, `<!doctype html><meta charset="utf-8"><title>enowX Contributor</title><body style="font:16px system-ui;max-width:32rem;margin:4rem auto;padding:0 1rem"><p>%s</p>`, html.EscapeString(text))
}

package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// github is what the bot asks of GitHub. It asks for no OAuth scope, which gives a
// token reading public data only, used for one sign-in and never kept.
type github struct {
	web, api, clientID, clientSecret, repo string
	http                                   *http.Client
}

func newGitHub(c config) *github {
	return &github{web: "https://github.com", api: "https://api.github.com", clientID: c.ClientID, clientSecret: c.ClientSecret, repo: c.Repo,
		http: &http.Client{Timeout: 15 * time.Second}}
}

// authorizeURL is where a member signs in. No scope is asked for: the identity alone.
func (g *github) authorizeURL(state, callback string) string {
	q := url.Values{"client_id": {g.clientID}, "redirect_uri": {callback}, "state": {state}, "allow_signup": {"false"}}
	return g.web + "/login/oauth/authorize?" + q.Encode()
}

// exchange turns a sign-in's code into a token.
func (g *github) exchange(ctx context.Context, code, callback string) (string, error) {
	form := url.Values{"client_id": {g.clientID}, "client_secret": {g.clientSecret}, "code": {code}, "redirect_uri": {callback}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.web+"/login/oauth/access_token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	var out struct {
		Token string `json:"access_token"`
		Error string `json:"error"`
	}
	if err := g.do(req, &out); err != nil {
		return "", err
	}
	if out.Token == "" {
		return "", fmt.Errorf("github: no token: %s", out.Error)
	}
	return out.Token, nil
}

// identity is who a token belongs to.
func (g *github) identity(ctx context.Context, token string) (id int64, login string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.api+"/user", nil)
	if err != nil {
		return 0, "", err
	}
	g.headers(req, token)
	var out struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
	}
	if err := g.do(req, &out); err != nil {
		return 0, "", err
	}
	if out.ID == 0 || out.Login == "" {
		return 0, "", errors.New("github: no identity in its answer")
	}
	return out.ID, out.Login, nil
}

// hasMergedPR says login has a merged pull request in the repository: public data.
func (g *github) hasMergedPR(ctx context.Context, token, login string) (bool, error) {
	q := url.Values{"q": {fmt.Sprintf("repo:%s is:pr is:merged author:%s", g.repo, login)}, "per_page": {"1"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.api+"/search/issues?"+q.Encode(), nil)
	if err != nil {
		return false, err
	}
	g.headers(req, token)
	var out struct {
		Total int `json:"total_count"`
	}
	if err := g.do(req, &out); err != nil {
		return false, err
	}
	return out.Total > 0, nil
}

func (g *github) headers(req *http.Request, token string) {
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}

func (g *github) do(req *http.Request, out any) error {
	resp, err := g.http.Do(req)
	if err != nil {
		return fmt.Errorf("github: %s: %w", req.URL.Path, redact(err))
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("github: %s: %s", req.URL.Path, resp.Status)
	}
	return json.Unmarshal(data, out)
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

// mergedAuthor reads a pull_request delivery: who wrote a pull request just merged into
// the repository, 0 for any other delivery.
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
	if err := json.Unmarshal(body, &ev); err != nil {
		return 0, err
	}
	if ev.Action != "closed" || !ev.PullRequest.Merged || !strings.EqualFold(ev.Repository.FullName, repo) {
		return 0, nil
	}
	return ev.PullRequest.User.ID, nil
}

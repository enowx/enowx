package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// maxBody bounds what's read of a request.
const maxBody = 1 << 20

// discord is what the bot asks of Discord's API, with the least it needs: changing one
// role of one member.
type discord struct {
	api, token, app, guild, role string
	http                         *http.Client
}

func newDiscord(c config) *discord {
	return &discord{api: "https://discord.com/api/v10", token: c.BotToken, app: c.AppID, guild: c.GuildID, role: c.RoleID,
		http: &http.Client{Timeout: 10 * time.Second}}
}

func (d *discord) call(ctx context.Context, method, path string, body any) error {
	var r io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		r = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, d.api+path, r)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bot "+d.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := d.http.Do(req)
	if err != nil {
		return fmt.Errorf("discord: %s %s: %w", method, path, redact(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		text, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("discord: %s %s: %s: %s", method, path, resp.Status, text)
	}
	return nil
}

// redact keeps a URL's query out of an error that goes to a log.
func redact(err error) error {
	if u, ok := err.(*url.Error); ok {
		return u.Err
	}
	return err
}

func (d *discord) rolePath(member string) string {
	return "/guilds/" + url.PathEscape(d.guild) + "/members/" + url.PathEscape(member) + "/roles/" + url.PathEscape(d.role)
}

// addRole gives member the role.
func (d *discord) addRole(ctx context.Context, member string) error {
	return d.call(ctx, http.MethodPut, d.rolePath(member), nil)
}

// removeRole takes the role from member.
func (d *discord) removeRole(ctx context.Context, member string) error {
	return d.call(ctx, http.MethodDelete, d.rolePath(member), nil)
}

// commands are the slash commands.
var commands = []map[string]any{
	{"name": "connect", "description": "Link your GitHub account, to get Contributor for a merged pull request", "type": 1},
	{"name": "disconnect", "description": "Remove your GitHub link and the Contributor role", "type": 1},
}

// registerCommands sets the server's commands, as `contributor-bot register` does.
func (d *discord) registerCommands(ctx context.Context) error {
	return d.call(ctx, http.MethodPut, "/applications/"+url.PathEscape(d.app)+"/guilds/"+url.PathEscape(d.guild)+"/commands", commands)
}

// verifyDiscord says an interaction is signed by Discord, as they require checking.
func verifyDiscord(publicKey ed25519.PublicKey, signature, timestamp string, body []byte) bool {
	sig, err := hex.DecodeString(signature)
	if err != nil || len(sig) != ed25519.SignatureSize || timestamp == "" {
		return false
	}
	return ed25519.Verify(publicKey, append([]byte(timestamp), body...), sig)
}

// Interaction types and replies, as Discord numbers them.
const (
	interactionPing    = 1
	interactionCommand = 2
	replyPong          = 1
	replyMessage       = 4
	flagEphemeral      = 1 << 6
)

// interaction is what's read of one.
type interaction struct {
	Type    int    `json:"type"`
	GuildID string `json:"guild_id"`
	Member  struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	} `json:"member"`
	Data struct {
		Name string `json:"name"`
	} `json:"data"`
}

// ephemeral is a reply only the member sees, mentioning nobody.
func ephemeral(text string) map[string]any {
	return map[string]any{"type": replyMessage, "data": map[string]any{
		"content": text, "flags": flagEphemeral, "allowed_mentions": map[string]any{"parse": []string{}}}}
}

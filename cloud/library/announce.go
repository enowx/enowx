package library

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/enowx/enowx/cloud/api"
	"github.com/enowx/enowx/cloud/store"
)

// Saying a pick on Discord, only when an admin asks and only if a channel is set.
// A title is other people's words: it can neither mention anyone nor make a link.

// Announce is where picks are said; without all of its first two, nowhere.
type Announce struct {
	BotToken  string
	ChannelID string
	// DiscordURL is Discord's API, its real one when empty (tests give their own).
	DiscordURL string
}

func (a Announce) on() bool { return a.BotToken != "" && a.ChannelID != "" }

var discordEscape = strings.NewReplacer(`\`, `\\`, `*`, `\*`, `_`, `\_`, `~`, `\~`, "`", "\\`", `|`, `\|`, `>`, `\>`, `<`, `\<`, `#`, `\#`, `[`, `\[`, `]`, `\]`, `(`, `\(`, `)`, `\)`, `:`, `\:`, `@`, `\@`)

// discordText is someone's text as a line in a Discord message: only itself.
func discordText(s string) string {
	return discordEscape.Replace(strings.Join(strings.Fields(s), " "))
}

// announcement is a pick's message.
func (s *Service) announcement(p api.LibraryPick) string {
	address := strings.TrimSuffix(s.cfg.PublicURL, "/") + "/design/" + p.ID + "/" + p.Slug
	return "A new pick on enowX UI: **" + discordText(p.Title) + "** by " + discordText(p.Creator) + "\n" + address
}

// say sends one message to the channel, mentioning nobody.
func (s *Service) say(ctx context.Context, text string) error {
	body, err := json.Marshal(map[string]any{"content": text, "allowed_mentions": map[string]any{"parse": []string{}}})
	if err != nil {
		return err
	}
	base := strings.TrimSuffix(s.cfg.Announce.DiscordURL, "/")
	if base == "" {
		base = "https://discord.com/api/v10"
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/channels/"+url.PathEscape(s.cfg.Announce.ChannelID)+"/messages", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bot "+s.cfg.Announce.BotToken)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
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

// announce says a pick once. The claim comes first, so two admins can't both send
// it; a message that doesn't go gives the claim back, both in the log. It's for
// the owner and admins: a moderator they appointed can't speak on Discord.
func (s *Service) announce(w http.ResponseWriter, r *http.Request, id, reason string) {
	a := asker(r)
	switch {
	case a.role != api.RoleLibraryOwner && a.role != api.RoleLibraryAdmin:
		fail(w, http.StatusForbidden, "this is for the owner and admins")
		return
	case !s.cfg.Announce.on():
		fail(w, http.StatusConflict, "this cloud has no Discord channel to say picks in")
		return
	}
	pick, err := s.db.ClaimLibraryAnnouncement(r.Context(), a.account.ID, id, reason)
	if errors.Is(err, store.ErrNotFound) {
		fail(w, http.StatusConflict, "only a live pick not yet announced can be announced")
		return
	}
	if err != nil {
		s.answer(w, nil, err)
		return
	}
	if err := s.say(r.Context(), s.announcement(pick)); err != nil {
		s.log.Warn("library: announce", "item", id, "err", err)
		if err := s.db.ReleaseLibraryAnnouncement(context.WithoutCancel(r.Context()), a.account.ID, id, "the message didn't go"); err != nil {
			s.log.Warn("library: announce", "item", id, "err", err)
		}
		fail(w, http.StatusBadGateway, "Discord didn't take the message: try again")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// picks are the live picks, and whether they can be announced here.
func (s *Service) picks(w http.ResponseWriter, r *http.Request) {
	list, err := s.db.LibraryPicks(r.Context())
	if err != nil {
		s.answer(w, nil, err)
		return
	}
	reply(w, api.LibraryPicks{Picks: list, AnnounceOn: s.cfg.Announce.on()})
}
